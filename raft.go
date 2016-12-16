package ipfscluster

import (
	"io/ioutil"
	"path/filepath"

	host "github.com/libp2p/go-libp2p-host"
	libp2praft "github.com/libp2p/go-libp2p-raft"

	peer "github.com/libp2p/go-libp2p-peer"

	hashiraft "github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
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

	logger.Debug("opening connections")
	transport.OpenConns()

	pstore := &libp2praft.Peerstore{}
	hPeers := host.Peerstore().Peers()
	strPeers := make([]string, 0, len(hPeers))
	for _, p := range hPeers {
		strPeers = append(strPeers, peer.IDB58Encode(p))
	}
	pstore.SetPeers(strPeers)

	logger.Debug("creating OpLog")
	cons := libp2praft.NewOpLog(state, op)

	raftCfg := hashiraft.DefaultConfig()
	raftCfg.EnableSingleNode = raftSingleMode
	if SilentRaft {
		raftCfg.LogOutput = ioutil.Discard
		raftCfg.Logger = nil
	}
	logger.Debug("creating file snapshot store")
	snapshots, err := hashiraft.NewFileSnapshotStore(cfg.RaftFolder, maxSnapshots, nil)
	if err != nil {
		logger.Error("creating file snapshot store: ", err)
		return nil, nil, nil, err
	}

	logger.Debug("creating BoltDB log store")
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.RaftFolder, "raft.db"))
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
