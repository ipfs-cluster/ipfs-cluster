package raft

import (
	"context"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/datastore/inmem"
	"github.com/ipfs/ipfs-cluster/state/dsstate"
	"github.com/ipfs/ipfs-cluster/test"
)

func TestApplyToPin(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	op := &LogOp{
		Cid:       api.PinCid(test.Cid1),
		Type:      LogOpPin,
		consensus: cc,
	}
	defer cleanRaft(1)
	defer cc.Shutdown(ctx)

	st, err := dsstate.New(inmem.New(), "", dsstate.DefaultHandle())
	if err != nil {
		t.Fatal(err)
	}
	op.ApplyTo(st)

	pins, err := st.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 1 || !pins[0].Cid.Equals(test.Cid1) {
		t.Error("the state was not modified correctly")
	}
}

func TestApplyToUnpin(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	op := &LogOp{
		Cid:       api.PinCid(test.Cid1),
		Type:      LogOpUnpin,
		consensus: cc,
	}
	defer cleanRaft(1)
	defer cc.Shutdown(ctx)

	st, err := dsstate.New(inmem.New(), "", dsstate.DefaultHandle())
	if err != nil {
		t.Fatal(err)
	}
	st.Add(ctx, testPin(test.Cid1))
	op.ApplyTo(st)
	pins, err := st.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
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
		Cid:  api.PinCid(test.Cid1),
		Type: LogOpUnpin,
	}

	var st interface{}
	op.ApplyTo(st)
}
