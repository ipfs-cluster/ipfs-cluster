package optracker

import (
	"context"
	"sync"

	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
)

var logger = logging.Logger("optracker")

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
)

// Operation represents an ongoing operation involving a
// particular Cid. It provides the type and phase of operation
// and a way to mark the operation finished (also used to cancel).
type Operation struct {
	Cid    *cid.Cid
	Op     OperationType
	Phase  Phase
	Ctx    context.Context
	cancel func()
}

// NewOperation creates a new Operation.
func NewOperation(ctx context.Context, c *cid.Cid, op OperationType) Operation {
	ctx, cancel := context.WithCancel(ctx)
	return Operation{
		Cid:    c,
		Op:     op,
		Phase:  PhaseQueued,
		Ctx:    ctx,
		cancel: cancel, // use *OperationTracker.Finish() instead
	}
}

// OperationTracker tracks and manages all inflight Operations.
type OperationTracker struct {
	ctx context.Context

	mu         sync.RWMutex
	operations map[string]Operation
}

// NewOperationTracker creates a new OperationTracker.
func NewOperationTracker(ctx context.Context) *OperationTracker {
	return &OperationTracker{
		ctx:        ctx,
		operations: make(map[string]Operation),
	}
}

// TrackNewOperation tracks a new operation, adding it to the OperationTracker's
// map of inflight operations.
func (opt *OperationTracker) TrackNewOperation(ctx context.Context, c *cid.Cid, op OperationType) {
	op2 := NewOperation(ctx, c, op)
	logger.Debugf(
		"'%s' on cid '%s' has been created with phase '%s'",
		op.String(),
		c.String(),
		op2.Phase.String(),
	)
	opt.Set(op2)
}

// UpdateOperationPhase updates the phase of the operation associated with
// the provided Cid.
func (opt *OperationTracker) UpdateOperationPhase(c *cid.Cid, p Phase) {
	opc, ok := opt.Get(c)
	if !ok {
		logger.Debugf(
			"attempted to update non-existent operation with cid: %s",
			c.String(),
		)
		return
	}
	opc.Phase = p
	opt.Set(opc)
	logger.Debugf(
		"'%s' on cid '%s' has been updated to phase '%s'",
		opc.Op.String(),
		c.String(),
		p.String(),
	)
}

// SetError sets the phase of the operation to PhaseError and cancels
// the operation context. Error is similar to Finish but doesn't
// delete the operation from the map.
func (opt *OperationTracker) SetError(c *cid.Cid) Operation {
	logger.Debugf("optracker: setting operation in error state: %s", c)
	opt.mu.Lock()
	defer opt.mu.Unlock()

	opc, ok := opt.operations[c.String()]
	if !ok {
		logger.Debugf(
			"attempted to remove non-existent operation with cid: %s",
			c.String(),
		)
		return Operation{}
	}

	opc.Phase = PhaseError
	logger.Debugf(
		"'%s' on cid '%s' has been updated to phase '%s'",
		opc.Op.String(),
		c.String(),
		PhaseError,
	)
	opc.cancel()
	return opc
}

// RemoveErroredOperations removes operations that have errored from
// the map.
func (opt *OperationTracker) RemoveErroredOperations(c *cid.Cid) {
	opt.mu.Lock()
	defer opt.mu.Unlock()

	if opc, ok := opt.operations[c.String()]; ok && opc.Phase == PhaseError {
		delete(opt.operations, c.String())
		logger.Debugf(
			"'%s' on cid '%s' has been removed",
			opc.Op.String(),
			c.String(),
		)
	}

	logger.Debugf(
		"attempted to remove non-errored operation with cid: %s",
		c.String(),
	)
	return
}

// Finish cancels the operation context and removes it from the map.
// If the Operations Phase has been set to PhaseError, the operation
// context will be cancelled but the operation won't be removed from
// the map.
func (opt *OperationTracker) Finish(c *cid.Cid) {
	opt.mu.Lock()
	defer opt.mu.Unlock()

	opc, ok := opt.operations[c.String()]
	if !ok {
		logger.Debugf(
			"attempted to remove non-existent operation with cid: %s",
			c.String(),
		)
		return
	}

	if opc.Phase != PhaseError {
		opc.cancel()
		delete(opt.operations, c.String())
		logger.Debugf(
			"'%s' on cid '%s' has been removed",
			opc.Op.String(),
			c.String(),
		)
	}
}

// Set sets the operation in the OperationTrackers map.
func (opt *OperationTracker) Set(oc Operation) {
	opt.mu.Lock()
	opt.operations[oc.Cid.String()] = oc
	opt.mu.Unlock()
}

// Get gets the operation associated with the Cid. If the
// there is no associated operation, Get will return Operation{}, false.
func (opt *OperationTracker) Get(c *cid.Cid) (Operation, bool) {
	opt.mu.RLock()
	opc, ok := opt.operations[c.String()]
	opt.mu.RUnlock()
	return opc, ok
}

// GetAll gets all inflight operations.
func (opt *OperationTracker) GetAll() []Operation {
	var ops []Operation
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	for _, op := range opt.operations {
		ops = append(ops, op)
	}
	return ops
}

// Filter returns a slice that only contains operations
// with the matching filter. Note, only supports
// filters of type OperationType or Phase, any other type
// will result in a nil slice being returned.
func (opt *OperationTracker) Filter(filter interface{}) []Operation {
	var ops []Operation
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	for _, op := range opt.operations {
		switch filter.(type) {
		case OperationType:
			if op.Op == filter {
				ops = append(ops, op)
			}
		case Phase:
			if op.Phase == filter {
				ops = append(ops, op)
			}
		default:
		}
	}
	return ops
}

// ToPinInfo converts an operation to an api.PinInfo.
func (op Operation) ToPinInfo(pid peer.ID) api.PinInfo {
	return api.PinInfo{Cid: op.Cid, Peer: pid, Status: op.ToTrackerStatus()}
}

// ToTrackerStatus converts an operation and it's phase to
// the most approriate api.TrackerStatus.
func (op Operation) ToTrackerStatus() api.TrackerStatus {
	switch op.Op {
	case OperationPin:
		switch op.Phase {
		case PhaseError:
			return api.TrackerStatusPinError
		case PhaseQueued:
			return api.TrackerStatusPinQueued
		case PhaseInProgress:
			return api.TrackerStatusPinning
		default:
			logger.Debugf("couldn't match operation to tracker status")
			return api.TrackerStatusBug
		}
	case OperationUnpin:
		switch op.Phase {
		case PhaseError:
			return api.TrackerStatusUnpinError
		case PhaseQueued:
			return api.TrackerStatusUnpinQueued
		case PhaseInProgress:
			return api.TrackerStatusUnpinning
		default:
			logger.Debugf("couldn't match operation to tracker status")
			return api.TrackerStatusBug
		}
	default:
		logger.Debugf("couldn't match operation to tracker status")
		return api.TrackerStatusBug
	}
}
