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

	crypto "gx/ipfs/QmP1DfoUjiWH2ZBo1PBH6FupdBucbDepx3HpWmEY6JMUpY/go-libp2p-crypto"
	basichost "gx/ipfs/QmQA5mdxru8Bh6dpC9PJfSkumqnmHgJX7knxSgBo5Lpime/go-libp2p/p2p/host/basic"
	logging "gx/ipfs/QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52/go-log"
	host "gx/ipfs/QmUywuGNZoUKV8B9iyvup9bPkLiMrhTsyVMkeSXW5VxAfC/go-libp2p-host"
	swarm "gx/ipfs/QmVkDnNm71vYyY6s6rXwtmyDYis3WkKyrEhMECwT6R12uJ/go-libp2p-swarm"
	peerstore "gx/ipfs/QmXZSd1qR5BxZkPyuwfT5jpqQFScZccoZvDneXsKzCNHWX/go-libp2p-peerstore"
	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
	ma "gx/ipfs/QmcyqRMCAXVtYPS4DiBrA7sezL9rRGfW8Ctx7cywL4TXJj/go-multiaddr"
	peer "gx/ipfs/QmdS9KpbDyPrieswibZhkod1oXqRwZJrUPzxCofAMWpFGq/go-libp2p-peer"
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
	cc, err := NewConsensus([]peer.ID{h.ID()},
		h, fmt.Sprintf("raftFolderFromTests%d", port), st)
	if err != nil {
		t.Fatal("cannot create Consensus:", err)
	}
	cc.SetClient(test.NewMockRPCClient(t))
	<-cc.Ready()
	return cc
}

func TestShutdownConsensus(t *testing.T) {
	// Bring it up twice to make sure shutdown cleans up properly
	// but also to make sure raft comes up ok when re-initialized
	defer cleanRaft(p2pPort)
	cc := testingConsensus(t, p2pPort)
	err := cc.Shutdown()
	if err != nil {
		t.Fatal("Consensus cannot shutdown:", err)
	}
	cc.Shutdown()
	cc = testingConsensus(t, p2pPort)
	err = cc.Shutdown()
	if err != nil {
		t.Fatal("Consensus cannot shutdown:", err)
	}
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
	defer cleanRaft(p2pPort)
	defer cc.Shutdown()

	err := cc.LogRmPeer(test.TestPeerID1)
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
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
