package ipfscluster

import (
	"context"
	"os"
	"testing"
	"time"

	cid "github.com/ipfs/go-cid"
)

func TestApplyToPin(t *testing.T) {
	op := &clusterLogOp{
		Cid:   testCid,
		Type:  LogOpPin,
		ctx:   context.Background(),
		rpcCh: make(chan ClusterRPC, 1),
	}

	st := NewMapState()
	op.ApplyTo(st)
	pins := st.ListPins()
	if len(pins) != 1 || pins[0].String() != testCid {
		t.Error("the state  was not modified correctly")
	}
}

func TestApplyToUnpin(t *testing.T) {
	op := &clusterLogOp{
		Cid:   testCid,
		Type:  LogOpUnpin,
		ctx:   context.Background(),
		rpcCh: make(chan ClusterRPC, 1),
	}

	st := NewMapState()
	c, _ := cid.Decode(testCid)
	st.AddPin(c)
	op.ApplyTo(st)
	pins := st.ListPins()
	if len(pins) != 0 {
		t.Error("the state  was not modified correctly")
	}
}

func TestApplyToBadState(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("should have recovered an error")
		}
	}()

	op := &clusterLogOp{
		Cid:   testCid,
		Type:  LogOpUnpin,
		ctx:   context.Background(),
		rpcCh: make(chan ClusterRPC, 1),
	}

	var st interface{}
	op.ApplyTo(st)
}

func TestApplyToBadCid(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("should have recovered an error")
		}
	}()

	op := &clusterLogOp{
		Cid:   "agadfaegf",
		Type:  LogOpPin,
		ctx:   context.Background(),
		rpcCh: make(chan ClusterRPC, 1),
	}

	st := NewMapState()
	op.ApplyTo(st)
}

func cleanRaft() {
	os.RemoveAll(testingConfig().RaftFolder)
}

func testingClusterConsensus(t *testing.T) *ClusterConsensus {
	//logging.SetDebugLogging()
	cfg := testingConfig()
	ctx := context.Background()
	h, err := makeHost(ctx, cfg)
	if err != nil {
		t.Fatal("cannot create host:", err)
	}
	st := NewMapState()
	cc, err := NewClusterConsensus(cfg, h, st)
	if err != nil {
		t.Fatal("cannot create ClusterConsensus:", err)
	}
	// Oxygen for Raft to declare leader
	time.Sleep(1 * time.Second)
	return cc
}

func TestShutdownClusterConsensus(t *testing.T) {
	// Bring it up twice to make sure shutdown cleans up properly
	// but also to make sure raft comes up ok when re-initialized
	defer cleanRaft()
	cc := testingClusterConsensus(t)
	err := cc.Shutdown()
	if err != nil {
		t.Fatal("ClusterConsensus cannot shutdown:", err)
	}
	cc = testingClusterConsensus(t)
	err = cc.Shutdown()
	if err != nil {
		t.Fatal("ClusterConsensus cannot shutdown:", err)
	}
}

func TestConsensusPin(t *testing.T) {
	cc := testingClusterConsensus(t)
	defer cleanRaft() // Remember defer runs in LIFO order
	defer cc.Shutdown()

	c, _ := cid.Decode(testCid)
	err := cc.AddPin(c)
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
	}

	time.Sleep(250 * time.Millisecond)
	st, err := cc.State()
	if err != nil {
		t.Error("error getting state:", err)
	}

	pins := st.ListPins()
	if len(pins) != 1 || pins[0].String() != testCid {
		t.Error("the added pin should be in the state")
	}
}

func TestConsensusUnpin(t *testing.T) {
	cc := testingClusterConsensus(t)
	defer cleanRaft()
	defer cc.Shutdown()

	c, _ := cid.Decode(testCid2)
	err := cc.RmPin(c)
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
	}
}

func TestConsensusLeader(t *testing.T) {
	cc := testingClusterConsensus(t)
	cfg := testingConfig()
	pId := cfg.ID
	defer cleanRaft()
	defer cc.Shutdown()
	if l := cc.Leader().Pretty(); l != pId {
		t.Errorf("expected %s but the leader appears as %s", pId, l)
	}
}
