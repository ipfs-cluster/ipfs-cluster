package maptracker

import (
	"context"
	"sync"

	"github.com/ipfs/go-cid"
)

//go:generate stringer -type=operationType

// operationType represents the kinds of operations that the PinTracker
// performs and the operationTracker tracks the status of.
type operationType int

const (
	operationUnknown operationType = iota
	operationPin
	operationUnpin
)

//go:generate stringer -type=phase

// phase represents the multiple phase that an operation can be in.
type phase int

const (
	phaseError phase = iota
	phaseQueued
	phaseInProgress
)

type operation struct {
	cid    *cid.Cid
	op     operationType
	phase  phase
	ctx    context.Context
	cancel func()
}

func newOperationCtxWithContext(ctx context.Context, c *cid.Cid, op operationType) operation {
	ctx, cancel := context.WithCancel(ctx)
	return operation{
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
	operations map[string]operation
}

func newOperationTracker(ctx context.Context) *operationTracker {
	return &operationTracker{
		ctx:        ctx,
		operations: make(map[string]operation),
	}
}

//TODO(ajl): return error or bool if there is already an ongoing operation
func (opt *operationTracker) trackNewOperationCtx(ctx context.Context, c *cid.Cid, op operationType) {
	op2 := newOperationCtxWithContext(ctx, c, op)
	logger.Debugf(
		"'%s' on cid '%s' has been created with phase '%s'",
		op.String(),
		c.String(),
		op2.phase.String(),
	)
	opt.set(op2)
}

func (opt *operationTracker) updateOperationPhase(c *cid.Cid, p phase) {
	opc, ok := opt.get(c)
	if !ok {
		logger.Debugf(
			"attempted to update non-existent operation with cid: %s",
			c.String(),
		)
		return
	}
	opc.phase = p
	opt.set(opc)
	logger.Debugf(
		"'%s' on cid '%s' has been updated to phase '%s'",
		opc.op.String(),
		c.String(),
		p.String(),
	)
}

func (opt *operationTracker) set(oc operation) {
	opt.mu.Lock()
	opt.operations[oc.cid.String()] = oc
	opt.mu.Unlock()
}

func (opt *operationTracker) get(c *cid.Cid) (operation, bool) {
	opt.mu.RLock()
	opc, ok := opt.operations[c.String()]
	opt.mu.RUnlock()
	return opc, ok
}

// finish cancels the operation context and removes it from the map
func (opt *operationTracker) finish(c *cid.Cid) {
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

	opc.cancel()
	delete(opt.operations, c.String())
	logger.Debugf(
		"'%s' on cid '%s' has been removed",
		opc.op.String(),
		c.String(),
	)
}
