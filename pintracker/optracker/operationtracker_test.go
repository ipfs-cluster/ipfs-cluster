package optracker

import (
	"context"
	"errors"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

func testOperationTracker(t *testing.T) *OperationTracker {
	ctx := context.Background()
	return NewOperationTracker(ctx, test.PeerID1, test.PeerName1)
}

func TestOperationTracker_TrackNewOperation(t *testing.T) {
	ctx := context.Background()
	opt := testOperationTracker(t)
	op := opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationPin, PhaseQueued)

	t.Run("track new operation", func(t *testing.T) {
		if op == nil {
			t.Fatal("nil op")
		}
		if op.Phase() != PhaseQueued {
			t.Error("bad phase")
		}

		if op.Type() != OperationPin {
			t.Error("bad type")
		}

		if op.Cancelled() != false {
			t.Error("should not be cancelled")
		}

		if op.ToTrackerStatus() != api.TrackerStatusPinQueued {
			t.Error("bad state")
		}
	})

	t.Run("track when ongoing operation", func(t *testing.T) {
		op2 := opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationPin, PhaseInProgress)
		if op2 != nil {
			t.Fatal("should not have created new operation")
		}
	})

	t.Run("track of different type", func(t *testing.T) {
		op2 := opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationUnpin, PhaseQueued)
		if op2 == nil {
			t.Fatal("should have created a new operation")
		}

		if !op.Cancelled() {
			t.Fatal("should have cancelled the original operation")
		}
	})

	t.Run("track of same type when done", func(t *testing.T) {
		op2 := opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationPin, PhaseDone)
		if op2 == nil {
			t.Fatal("should have created a new operation")
		}

		op3 := opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationPin, PhaseQueued)
		if op3 == nil {
			t.Fatal("should have created a new operation when other is in Done")
		}
	})

	t.Run("track of same type when error", func(t *testing.T) {
		op4 := opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationUnpin, PhaseError)
		if op4 == nil {
			t.Fatal("should have created a new operation")
		}

		op5 := opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationUnpin, PhaseQueued)
		if op5 == nil {
			t.Fatal("should have created a new operation")
		}
	})
}

func TestOperationTracker_Clean(t *testing.T) {
	ctx := context.Background()
	opt := testOperationTracker(t)
	op := opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationPin, PhaseQueued)
	op2 := opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationUnpin, PhaseQueued)
	t.Run("clean older operation", func(t *testing.T) {
		opt.Clean(ctx, op)
		st, ok := opt.Status(ctx, test.Cid1)
		if !ok || st != api.TrackerStatusUnpinQueued {
			t.Fatal("should not have cleaned the latest op")
		}
	})

	t.Run("clean current operation", func(t *testing.T) {
		opt.Clean(ctx, op2)
		_, ok := opt.Status(ctx, test.Cid1)
		if ok {
			t.Fatal("should have cleaned the latest op")
		}
	})
}

func TestOperationTracker_Status(t *testing.T) {
	ctx := context.Background()
	opt := testOperationTracker(t)
	opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationRemote, PhaseDone)
	st, ok := opt.Status(ctx, test.Cid1)
	if !ok || st != api.TrackerStatusRemote {
		t.Error("should provide status remote")
	}

	_, ok = opt.Status(ctx, test.Cid1)
	if !ok {
		t.Error("should signal unexistent status")
	}
}

func TestOperationTracker_SetError(t *testing.T) {
	ctx := context.Background()
	opt := testOperationTracker(t)
	opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationPin, PhaseDone)
	opt.SetError(ctx, test.Cid1, errors.New("fake error"))
	pinfo := opt.Get(ctx, test.Cid1)
	if pinfo.Status != api.TrackerStatusPinError {
		t.Error("should have updated the status")
	}
	if pinfo.Error != "fake error" {
		t.Error("should have set the error message")
	}

	opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationUnpin, PhaseQueued)
	opt.SetError(ctx, test.Cid1, errors.New("fake error"))
	st, ok := opt.Status(ctx, test.Cid1)
	if !ok || st != api.TrackerStatusUnpinQueued {
		t.Error("should not have set an error on in-flight items")
	}
}

func TestOperationTracker_Get(t *testing.T) {
	ctx := context.Background()
	opt := testOperationTracker(t)
	opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationPin, PhaseDone)

	t.Run("Get with existing item", func(t *testing.T) {
		pinfo := opt.Get(ctx, test.Cid1)
		if pinfo.Status != api.TrackerStatusPinned {
			t.Error("bad status")
		}
		if !pinfo.Cid.Equals(test.Cid1) {
			t.Error("bad cid")
		}

		if pinfo.Peer != test.PeerID1 {
			t.Error("bad peer ID")
		}

	})

	t.Run("Get with unexisting item", func(t *testing.T) {
		pinfo := opt.Get(ctx, test.Cid2)
		if pinfo.Status != api.TrackerStatusUnpinned {
			t.Error("bad status")
		}
		if !pinfo.Cid.Equals(test.Cid2) {
			t.Error("bad cid")
		}

		if pinfo.Peer != test.PeerID1 {
			t.Error("bad peer ID")
		}
	})
}

func TestOperationTracker_GetAll(t *testing.T) {
	ctx := context.Background()
	opt := testOperationTracker(t)
	opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationPin, PhaseInProgress)
	pinfos := opt.GetAll(ctx)
	if len(pinfos) != 1 {
		t.Fatal("expected 1 item")
	}
	if pinfos[0].Status != api.TrackerStatusPinning {
		t.Fatal("bad status")
	}
}

func TestOperationTracker_OpContext(t *testing.T) {
	ctx := context.Background()
	opt := testOperationTracker(t)
	op := opt.TrackNewOperation(ctx, api.PinCid(test.Cid1), OperationPin, PhaseInProgress)
	ctx1 := op.Context()
	ctx2 := opt.OpContext(ctx, test.Cid1)
	if ctx1 != ctx2 {
		t.Fatal("didn't get the right context")
	}
}

func TestOperationTracker_filterOps(t *testing.T) {
	ctx := context.Background()
	testOpsMap := map[string]*Operation{
		test.Cid1.String(): {pin: api.PinCid(test.Cid1), opType: OperationPin, phase: PhaseQueued},
		test.Cid2.String(): {pin: api.PinCid(test.Cid2), opType: OperationPin, phase: PhaseInProgress},
		test.Cid3.String(): {pin: api.PinCid(test.Cid3), opType: OperationUnpin, phase: PhaseInProgress},
	}
	opt := &OperationTracker{ctx: ctx, operations: testOpsMap}

	t.Run("filter ops to pin operations", func(t *testing.T) {
		wantLen := 2
		wantOp := OperationPin
		got := opt.filterOps(ctx, wantOp)
		if len(got) != wantLen {
			t.Errorf("want: %d %s operations; got: %d", wantLen, wantOp.String(), len(got))
		}
		for i := range got {
			if got[i].Type() != wantOp {
				t.Errorf("want: %v; got: %v", wantOp.String(), got[i])
			}
		}
	})

	t.Run("filter ops to in progress phase", func(t *testing.T) {
		wantLen := 2
		wantPhase := PhaseInProgress
		got := opt.filterOps(ctx, PhaseInProgress)
		if len(got) != wantLen {
			t.Errorf("want: %d %s operations; got: %d", wantLen, wantPhase.String(), len(got))
		}
		for i := range got {
			if got[i].Phase() != wantPhase {
				t.Errorf("want: %s; got: %v", wantPhase.String(), got[i])
			}
		}
	})

	t.Run("filter ops to queued pins", func(t *testing.T) {
		wantLen := 1
		wantPhase := PhaseQueued
		wantOp := OperationPin
		got := opt.filterOps(ctx, OperationPin, PhaseQueued)
		if len(got) != wantLen {
			t.Errorf("want: %d %s operations; got: %d", wantLen, wantPhase.String(), len(got))
		}
		for i := range got {
			if got[i].Phase() != wantPhase {
				t.Errorf("want: %s; got: %v", wantPhase.String(), got[i])
			}

			if got[i].Type() != wantOp {
				t.Errorf("want: %s; got: %v", wantOp.String(), got[i])
			}
		}
	})
}
