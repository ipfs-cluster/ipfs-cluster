package optracker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"go.opencensus.io/trace"
)

//go:generate stringer -type=OperationType

// OperationType represents the kinds of operations that the PinTracker
// performs and the operationTracker tracks the status of.
type OperationType int

const (
	// OperationUnknown represents an unknown operation.
	OperationUnknown OperationType = iota
	// OperationPin represents a pin operation.
	OperationPin
	// OperationUnpin represents an unpin operation.
	OperationUnpin
	// OperationRemote represents an noop operation
	OperationRemote
	// OperationShard represents a meta pin. We don't
	// pin these.
	OperationShard
)

//go:generate stringer -type=Phase

// Phase represents the multiple phase that an operation can be in.
type Phase int

const (
	// PhaseError represents an error state.
	PhaseError Phase = iota
	// PhaseQueued represents the queued phase of an operation.
	PhaseQueued
	// PhaseInProgress represents the operation as in progress.
	PhaseInProgress
	// PhaseDone represents the operation once finished.
	PhaseDone
)

// Operation represents an ongoing operation involving a
// particular Cid. It provides the type and phase of operation
// and a way to mark the operation finished (also used to cancel).
type Operation struct {
	ctx    context.Context
	cancel func()

	tracker *OperationTracker

	// RO fields
	opType OperationType
	pin    api.Pin

	// RW fields
	mu           sync.RWMutex
	phase        Phase
	attemptCount int
	priority     bool
	error        string
	ts           time.Time
}

// newOperation creates a new Operation.
func newOperation(ctx context.Context, pin api.Pin, typ OperationType, ph Phase, tracker *OperationTracker) *Operation {
	ctx, span := trace.StartSpan(ctx, "optracker/NewOperation")
	defer span.End()

	ctx, cancel := context.WithCancel(ctx)
	op := &Operation{
		ctx:    ctx,
		cancel: cancel,

		tracker: tracker,

		pin:          pin,
		opType:       typ,
		phase:        ph,
		attemptCount: 0,
		priority:     false,
		ts:           time.Now(),
		error:        "",
	}
	return op
}

// String returns a string representation of an Operation.
func (op *Operation) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "type: %s\n", op.Type().String())
	fmt.Fprint(&b, "pin:\n")
	pinstr := op.Pin().String()
	pinstrs := strings.Split(pinstr, "\n")
	for _, s := range pinstrs {
		fmt.Fprintf(&b, "\t%s\n", s)
	}
	fmt.Fprintf(&b, "phase: %s\n", op.Phase().String())
	fmt.Fprintf(&b, "attemptCount: %d\n", op.AttemptCount())
	fmt.Fprintf(&b, "error: %s\n", op.Error())
	fmt.Fprintf(&b, "timestamp: %s\n", op.Timestamp().String())

	return b.String()
}

// Cid returns the Cid associated to this operation.
func (op *Operation) Cid() api.Cid {
	return op.pin.Cid
}

// Context returns the context associated to this operation.
func (op *Operation) Context() context.Context {
	return op.ctx
}

// Cancel will cancel the context associated to this operation.
func (op *Operation) Cancel() {
	_, span := trace.StartSpan(op.ctx, "optracker/Cancel")
	op.cancel()
	span.End()
}

// Phase returns the Phase.
func (op *Operation) Phase() Phase {
	var ph Phase

	op.mu.RLock()
	ph = op.phase
	op.mu.RUnlock()

	return ph
}

// SetPhase changes the Phase and updates the timestamp.
func (op *Operation) SetPhase(ph Phase) {
	_, span := trace.StartSpan(op.ctx, "optracker/SetPhase")
	op.mu.Lock()
	{
		op.tracker.recordMetricUnsafe(op, -1)
		op.phase = ph
		op.ts = time.Now()
		op.tracker.recordMetricUnsafe(op, 1)
	}
	op.mu.Unlock()

	span.End()
}

// AttemptCount returns the number of times that this operation has been in
// progress.
func (op *Operation) AttemptCount() int {
	var retries int

	op.mu.RLock()
	retries = op.attemptCount
	op.mu.RUnlock()

	return retries
}

// IncAttempt does a plus-one on the AttemptCount.
func (op *Operation) IncAttempt() {
	op.mu.Lock()
	op.attemptCount++
	op.mu.Unlock()
}

// PriorityPin returns true if the pin has been marked as priority pin.
func (op *Operation) PriorityPin() bool {
	var p bool
	op.mu.RLock()
	p = op.priority
	op.mu.RUnlock()
	return p
}

// SetPriorityPin returns true if the pin has been marked as priority pin.
func (op *Operation) SetPriorityPin(p bool) {
	op.mu.Lock()
	op.priority = p
	op.mu.Unlock()
}

// Error returns any error message attached to the operation.
func (op *Operation) Error() string {
	var err string
	op.mu.RLock()
	err = op.error
	op.mu.RUnlock()
	return err
}

// SetError sets the phase to PhaseError along with
// an error message. It updates the timestamp.
func (op *Operation) SetError(err error) {
	_, span := trace.StartSpan(op.ctx, "optracker/SetError")
	op.mu.Lock()
	{
		op.tracker.recordMetricUnsafe(op, -1)
		op.phase = PhaseError
		op.error = err.Error()
		op.ts = time.Now()
		op.tracker.recordMetricUnsafe(op, 1)
	}
	op.mu.Unlock()
	span.End()
}

// Type returns the operation Type.
func (op *Operation) Type() OperationType {
	return op.opType
}

// Pin returns the Pin object associated to the operation.
func (op *Operation) Pin() api.Pin {
	return op.pin
}

// Timestamp returns the time when this operation was
// last modified (phase changed, error was set...).
func (op *Operation) Timestamp() time.Time {
	var ts time.Time
	op.mu.RLock()
	ts = op.ts
	op.mu.RUnlock()
	return ts
}

// Canceled returns whether the context for this
// operation has been canceled.
func (op *Operation) Canceled() bool {
	ctx, span := trace.StartSpan(op.ctx, "optracker/Canceled")
	_ = ctx
	defer span.End()
	select {
	case <-op.ctx.Done():
		return true
	default:
		return false
	}
}

// ToTrackerStatus returns an api.TrackerStatus reflecting
// the current status of this operation. It's a translation
// from the Type and the Phase.
func (op *Operation) ToTrackerStatus() api.TrackerStatus {
	typ := op.Type()
	ph := op.Phase()
	switch typ {
	case OperationPin:
		switch ph {
		case PhaseError:
			return api.TrackerStatusPinError
		case PhaseQueued:
			return api.TrackerStatusPinQueued
		case PhaseInProgress:
			return api.TrackerStatusPinning
		case PhaseDone:
			return api.TrackerStatusPinned
		default:
			return api.TrackerStatusUndefined
		}
	case OperationUnpin:
		switch ph {
		case PhaseError:
			return api.TrackerStatusUnpinError
		case PhaseQueued:
			return api.TrackerStatusUnpinQueued
		case PhaseInProgress:
			return api.TrackerStatusUnpinning
		case PhaseDone:
			return api.TrackerStatusUnpinned
		default:
			return api.TrackerStatusUndefined
		}
	case OperationRemote:
		return api.TrackerStatusRemote
	case OperationShard:
		return api.TrackerStatusSharded
	default:
		return api.TrackerStatusUndefined
	}

}

// TrackerStatusToOperationPhase takes an api.TrackerStatus and
// converts it to an OpType and Phase.
func TrackerStatusToOperationPhase(status api.TrackerStatus) (OperationType, Phase) {
	switch status {
	case api.TrackerStatusPinError:
		return OperationPin, PhaseError
	case api.TrackerStatusPinQueued:
		return OperationPin, PhaseQueued
	case api.TrackerStatusPinning:
		return OperationPin, PhaseInProgress
	case api.TrackerStatusPinned:
		return OperationPin, PhaseDone
	case api.TrackerStatusUnpinError:
		return OperationUnpin, PhaseError
	case api.TrackerStatusUnpinQueued:
		return OperationUnpin, PhaseQueued
	case api.TrackerStatusUnpinning:
		return OperationUnpin, PhaseInProgress
	case api.TrackerStatusUnpinned:
		return OperationUnpin, PhaseDone
	case api.TrackerStatusRemote:
		return OperationRemote, PhaseDone
	case api.TrackerStatusSharded:
		return OperationShard, PhaseDone
	default:
		return OperationUnknown, PhaseError
	}
}
