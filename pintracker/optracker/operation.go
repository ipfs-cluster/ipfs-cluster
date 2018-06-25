package optracker

import (
	"context"
	"sync"
	"time"

	cid "github.com/ipfs/go-cid"

	"github.com/ipfs/ipfs-cluster/api"
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

	// RO fields
	opType OperationType
	pin    api.Pin

	// RW fields
	mu    sync.RWMutex
	phase Phase
	error string
	ts    time.Time
}

// NewOperation creates a new Operation.
func NewOperation(ctx context.Context, pin api.Pin, typ OperationType, ph Phase) *Operation {
	ctx, cancel := context.WithCancel(ctx)
	return &Operation{
		ctx:    ctx,
		cancel: cancel,

		pin:    pin,
		opType: typ,
		phase:  ph,
		ts:     time.Now(),
		error:  "",
	}
}

// Cid returns the Cid associated to this operation.
func (op *Operation) Cid() *cid.Cid {
	op.mu.RLock()
	defer op.mu.RUnlock()
	return op.pin.Cid
}

// Context returns the context associated to this operation.
func (op *Operation) Context() context.Context {
	return op.ctx
}

// Cancel will cancel the context associated to this operation.
func (op *Operation) Cancel() {
	op.cancel()
}

// Phase returns the Phase.
func (op *Operation) Phase() Phase {
	op.mu.RLock()
	defer op.mu.RUnlock()
	return op.phase
}

// SetPhase changes the Phase and updates the timestamp.
func (op *Operation) SetPhase(ph Phase) {
	op.mu.Lock()
	defer op.mu.Unlock()
	op.phase = ph
	op.ts = time.Now()
}

// Error returns any error message attached to the operation.
func (op *Operation) Error() string {
	op.mu.RLock()
	defer op.mu.RUnlock()
	return op.error
}

// SetError sets the phase to PhaseError along with
// an error message. It updates the timestamp.
func (op *Operation) SetError(err error) {
	op.mu.Lock()
	defer op.mu.Unlock()
	op.phase = PhaseError
	op.error = err.Error()
	op.ts = time.Now()
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
	op.mu.RLock()
	defer op.mu.RUnlock()
	return op.ts
}

// Cancelled returns whether the context for this
// operation has been cancelled.
func (op *Operation) Cancelled() bool {
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
			return api.TrackerStatusBug
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
			return api.TrackerStatusBug
		}
	case OperationRemote:
		return api.TrackerStatusRemote
	default:
		return api.TrackerStatusBug
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
	default:
		return OperationUnknown, PhaseError
	}
}
