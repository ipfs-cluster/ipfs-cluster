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
	"go.opencensus.io/trace"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
)

var logger = logging.Logger("optracker")

// OperationTracker tracks and manages all inflight Operations.
type OperationTracker struct {
	ctx      context.Context // parent context for all ops
	pid      peer.ID
	peerName string

	mu         sync.RWMutex
	operations map[string]*Operation
}

// NewOperationTracker creates a new OperationTracker.
func NewOperationTracker(ctx context.Context, pid peer.ID, peerName string) *OperationTracker {
	return &OperationTracker{
		ctx:        ctx,
		pid:        pid,
		peerName:   peerName,
		operations: make(map[string]*Operation),
	}
}

// TrackNewOperation will create, track and return a new operation unless
// one already exists to do the same thing, in which case nil is returned.
//
// If an operation exists it is of different type, it is
// cancelled and the new one replaces it in the tracker.
func (opt *OperationTracker) TrackNewOperation(ctx context.Context, pin api.Pin, typ OperationType, ph Phase) *Operation {
	ctx = trace.NewContext(opt.ctx, trace.FromContext(ctx))
	ctx, span := trace.StartSpan(ctx, "optracker/TrackNewOperation")
	defer span.End()

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

	op2 := NewOperation(ctx, pin, typ, ph)
	logger.Debugf("'%s' on cid '%s' has been created with phase '%s'", typ, cidStr, ph)
	opt.operations[cidStr] = op2
	return op2
}

// Clean deletes an operation from the tracker if it is the one we are tracking
// (compares pointers).
func (opt *OperationTracker) Clean(ctx context.Context, op *Operation) {
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
func (opt *OperationTracker) Status(ctx context.Context, c cid.Cid) (api.TrackerStatus, bool) {
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
// Remote pins are ignored too.
func (opt *OperationTracker) SetError(ctx context.Context, c cid.Cid, err error) {
	opt.mu.Lock()
	defer opt.mu.Unlock()
	op, ok := opt.operations[c.String()]
	if !ok {
		return
	}

	if ty := op.Type(); ty == OperationRemote {
		return
	}

	if ph := op.Phase(); ph == PhaseDone || ph == PhaseError {
		op.SetPhase(PhaseError)
		op.SetError(err)
	}
}

func (opt *OperationTracker) unsafePinInfo(ctx context.Context, op *Operation) api.PinInfo {
	if op == nil {
		return api.PinInfo{
			Cid:      cid.Undef,
			Peer:     opt.pid,
			PeerName: opt.peerName,
			Status:   api.TrackerStatusUnpinned,
			TS:       time.Now(),
			Error:    "",
		}
	}
	return api.PinInfo{
		Cid:      op.Cid(),
		Peer:     opt.pid,
		PeerName: opt.peerName,
		Status:   op.ToTrackerStatus(),
		TS:       op.Timestamp(),
		Error:    op.Error(),
	}
}

// Get returns a PinInfo object for Cid.
func (opt *OperationTracker) Get(ctx context.Context, c cid.Cid) api.PinInfo {
	ctx, span := trace.StartSpan(ctx, "optracker/GetAll")
	defer span.End()

	opt.mu.RLock()
	defer opt.mu.RUnlock()
	op := opt.operations[c.String()]
	pInfo := opt.unsafePinInfo(ctx, op)
	if pInfo.Cid == cid.Undef {
		pInfo.Cid = c
	}
	return pInfo
}

// GetExists returns a PinInfo object for a Cid only if there exists
// an associated Operation.
func (opt *OperationTracker) GetExists(ctx context.Context, c cid.Cid) (api.PinInfo, bool) {
	ctx, span := trace.StartSpan(ctx, "optracker/GetExists")
	defer span.End()

	opt.mu.RLock()
	defer opt.mu.RUnlock()
	op, ok := opt.operations[c.String()]
	if !ok {
		return api.PinInfo{}, false
	}
	pInfo := opt.unsafePinInfo(ctx, op)
	return pInfo, true
}

// GetAll returns PinInfo objets for all known operations.
func (opt *OperationTracker) GetAll(ctx context.Context) []api.PinInfo {
	ctx, span := trace.StartSpan(ctx, "optracker/GetAll")
	defer span.End()

	var pinfos []api.PinInfo
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	for _, op := range opt.operations {
		pinfos = append(pinfos, opt.unsafePinInfo(ctx, op))
	}
	return pinfos
}

// CleanError removes the associated Operation, if it is
// in PhaseError.
func (opt *OperationTracker) CleanError(ctx context.Context, c cid.Cid) {
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	errop, ok := opt.operations[c.String()]
	if !ok {
		return
	}

	if errop.Phase() != PhaseError {
		return
	}

	opt.Clean(ctx, errop)
	return
}

// CleanAllDone deletes any operation from the tracker that is in PhaseDone.
func (opt *OperationTracker) CleanAllDone(ctx context.Context) {
	opt.mu.Lock()
	defer opt.mu.Unlock()
	for _, op := range opt.operations {
		if op.Phase() == PhaseDone {
			delete(opt.operations, op.Cid().String())
		}
	}
}

// OpContext gets the context of an operation, if any.
func (opt *OperationTracker) OpContext(ctx context.Context, c cid.Cid) context.Context {
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
func (opt *OperationTracker) Filter(ctx context.Context, filters ...interface{}) []api.PinInfo {
	var pinfos []api.PinInfo
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	ops := filterOpsMap(ctx, opt.operations, filters)
	for _, op := range ops {
		pinfos = append(pinfos, opt.unsafePinInfo(ctx, op))
	}
	return pinfos
}

// filterOps returns a slice that only contains operations
// with the matching filter. Note, only supports
// filters of type OperationType or Phase, any other type
// will result in a nil slice being returned.
func (opt *OperationTracker) filterOps(ctx context.Context, filters ...interface{}) []*Operation {
	var fltops []*Operation
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	for _, op := range filterOpsMap(ctx, opt.operations, filters) {
		fltops = append(fltops, op)
	}
	return fltops
}

func filterOpsMap(ctx context.Context, ops map[string]*Operation, filters []interface{}) map[string]*Operation {
	fltops := make(map[string]*Operation)
	if len(filters) < 1 {
		return nil
	}

	if len(filters) == 1 {
		filter(ctx, ops, fltops, filters[0])
		return fltops
	}

	mainFilter, filters := filters[0], filters[1:]
	filter(ctx, ops, fltops, mainFilter)

	return filterOpsMap(ctx, fltops, filters)
}

func filter(ctx context.Context, in, out map[string]*Operation, filter interface{}) {
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
