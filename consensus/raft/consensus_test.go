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

	crypto "gx/ipfs/QmNiCwBNA8MWDADTFVq1BonUEJbS2SvjAoNkZZrhEwcuUi/go-libp2p-crypto"
	peerstore "gx/ipfs/QmQMQ2RUjnaEEX8ybmrhuFFGhAwPjyL1Eo6ZoJGD7aAccM/go-libp2p-peerstore"
	basichost "gx/ipfs/QmSNJRX4uphb3Eyp69uYbpRVvgqjPxfjnJmjcdMWkDH5Pn/go-libp2p/p2p/host/basic"
	ma "gx/ipfs/QmSWLfmj5frN9xVLMMN846dMDriy5wN5jeghUm7aTW3DAG/go-multiaddr"
	logging "gx/ipfs/QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52/go-log"
	swarm "gx/ipfs/QmY8hduizbuACvYmL4aZQbpFeKhEQJ1Nom2jY6kv6rL8Gf/go-libp2p-swarm"
	peer "gx/ipfs/QmZcUPvPhD1Xvk6mwijYF8AfR3mG31S1YsEfHG4khrFPRr/go-libp2p-peer"
	host "gx/ipfs/QmbzbRyd22gcW92U1rA2yKagB3myMYhk45XBknJ49F9XWJ/go-libp2p-host"
	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
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
