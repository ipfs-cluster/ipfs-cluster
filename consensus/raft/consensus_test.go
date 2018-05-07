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
	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-host"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"
)

func cleanRaft(idn int) {
	os.RemoveAll(fmt.Sprintf("raftFolderFromTests-%d", idn))
}

func consensusListenAddr(c *Consensus) ma.Multiaddr {
	return c.host.Addrs()[0]
}

func consensusAddr(c *Consensus) ma.Multiaddr {
	cAddr, _ := ma.NewMultiaddr(fmt.Sprintf("%s/ipfs/%s", consensusListenAddr(c), c.host.ID().Pretty()))
	return cAddr
}

func init() {
	_ = logging.LevelDebug
	//logging.SetLogLevel("consensus", "DEBUG")
}

func makeTestingHost(t *testing.T) host.Host {
	h, err := libp2p.New(
		context.Background(),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func testingConsensus(t *testing.T, idn int) *Consensus {
	cleanRaft(idn)
	h := makeTestingHost(t)
	st := mapstate.NewMapState()

	cfg := &Config{}
	cfg.Default()
	cfg.DataFolder = fmt.Sprintf("raftFolderFromTests-%d", idn)
	cfg.hostShutdown = true

	cc, err := NewConsensus(h, cfg, st, false)
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
	cc := testingConsensus(t, 1)
	defer cleanRaft(1)
	err := cc.Shutdown()
	if err != nil {
		t.Fatal("Consensus cannot shutdown:", err)
	}
	err = cc.Shutdown() // should be fine to shutdown twice
	if err != nil {
		t.Fatal("Consensus should be able to shutdown several times")
	}
	cleanRaft(1)

	cc = testingConsensus(t, 1)
	err = cc.Shutdown()
	if err != nil {
		t.Fatal("Consensus cannot shutdown:", err)
	}
	cleanRaft(1)
}

func TestConsensusPin(t *testing.T) {
	cc := testingConsensus(t, 1)
	defer cleanRaft(1) // Remember defer runs in LIFO order
	defer cc.Shutdown()

	c, _ := cid.Decode(test.TestCid1)
	err := cc.LogPin(api.Pin{Cid: c, ReplicationFactorMin: -1, ReplicationFactorMax: -1})
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
	cc := testingConsensus(t, 1)
	defer cleanRaft(1)
	defer cc.Shutdown()

	c, _ := cid.Decode(test.TestCid2)
	err := cc.LogUnpin(api.PinCid(c))
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
	}
}

func TestConsensusAddPeer(t *testing.T) {
	cc := testingConsensus(t, 1)
	cc2 := testingConsensus(t, 2)
	t.Log(cc.host.ID().Pretty())
	t.Log(cc2.host.ID().Pretty())
	defer cleanRaft(1)
	defer cleanRaft(2)
	defer cc.Shutdown()
	defer cc2.Shutdown()

	cc.host.Peerstore().AddAddr(cc2.host.ID(), consensusListenAddr(cc2), peerstore.PermanentAddrTTL)
	err := cc.AddPeer(cc2.host.ID())
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = cc2.raft.WaitForPeer(ctx, cc.host.ID().Pretty(), false)
	if err != nil {
		t.Fatal(err)
	}

	peers, err := cc2.raft.Peers()
	if err != nil {
		t.Fatal(err)
	}

	if len(peers) != 2 {
		t.Error("peer was not added")
	}
}

func TestConsensusRmPeer(t *testing.T) {
	cc := testingConsensus(t, 1)
	cc2 := testingConsensus(t, 2)
	defer cleanRaft(1)
	defer cleanRaft(2)
	defer cc.Shutdown()
	defer cc2.Shutdown()

	cc.host.Peerstore().AddAddr(cc2.host.ID(), consensusListenAddr(cc2), peerstore.PermanentAddrTTL)

	err := cc.AddPeer(cc2.host.ID())
	if err != nil {
		t.Error("could not add peer:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = cc.raft.WaitForPeer(ctx, cc2.host.ID().Pretty(), false)
	if err != nil {
		t.Fatal(err)
	}
	cc.raft.WaitForLeader(ctx)

	c, _ := cid.Decode(test.TestCid1)
	err = cc.LogPin(api.Pin{Cid: c, ReplicationFactorMin: -1, ReplicationFactorMax: -1})
	if err != nil {
		t.Error("could not pin after adding peer:", err)
	}

	time.Sleep(2 * time.Second)

	// Remove unexisting peer
	err = cc.RmPeer(test.TestPeerID1)
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
	}

	// Remove real peer. At least the leader can succeed
	err = cc2.RmPeer(cc.host.ID())
	err2 := cc.RmPeer(cc2.host.ID())
	if err != nil && err2 != nil {
		t.Error("could not remove peer:", err, err2)
	}

	err = cc.raft.WaitForPeer(ctx, cc2.host.ID().Pretty(), true)
	if err != nil {
		t.Fatal(err)
	}
}

func TestConsensusLeader(t *testing.T) {
	cc := testingConsensus(t, 1)
	pID := cc.host.ID()
	defer cleanRaft(1)
	defer cc.Shutdown()
	l, err := cc.Leader()
	if err != nil {
		t.Fatal("No leader:", err)
	}

	if l != pID {
		t.Errorf("expected %s but the leader appears as %s", pID, l)
	}
}

func TestRaftLatestSnapshot(t *testing.T) {
	cc := testingConsensus(t, 1)
	defer cleanRaft(1)
	defer cc.Shutdown()

	// Make pin 1
	c1, _ := cid.Decode(test.TestCid1)
	err := cc.LogPin(api.Pin{Cid: c1, ReplicationFactorMin: -1, ReplicationFactorMax: -1})
	if err != nil {
		t.Error("the first pin did not make it to the log:", err)
	}

	time.Sleep(250 * time.Millisecond)
	err = cc.raft.Snapshot()
	if err != nil {
		t.Error("the first snapshot was not taken successfully")
	}

	// Make pin 2
	c2, _ := cid.Decode(test.TestCid2)
	err = cc.LogPin(api.Pin{Cid: c2, ReplicationFactorMin: -1, ReplicationFactorMax: -1})
	if err != nil {
		t.Error("the second pin did not make it to the log:", err)
	}

	time.Sleep(250 * time.Millisecond)
	err = cc.raft.Snapshot()
	if err != nil {
		t.Error("the second snapshot was not taken successfully")
	}

	// Make pin 3
	c3, _ := cid.Decode(test.TestCid3)
	err = cc.LogPin(api.Pin{Cid: c3, ReplicationFactorMin: -1, ReplicationFactorMax: -1})
	if err != nil {
		t.Error("the third pin did not make it to the log:", err)
	}

	time.Sleep(250 * time.Millisecond)
	err = cc.raft.Snapshot()
	if err != nil {
		t.Error("the third snapshot was not taken successfully")
	}

	// Call raft.LastState and ensure we get the correct state
	snapState := mapstate.NewMapState()
	r, snapExists, err := LastStateRaw(cc.config)
	if !snapExists {
		t.Fatal("No snapshot found by LastStateRaw")
	}
	if err != nil {
		t.Fatal("Error while taking snapshot", err)
	}
	err = snapState.Migrate(r)
	if err != nil {
		t.Fatal("Snapshot bytes returned could not restore to state")
	}
	pins := snapState.List()
	if len(pins) != 3 {
		t.Fatal("Latest snapshot not read")
	}
}
