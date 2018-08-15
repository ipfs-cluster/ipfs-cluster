// Package optracker implements functionality to track the status of pin and
// operations as needed by implementations of the pintracker component.
// It particularly allows to obtain status information for a given Cid,
// to skip re-tracking already ongoing operations, or to cancel ongoing
// operations when opposing ones arrive.
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

// TrackNewOperation will create, track and return a new operation unless
// one already exists to do the same thing, in which case nil is returned.
//
// If an operation exists it is of different type, it is
// cancelled and the new one replaces it in the tracker.
func (opt *OperationTracker) TrackNewOperation(pin api.Pin, typ OperationType, ph Phase) *Operation {
	cidStr := pin.Cid.String()

	opt.mu.Lock()
	defer opt.mu.Unlock()

	op, ok := opt.operations[cidStr]
	if ok { // operation exists
		if op.Type() == typ && op.Phase() != PhaseError && op.Phase() != PhaseDone {
			return nil // an ongoing operation of the same sign exists
		}
		op.Cancel() // cancel ongoing operation and replace it
	}

	op2 := NewOperation(opt.ctx, pin, typ, ph)
	logger.Debugf("'%s' on cid '%s' has been created with phase '%s'", typ, cidStr, ph)
	opt.operations[cidStr] = op2
	return op2
}

// Clean deletes an operation from the tracker if it is the one we are tracking
// (compares pointers).
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

// GetExists returns a PinInfo object for a Cid only if there exists
// an associated Operation.
func (opt *OperationTracker) GetExists(c *cid.Cid) (api.PinInfo, bool) {
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	op, ok := opt.operations[c.String()]
	if !ok {
		return api.PinInfo{}, false
	}
	pInfo := opt.unsafePinInfo(op)
	return pInfo, true
}

// GetAll returns PinInfo objets for all known operations.
func (opt *OperationTracker) GetAll() []api.PinInfo {
	var pinfos []api.PinInfo
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	for _, op := range opt.operations {
		pinfos = append(pinfos, opt.unsafePinInfo(op))
	}
	return pinfos
}

// CleanError removes the associated Operation, if it is
// in PhaseError.
func (opt *OperationTracker) CleanError(c *cid.Cid) {
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	errop, ok := opt.operations[c.String()]
	if !ok {
		return
	}

	if errop.Phase() != PhaseError {
		return
	}

	opt.Clean(errop)
	return
}

// CleanAllDone deletes any operation from the tracker that is in PhaseDone.
func (opt *OperationTracker) CleanAllDone() {
	opt.mu.Lock()
	defer opt.mu.Unlock()
	for _, op := range opt.operations {
		if op.Phase() == PhaseDone {
			delete(opt.operations, op.Cid().String())
		}
	}
}

// OpContext gets the context of an operation, if any.
func (opt *OperationTracker) OpContext(c *cid.Cid) context.Context {
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	op, ok := opt.operations[c.String()]
	if !ok {
		return nil
	}
	return op.Context()
}

// Filter returns a slice of api.PinInfos that had associated
// Operations that matched the provided filter. Note, only supports
// filters of type OperationType or Phase, any other type
// will result in a nil slice being returned.
func (opt *OperationTracker) Filter(filters ...interface{}) []api.PinInfo {
	var pinfos []api.PinInfo
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	ops := filterOpsMap(opt.operations, filters)
	for _, op := range ops {
		pinfos = append(pinfos, opt.unsafePinInfo(op))
	}
	return pinfos
}

// filterOps returns a slice that only contains operations
// with the matching filter. Note, only supports
// filters of type OperationType or Phase, any other type
// will result in a nil slice being returned.
func (opt *OperationTracker) filterOps(filters ...interface{}) []*Operation {
	var fltops []*Operation
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	for _, op := range filterOpsMap(opt.operations, filters) {
		fltops = append(fltops, op)
	}
	return fltops
}

func filterOpsMap(ops map[string]*Operation, filters []interface{}) map[string]*Operation {
	fltops := make(map[string]*Operation)
	if len(filters) < 1 {
		return nil
	}

	if len(filters) == 1 {
		filter(ops, fltops, filters[0])
		return fltops
	}

	mainFilter, filters := filters[0], filters[1:]
	filter(ops, fltops, mainFilter)

	return filterOpsMap(fltops, filters)
}

func filter(in, out map[string]*Operation, filter interface{}) {
	for _, op := range in {
		switch filter.(type) {
		case OperationType:
			if op.Type() == filter {
				out[op.Cid().String()] = op
			}
		case Phase:
			if op.Phase() == filter {
				out[op.Cid().String()] = op
			}
		}
	}
}
