package maptracker

import (
	"context"
	"fmt"
	"testing"

	"github.com/ipfs/go-cid"

	"github.com/ipfs/ipfs-cluster/test"
)

func testOperationTracker(t *testing.T, ctx context.Context) *operationTracker {
	return newOperationTracker(ctx)
}

func TestOperationTracker_trackNewOperationWithCtx(t *testing.T) {
	ctx := context.Background()
	opt := testOperationTracker(t, ctx)

	h, _ := cid.Decode(test.TestCid1)
	opt.trackNewOperationCtx(ctx, h, operationPin)

	opc, ok := opt.get(h)
	if !ok {
		t.Errorf("operationCtx wasn't set in operationTracker")
	}

	testopc1 := operationCtx{
		cid:   h,
		op:    operationPin,
		phase: phaseQueued,
	}

	if opc.cid != testopc1.cid {
		t.Fail()
	}
	if opc.op != testopc1.op {
		t.Fail()
	}
	if opc.phase != testopc1.phase {
		t.Fail()
	}
	if t.Failed() {
		fmt.Printf("got %#v\nwant %#v", opc, testopc1)
	}
}

func TestOperationTracker_cancelOperation(t *testing.T) {
	ctx := context.Background()
	opt := testOperationTracker(t, ctx)

	h, _ := cid.Decode(test.TestCid1)
	opt.trackNewOperationCtx(ctx, h, operationPin)

	opt.cancelOperation(h)
	_, ok := opt.get(h)
	if ok {
		t.Error("cancelling operation failed to remove it from the map of ongoing operation")
	}
}

func TestOperationTracker_updateOperationPhase(t *testing.T) {
	ctx := context.Background()
	opt := testOperationTracker(t, ctx)

	h, _ := cid.Decode(test.TestCid1)
	opt.trackNewOperationCtx(ctx, h, operationPin)

	opt.updateOperationPhase(h, phaseInProgress)
	opc, ok := opt.get(h)
	if !ok {
		t.Error("error getting operation context after updating phase")
	}

	if opc.phase != phaseInProgress {
		t.Errorf("operation phase failed to be updated to %s, got %s", phaseInProgress.String(), opc.phase.String())
	}
}
