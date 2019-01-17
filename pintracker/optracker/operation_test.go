package optracker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elastos/Elastos.NET.Hive.Cluster/api"
	"github.com/elastos/Elastos.NET.Hive.Cluster/test"
)

func TestOperation(t *testing.T) {
	tim := time.Now().Add(-2 * time.Second)
	h := test.MustDecodeCid(test.TestCid1)
	op := NewOperation(context.Background(), api.PinCid(h), OperationUnpin, PhaseQueued)
	if op.Cid() != h {
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
