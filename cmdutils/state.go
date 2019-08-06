package cmdutils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/config"
	"github.com/ipfs/ipfs-cluster/consensus/crdt"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/datastore/badger"
	"github.com/ipfs/ipfs-cluster/datastore/inmem"
	"github.com/ipfs/ipfs-cluster/pstoremgr"
	"github.com/ipfs/ipfs-cluster/state"

	ds "github.com/ipfs/go-datastore"
)

// StateManager is the interface that allows to import, export and clean
// different cluster states depending on the consensus component used.
type StateManager interface {
	ImportState(io.Reader) error
	ExportState(io.Writer) error
	GetStore() (ds.Datastore, error)
	Clean() error
}

// NewStateManager returns an state manager implementation for the given
// consensus ("raft" or "crdt"). It will need initialized configs
func NewStateManager(consensus string, ident *config.Identity, cfgs *Configs) (StateManager, error) {
	switch consensus {
	case "raft":
		return &raftStateManager{ident, cfgs}, nil
	case "crdt":
		return &crdtStateManager{ident, cfgs}, nil
	case "":
		return nil, errors.New("unspecified consensus component")
	default:
		return nil, fmt.Errorf("unknown consensus component '%s'", consensus)
	}
}

type raftStateManager struct {
	ident *config.Identity
	cfgs  *Configs
}

func (raftsm *raftStateManager) GetStore() (ds.Datastore, error) {
	return inmem.New(), nil
}

func (raftsm *raftStateManager) getOfflineState(store ds.Datastore) (state.State, error) {
	return raft.OfflineState(raftsm.cfgs.Raft, store)
}

func (raftsm *raftStateManager) ImportState(r io.Reader) error {
	err := raftsm.Clean()
	if err != nil {
		return err
	}

	store, err := raftsm.GetStore()
	if err != nil {
		return err
	}
	defer store.Close()
	st, err := raftsm.getOfflineState(store)
	if err != nil {
		return err
	}
	err = importState(r, st)
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
	st, err := raftsm.getOfflineState(store)
	if err != nil {
		return err
	}
	return exportState(w, st)
}

func (raftsm *raftStateManager) Clean() error {
	return raft.CleanupRaft(raftsm.cfgs.Raft)
}

type crdtStateManager struct {
	ident *config.Identity
	cfgs  *Configs
}

func (crdtsm *crdtStateManager) GetStore() (ds.Datastore, error) {
	bds, err := badger.New(crdtsm.cfgs.Badger)
	if err != nil {
		return nil, err
	}
	return bds, nil
}

func (crdtsm *crdtStateManager) getOfflineState(store ds.Datastore) (state.BatchingState, error) {
	return crdt.OfflineState(crdtsm.cfgs.Crdt, store)
}

func (crdtsm *crdtStateManager) ImportState(r io.Reader) error {
	err := crdtsm.Clean()
	if err != nil {
		return err
	}

	store, err := crdtsm.GetStore()
	if err != nil {
		return err
	}
	defer store.Close()
	st, err := crdtsm.getOfflineState(store)
	if err != nil {
		return err
	}

	err = importState(r, st)
	if err != nil {
		return err
	}

	return st.Commit(context.Background())
}

func (crdtsm *crdtStateManager) ExportState(w io.Writer) error {
	store, err := crdtsm.GetStore()
	if err != nil {
		return err
	}
	defer store.Close()
	st, err := crdtsm.getOfflineState(store)
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

func importState(r io.Reader, st state.State) error {
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
		err = st.Add(ctx, &pin)
		if err != nil {
			return err
		}
	}
}

// ExportState saves a json representation of a state
func exportState(w io.Writer, st state.State) error {
	pins, err := st.List(context.Background())
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	for _, pin := range pins {
		err := enc.Encode(pin)
		if err != nil {
			return err
		}
	}
	return nil
}
