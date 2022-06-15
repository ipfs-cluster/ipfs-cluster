package raft

import (
	"context"
	"testing"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/inmem"
	"github.com/ipfs-cluster/ipfs-cluster/state/dsstate"
	"github.com/ipfs-cluster/ipfs-cluster/test"
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

	st, err := dsstate.New(ctx, inmem.New(), "", dsstate.DefaultHandle())
	if err != nil {
		t.Fatal(err)
	}
	op.ApplyTo(st)

	out := make(chan api.Pin, 100)
	err = st.List(ctx, out)
	if err != nil {
		t.Fatal(err)
	}

	var pins []api.Pin
	for p := range out {
		pins = append(pins, p)
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

	st, err := dsstate.New(ctx, inmem.New(), "", dsstate.DefaultHandle())
	if err != nil {
		t.Fatal(err)
	}
	st.Add(ctx, testPin(test.Cid1))
	op.ApplyTo(st)

	out := make(chan api.Pin, 100)
	err = st.List(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
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
