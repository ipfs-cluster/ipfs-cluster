package raft

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/inmem"
	"github.com/ipfs-cluster/ipfs-cluster/state/dsstate"
	"github.com/ipfs-cluster/ipfs-cluster/test"

	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p/core/host"
	peerstore "github.com/libp2p/go-libp2p/core/peerstore"
)

func cleanRaft(idn int) {
	os.RemoveAll(fmt.Sprintf("raftFolderFromTests-%d", idn))
}

func testPin(c api.Cid) api.Pin {
	p := api.PinCid(c)
	p.ReplicationFactorMin = -1
	p.ReplicationFactorMax = -1
	return p
}

func makeTestingHost(t *testing.T) host.Host {
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func testingConsensus(t *testing.T, idn int) *Consensus {
	ctx := context.Background()
	cleanRaft(idn)
	h := makeTestingHost(t)

	cfg := &Config{}
	cfg.Default()
	cfg.DataFolder = fmt.Sprintf("raftFolderFromTests-%d", idn)
	cfg.hostShutdown = true

	cc, err := NewConsensus(h, cfg, inmem.New(), false)
	if err != nil {
		t.Fatal("cannot create Consensus:", err)
	}
	cc.SetClient(test.NewMockRPCClientWithHost(t, h))
	<-cc.Ready(ctx)
	return cc
}

func TestShutdownConsensus(t *testing.T) {
	ctx := context.Background()
	// Bring it up twice to make sure shutdown cleans up properly
	// but also to make sure raft comes up ok when re-initialized
	cc := testingConsensus(t, 1)
	defer cleanRaft(1)
	err := cc.Shutdown(ctx)
	if err != nil {
		t.Fatal("Consensus cannot shutdown:", err)
	}
	err = cc.Shutdown(ctx) // should be fine to shutdown twice
	if err != nil {
		t.Fatal("Consensus should be able to shutdown several times")
	}
	cleanRaft(1)

	cc = testingConsensus(t, 1)
	err = cc.Shutdown(ctx)
	if err != nil {
		t.Fatal("Consensus cannot shutdown:", err)
	}
	cleanRaft(1)
}

func TestConsensusPin(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	defer cleanRaft(1) // Remember defer runs in LIFO order
	defer cc.Shutdown(ctx)

	err := cc.LogPin(ctx, testPin(test.Cid1))
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
	}

	time.Sleep(250 * time.Millisecond)
	st, err := cc.State(ctx)
	if err != nil {
		t.Fatal("error getting state:", err)
	}

	out := make(chan api.Pin, 10)
	err = st.List(ctx, out)
	if err != nil {
		t.Fatal(err)
	}

	var pins []api.Pin
	for p := range out {
		pins = append(pins, p)
	}

	if len(pins) != 1 || !pins[0].Cid.Equals(test.Cid1) {
		t.Error("the added pin should be in the state")
	}
}

func TestConsensusUnpin(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	defer cleanRaft(1)
	defer cc.Shutdown(ctx)

	err := cc.LogUnpin(ctx, api.PinCid(test.Cid1))
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
	}
}

func TestConsensusUpdate(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	defer cleanRaft(1)
	defer cc.Shutdown(ctx)

	// Pin first
	pin := testPin(test.Cid1)
	pin.Type = api.ShardType
	err := cc.LogPin(ctx, pin)
	if err != nil {
		t.Fatal("the initial operation did not make it to the log:", err)
	}
	time.Sleep(250 * time.Millisecond)

	// Update pin
	pin.Reference = &test.Cid2
	err = cc.LogPin(ctx, pin)
	if err != nil {
		t.Error("the update op did not make it to the log:", err)
	}

	time.Sleep(250 * time.Millisecond)
	st, err := cc.State(ctx)
	if err != nil {
		t.Fatal("error getting state:", err)
	}

	out := make(chan api.Pin, 10)
	err = st.List(ctx, out)
	if err != nil {
		t.Fatal(err)
	}

	var pins []api.Pin
	for p := range out {
		pins = append(pins, p)
	}

	if len(pins) != 1 || !pins[0].Cid.Equals(test.Cid1) {
		t.Error("the added pin should be in the state")
	}
	if !pins[0].Reference.Equals(test.Cid2) {
		t.Error("pin updated incorrectly")
	}
}

func TestConsensusAddPeer(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	cc2 := testingConsensus(t, 2)
	t.Log(cc.host.ID())
	t.Log(cc2.host.ID())
	defer cleanRaft(1)
	defer cleanRaft(2)
	defer cc.Shutdown(ctx)
	defer cc2.Shutdown(ctx)

	cc.host.Peerstore().AddAddrs(cc2.host.ID(), cc2.host.Addrs(), peerstore.PermanentAddrTTL)
	err := cc.AddPeer(ctx, cc2.host.ID())
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = cc2.raft.WaitForPeer(ctx, cc.host.ID().String(), false)
	if err != nil {
		t.Fatal(err)
	}

	peers, err := cc2.raft.Peers(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(peers) != 2 {
		t.Error("peer was not added")
	}
}

func TestConsensusRmPeer(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	cc2 := testingConsensus(t, 2)
	defer cleanRaft(1)
	defer cleanRaft(2)
	defer cc.Shutdown(ctx)
	defer cc2.Shutdown(ctx)

	cc.host.Peerstore().AddAddrs(cc2.host.ID(), cc2.host.Addrs(), peerstore.PermanentAddrTTL)

	err := cc.AddPeer(ctx, cc2.host.ID())
	if err != nil {
		t.Error("could not add peer:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = cc.raft.WaitForPeer(ctx, cc2.host.ID().String(), false)
	if err != nil {
		t.Fatal(err)
	}
	cc.raft.WaitForLeader(ctx)

	err = cc.LogPin(ctx, testPin(test.Cid1))
	if err != nil {
		t.Error("could not pin after adding peer:", err)
	}

	time.Sleep(2 * time.Second)

	// Remove unexisting peer
	err = cc.RmPeer(ctx, test.PeerID1)
	if err != nil {
		t.Fatal("the operation did not make it to the log:", err)
	}

	// Remove real peer. At least the leader can succeed
	err = cc2.RmPeer(ctx, cc.host.ID())
	err2 := cc.RmPeer(ctx, cc2.host.ID())
	if err != nil && err2 != nil {
		t.Fatal("could not remove peer:", err, err2)
	}

	err = cc.raft.WaitForPeer(ctx, cc2.host.ID().String(), true)
	if err != nil {
		t.Fatal(err)
	}
}

func TestConsensusLeader(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	pID := cc.host.ID()
	defer cleanRaft(1)
	defer cc.Shutdown(ctx)
	l, err := cc.Leader(ctx)
	if err != nil {
		t.Fatal("No leader:", err)
	}

	if l != pID {
		t.Errorf("expected %s but the leader appears as %s", pID, l)
	}
}

func TestRaftLatestSnapshot(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	defer cleanRaft(1)
	defer cc.Shutdown(ctx)

	// Make pin 1
	err := cc.LogPin(ctx, testPin(test.Cid1))
	if err != nil {
		t.Error("the first pin did not make it to the log:", err)
	}

	time.Sleep(250 * time.Millisecond)
	err = cc.raft.Snapshot()
	if err != nil {
		t.Error("the first snapshot was not taken successfully")
	}

	// Make pin 2
	err = cc.LogPin(ctx, testPin(test.Cid2))
	if err != nil {
		t.Error("the second pin did not make it to the log:", err)
	}

	time.Sleep(250 * time.Millisecond)
	err = cc.raft.Snapshot()
	if err != nil {
		t.Error("the second snapshot was not taken successfully")
	}

	// Make pin 3
	err = cc.LogPin(ctx, testPin(test.Cid3))
	if err != nil {
		t.Error("the third pin did not make it to the log:", err)
	}

	time.Sleep(250 * time.Millisecond)
	err = cc.raft.Snapshot()
	if err != nil {
		t.Error("the third snapshot was not taken successfully")
	}

	// Call raft.LastState and ensure we get the correct state
	snapState, err := dsstate.New(ctx, inmem.New(), "", dsstate.DefaultHandle())
	if err != nil {
		t.Fatal(err)
	}
	r, snapExists, err := LastStateRaw(cc.config)
	if !snapExists {
		t.Fatal("No snapshot found by LastStateRaw")
	}
	if err != nil {
		t.Fatal("Error while taking snapshot", err)
	}
	err = snapState.Unmarshal(r)
	if err != nil {
		t.Fatal("Snapshot bytes returned could not restore to state: ", err)
	}

	out := make(chan api.Pin, 100)
	err = snapState.List(ctx, out)
	if err != nil {
		t.Fatal(err)
	}

	var pins []api.Pin
	for p := range out {
		pins = append(pins, p)
	}

	if len(pins) != 3 {
		t.Fatal("Latest snapshot not read")
	}
}
