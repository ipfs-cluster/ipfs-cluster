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
	pid := test.TestPeerID1
	return NewOperationTracker(ctx, pid)
}

func TestOperationTracker_TrackNewOperation(t *testing.T) {
	opt := testOperationTracker(t)
	h := test.MustDecodeCid(test.TestCid1)
	op := opt.TrackNewOperation(api.PinCid(h), OperationPin, PhaseQueued)

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
		op2 := opt.TrackNewOperation(api.PinCid(h), OperationPin, PhaseInProgress)
		if op2 != nil {
			t.Fatal("should not have created new operation")
		}
	})

	t.Run("track of different type", func(t *testing.T) {
		op2 := opt.TrackNewOperation(api.PinCid(h), OperationUnpin, PhaseQueued)
		if op2 == nil {
			t.Fatal("should have created a new operation")
		}

		if !op.Cancelled() {
			t.Fatal("should have cancelled the original operation")
		}
	})

	t.Run("track of same type when done", func(t *testing.T) {
		op2 := opt.TrackNewOperation(api.PinCid(h), OperationPin, PhaseDone)
		if op2 == nil {
			t.Fatal("should have created a new operation")
		}

		op3 := opt.TrackNewOperation(api.PinCid(h), OperationPin, PhaseQueued)
		if op3 == nil {
			t.Fatal("should have created a new operation when other is in Done")
		}
	})

	t.Run("track of same type when error", func(t *testing.T) {
		op4 := opt.TrackNewOperation(api.PinCid(h), OperationUnpin, PhaseError)
		if op4 == nil {
			t.Fatal("should have created a new operation")
		}

		op5 := opt.TrackNewOperation(api.PinCid(h), OperationUnpin, PhaseQueued)
		if op5 == nil {
			t.Fatal("should have created a new operation")
		}
	})
}

func TestOperationTracker_Clean(t *testing.T) {
	opt := testOperationTracker(t)
	h := test.MustDecodeCid(test.TestCid1)
	op := opt.TrackNewOperation(api.PinCid(h), OperationPin, PhaseQueued)
	op2 := opt.TrackNewOperation(api.PinCid(h), OperationUnpin, PhaseQueued)
	t.Run("clean older operation", func(t *testing.T) {
		opt.Clean(op)
		st, ok := opt.Status(h)
		if !ok || st != api.TrackerStatusUnpinQueued {
			t.Fatal("should not have cleaned the latest op")
		}
	})

	t.Run("clean current operation", func(t *testing.T) {
		opt.Clean(op2)
		_, ok := opt.Status(h)
		if ok {
			t.Fatal("should have cleaned the latest op")
		}
	})
}

func TestOperationTracker_Status(t *testing.T) {
	opt := testOperationTracker(t)
	h := test.MustDecodeCid(test.TestCid1)
	opt.TrackNewOperation(api.PinCid(h), OperationRemote, PhaseDone)
	st, ok := opt.Status(h)
	if !ok || st != api.TrackerStatusRemote {
		t.Error("should provide status remote")
	}

	_, ok = opt.Status(h)
	if !ok {
		t.Error("should signal unexistent status")
	}
}

func TestOperationTracker_SetError(t *testing.T) {
	opt := testOperationTracker(t)
	h := test.MustDecodeCid(test.TestCid1)
	opt.TrackNewOperation(api.PinCid(h), OperationPin, PhaseDone)
	opt.SetError(h, errors.New("fake error"))
	pinfo := opt.Get(h)
	if pinfo.Status != api.TrackerStatusPinError {
		t.Error("should have updated the status")
	}
	if pinfo.Error != "fake error" {
		t.Error("should have set the error message")
	}

	opt.TrackNewOperation(api.PinCid(h), OperationUnpin, PhaseQueued)
	opt.SetError(h, errors.New("fake error"))
	st, ok := opt.Status(h)
	if !ok || st != api.TrackerStatusUnpinQueued {
		t.Error("should not have set an error on in-flight items")
	}
}

func TestOperationTracker_Get(t *testing.T) {
	opt := testOperationTracker(t)
	h := test.MustDecodeCid(test.TestCid1)
	h2 := test.MustDecodeCid(test.TestCid2)
	opt.TrackNewOperation(api.PinCid(h), OperationPin, PhaseDone)

	t.Run("Get with existing item", func(t *testing.T) {
		pinfo := opt.Get(h)
		if pinfo.Status != api.TrackerStatusPinned {
			t.Error("bad status")
		}
		if pinfo.Cid != h {
			t.Error("bad cid")
		}

		if pinfo.Peer != test.TestPeerID1 {
			t.Error("bad peer ID")
		}

	})

	t.Run("Get with unexisting item", func(t *testing.T) {
		pinfo := opt.Get(h2)
		if pinfo.Status != api.TrackerStatusUnpinned {
			t.Error("bad status")
		}
		if pinfo.Cid != h2 {
			t.Error("bad cid")
		}

		if pinfo.Peer != test.TestPeerID1 {
			t.Error("bad peer ID")
		}
	})
}

func TestOperationTracker_GetAll(t *testing.T) {
	opt := testOperationTracker(t)
	h := test.MustDecodeCid(test.TestCid1)
	opt.TrackNewOperation(api.PinCid(h), OperationPin, PhaseInProgress)
	pinfos := opt.GetAll()
	if len(pinfos) != 1 {
		t.Fatal("expected 1 item")
	}
	if pinfos[0].Status != api.TrackerStatusPinning {
		t.Fatal("bad status")
	}
}

func TestOperationTracker_GetOpContext(t *testing.T) {
	opt := testOperationTracker(t)
	h := test.MustDecodeCid(test.TestCid1)
	op := opt.TrackNewOperation(api.PinCid(h), OperationPin, PhaseInProgress)
	ctx1 := op.Context()
	ctx2 := opt.GetOpContext(h)
	if ctx1 != ctx2 {
		t.Fatal("didn't get the right context")
	}
}
