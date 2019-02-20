// Package dsstate implements the IPFS Cluster state interface using
// an underlying go-datastore.
package dsstate

import (
	"context"
	"io"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/state"

	cid "github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	query "github.com/ipfs/go-datastore/query"
	dshelp "github.com/ipfs/go-ipfs-ds-help"
	logging "github.com/ipfs/go-log"
	codec "github.com/ugorji/go/codec"

	trace "go.opencensus.io/trace"
)

var _ state.State = (*State)(nil)

var logger = logging.Logger("dsstate")

// State implements the IPFS Cluster "state" interface by wrapping
// a go-datastore and choosing how api.Pin objects are stored
// in it. It also provides serialization methods for the whole
// state which are datastore-independent.
type State struct {
	ds          ds.Datastore
	codecHandle codec.Handle
	namespace   ds.Key
	version     int
}

// DefaultHandle returns the codec handler of choice (Msgpack).
func DefaultHandle() codec.Handle {
	h := &codec.MsgpackHandle{}
	return h
}

// New returns a new state using the given datastore.
//
// All keys are namespaced with the given string when written. Thus the same
// go-datastore can be sharded for different uses.
//
//
// The Handle controls options for the serialization of items and the state
// itself.
func New(dstore ds.Datastore, namespace string, handle codec.Handle) (*State, error) {
	if handle == nil {
		handle = DefaultHandle()
	}

	st := &State{
		ds:          dstore,
		codecHandle: handle,
		namespace:   ds.NewKey(namespace),
		version:     0, // TODO: Remove when all migrated
	}

	return st, nil
}

// Add adds a new Pin or replaces an existing one.
func (st *State) Add(ctx context.Context, c api.Pin) error {
	_, span := trace.StartSpan(ctx, "state/dsstate/Add")
	defer span.End()

	ps, err := st.serializePin(&c)
	if err != nil {
		return err
	}
	return st.ds.Put(st.key(c.Cid), ps)
}

// Rm removes an existing Pin. It is a no-op when the
// item does not exist.
func (st *State) Rm(ctx context.Context, c cid.Cid) error {
	_, span := trace.StartSpan(ctx, "state/dsstate/Rm")
	defer span.End()

	err := st.ds.Delete(st.key(c))
	if err == ds.ErrNotFound {
		return nil
	}
	return err
}

// Get returns a Pin from the store and whether it
// was present. When not present, a default pin
// is returned.
func (st *State) Get(ctx context.Context, c cid.Cid) (api.Pin, bool) {
	_, span := trace.StartSpan(ctx, "state/dsstate/Get")
	defer span.End()

	v, err := st.ds.Get(st.key(c))
	if err != nil {
		return api.PinCid(c), false
	}
	p, err := st.deserializePin(c, v)
	if err != nil {
		return api.PinCid(c), false
	}
	return *p, true
}

// Has returns whether a Cid is stored.
func (st *State) Has(ctx context.Context, c cid.Cid) bool {
	_, span := trace.StartSpan(ctx, "state/dsstate/Has")
	defer span.End()

	ok, err := st.ds.Has(st.key(c))
	if err != nil {
		logger.Error(err)
	}
	return ok && err == nil
}

// List returns the unsorted list of all Pins that have been added to the
// datastore.
func (st *State) List(ctx context.Context) []api.Pin {
	_, span := trace.StartSpan(ctx, "state/dsstate/List")
	defer span.End()

	q := query.Query{
		Prefix: st.namespace.String(),
	}

	results, err := st.ds.Query(q)
	if err != nil {
		return []api.Pin{}
	}
	defer results.Close()

	var pins []api.Pin

	for r := range results.Next() {
		if r.Error != nil {
			logger.Errorf("error in query result: %s", r.Error)
			return pins
		}
		k := ds.NewKey(r.Key)
		ci, err := st.unkey(k)
		if err != nil {
			logger.Error("key: ", k, "error: ", err)
			logger.Error(string(r.Value))
			continue
		}

		p, err := st.deserializePin(ci, r.Value)
		if err != nil {
			logger.Errorf("error deserializing pin (%s): %s", r.Key, err)
			continue
		}

		pins = append(pins, *p)
	}
	return pins
}

// Migrate migrates an older state version to the current one.
// This is a no-op for now.
func (st *State) Migrate(ctx context.Context, r io.Reader) error {
	ctx, span := trace.StartSpan(ctx, "state/map/Migrate")
	defer span.End()
	return nil
}

// GetVersion returns the current state version.
func (st *State) GetVersion() int {
	return st.version
}

// SetVersion allows to manually modify the state version.
func (st *State) SetVersion(v int) error {
	st.version = v
	return nil
}

type serialEntry struct {
	Key   string `codec:"k"`
	Value []byte `codec:"v"`
}

// Marshal dumps the state to a writer. It does this by encoding every
// key/value in the store. The keys are stored without the namespace part to
// reduce the size of the snapshot.
func (st *State) Marshal(w io.Writer) error {
	q := query.Query{
		Prefix: st.namespace.String(),
	}

	results, err := st.ds.Query(q)
	if err != nil {
		return err
	}
	defer results.Close()

	enc := codec.NewEncoder(w, st.codecHandle)

	for r := range results.Next() {
		if r.Error != nil {
			logger.Errorf("error in query result: %s", r.Error)
			return r.Error
		}

		k := ds.NewKey(r.Key)
		// reduce snapshot size by not storing the prefix
		err := enc.Encode(serialEntry{
			Key:   k.BaseNamespace(),
			Value: r.Value,
		})
		if err != nil {
			logger.Error(err)
			return err
		}
	}
	return nil
}

// Unmarshal reads and parses a previous dump of the state.
// All the parsed key/values are added to the store. As of now,
// Unmarshal does not empty the existing store from any values
// before unmarshaling from the given reader.
func (st *State) Unmarshal(r io.Reader) error {
	dec := codec.NewDecoder(r, st.codecHandle)
	for {
		var entry serialEntry
		if err := dec.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		k := st.namespace.Child(ds.NewKey(entry.Key))
		err := st.ds.Put(k, entry.Value)
		if err != nil {
			logger.Error("error adding unmarshaled key to datastore:", err)
			return err
		}
	}

	return nil
}

// convert Cid to /namespace/cidKey
func (st *State) key(c cid.Cid) ds.Key {
	k := dshelp.CidToDsKey(c)
	return st.namespace.Child(k)
}

// convert /namespace/cidKey to Cid
func (st *State) unkey(k ds.Key) (cid.Cid, error) {
	return dshelp.DsKeyToCid(ds.NewKey(k.BaseNamespace()))
}

// this decides how a Pin object is serialized to be stored in the
// datastore. Changing this may require a migration!
func (st *State) serializePin(c *api.Pin) ([]byte, error) {
	return c.ProtoMarshal()
}

// this deserializes a Pin object from the datastore. It should be
// the exact opposite from serializePin.
func (st *State) deserializePin(c cid.Cid, buf []byte) (*api.Pin, error) {
	p := &api.Pin{}
	err := p.ProtoUnmarshal(buf)
	p.Cid = c
	return p, err
}
