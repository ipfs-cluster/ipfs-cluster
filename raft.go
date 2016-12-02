package ipfscluster

import (
	"os"
	"path/filepath"

	host "gx/ipfs/QmPTGbC34bPKaUm9wTxBo7zSCac7pDuG42ZmnXC718CKZZ/go-libp2p-host"
	libp2praft "gx/ipfs/QmdHo2LQKmGQ6rDAWFxnzNuW3z8b6Xmw3wEFsMQaj9Rsqj/go-libp2p-raft"

	peer "gx/ipfs/QmfMmLGoKzCHDN7cGgk64PJr4iipzidDRME8HABSJqvmhC/go-libp2p-peer"

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
}

// This function does all heavy the work which is specifically related to
// hashicorp's Raft. Other places should just rely on the Consensus interface.
func makeLibp2pRaft(cfg *ClusterConfig, host host.Host, state ClusterState, op clusterLogOp) (*libp2praft.Consensus, *libp2praft.Actor, *libp2pRaftWrap, error) {
	transport, err := libp2praft.NewLibp2pTransportWithHost(host)
	if err != nil {
		logger.Error("creating libp2p-raft transport: ", err)
		return nil, nil, nil, err
	}

	transport.OpenConns()

	pstore := &libp2praft.Peerstore{}
	hPeers := host.Peerstore().Peers()
	strPeers := make([]string, 0, len(hPeers))
	for _, p := range hPeers {
		strPeers = append(strPeers, peer.IDB58Encode(p))
	}
	pstore.SetPeers(strPeers)

	cons := libp2praft.NewOpLog(state, op)

	raftCfg := hashiraft.DefaultConfig()
	raftCfg.EnableSingleNode = raftSingleMode
	snapshots, err := hashiraft.NewFileSnapshotStore(cfg.RaftFolder, maxSnapshots, nil)
	if err != nil {
		logger.Error("creating file snapshot store: ", err)
		return nil, nil, nil, err
	}

	logStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.RaftFolder, "raft.db"))
	if err != nil {
		logger.Error("creating bolt store: ", err)
		return nil, nil, nil, err
	}

	r, err := hashiraft.NewRaft(raftCfg, cons, logStore, logStore, snapshots, pstore, transport)
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
	}, nil
}

func cleanRaft(cfg *ClusterConfig) {
	os.RemoveAll(cfg.RaftFolder)
}
