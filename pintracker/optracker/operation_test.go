package optracker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/test"
)

func TestOperation(t *testing.T) {
	tim := time.Now().Add(-2 * time.Second)
	op := newOperation(context.Background(), api.PinCid(test.Cid1), OperationUnpin, PhaseQueued, nil)
	if !op.Cid().Equals(test.Cid1) {
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

	if op.Canceled() {
		t.Error("should not be canceled")
	}

	op.Cancel()
	if !op.Canceled() {
		t.Error("should be canceled")
	}

	if op.ToTrackerStatus() != api.TrackerStatusUnpinning {
		t.Error("should be in unpin error")
	}
}
