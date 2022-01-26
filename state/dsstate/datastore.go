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
	logging "github.com/ipfs/go-log/v2"
	codec "github.com/ugorji/go/codec"

	trace "go.opencensus.io/trace"
)

var _ state.State = (*State)(nil)
var _ state.BatchingState = (*BatchingState)(nil)

var logger = logging.Logger("dsstate")

// State implements the IPFS Cluster "state" interface by wrapping
// a go-datastore and choosing how api.Pin objects are stored
// in it. It also provides serialization methods for the whole
// state which are datastore-independent.
type State struct {
	dsRead      ds.Read
	dsWrite     ds.Write
	codecHandle codec.Handle
	namespace   ds.Key
	// version     int
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
// The Handle controls options for the serialization of the full state
// (marshaling/unmarshaling).
func New(dstore ds.Datastore, namespace string, handle codec.Handle) (*State, error) {
	if handle == nil {
		handle = DefaultHandle()
	}

	st := &State{
		dsRead:      dstore,
		dsWrite:     dstore,
		codecHandle: handle,
		namespace:   ds.NewKey(namespace),
	}

	return st, nil
}

// Add adds a new Pin or replaces an existing one.
func (st *State) Add(ctx context.Context, c *api.Pin) error {
	_, span := trace.StartSpan(ctx, "state/dsstate/Add")
	defer span.End()

	ps, err := st.serializePin(c)
	if err != nil {
		return err
	}
	return st.dsWrite.Put(ctx, st.key(c.Cid), ps)
}

// Rm removes an existing Pin. It is a no-op when the
// item does not exist.
func (st *State) Rm(ctx context.Context, c cid.Cid) error {
	_, span := trace.StartSpan(ctx, "state/dsstate/Rm")
	defer span.End()

	err := st.dsWrite.Delete(ctx, st.key(c))
	if err == ds.ErrNotFound {
		return nil
	}
	return err
}

// Get returns a Pin from the store and whether it
// was present. When not present, a default pin
// is returned.
func (st *State) Get(ctx context.Context, c cid.Cid) (*api.Pin, error) {
	_, span := trace.StartSpan(ctx, "state/dsstate/Get")
	defer span.End()

	v, err := st.dsRead.Get(ctx, st.key(c))
	if err != nil {
		if err == ds.ErrNotFound {
			return nil, state.ErrNotFound
		}
		return nil, err
	}
	p, err := st.deserializePin(c, v)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Has returns whether a Cid is stored.
func (st *State) Has(ctx context.Context, c cid.Cid) (bool, error) {
	_, span := trace.StartSpan(ctx, "state/dsstate/Has")
	defer span.End()

	ok, err := st.dsRead.Has(ctx, st.key(c))
	if err != nil {
		return false, err
	}
	return ok, nil
}

// List returns the unsorted list of all Pins that have been added to the
// datastore.
func (st *State) List(ctx context.Context) ([]*api.Pin, error) {
	_, span := trace.StartSpan(ctx, "state/dsstate/List")
	defer span.End()

	q := query.Query{
		Prefix: st.namespace.String(),
	}

	results, err := st.dsRead.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer results.Close()

	var pins []*api.Pin

	total := 0
	for r := range results.Next() {
		// Abort if we shutdown.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if r.Error != nil {
			logger.Errorf("error in query result: %s", r.Error)
			return pins, r.Error
		}
		k := ds.NewKey(r.Key)
		ci, err := st.unkey(k)
		if err != nil {
			logger.Warn("bad key (ignoring). key: ", k, "error: ", err)
			continue
		}

		p, err := st.deserializePin(ci, r.Value)
		if err != nil {
			logger.Errorf("error deserializing pin (%s): %s", r.Key, err)
			continue
		}

		if total > 0 && total%500000 == 0 {
			logger.Infof("Full pinset listing in progress: %d pins so far", total)
		}
		total++
		pins = append(pins, p)
	}
	if total >= 500000 {
		logger.Infof("Full pinset listing finished: %d pins", total)
	}
	return pins, nil
}

// Migrate migrates an older state version to the current one.
// This is a no-op for now.
func (st *State) Migrate(ctx context.Context, r io.Reader) error {
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

	results, err := st.dsRead.Query(context.Background(), q)
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
		err := st.dsWrite.Put(context.Background(), k, entry.Value)
		if err != nil {
			logger.Error("error adding unmarshaled key to datastore:", err)
			return err
		}
	}

	return nil
}

// used to be on go-ipfs-ds-help
func cidToDsKey(c cid.Cid) ds.Key {
	return dshelp.NewKeyFromBinary(c.Bytes())
}

// used to be on go-ipfs-ds-help
func dsKeyToCid(k ds.Key) (cid.Cid, error) {
	kb, err := dshelp.BinaryFromDsKey(k)
	if err != nil {
		return cid.Undef, err
	}
	return cid.Cast(kb)
}

// convert Cid to /namespace/cid1Key
func (st *State) key(c cid.Cid) ds.Key {
	k := cidToDsKey(c)
	return st.namespace.Child(k)
}

// convert /namespace/cidKey to Cid
func (st *State) unkey(k ds.Key) (cid.Cid, error) {
	return dsKeyToCid(ds.NewKey(k.BaseNamespace()))
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

// BatchingState implements the IPFS Cluster "state" interface by wrapping a
// batching go-datastore. All writes are batched and only written disk
// when Commit() is called.
type BatchingState struct {
	*State
	batch ds.Batch
}

// NewBatching returns a new batching statate using the given datastore.
//
// All keys are namespaced with the given string when written. Thus the same
// go-datastore can be sharded for different uses.
//
// The Handle controls options for the serialization of the full state
// (marshaling/unmarshaling).
func NewBatching(dstore ds.Batching, namespace string, handle codec.Handle) (*BatchingState, error) {
	if handle == nil {
		handle = DefaultHandle()
	}

	batch, err := dstore.Batch(context.Background())
	if err != nil {
		return nil, err
	}

	st := &State{
		dsRead:      dstore,
		dsWrite:     batch,
		codecHandle: handle,
		namespace:   ds.NewKey(namespace),
	}

	bst := &BatchingState{}
	bst.State = st
	bst.batch = batch
	return bst, nil
}

// Commit persists the batched write operations.
func (bst *BatchingState) Commit(ctx context.Context) error {
	_, span := trace.StartSpan(ctx, "state/dsstate/Commit")
	defer span.End()
	return bst.batch.Commit(ctx)
}
