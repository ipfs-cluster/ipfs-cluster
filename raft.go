package ipfscluster

import (
	"io/ioutil"
	"path/filepath"

	hashiraft "github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	libp2praft "github.com/libp2p/go-libp2p-raft"
)

// libp2pRaftWrap wraps the stuff that we need to run
// hashicorp raft. We carry it around for convenience
type libp2pRaftWrap struct {
	raft          *hashiraft.Raft
	transport     *libp2praft.Libp2pTransport
	snapshotStore hashiraft.SnapshotStore
	logStore      hashiraft.LogStore
	stableStore   hashiraft.StableStore
	peerstore     *libp2praft.Peerstore
	boltdb        *raftboltdb.BoltStore
}

// This function does all heavy the work which is specifically related to
// hashicorp's Raft. Other places should just rely on the Consensus interface.
func makeLibp2pRaft(cfg *Config, host host.Host, state State, op *clusterLogOp) (*libp2praft.Consensus, *libp2praft.Actor, *libp2pRaftWrap, error) {
	logger.Debug("creating libp2p Raft transport")
	transport, err := libp2praft.NewLibp2pTransportWithHost(host)
	if err != nil {
		logger.Error("creating libp2p-raft transport: ", err)
		return nil, nil, nil, err
	}

	//logger.Debug("opening connections")
	//transport.OpenConns()

	pstore := &libp2praft.Peerstore{}
	strPeers := []string{peer.IDB58Encode(host.ID())}
	for _, addr := range cfg.ClusterPeers {
		p, _, err := multiaddrSplit(addr)
		if err != nil {
			return nil, nil, nil, err
		}
		strPeers = append(strPeers, p.Pretty())
	}
	pstore.SetPeers(strPeers)

	logger.Debug("creating OpLog")
	cons := libp2praft.NewOpLog(state, op)

	raftCfg := cfg.RaftConfig
	if raftCfg == nil {
		raftCfg = hashiraft.DefaultConfig()
		raftCfg.EnableSingleNode = raftSingleMode
	}
	raftCfg.LogOutput = ioutil.Discard
	raftCfg.Logger = raftStdLogger

	logger.Debug("creating file snapshot store")
	snapshots, err := hashiraft.NewFileSnapshotStoreWithLogger(cfg.ConsensusDataFolder, maxSnapshots, raftStdLogger)
	if err != nil {
		logger.Error("creating file snapshot store: ", err)
		return nil, nil, nil, err
	}

	logger.Debug("creating BoltDB log store")
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.ConsensusDataFolder, "raft.db"))
	if err != nil {
		logger.Error("creating bolt store: ", err)
		return nil, nil, nil, err
	}

	logger.Debug("creating Raft")
	r, err := hashiraft.NewRaft(raftCfg, cons.FSM(), logStore, logStore, snapshots, pstore, transport)
	if err != nil {
		logger.Error("initializing raft: ", err)
		return nil, nil, nil, err
	}

	return cons, libp2praft.NewActor(r), &libp2pRaftWrap{
		raft:          r,
		transport:     transport,
		snapshotStore: snapshots,
		logStore:      logStore,
		stableStore:   logStore,
		peerstore:     pstore,
		boltdb:        logStore,
	}, nil
}
