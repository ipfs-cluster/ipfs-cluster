// Package optracker implements functionality to track the status of pin and
// operations as needed by implementations of the pintracker component.
// It particularly allows to obtain status information for a given Cid,
// to skip re-tracking already ongoing operations, or to cancel ongoing
// operations when contradictory ones arrive.
package optracker

import (
	"context"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
)

var logger = logging.Logger("optracker")

// OperationTracker tracks and manages all inflight Operations.
type OperationTracker struct {
	ctx context.Context // parent context for all ops
	pid peer.ID

	mu         sync.RWMutex
	operations map[string]*Operation
}

// NewOperationTracker creates a new OperationTracker.
func NewOperationTracker(ctx context.Context, pid peer.ID) *OperationTracker {
	return &OperationTracker{
		ctx:        ctx,
		pid:        pid,
		operations: make(map[string]*Operation),
	}
}

// TrackNewOperation creates and stores a new operation when no operation for
// the same pin Cid exists unless an ongoing operation of the same type exists.
// Otherwise it will cancel and replace existing operations with new ones.
func (opt *OperationTracker) TrackNewOperation(pin api.Pin, typ OperationType, ph Phase) *Operation {
	cidStr := pin.Cid.String()

	opt.mu.Lock()
	defer opt.mu.Unlock()

	op, ok := opt.operations[cidStr]
	if ok { // operation exists
		if op.Type() == typ && op.Phase() != PhaseError && op.Phase() != PhaseDone {
			return nil // an ongoing operation of the same sign exists
		}
		op.cancel() // cancel ongoing operation and replace it
	}

	op2 := NewOperation(opt.ctx, pin, typ, ph)
	logger.Debugf("'%s' on cid '%s' has been created with phase '%s'", typ, cidStr, ph)
	opt.operations[cidStr] = op2
	return op2
}

// Clean deletes an operation from the tracker if it is the one we are tracking
// (compares pointers)
func (opt *OperationTracker) Clean(op *Operation) {
	cidStr := op.Cid().String()

	opt.mu.Lock()
	defer opt.mu.Unlock()
	op2, ok := opt.operations[cidStr]
	if ok && op == op2 { // same pointer
		delete(opt.operations, cidStr)
	}
}

// Status returns the TrackerStatus associated to the last operation known
// with the given Cid. It returns false if we are not tracking any operation
// for the given Cid.
func (opt *OperationTracker) Status(c *cid.Cid) (api.TrackerStatus, bool) {
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	op, ok := opt.operations[c.String()]
	if !ok {
		return 0, false
	}

	return op.ToTrackerStatus(), true
}

// SetError transitions an operation for a Cid into PhaseError if its Status
// is PhaseDone. Any other phases are considered in-flight and not touched.
// For things already in error, the error message is updated.
func (opt *OperationTracker) SetError(c *cid.Cid, err error) {
	opt.mu.Lock()
	defer opt.mu.Unlock()
	op, ok := opt.operations[c.String()]
	if !ok {
		return
	}

	if ph := op.Phase(); ph == PhaseDone || ph == PhaseError {
		op.SetPhase(PhaseError)
		op.SetError(err)
	}
}

func (opt *OperationTracker) unsafePinInfo(op *Operation) api.PinInfo {
	if op == nil {
		return api.PinInfo{
			Cid:    nil,
			Peer:   opt.pid,
			Status: api.TrackerStatusUnpinned,
			TS:     time.Now(),
			Error:  "",
		}
	}

	return api.PinInfo{
		Cid:    op.Cid(),
		Peer:   opt.pid,
		Status: op.ToTrackerStatus(),
		TS:     op.Timestamp(),
		Error:  op.Error(),
	}
}

// Get returns a PinInfo object for Cid.
func (opt *OperationTracker) Get(c *cid.Cid) api.PinInfo {
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	op := opt.operations[c.String()]
	pInfo := opt.unsafePinInfo(op)
	if pInfo.Cid == nil {
		pInfo.Cid = c
	}
	return pInfo
}

// GetAll returns PinInfo objets for all known operations
func (opt *OperationTracker) GetAll() []api.PinInfo {
	var pinfos []api.PinInfo
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	for _, op := range opt.operations {
		pinfos = append(pinfos, opt.unsafePinInfo(op))
	}
	return pinfos
}

// GetOpContext gets the context of an operation, if any.
func (opt *OperationTracker) GetOpContext(c *cid.Cid) context.Context {
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	op, ok := opt.operations[c.String()]
	if !ok {
		return nil
	}
	return op.Context()
}
