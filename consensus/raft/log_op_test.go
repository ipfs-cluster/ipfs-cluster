package raft

import (
	"testing"

	cid "gx/ipfs/QmPSQnBKM9g7BaUcZCvswUJVscQ1ipjmwxN5PXCjkp9EQ7/go-cid"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/state/mapstate"
	"github.com/ipfs/ipfs-cluster/test"
)

func TestApplyToPin(t *testing.T) {
	cc := testingConsensus(t, 1)
	op := &LogOp{
		Cid:       api.PinSerial{Cid: test.TestCid1},
		Type:      LogOpPin,
		consensus: cc,
	}
	defer cleanRaft(1)
	defer cc.Shutdown()

	st := mapstate.NewMapState()
	op.ApplyTo(st)
	pins := st.List()
	if len(pins) != 1 || pins[0].Cid.String() != test.TestCid1 {
		t.Error("the state was not modified correctly")
	}
}

func TestApplyToUnpin(t *testing.T) {
	cc := testingConsensus(t, 1)
	op := &LogOp{
		Cid:       api.PinSerial{Cid: test.TestCid1},
		Type:      LogOpUnpin,
		consensus: cc,
	}
	defer cleanRaft(1)
	defer cc.Shutdown()

	st := mapstate.NewMapState()
	c, _ := cid.Decode(test.TestCid1)
	st.Add(testPin(c))
	op.ApplyTo(st)
	pins := st.List()
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

	op := &LogOp{
		Cid:  api.PinSerial{Cid: test.TestCid1},
		Type: LogOpUnpin,
	}

	var st interface{}
	op.ApplyTo(st)
}

// func TestApplyToBadCid(t *testing.T) {
// 	defer func() {
// 		if r := recover(); r == nil {
// 			t.Error("should have recovered an error")
// 		}
// 	}()

// 	op := &LogOp{
// 		Cid:       api.PinSerial{Cid: "agadfaegf"},
// 		Type:      LogOpPin,
// 		ctx:       context.Background(),
// 		rpcClient: test.NewMockRPCClient(t),
// 	}

// 	st := mapstate.NewMapState()
// 	op.ApplyTo(st)
// }
