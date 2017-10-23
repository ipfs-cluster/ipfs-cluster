package raft

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/state/mapstate"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	crypto "github.com/libp2p/go-libp2p-crypto"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	swarm "github.com/libp2p/go-libp2p-swarm"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	ma "github.com/multiformats/go-multiaddr"
)

var p2pPort = 10000
var p2pPortAlt = 11000

func cleanRaft(port int) {
	os.RemoveAll(fmt.Sprintf("raftFolderFromTests%d", port))
}

func init() {
	_ = logging.LevelDebug
	//logging.SetLogLevel("consensus", "DEBUG")
}

func makeTestingHost(t *testing.T, port int) host.Host {
	priv, pub, _ := crypto.GenerateKeyPair(crypto.RSA, 2048)
	pid, _ := peer.IDFromPublicKey(pub)
	maddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port))
	ps := peerstore.NewPeerstore()
	ps.AddPubKey(pid, pub)
	ps.AddPrivKey(pid, priv)
	n, _ := swarm.NewNetwork(
		context.Background(),
		[]ma.Multiaddr{maddr},
		pid, ps, nil)
	return basichost.New(n)
}

func testingConsensus(t *testing.T, port int) *Consensus {
	h := makeTestingHost(t, port)
	st := mapstate.NewMapState()

	cfg := &Config{}
	cfg.Default()
	cfg.DataFolder = fmt.Sprintf("raftFolderFromTests%d", port)

	cc, err := NewConsensus([]peer.ID{h.ID()}, h, cfg, st)
	if err != nil {
		t.Fatal("cannot create Consensus:", err)
	}
	cc.SetClient(test.NewMockRPCClientWithHost(t, h))
	<-cc.Ready()
	return cc
}

func TestShutdownConsensus(t *testing.T) {
	// Bring it up twice to make sure shutdown cleans up properly
	// but also to make sure raft comes up ok when re-initialized
	cc := testingConsensus(t, p2pPort)
	err := cc.Shutdown()
	if err != nil {
		t.Fatal("Consensus cannot shutdown:", err)
	}
	err = cc.Shutdown() // should be fine to shutdown twice
	if err != nil {
		t.Fatal("Consensus should be able to shutdown several times")
	}
	cleanRaft(p2pPort)

	cc = testingConsensus(t, p2pPort)
	err = cc.Shutdown()
	if err != nil {
		t.Fatal("Consensus cannot shutdown:", err)
	}
	cleanRaft(p2pPort)
}

func TestConsensusPin(t *testing.T) {
	cc := testingConsensus(t, p2pPort)
	defer cleanRaft(p2pPort) // Remember defer runs in LIFO order
	defer cc.Shutdown()

	c, _ := cid.Decode(test.TestCid1)
	err := cc.LogPin(api.Pin{Cid: c, ReplicationFactor: -1})
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
	}

	time.Sleep(250 * time.Millisecond)
	st, err := cc.State()
	if err != nil {
		t.Fatal("error gettinng state:", err)
	}

	pins := st.List()
	if len(pins) != 1 || pins[0].Cid.String() != test.TestCid1 {
		t.Error("the added pin should be in the state")
	}
}

func TestConsensusUnpin(t *testing.T) {
	cc := testingConsensus(t, p2pPort)
	defer cleanRaft(p2pPort)
	defer cc.Shutdown()

	c, _ := cid.Decode(test.TestCid2)
	err := cc.LogUnpin(api.PinCid(c))
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
	}
}

func TestConsensusLogAddPeer(t *testing.T) {
	cc := testingConsensus(t, p2pPort)
	cc2 := testingConsensus(t, p2pPortAlt)
	t.Log(cc.host.ID().Pretty())
	t.Log(cc2.host.ID().Pretty())
	defer cleanRaft(p2pPort)
	defer cleanRaft(p2pPortAlt)
	defer cc.Shutdown()
	defer cc2.Shutdown()

	addr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", p2pPortAlt))
	haddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", cc2.host.ID().Pretty()))

	cc.host.Peerstore().AddAddr(cc2.host.ID(), addr, peerstore.TempAddrTTL)
	err := cc.LogAddPeer(addr.Encapsulate(haddr))
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
	}
}

func TestConsensusLogRmPeer(t *testing.T) {
	cc := testingConsensus(t, p2pPort)
	cc2 := testingConsensus(t, p2pPortAlt)
	defer cleanRaft(p2pPort)
	defer cleanRaft(p2pPortAlt)
	defer cc.Shutdown()
	defer cc2.Shutdown()

	addr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", p2pPortAlt))
	haddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", cc2.host.ID().Pretty()))
	cc.host.Peerstore().AddAddr(cc2.host.ID(), addr, peerstore.TempAddrTTL)

	err := cc.LogAddPeer(addr.Encapsulate(haddr))
	if err != nil {
		t.Error("could not add peer:", err)
	}

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	cc.raft.WaitForLeader(ctx)

	c, _ := cid.Decode(test.TestCid1)
	err = cc.LogPin(api.Pin{Cid: c, ReplicationFactor: -1})
	if err != nil {
		t.Error("could not pin after adding peer:", err)
	}

	time.Sleep(2 * time.Second)

	// Remove unexisting peer
	err = cc.LogRmPeer(test.TestPeerID1)
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
	}

	// Remove real peer. At least the leader can succeed
	err = cc2.LogRmPeer(cc.host.ID())
	err2 := cc.LogRmPeer(cc2.host.ID())
	if err != nil && err2 != nil {
		t.Error("could not remove peer:", err, err2)
	}
}

func TestConsensusLeader(t *testing.T) {
	cc := testingConsensus(t, p2pPort)
	pID := cc.host.ID()
	defer cleanRaft(p2pPort)
	defer cc.Shutdown()
	l, err := cc.Leader()
	if err != nil {
		t.Fatal("No leader:", err)
	}

	if l != pID {
		t.Errorf("expected %s but the leader appears as %s", pID, l)
	}
}
