// Package dsstate implements the IPFS Cluster state interface using
// an underlying go-datastore.
package dsstate

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/observations"
	"github.com/ipfs-cluster/ipfs-cluster/state"

	dshelp "github.com/ipfs/boxo/datastore/dshelp"
	ds "github.com/ipfs/go-datastore"
	query "github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log/v2"
	codec "github.com/ugorji/go/codec"

	"go.opencensus.io/stats"
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

	totalPins int64
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
func New(ctx context.Context, dstore ds.Datastore, namespace string, handle codec.Handle) (*State, error) {
	if handle == nil {
		handle = DefaultHandle()
	}

	st := &State{
		dsRead:      dstore,
		dsWrite:     dstore,
		codecHandle: handle,
		namespace:   ds.NewKey(namespace),
		totalPins:   0,
	}

	stats.Record(ctx, observations.Pins.M(0))

	return st, nil
}

// Add adds a new Pin or replaces an existing one.
func (st *State) Add(ctx context.Context, c api.Pin) (err error) {
	_, span := trace.StartSpan(ctx, "state/dsstate/Add")
	defer span.End()

	ps, err := st.serializePin(c)
	if err != nil {
		return
	}

	has, _ := st.Has(ctx, c.Cid)
	defer func() {
		if !has && err == nil {
			total := atomic.AddInt64(&st.totalPins, 1)
			stats.Record(ctx, observations.Pins.M(total))
		}
	}()

	err = st.dsWrite.Put(ctx, st.key(c.Cid), ps)
	return
}

// Rm removes an existing Pin. It is a no-op when the
// item does not exist.
func (st *State) Rm(ctx context.Context, c api.Cid) error {
	_, span := trace.StartSpan(ctx, "state/dsstate/Rm")
	defer span.End()

	err := st.dsWrite.Delete(ctx, st.key(c))
	if err == ds.ErrNotFound {
		return nil
	}
	if err == nil {
		total := atomic.AddInt64(&st.totalPins, -1)
		stats.Record(ctx, observations.Pins.M(total))
	}

	return err
}

// Get returns a Pin from the store and whether it
// was present. When not present, a default pin
// is returned.
func (st *State) Get(ctx context.Context, c api.Cid) (api.Pin, error) {
	_, span := trace.StartSpan(ctx, "state/dsstate/Get")
	defer span.End()

	v, err := st.dsRead.Get(ctx, st.key(c))
	if err != nil {
		if err == ds.ErrNotFound {
			return api.Pin{}, state.ErrNotFound
		}
		return api.Pin{}, err
	}
	p, err := st.deserializePin(c, v)
	if err != nil {
		return api.Pin{}, err
	}
	return p, nil
}

// Has returns whether a Cid is stored.
func (st *State) Has(ctx context.Context, c api.Cid) (bool, error) {
	_, span := trace.StartSpan(ctx, "state/dsstate/Has")
	defer span.End()

	ok, err := st.dsRead.Has(ctx, st.key(c))
	if err != nil {
		return false, err
	}
	return ok, nil
}

// List sends all the pins on the pinset on the given channel.
// Returns and closes channel when done.
func (st *State) List(ctx context.Context, out chan<- api.Pin) error {
	defer close(out)

	_, span := trace.StartSpan(ctx, "state/dsstate/List")
	defer span.End()

	q := query.Query{
		Prefix: st.namespace.String(),
	}

	results, err := st.dsRead.Query(ctx, q)
	if err != nil {
		return err
	}
	defer results.Close()

	var total int64
	for r := range results.Next() {
		// Abort if we shutdown.
		select {
		case <-ctx.Done():
			err = fmt.Errorf("full pinset listing aborted: %w", ctx.Err())
			logger.Warning(err)
			return err
		default:
		}
		if r.Error != nil {
			err := fmt.Errorf("error in query result: %w", r.Error)
			logger.Error(err)
			return err
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
		out <- p

		if total > 0 && total%500000 == 0 {
			logger.Infof("Full pinset listing in progress: %d pins so far", total)
		}
		total++
	}
	if total >= 500000 {
		logger.Infof("Full pinset listing finished: %d pins", total)
	}
	atomic.StoreInt64(&st.totalPins, total)
	stats.Record(ctx, observations.Pins.M(total))
	return nil
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
func cidToDsKey(c api.Cid) ds.Key {
	return dshelp.NewKeyFromBinary(c.Bytes())
}

// used to be on go-ipfs-ds-help
func dsKeyToCid(k ds.Key) (api.Cid, error) {
	kb, err := dshelp.BinaryFromDsKey(k)
	if err != nil {
		return api.CidUndef, err
	}
	c, err := api.CastCid(kb)
	return c, err
}

// convert Cid to /namespace/cid1Key
func (st *State) key(c api.Cid) ds.Key {
	k := cidToDsKey(c)
	return st.namespace.Child(k)
}

// convert /namespace/cidKey to Cid
func (st *State) unkey(k ds.Key) (api.Cid, error) {
	return dsKeyToCid(ds.NewKey(k.BaseNamespace()))
}

// this decides how a Pin object is serialized to be stored in the
// datastore. Changing this may require a migration!
func (st *State) serializePin(c api.Pin) ([]byte, error) {
	return c.ProtoMarshal()
}

// this deserializes a Pin object from the datastore. It should be
// the exact opposite from serializePin.
func (st *State) deserializePin(c api.Cid, buf []byte) (api.Pin, error) {
	p := api.Pin{}
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
func NewBatching(ctx context.Context, dstore ds.Batching, namespace string, handle codec.Handle) (*BatchingState, error) {
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

	stats.Record(ctx, observations.Pins.M(0))
	return bst, nil
}

// Commit persists the batched write operations.
func (bst *BatchingState) Commit(ctx context.Context) error {
	_, span := trace.StartSpan(ctx, "state/dsstate/Commit")
	defer span.End()
	return bst.batch.Commit(ctx)
}
