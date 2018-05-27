package optracker

import (
	"context"
	"fmt"
	"testing"

	"github.com/ipfs/ipfs-cluster/test"
)

func testOperationTracker(ctx context.Context, t *testing.T) *OperationTracker {
	return NewOperationTracker(ctx)
}

func TestOperationTracker_TrackNewOperationWithCtx(t *testing.T) {
	ctx := context.Background()
	opt := testOperationTracker(ctx, t)

	h := test.MustDecodeCid(test.TestCid1)
	opt.TrackNewOperation(ctx, h, OperationPin)

	opc, ok := opt.Get(h)
	if !ok {
		t.Errorf("operation wasn't set in operationTracker")
	}

	testopc1 := Operation{
		Cid:   h,
		Op:    OperationPin,
		Phase: PhaseQueued,
	}

	if opc.Cid != testopc1.Cid {
		t.Fail()
	}
	if opc.Op != testopc1.Op {
		t.Fail()
	}
	if opc.Phase != testopc1.Phase {
		t.Fail()
	}
	if t.Failed() {
		fmt.Printf("got %#v\nwant %#v", opc, testopc1)
	}
}

func TestOperationTracker_Finish(t *testing.T) {
	ctx := context.Background()
	opt := testOperationTracker(ctx, t)

	h := test.MustDecodeCid(test.TestCid1)
	opt.TrackNewOperation(ctx, h, OperationPin)

	opt.Finish(h)
	_, ok := opt.Get(h)
	if ok {
		t.Error("cancelling operation failed to remove it from the map of ongoing operation")
	}
}

func TestOperationTracker_UpdateOperationPhase(t *testing.T) {
	ctx := context.Background()
	opt := testOperationTracker(ctx, t)

	h := test.MustDecodeCid(test.TestCid1)
	opt.TrackNewOperation(ctx, h, OperationPin)

	opt.UpdateOperationPhase(h, PhaseInProgress)
	opc, ok := opt.Get(h)
	if !ok {
		t.Error("error getting operation context after updating phase")
	}

	if opc.Phase != PhaseInProgress {
		t.Errorf("operation phase failed to be updated to %s, got %s", PhaseInProgress.String(), opc.Phase.String())
	}
}

func TestOperationTracker_Filter(t *testing.T) {
	ctx := context.Background()
	testOpsMap := map[string]Operation{
		test.TestCid1: Operation{Cid: test.MustDecodeCid(test.TestCid1), Op: OperationPin, Phase: PhaseQueued},
		test.TestCid2: Operation{Cid: test.MustDecodeCid(test.TestCid2), Op: OperationPin, Phase: PhaseInProgress},
		test.TestCid3: Operation{Cid: test.MustDecodeCid(test.TestCid3), Op: OperationUnpin, Phase: PhaseInProgress},
	}
	opt := &OperationTracker{ctx: ctx, operations: testOpsMap}

	t.Run("filter to pin operations", func(t *testing.T) {
		wantLen := 2
		wantOp := OperationPin
		got := opt.Filter(wantOp)
		if len(got) != wantLen {
			t.Errorf("want: %d %s operations; got: %d", wantLen, wantOp.String(), len(got))
		}
		for i := range got {
			if got[i].Op != wantOp {
				t.Errorf("want: %v; got: %v", wantOp.String(), got[i])
			}
		}
	})

	t.Run("filter to in progress phase", func(t *testing.T) {
		wantLen := 2
		wantPhase := PhaseInProgress
		got := opt.Filter(PhaseInProgress)
		if len(got) != wantLen {
			t.Errorf("want: %d %s operations; got: %d", wantLen, wantPhase.String(), len(got))
		}
		for i := range got {
			if got[i].Phase != wantPhase {
				t.Errorf("want: %s; got: %v", wantPhase.String(), got[i])
			}
		}
	})
}
