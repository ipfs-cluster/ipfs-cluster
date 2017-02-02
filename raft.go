package ipfscluster

import (
	"context"
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	hashiraft "github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	libp2praft "github.com/libp2p/go-libp2p-raft"
)

// DefaultRaftConfig allows to tweak Raft configuration used by Cluster from
// from the outside.
var DefaultRaftConfig = hashiraft.DefaultConfig()

// RaftMaxSnapshots indicates how many snapshots to keep in the consensus data
// folder.
var RaftMaxSnapshots = 5

// is this running 64 bits arch? https://groups.google.com/forum/#!topic/golang-nuts/vAckmhUMAdQ
const sixtyfour = uint64(^uint(0)) == ^uint64(0)

// Raft performs all Raft-specific operations which are needed by Cluster but
// are not fulfilled by the consensus interface. It should contain most of the
// Raft-related stuff so it can be easily replaced in the future, if need be.
type Raft struct {
	raft          *hashiraft.Raft
	transport     *libp2praft.Libp2pTransport
	snapshotStore hashiraft.SnapshotStore
	logStore      hashiraft.LogStore
	stableStore   hashiraft.StableStore
	peerstore     *libp2praft.Peerstore
	boltdb        *raftboltdb.BoltStore
}

func defaultRaftConfig() *hashiraft.Config {
	// These options are imposed over any Default Raft Config.
	// Changing them causes cluster peers difficult-to-understand,
	// behaviours, usually around the add/remove of peers.
	// That means that changing them will make users wonder why something
	// does not work the way it is expected to.
	// i.e. ShutdownOnRemove will cause that no snapshot will be taken
	// when trying to shutdown a peer after removing it from a cluster.
	DefaultRaftConfig.DisableBootstrapAfterElect = false
	DefaultRaftConfig.EnableSingleNode = true
	DefaultRaftConfig.ShutdownOnRemove = false

	// Set up logging
	DefaultRaftConfig.LogOutput = ioutil.Discard
	DefaultRaftConfig.Logger = raftStdLogger // see logging.go
	return DefaultRaftConfig
}

// NewRaft launches a go-libp2p-raft consensus peer.
func NewRaft(peers []peer.ID, host host.Host, dataFolder string, fsm hashiraft.FSM) (*Raft, error) {
	logger.Debug("creating libp2p Raft transport")
	transport, err := libp2praft.NewLibp2pTransportWithHost(host)
	if err != nil {
		logger.Error("creating libp2p-raft transport: ", err)
		return nil, err
	}

	pstore := &libp2praft.Peerstore{}
	peersStr := make([]string, len(peers), len(peers))
	for i, p := range peers {
		peersStr[i] = peer.IDB58Encode(p)
	}
	pstore.SetPeers(peersStr)

	logger.Debug("creating file snapshot store")
	snapshots, err := hashiraft.NewFileSnapshotStoreWithLogger(dataFolder, RaftMaxSnapshots, raftStdLogger)
	if err != nil {
		logger.Error("creating file snapshot store: ", err)
		return nil, err
	}

	logger.Debug("creating BoltDB log store")
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(dataFolder, "raft.db"))
	if err != nil {
		logger.Error("creating bolt store: ", err)
		return nil, err
	}

	logger.Debug("creating Raft")
	r, err := hashiraft.NewRaft(defaultRaftConfig(), fsm, logStore, logStore, snapshots, pstore, transport)
	if err != nil {
		logger.Error("initializing raft: ", err)
		return nil, err
	}

	return &Raft{
		raft:          r,
		transport:     transport,
		snapshotStore: snapshots,
		logStore:      logStore,
		stableStore:   logStore,
		peerstore:     pstore,
		boltdb:        logStore,
	}, nil
}

// WaitForLeader holds until Raft says we have a leader
func (r *Raft) WaitForLeader(ctx context.Context) {
	// Using Raft observers panics on non-64 architectures.
	// This is a work around
	if sixtyfour {
		r.waitForLeader(ctx)
	} else {
		r.waitForLeaderLegacy(ctx)
	}
}

func (r *Raft) waitForLeader(ctx context.Context) {
	obsCh := make(chan hashiraft.Observation)
	filter := func(o *hashiraft.Observation) bool {
		switch o.Data.(type) {
		case hashiraft.LeaderObservation:
			return true
		default:
			return false
		}
	}
	observer := hashiraft.NewObserver(obsCh, true, filter)
	r.raft.RegisterObserver(observer)
	defer r.raft.DeregisterObserver(observer)
	select {
	case obs := <-obsCh:
		leaderObs := obs.Data.(hashiraft.LeaderObservation)
		logger.Infof("Raft Leader elected: %s", leaderObs.Leader)

	case <-ctx.Done():
		return
	}
}

func (r *Raft) waitForLeaderLegacy(ctx context.Context) {
	for {
		leader := r.raft.Leader()
		if leader != "" {
			logger.Infof("Raft Leader elected: %s", leader)
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// WaitForUpdates holds until Raft has synced to the last index in the log
func (r *Raft) WaitForUpdates(ctx context.Context) {
	logger.Debug("Raft state is catching up")
	for {
		select {
		case <-ctx.Done():
			return
		default:
			lai := r.raft.AppliedIndex()
			li := r.raft.LastIndex()
			logger.Debugf("current Raft index: %d/%d",
				lai, li)
			if lai == li {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}

	}
}

// Snapshot tells Raft to take a snapshot.
func (r *Raft) Snapshot() error {
	future := r.raft.Snapshot()
	err := future.Error()
	if err != nil && !strings.Contains(err.Error(), "Nothing new to snapshot") {
		return errors.New("could not take snapshot: " + err.Error())
	}
	return nil
}

// Shutdown shutdown Raft and closes the BoltDB.
func (r *Raft) Shutdown() error {
	future := r.raft.Shutdown()
	err := future.Error()
	errMsgs := ""
	if err != nil {
		errMsgs += "could not shutdown raft: " + err.Error() + ".\n"
	}

	err = r.boltdb.Close() // important!
	if err != nil {
		errMsgs += "could not close boltdb: " + err.Error()
	}
	if errMsgs != "" {
		return errors.New(errMsgs)
	}
	return nil
}

// AddPeer adds a peer to Raft
func (r *Raft) AddPeer(peer string) error {
	future := r.raft.AddPeer(peer)
	err := future.Error()
	return err
}

// RemovePeer removes a peer from Raft
func (r *Raft) RemovePeer(peer string) error {
	future := r.raft.RemovePeer(peer)
	err := future.Error()
	return err
}

// Leader returns Raft's leader. It may be an empty string if
// there is no leader or it is unknown.
func (r *Raft) Leader() string {
	return r.raft.Leader()
}
