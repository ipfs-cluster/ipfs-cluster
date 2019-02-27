package optracker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

func TestOperation(t *testing.T) {
	tim := time.Now().Add(-2 * time.Second)
	op := NewOperation(context.Background(), api.PinCid(test.TestCid1), OperationUnpin, PhaseQueued)
	if !op.Cid().Equals(test.TestCid1) {
		t.Error("bad cid")
	}
	if op.Phase() != PhaseQueued {
		t.Error("bad phase")
	}

	op.SetError(errors.New("fake error"))
	if op.Error() != "fake error" {
		t.Error("bad error")
	}

	op.SetPhase(PhaseInProgress)
	if op.Phase() != PhaseInProgress {
		t.Error("bad phase")
	}

	if op.Type() != OperationUnpin {
		t.Error("bad type")
	}

	if !op.Timestamp().After(tim) {
		t.Error("bad timestamp")
	}

	if op.Cancelled() {
		t.Error("should not be cancelled")
	}

	op.Cancel()
	if !op.Cancelled() {
		t.Error("should be cancelled")
	}

	if op.ToTrackerStatus() != api.TrackerStatusUnpinning {
		t.Error("should be in unpin error")
	}
}
