package cmdutils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	ipfscluster "github.com/ipfs-cluster/ipfs-cluster"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/config"
	"github.com/ipfs-cluster/ipfs-cluster/consensus/crdt"
	"github.com/ipfs-cluster/ipfs-cluster/consensus/raft"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/badger"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/badger3"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/inmem"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/leveldb"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/pebble"
	"github.com/ipfs-cluster/ipfs-cluster/pstoremgr"
	"github.com/ipfs-cluster/ipfs-cluster/state"

	ds "github.com/ipfs/go-datastore"
)

// StateManager is the interface that allows to import, export and clean
// different cluster states depending on the consensus component used.
type StateManager interface {
	ImportState(io.Reader, api.PinOptions) error
	ExportState(io.Writer) error
	GetStore() (ds.Datastore, error)
	GetOfflineState(ds.Datastore) (state.State, error)
	Clean() error
}

// NewStateManager returns an state manager implementation for the given
// consensus ("raft" or "crdt"). It will need initialized configs.
func NewStateManager(consensus string, datastore string, ident *config.Identity, cfgs *Configs) (StateManager, error) {
	switch consensus {
	case cfgs.Raft.ConfigKey():
		return &raftStateManager{ident, cfgs}, nil
	case cfgs.Crdt.ConfigKey():
		return &crdtStateManager{
			cfgs:      cfgs,
			datastore: datastore,
		}, nil
	case "":
		return nil, errors.New("could not determine the consensus component")
	default:
		return nil, fmt.Errorf("unknown consensus component '%s'", consensus)
	}
}

// NewStateManagerWithHelper returns a state manager initialized using the
// configuration and identity provided by the given config helper.
func NewStateManagerWithHelper(cfgHelper *ConfigHelper) (StateManager, error) {
	return NewStateManager(
		cfgHelper.GetConsensus(),
		cfgHelper.GetDatastore(),
		cfgHelper.Identity(),
		cfgHelper.Configs(),
	)
}

type raftStateManager struct {
	ident *config.Identity
	cfgs  *Configs
}

func (raftsm *raftStateManager) GetStore() (ds.Datastore, error) {
	return inmem.New(), nil
}

func (raftsm *raftStateManager) GetOfflineState(store ds.Datastore) (state.State, error) {
	return raft.OfflineState(raftsm.cfgs.Raft, store)
}

func (raftsm *raftStateManager) ImportState(r io.Reader, opts api.PinOptions) error {
	err := raftsm.Clean()
	if err != nil {
		return err
	}

	store, err := raftsm.GetStore()
	if err != nil {
		return err
	}
	defer store.Close()
	st, err := raftsm.GetOfflineState(store)
	if err != nil {
		return err
	}
	err = importState(r, st, opts)
	if err != nil {
		return err
	}
	pm := pstoremgr.New(context.Background(), nil, raftsm.cfgs.Cluster.GetPeerstorePath())
	raftPeers := append(
		ipfscluster.PeersFromMultiaddrs(pm.LoadPeerstore()),
		raftsm.ident.ID,
	)
	return raft.SnapshotSave(raftsm.cfgs.Raft, st, raftPeers)
}

func (raftsm *raftStateManager) ExportState(w io.Writer) error {
	store, err := raftsm.GetStore()
	if err != nil {
		return err
	}
	defer store.Close()
	st, err := raftsm.GetOfflineState(store)
	if err != nil {
		return err
	}
	return exportState(w, st)
}

func (raftsm *raftStateManager) Clean() error {
	return raft.CleanupRaft(raftsm.cfgs.Raft)
}

type crdtStateManager struct {
	cfgs      *Configs
	datastore string
}

func (crdtsm *crdtStateManager) GetStore() (ds.Datastore, error) {
	switch crdtsm.datastore {
	case crdtsm.cfgs.Badger.ConfigKey():
		return badger.New(crdtsm.cfgs.Badger)
	case crdtsm.cfgs.Badger3.ConfigKey():
		return badger3.New(crdtsm.cfgs.Badger3)
	case crdtsm.cfgs.LevelDB.ConfigKey():
		return leveldb.New(crdtsm.cfgs.LevelDB)
	case crdtsm.cfgs.Pebble.ConfigKey():
		return pebble.New(crdtsm.cfgs.Pebble)
	default:
		return nil, errors.New("unknown datastore")
	}

}

func (crdtsm *crdtStateManager) GetOfflineState(store ds.Datastore) (state.State, error) {
	return crdt.OfflineState(crdtsm.cfgs.Crdt, store)
}

func (crdtsm *crdtStateManager) ImportState(r io.Reader, opts api.PinOptions) error {
	err := crdtsm.Clean()
	if err != nil {
		return err
	}

	store, err := crdtsm.GetStore()
	if err != nil {
		return err
	}
	defer store.Close()
	st, err := crdtsm.GetOfflineState(store)
	if err != nil {
		return err
	}
	batchingSt := st.(state.BatchingState)

	err = importState(r, batchingSt, opts)
	if err != nil {
		return err
	}

	return batchingSt.Commit(context.Background())
}

func (crdtsm *crdtStateManager) ExportState(w io.Writer) error {
	store, err := crdtsm.GetStore()
	if err != nil {
		return err
	}
	defer store.Close()
	st, err := crdtsm.GetOfflineState(store)
	if err != nil {
		return err
	}
	return exportState(w, st)
}

func (crdtsm *crdtStateManager) Clean() error {
	store, err := crdtsm.GetStore()
	if err != nil {
		return err
	}
	defer store.Close()
	return crdt.Clean(context.Background(), crdtsm.cfgs.Crdt, store)
}

func importState(r io.Reader, st state.State, opts api.PinOptions) error {
	ctx := context.Background()
	dec := json.NewDecoder(r)
	for {
		var pin api.Pin
		err := dec.Decode(&pin)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if opts.ReplicationFactorMax > 0 {
			pin.ReplicationFactorMax = opts.ReplicationFactorMax
		}

		if opts.ReplicationFactorMin > 0 {
			pin.ReplicationFactorMin = opts.ReplicationFactorMin
		}

		if len(opts.UserAllocations) > 0 {
			// We are injecting directly to the state.
			// UserAllocation option is not stored in the state.
			// We need to set Allocations directly.
			pin.Allocations = opts.UserAllocations
		}

		err = st.Add(ctx, pin)
		if err != nil {
			return err
		}
	}
}

// ExportState saves a json representation of a state
func exportState(w io.Writer, st state.State) error {
	out := make(chan api.Pin, 10000)
	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		errCh <- st.List(context.Background(), out)
	}()
	var err error
	enc := json.NewEncoder(w)
	for pin := range out {
		if err == nil {
			err = enc.Encode(pin)
		}
	}
	if err != nil {
		return err
	}
	err = <-errCh
	return err
}
