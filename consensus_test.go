package ipfscluster

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/state/mapstate"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
)

func TestApplyToPin(t *testing.T) {
	op := &clusterLogOp{
		Arg:       test.TestCid1,
		Type:      LogOpPin,
		ctx:       context.Background(),
		rpcClient: test.NewMockRPCClient(t),
	}

	st := mapstate.NewMapState()
	op.ApplyTo(st)
	pins := st.ListPins()
	if len(pins) != 1 || pins[0].String() != test.TestCid1 {
		t.Error("the state was not modified correctly")
	}
}

func TestApplyToUnpin(t *testing.T) {
	op := &clusterLogOp{
		Arg:       test.TestCid1,
		Type:      LogOpUnpin,
		ctx:       context.Background(),
		rpcClient: test.NewMockRPCClient(t),
	}

	st := mapstate.NewMapState()
	c, _ := cid.Decode(test.TestCid1)
	st.AddPin(c)
	op.ApplyTo(st)
	pins := st.ListPins()
	if len(pins) != 0 {
		t.Error("the state was not modified correctly")
	}
}

func TestApplyToBadState(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("should have recovered an error")
		}
	}()

	op := &clusterLogOp{
		Arg:       test.TestCid1,
		Type:      LogOpUnpin,
		ctx:       context.Background(),
		rpcClient: test.NewMockRPCClient(t),
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
		Arg:       "agadfaegf",
		Type:      LogOpPin,
		ctx:       context.Background(),
		rpcClient: test.NewMockRPCClient(t),
	}

	st := mapstate.NewMapState()
	op.ApplyTo(st)
}

func cleanRaft() {
	os.RemoveAll(testingConfig().ConsensusDataFolder)
}

func testingConsensus(t *testing.T) *Consensus {
	//logging.SetDebugLogging()
	cfg := testingConfig()
	ctx := context.Background()
	h, err := makeHost(ctx, cfg)
	if err != nil {
		t.Fatal("cannot create host:", err)
	}
	st := mapstate.NewMapState()
	cc, err := NewConsensus([]peer.ID{cfg.ID}, h, cfg.ConsensusDataFolder, st)
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
	defer cleanRaft()
	cc := testingConsensus(t)
	err := cc.Shutdown()
	if err != nil {
		t.Fatal("Consensus cannot shutdown:", err)
	}
	cc.Shutdown()
	cc = testingConsensus(t)
	err = cc.Shutdown()
	if err != nil {
		t.Fatal("Consensus cannot shutdown:", err)
	}
}

func TestConsensusPin(t *testing.T) {
	cc := testingConsensus(t)
	defer cleanRaft() // Remember defer runs in LIFO order
	defer cc.Shutdown()

	c, _ := cid.Decode(test.TestCid1)
	err := cc.LogPin(c)
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
	}

	time.Sleep(250 * time.Millisecond)
	st, err := cc.State()
	if err != nil {
		t.Fatal("error gettinng state:", err)
	}

	pins := st.ListPins()
	if len(pins) != 1 || pins[0].String() != test.TestCid1 {
		t.Error("the added pin should be in the state")
	}
}

func TestConsensusUnpin(t *testing.T) {
	cc := testingConsensus(t)
	defer cleanRaft()
	defer cc.Shutdown()

	c, _ := cid.Decode(test.TestCid2)
	err := cc.LogUnpin(c)
	if err != nil {
		t.Error("the operation did not make it to the log:", err)
	}
}

func TestConsensusLeader(t *testing.T) {
	cc := testingConsensus(t)
	cfg := testingConfig()
	pID := cfg.ID
	defer cleanRaft()
	defer cc.Shutdown()
	l, err := cc.Leader()
	if err != nil {
		t.Fatal("No leader:", err)
	}

	if l != pID {
		t.Errorf("expected %s but the leader appears as %s", pID, l)
	}
}
