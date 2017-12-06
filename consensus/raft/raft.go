package raft

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	hraft "github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	p2praft "github.com/libp2p/go-libp2p-raft"

	"github.com/ipfs/ipfs-cluster/state"
)

// errBadRaftState is returned when the consensus component cannot start
// because the cluster peers do not match the raft peers.
var errBadRaftState = errors.New("cluster peers do not match raft peers")

// ErrWaitingForSelf is returned when we are waiting for ourselves to depart
// the peer set, which won't happen
var errWaitingForSelf = errors.New("waiting for ourselves to depart")

// RaftMaxSnapshots indicates how many snapshots to keep in the consensus data
// folder.
// TODO: Maybe include this in Config. Not sure how useful it is to touch
// this anyways.
var RaftMaxSnapshots = 5

// RaftLogCacheSize is the maximum number of logs to cache in-memory.
// This is used to reduce disk I/O for the recently committed entries.
var RaftLogCacheSize = 512

// Are we compiled on a 64-bit architecture?
// https://groups.google.com/forum/#!topic/golang-nuts/vAckmhUMAdQ
// This is used below because raft Observers panic on 32-bit.
const sixtyfour = uint64(^uint(0)) == ^uint64(0)

// raftWrapper performs all Raft-specific operations which are needed by
// Cluster but are not fulfilled by the consensus interface. It should contain
// most of the Raft-related stuff so it can be easily replaced in the future,
// if need be.
type raftWrapper struct {
	raft          *hraft.Raft
	dataFolder    string
	srvConfig     hraft.Configuration
	transport     *hraft.NetworkTransport
	snapshotStore hraft.SnapshotStore
	logStore      hraft.LogStore
	stableStore   hraft.StableStore
	boltdb        *raftboltdb.BoltStore
}

// newRaft launches a go-libp2p-raft consensus peer.
func newRaftWrapper(peers []peer.ID, host host.Host, cfg *Config, fsm hraft.FSM) (*raftWrapper, error) {
	// Set correct LocalID
	cfg.RaftConfig.LocalID = hraft.ServerID(peer.IDB58Encode(host.ID()))

	// Prepare data folder
	dataFolder, err := makeDataFolder(cfg.BaseDir, cfg.DataFolder)
	if err != nil {
		return nil, err
	}
	srvCfg := makeServerConf(peers)

	logger.Debug("creating libp2p Raft transport")
	transport, err := p2praft.NewLibp2pTransport(host, cfg.NetworkTimeout)
	if err != nil {
		return nil, err
	}

	var log hraft.LogStore
	var stable hraft.StableStore
	var snap hraft.SnapshotStore

	logger.Debug("creating raft snapshot store")
	snapstore, err := hraft.NewFileSnapshotStoreWithLogger(
		dataFolder, RaftMaxSnapshots, raftStdLogger)
	if err != nil {
		return nil, err
	}

	logger.Debug("creating BoltDB store")
	store, err := raftboltdb.NewBoltStore(
		filepath.Join(dataFolder, "raft.db"))
	if err != nil {
		return nil, err
	}

	// wraps the store in a LogCache to improve performance.
	// See consul/agent/consul/serger.go
	cacheStore, err := hraft.NewLogCache(RaftLogCacheSize, store)
	if err != nil {
		return nil, err
	}

	stable = store
	log = cacheStore
	snap = snapstore

	logger.Debug("checking for existing raft states")
	hasState, err := hraft.HasExistingState(log, stable, snap)
	if err != nil {
		return nil, err
	}
	if !hasState {
		logger.Info("bootstrapping raft cluster")
		err := hraft.BootstrapCluster(cfg.RaftConfig,
			log, stable, snap, transport, srvCfg)
		if err != nil {
			logger.Error("bootstrapping cluster: ", err)
			return nil, err
		}
	} else {
		logger.Info("raft cluster is already bootstrapped")
	}

	logger.Debug("creating Raft")
	r, err := hraft.NewRaft(cfg.RaftConfig,
		fsm, log, stable, snap, transport)
	if err != nil {
		logger.Error("initializing raft: ", err)
		return nil, err
	}

	raftW := &raftWrapper{
		raft:          r,
		dataFolder:    dataFolder,
		srvConfig:     srvCfg,
		transport:     transport,
		snapshotStore: snap,
		logStore:      log,
		stableStore:   stable,
		boltdb:        store,
	}

	// Handle existing, different configuration
	if hasState {
		cf := r.GetConfiguration()
		if err := cf.Error(); err != nil {
			return nil, err
		}
		currentCfg := cf.Configuration()
		added, removed := diffConfigurations(srvCfg, currentCfg)
		if len(added)+len(removed) > 0 {
			raftW.Shutdown()
			logger.Error("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
			logger.Error("Raft peers do not match cluster peers from the configuration.")
			logger.Error("This likely indicates that this peer has left the cluster and/or")
			logger.Error("has a dirty state. Clean the raft state for this peer")
			logger.Errorf("(%s)", dataFolder)
			logger.Error("bootstrap it to a working cluster.")
			logger.Error("Raft peers:")
			for _, s := range currentCfg.Servers {
				logger.Errorf("  - %s", s.ID)
			}
			logger.Error("Cluster configuration peers:")
			for _, s := range srvCfg.Servers {
				logger.Errorf("  - %s", s.ID)
			}
			logger.Errorf("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
			return nil, errBadRaftState
			//return nil, errors.New("Bad cluster peers")
		}
	}

	return raftW, nil
}

// returns the folder path after creating it.
// if folder is empty, it uses baseDir+Default.
func makeDataFolder(baseDir, folder string) (string, error) {
	if folder == "" {
		folder = filepath.Join(baseDir, DefaultDataSubFolder)
	}

	err := os.MkdirAll(folder, 0700)
	if err != nil {
		return "", err
	}
	return folder, nil
}

// create Raft servers configuration
func makeServerConf(peers []peer.ID) hraft.Configuration {
	sm := make(map[string]struct{})

	servers := make([]hraft.Server, 0)
	for _, pid := range peers {
		p := peer.IDB58Encode(pid)
		_, ok := sm[p]
		if !ok { // avoid dups
			sm[p] = struct{}{}
			servers = append(servers, hraft.Server{
				Suffrage: hraft.Voter,
				ID:       hraft.ServerID(p),
				Address:  hraft.ServerAddress(p),
			})
		}
	}
	return hraft.Configuration{
		Servers: servers,
	}
}

// diffConfigurations returns the serverIDs added and removed from
// c2 in relation to c1.
func diffConfigurations(
	c1, c2 hraft.Configuration) (added, removed []hraft.ServerID) {
	m1 := make(map[hraft.ServerID]struct{})
	m2 := make(map[hraft.ServerID]struct{})
	added = make([]hraft.ServerID, 0)
	removed = make([]hraft.ServerID, 0)
	for _, s := range c1.Servers {
		m1[s.ID] = struct{}{}
	}
	for _, s := range c2.Servers {
		m2[s.ID] = struct{}{}
	}
	for k := range m1 {
		_, ok := m2[k]
		if !ok {
			removed = append(removed, k)
		}
	}
	for k := range m2 {
		_, ok := m1[k]
		if !ok {
			added = append(added, k)
		}
	}
	return
}

// WaitForLeader holds until Raft says we have a leader.
// Returns uf ctx is cancelled.
func (rw *raftWrapper) WaitForLeader(ctx context.Context) (string, error) {
	obsCh := make(chan hraft.Observation, 1)
	if sixtyfour { // 32-bit systems don't support observers
		observer := hraft.NewObserver(obsCh, false, nil)
		rw.raft.RegisterObserver(observer)
		defer rw.raft.DeregisterObserver(observer)
	}
	ticker := time.NewTicker(time.Second / 2)
	for {
		select {
		case obs := <-obsCh:
			_ = obs
			// See https://github.com/hashicorp/raft/issues/254
			// switch obs.Data.(type) {
			// case hraft.LeaderObservation:
			// 	lObs := obs.Data.(hraft.LeaderObservation)
			// 	logger.Infof("Raft Leader elected: %s",
			// 		lObs.Leader)
			// 	return string(lObs.Leader), nil
			// }
		case <-ticker.C:
			if l := rw.raft.Leader(); l != "" {
				logger.Debug("waitForleaderTimer")
				logger.Infof("Raft Leader elected: %s", l)
				ticker.Stop()
				return string(l), nil
			}
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

// WaitForUpdates holds until Raft has synced to the last index in the log
func (rw *raftWrapper) WaitForUpdates(ctx context.Context) error {
	logger.Info("Raft state is catching up")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			lai := rw.raft.AppliedIndex()
			li := rw.raft.LastIndex()
			logger.Debugf("current Raft index: %d/%d",
				lai, li)
			if lai == li {
				return nil
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (rw *raftWrapper) WaitForPeer(ctx context.Context, pid string, depart bool) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			peers, err := rw.Peers()
			if err != nil {
				return err
			}

			if len(peers) == 1 && pid == peers[0] && depart {
				return errWaitingForSelf
			}

			found := find(peers, pid)

			// departing
			if depart && !found {
				return nil
			}

			// joining
			if !depart && found {
				return nil
			}

			time.Sleep(50 * time.Millisecond)
		}
	}
}

// Snapshot tells Raft to take a snapshot.
func (rw *raftWrapper) Snapshot() error {
	future := rw.raft.Snapshot()
	err := future.Error()
	if err != nil && err.Error() != hraft.ErrNothingNewToSnapshot.Error() {
		return err
	}
	return nil
}

// Shutdown shutdown Raft and closes the BoltDB.
func (rw *raftWrapper) Shutdown() error {
	errMsgs := ""
	err := rw.Snapshot()
	if err != nil {
		errMsgs += "could not snapshot raft: " + err.Error() + ".\n"
	}
	future := rw.raft.Shutdown()
	err = future.Error()
	if err != nil {
		errMsgs += "could not shutdown raft: " + err.Error() + ".\n"
	}

	err = rw.boltdb.Close() // important!
	if err != nil {
		errMsgs += "could not close boltdb: " + err.Error()
	}

	if errMsgs != "" {
		return errors.New(errMsgs)
	}

	return nil
}

// AddPeer adds a peer to Raft
func (rw *raftWrapper) AddPeer(peer string) error {
	// Check that we don't have it to not waste
	// log entries if so.
	peers, err := rw.Peers()
	if err != nil {
		return err
	}
	if find(peers, peer) {
		logger.Infof("%s is already a raft peer", peer)
		return nil
	}

	future := rw.raft.AddVoter(
		hraft.ServerID(peer),
		hraft.ServerAddress(peer),
		0,
		0) // TODO: Extra cfg value?
	err = future.Error()
	if err != nil {
		logger.Error("raft cannot add peer: ", err)
	}
	return err
}

// RemovePeer removes a peer from Raft
func (rw *raftWrapper) RemovePeer(peer string) error {
	// Check that we have it to not waste
	// log entries if we don't.
	peers, err := rw.Peers()
	if err != nil {
		return err
	}
	if !find(peers, peer) {
		logger.Infof("%s is not among raft peers", peer)
		return nil
	}

	if len(peers) == 1 && peers[0] == peer {
		return errors.New("cannot remove ourselves from a 1-peer cluster")
	}

	rmFuture := rw.raft.RemoveServer(
		hraft.ServerID(peer),
		0,
		0) // TODO: Extra cfg value?
	err = rmFuture.Error()
	if err != nil {
		logger.Error("raft cannot remove peer: ", err)
		return err
	}

	return nil
}

// Leader returns Raft's leader. It may be an empty string if
// there is no leader or it is unknown.
func (rw *raftWrapper) Leader() string {
	return string(rw.raft.Leader())
}

func (rw *raftWrapper) Peers() ([]string, error) {
	ids := make([]string, 0)

	configFuture := rw.raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		return nil, err
	}

	for _, server := range configFuture.Configuration().Servers {
		ids = append(ids, string(server.ID))
	}

	return ids, nil
}

// latestSnapshot looks for the most recent raft snapshot stored at the
// provided basedir.  It returns a boolean indicating if any snapshot is
// readable, the snapshot's metadata, and a reader to the snapshot's bytes
func latestSnapshot(raftDataFolder string) (*hraft.SnapshotMeta, io.ReadCloser, error) {
	store, err := hraft.NewFileSnapshotStore(raftDataFolder, RaftMaxSnapshots, nil)
	if err != nil {
		return nil, nil, err
	}
	snapMetas, err := store.List()
	if err != nil {
		return nil, nil, err
	}
	if len(snapMetas) == 0 { // no error if snapshot isn't found
		return nil, nil, nil
	}
	meta, r, err := store.Open(snapMetas[0].ID)
	if err != nil {
		return nil, nil, err
	}
	return meta, r, nil
}

// LastStateRaw returns the bytes of the last snapshot stored, its metadata,
// and a flag indicating whether any snapshot was found.
func LastStateRaw(cfg *Config) (io.Reader, bool, error) {
	// Read most recent snapshot
	dataFolder, err := makeDataFolder(cfg.BaseDir, cfg.DataFolder)
	if err != nil {
		return nil, false, err
	}
	meta, r, err := latestSnapshot(dataFolder)
	if err != nil {
		return nil, false, err
	}
	if meta == nil { // no snapshots could be read
		return nil, false, nil
	}
	return r, true, nil
}

// SnapshotSave saves the provided state to a snapshot in the
// raft data path.  Old raft data is backed up and replaced
// by the new snapshot
func SnapshotSave(cfg *Config, newState state.State, pid peer.ID) error {
	newStateBytes, err := p2praft.EncodeSnapshot(newState)
	if err != nil {
		return err
	}
	dataFolder, err := makeDataFolder(cfg.BaseDir, cfg.DataFolder)
	if err != nil {
		return err
	}
	meta, _, err := latestSnapshot(dataFolder)
	if err != nil {
		return err
	}

	// make a new raft snapshot
	var raftSnapVersion hraft.SnapshotVersion
	raftSnapVersion = 1 // As of hraft v1.0.0 this is always 1
	configIndex := uint64(1)
	var raftIndex uint64
	var raftTerm uint64
	var srvCfg hraft.Configuration
	if meta != nil {
		raftIndex = meta.Index
		raftTerm = meta.Term
		srvCfg = meta.Configuration
		cleanupRaft(dataFolder)
	} else {
		raftIndex = uint64(1)
		raftTerm = uint64(1)
		srvCfg = makeServerConf([]peer.ID{pid})
	}

	snapshotStore, err := hraft.NewFileSnapshotStoreWithLogger(dataFolder, RaftMaxSnapshots, nil)
	if err != nil {
		return err
	}
	_, dummyTransport := hraft.NewInmemTransport("")

	sink, err := snapshotStore.Create(raftSnapVersion, raftIndex, raftTerm, srvCfg, configIndex, dummyTransport)
	if err != nil {
		return err
	}

	_, err = sink.Write(newStateBytes)
	if err != nil {
		sink.Cancel()
		return err
	}
	err = sink.Close()
	if err != nil {
		return err
	}
	return nil
}

func cleanupRaft(dataFolder string) error {
	dbh := newDataBackupHelper(dataFolder)
	err := dbh.makeBackup()
	if err != nil {
		logger.Warning(err)
		logger.Warning("the state could not be cleaned properly")
		logger.Warning("manual intervention may be needed before starting cluster again")
	}
	return nil
}

// only call when Raft is shutdown
func (rw *raftWrapper) Clean() error {
	return cleanupRaft(rw.dataFolder)
}

func find(s []string, elem string) bool {
	for _, selem := range s {
		if selem == elem {
			return true
		}
	}
	return false
}
