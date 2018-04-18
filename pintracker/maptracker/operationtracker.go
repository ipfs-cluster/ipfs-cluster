package maptracker

import (
	"context"
	"sync"

	"github.com/ipfs/go-cid"
)

//go:generate stringer -type=operation

// operation represents the kinds of operations that the PinTracker
// performs and the operationTracker tracks the status of.
type operation int

const (
	operationUnknown operation = iota
	operationPin
	operationUnpin
	operationSync
	operationRecover
)

//go:generate stringer -type=phase

// phase represents the multiple phase that an operation can be in.
type phase int

const (
	phaseError phase = iota
	phaseQueued
	phaseInProgress
)

type operationCtx struct {
	cid    *cid.Cid
	op     operation
	phase  phase
	ctx    context.Context
	cancel func()
}

func newOperationCtxWithContext(ctx context.Context, c *cid.Cid, op operation) operationCtx {
	ctx, cancel := context.WithCancel(ctx)
	return operationCtx{
		cid:    c,
		op:     op,
		phase:  phaseQueued,
		ctx:    ctx,
		cancel: cancel, // use *operationTracker.cancelOperation() instead
	}
}

type operationTracker struct {
	ctx context.Context

	mu         sync.RWMutex
	operations map[string]operationCtx
}

func newOperationTracker(ctx context.Context) *operationTracker {
	return &operationTracker{
		ctx:        ctx,
		operations: make(map[string]operationCtx),
	}
}

//TODO(ajl): return error or bool if there is already an ongoing operation
func (opt *operationTracker) trackNewOperationCtx(ctx context.Context, c *cid.Cid, op operation) {
	opt.set(newOperationCtxWithContext(ctx, c, op))
}

func (opt *operationTracker) cancelOperation(c *cid.Cid) {
	opc, ok := opt.get(c)
	if !ok {
		logger.Debugf("attempted to cancel non-existent operation with cid: %s", c.String())
		return
	}
	opc.cancel()
	logger.Infof("operation '%s' on cid '%s' has been cancelled", opc.op.String(), c.String())
	opt.remove(c)
	logger.Infof("operation '%s' on cid '%s' has been deleted", opc.op.String(), c.String())
}

func (opt *operationTracker) updateOperationPhase(c *cid.Cid, p phase) {
	opc, ok := opt.get(c)
	if !ok {
		logger.Debugf("attempted to update non-existent operation with cid: %s", c.String())
		return
	}
	opc.phase = p
	opt.set(opc)
	logger.Infof("operation '%s' on cid '%s' has been updated to phase '%s'", opc.op.String(), c.String(), p.String())
}

func (opt *operationTracker) operationComplete(c *cid.Cid) {
	opt.remove(c)
}

func (opt *operationTracker) set(oc operationCtx) {
	opt.mu.Lock()
	opt.operations[oc.cid.String()] = oc
	opt.mu.Unlock()
}

func (opt *operationTracker) get(c *cid.Cid) (operationCtx, bool) {
	opt.mu.RLock()
	opc, ok := opt.operations[c.String()]
	opt.mu.RUnlock()
	return opc, ok
}

func (opt *operationTracker) remove(c *cid.Cid) {
	opt.mu.Lock()
	delete(opt.operations, c.String())
	opt.mu.Unlock()
}
