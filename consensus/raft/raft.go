package raft

import (
	"context"
	"errors"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"time"

	logging "gx/ipfs/QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52/go-log"
	raftboltdb "gx/ipfs/QmUDCcPkPMPJ149YBpfFLWJtRFeqace5GNdBPD2cW4Z8E6/raft-boltdb"
	libp2praft "gx/ipfs/QmW92B7boZiW7qBEUE2aT8vi3WNLWpk6on4mxg1CpEzLpB/go-libp2p-raft"
	hashiraft "gx/ipfs/QmWRzh5sntXhuZaxmGDEjjBhg1nX7DgaMdhBeik42LZdEv/raft"
	peer "gx/ipfs/QmZcUPvPhD1Xvk6mwijYF8AfR3mG31S1YsEfHG4khrFPRr/go-libp2p-peer"
	host "gx/ipfs/QmbzbRyd22gcW92U1rA2yKagB3myMYhk45XBknJ49F9XWJ/go-libp2p-host"
)

// DefaultRaftConfig allows to tweak Raft configuration used by Cluster from
// from the outside.
var DefaultRaftConfig = hashiraft.DefaultConfig()

// RaftMaxSnapshots indicates how many snapshots to keep in the consensus data
// folder.
var RaftMaxSnapshots = 5

// is this running 64 bits arch? https://groups.google.com/forum/#!topic/golang-nuts/vAckmhUMAdQ
const sixtyfour = uint64(^uint(0)) == ^uint64(0)

type logForwarder struct{}

var raftStdLogger = log.New(&logForwarder{}, "", 0)
var raftLogger = logging.Logger("raft")

// Write forwards to our go-log logger.
// According to https://golang.org/pkg/log/#Logger.Output
// it is called per line.
func (fw *logForwarder) Write(p []byte) (n int, err error) {
	t := strings.TrimSuffix(string(p), "\n")
	switch {
	case strings.Contains(t, "[DEBUG]"):
		raftLogger.Debug(strings.TrimPrefix(t, "[DEBUG] raft: "))
	case strings.Contains(t, "[WARN]"):
		raftLogger.Warning(strings.TrimPrefix(t, "[WARN]  raft: "))
	case strings.Contains(t, "[ERR]"):
		raftLogger.Error(strings.TrimPrefix(t, "[ERR] raft: "))
	case strings.Contains(t, "[INFO]"):
		raftLogger.Info(strings.TrimPrefix(t, "[INFO] raft: "))
	default:
		raftLogger.Debug(t)
	}
	return len(p), nil
}

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
	dataFolder    string
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

	cfg := defaultRaftConfig()
	logger.Debug("creating Raft")
	r, err := hashiraft.NewRaft(cfg, fsm, logStore, logStore, snapshots, pstore, transport)
	if err != nil {
		logger.Error("initializing raft: ", err)
		return nil, err
	}

	raft := &Raft{
		raft:          r,
		transport:     transport,
		snapshotStore: snapshots,
		logStore:      logStore,
		stableStore:   logStore,
		peerstore:     pstore,
		boltdb:        logStore,
		dataFolder:    dataFolder,
	}

	return raft, nil
}

// WaitForLeader holds until Raft says we have a leader.
// Returns an error if we don't.
func (r *Raft) WaitForLeader(ctx context.Context) error {
	// Using Raft observers panics on non-64 architectures.
	// This is a work around
	if sixtyfour {
		return r.waitForLeader(ctx)
	}
	return r.waitForLeaderLegacy(ctx)
}

func (r *Raft) waitForLeader(ctx context.Context) error {
	obsCh := make(chan hashiraft.Observation, 1)
	filter := func(o *hashiraft.Observation) bool {
		switch o.Data.(type) {
		case hashiraft.LeaderObservation:
			return true
		default:
			return false
		}
	}
	observer := hashiraft.NewObserver(obsCh, false, filter)
	r.raft.RegisterObserver(observer)
	defer r.raft.DeregisterObserver(observer)
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case obs := <-obsCh:
			switch obs.Data.(type) {
			case hashiraft.LeaderObservation:
				leaderObs := obs.Data.(hashiraft.LeaderObservation)
				logger.Infof("Raft Leader elected: %s", leaderObs.Leader)
				return nil
			}
		case <-ticker.C:
			if l := r.raft.Leader(); l != "" { //we missed or there was no election
				logger.Debug("waitForleaderTimer")
				logger.Infof("Raft Leader elected: %s", l)
				ticker.Stop()
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// 32-bit systems should use this.
func (r *Raft) waitForLeaderLegacy(ctx context.Context) error {
	for {
		leader := r.raft.Leader()
		if leader != "" {
			logger.Infof("Raft Leader elected: %s", leader)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// WaitForUpdates holds until Raft has synced to the last index in the log
func (r *Raft) WaitForUpdates(ctx context.Context) error {
	logger.Debug("Raft state is catching up")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			lai := r.raft.AppliedIndex()
			li := r.raft.LastIndex()
			logger.Debugf("current Raft index: %d/%d",
				lai, li)
			if lai == li {
				return nil
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

	// If the shutdown worked correctly
	// (including snapshot) we can remove the Raft
	// database (which traces peers additions
	// and removals). It makes re-start of the peer
	// way less confusing for Raft while the state
	// can be restored from the snapshot.
	//os.Remove(filepath.Join(r.dataFolder, "raft.db"))
	return nil
}

// AddPeer adds a peer to Raft
func (r *Raft) AddPeer(peer string) error {
	if r.hasPeer(peer) {
		logger.Debug("skipping raft add as already in peer set")
		return nil
	}

	future := r.raft.AddPeer(peer)
	err := future.Error()
	if err != nil {
		logger.Error("raft cannot add peer: ", err)
		return err
	}
	peers, _ := r.peerstore.Peers()
	logger.Debugf("raft peerstore: %s", peers)
	return err
}

// RemovePeer removes a peer from Raft
func (r *Raft) RemovePeer(peer string) error {
	if !r.hasPeer(peer) {
		return nil
	}

	future := r.raft.RemovePeer(peer)
	err := future.Error()
	if err != nil {
		logger.Error("raft cannot remove peer: ", err)
		return err
	}
	peers, _ := r.peerstore.Peers()
	logger.Debugf("raft peerstore: %s", peers)
	return err
}

// func (r *Raft) SetPeers(peers []string) error {
// 	logger.Debugf("SetPeers(): %s", peers)
// 	future := r.raft.SetPeers(peers)
// 	err := future.Error()
// 	if err != nil {
// 		logger.Error(err)
// 	}
// 	return err
// }

// Leader returns Raft's leader. It may be an empty string if
// there is no leader or it is unknown.
func (r *Raft) Leader() string {
	return r.raft.Leader()
}

func (r *Raft) hasPeer(peer string) bool {
	found := false
	peers, _ := r.peerstore.Peers()
	for _, p := range peers {
		if p == peer {
			found = true
			break
		}
	}

	return found
}
