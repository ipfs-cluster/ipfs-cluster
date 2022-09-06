// Package optracker implements functionality to track the status of pin and
// operations as needed by implementations of the pintracker component.
// It particularly allows to obtain status information for a given Cid,
// to skip re-tracking already ongoing operations, or to cancel ongoing
// operations when opposing ones arrive.
package optracker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/observations"

	logging "github.com/ipfs/go-log/v2"
	peer "github.com/libp2p/go-libp2p/core/peer"

	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
)

var logger = logging.Logger("optracker")

// OperationTracker tracks and manages all inflight Operations.
type OperationTracker struct {
	// struct alignment. This fields must be upfront!
	pinningCount   int64
	pinErrorCount  int64
	pinQueuedCount int64

	ctx      context.Context // parent context for all ops
	pid      peer.ID
	peerName string

	mu         sync.RWMutex
	operations map[api.Cid]*Operation
}

func (opt *OperationTracker) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "pid: %v\n", opt.pid)
	fmt.Fprintf(&b, "name: %s\n", opt.peerName)

	fmt.Fprint(&b, "operations:\n")
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	for _, op := range opt.operations {
		opstr := op.String()
		opstrs := strings.Split(opstr, "\n")
		for _, s := range opstrs {
			fmt.Fprintf(&b, "\t%s\n", s)
		}
	}
	return b.String()
}

// NewOperationTracker creates a new OperationTracker.
func NewOperationTracker(ctx context.Context, pid peer.ID, peerName string) *OperationTracker {
	initializeMetrics(ctx)

	return &OperationTracker{
		ctx:        ctx,
		pid:        pid,
		peerName:   peerName,
		operations: make(map[api.Cid]*Operation),
	}
}

// TrackNewOperation will create, track and return a new operation unless
// one already exists to do the same thing, in which case nil is returned.
//
// If an operation exists it is of different type, it is
// canceled and the new one replaces it in the tracker.
func (opt *OperationTracker) TrackNewOperation(ctx context.Context, pin api.Pin, typ OperationType, ph Phase) *Operation {
	ctx = trace.NewContext(opt.ctx, trace.FromContext(ctx))
	ctx, span := trace.StartSpan(ctx, "optracker/TrackNewOperation")
	defer span.End()

	opt.mu.Lock()
	defer opt.mu.Unlock()

	op, ok := opt.operations[pin.Cid]
	if ok { // operation exists for the CID
		if op.Type() == typ && op.Phase() != PhaseError && op.Phase() != PhaseDone {
			// an ongoing operation of the same
			// type. i.e. pinning, or queued.
			return nil
		}
		// i.e. operations in error phase
		// i.e. pin operations that need to be canceled for unpinning
		op.tracker.recordMetric(op, -1)
		op.Cancel() // cancel ongoing operation and replace it
	}

	op2 := newOperation(ctx, pin, typ, ph, opt)
	if ok && op.Type() == typ {
		// Carry over the attempt count when doing an operation of the
		// same type.  The old operation exists and was canceled.
		op2.attemptCount = op.AttemptCount() // carry the count
	}
	logger.Debugf("'%s' on cid '%s' has been created with phase '%s'", typ, pin.Cid, ph)
	opt.operations[pin.Cid] = op2
	opt.recordMetricUnsafe(op2, 1)
	return op2
}

// Clean deletes an operation from the tracker if it is the one we are tracking
// (compares pointers).
func (opt *OperationTracker) Clean(ctx context.Context, op *Operation) {
	opt.mu.Lock()
	defer opt.mu.Unlock()
	op2, ok := opt.operations[op.Cid()]
	if ok && op == op2 { // same pointer
		delete(opt.operations, op.Cid())
	}
}

// Status returns the TrackerStatus associated to the last operation known
// with the given Cid. It returns false if we are not tracking any operation
// for the given Cid.
func (opt *OperationTracker) Status(ctx context.Context, c api.Cid) (api.TrackerStatus, bool) {
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	op, ok := opt.operations[c]
	if !ok {
		return 0, false
	}

	return op.ToTrackerStatus(), true
}

// SetError transitions an operation for a Cid into PhaseError if its Status
// is PhaseDone. Any other phases are considered in-flight and not touched.
// For things already in error, the error message is updated.
// Remote pins are ignored too.
// Only used in tests right now.
func (opt *OperationTracker) SetError(ctx context.Context, c api.Cid, err error) {
	opt.mu.Lock()
	defer opt.mu.Unlock()
	op, ok := opt.operations[c]
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

func (opt *OperationTracker) unsafePinInfo(ctx context.Context, op *Operation, ipfs api.IPFSID) api.PinInfo {
	if op == nil {
		return api.PinInfo{
			Cid:     api.CidUndef,
			Name:    "",
			Peer:    opt.pid,
			Origins: nil,
			//Created:  0,
			Metadata: nil,
			PinInfoShort: api.PinInfoShort{
				PeerName:     opt.peerName,
				IPFS:         "",
				Status:       api.TrackerStatusUnpinned,
				TS:           time.Now(),
				AttemptCount: 0,
				PriorityPin:  false,
				Error:        "",
			},
		}
	}
	return api.PinInfo{
		Cid:         op.Cid(),
		Name:        op.Pin().Name,
		Peer:        opt.pid,
		Allocations: op.Pin().Allocations,
		Origins:     op.Pin().Origins,
		Created:     op.Pin().Timestamp,
		Metadata:    op.Pin().Metadata,
		PinInfoShort: api.PinInfoShort{
			PeerName:      opt.peerName,
			IPFS:          ipfs.ID,
			IPFSAddresses: ipfs.Addresses,
			Status:        op.ToTrackerStatus(),
			TS:            op.Timestamp(),
			AttemptCount:  op.AttemptCount(),
			PriorityPin:   op.PriorityPin(),
			Error:         op.Error(),
		},
	}
}

// Get returns a PinInfo object for Cid.
func (opt *OperationTracker) Get(ctx context.Context, c api.Cid, ipfs api.IPFSID) api.PinInfo {
	ctx, span := trace.StartSpan(ctx, "optracker/GetAll")
	defer span.End()

	opt.mu.RLock()
	defer opt.mu.RUnlock()
	op := opt.operations[c]
	pInfo := opt.unsafePinInfo(ctx, op, ipfs)
	if !pInfo.Cid.Defined() {
		pInfo.Cid = c
	}
	return pInfo
}

// GetExists returns a PinInfo object for a Cid only if there exists
// an associated Operation.
func (opt *OperationTracker) GetExists(ctx context.Context, c api.Cid, ipfs api.IPFSID) (api.PinInfo, bool) {
	ctx, span := trace.StartSpan(ctx, "optracker/GetExists")
	defer span.End()

	opt.mu.RLock()
	defer opt.mu.RUnlock()
	op, ok := opt.operations[c]
	if !ok {
		return api.PinInfo{}, false
	}
	pInfo := opt.unsafePinInfo(ctx, op, ipfs)
	return pInfo, true
}

// GetAll returns PinInfo objects for all known operations.
func (opt *OperationTracker) GetAll(ctx context.Context, ipfs api.IPFSID) []api.PinInfo {
	ctx, span := trace.StartSpan(ctx, "optracker/GetAll")
	defer span.End()

	ch := make(chan api.PinInfo, 1024)
	var pinfos []api.PinInfo
	go opt.GetAllChannel(ctx, api.TrackerStatusUndefined, ipfs, ch)
	for pinfo := range ch {
		pinfos = append(pinfos, pinfo)
	}
	return pinfos
}

// GetAllChannel returns all known operations that match the filter on the
// provided channel. Blocks until done.
func (opt *OperationTracker) GetAllChannel(ctx context.Context, filter api.TrackerStatus, ipfs api.IPFSID, out chan<- api.PinInfo) error {
	defer close(out)

	opt.mu.RLock()
	defer opt.mu.RUnlock()

	for _, op := range opt.operations {
		pinfo := opt.unsafePinInfo(ctx, op, ipfs)
		if pinfo.Status.Match(filter) {
			select {
			case <-ctx.Done():
				return fmt.Errorf("listing operations aborted: %w", ctx.Err())
			default:
			}

			select {
			case <-ctx.Done():
				return fmt.Errorf("listing operations aborted: %w", ctx.Err())
			case out <- pinfo:
			}
		}
	}
	return nil
}

// CleanAllDone deletes any operation from the tracker that is in PhaseDone.
func (opt *OperationTracker) CleanAllDone(ctx context.Context) {
	opt.mu.Lock()
	defer opt.mu.Unlock()
	for _, op := range opt.operations {
		if op.Phase() == PhaseDone {
			delete(opt.operations, op.Cid())
		}
	}
}

// OpContext gets the context of an operation, if any.
func (opt *OperationTracker) OpContext(ctx context.Context, c api.Cid) context.Context {
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	op, ok := opt.operations[c]
	if !ok {
		return nil
	}
	return op.Context()
}

// Filter returns a slice of api.PinInfos that had associated
// Operations that matched the provided filter. Note, only supports
// filters of type OperationType or Phase, any other type
// will result in a nil slice being returned.
func (opt *OperationTracker) Filter(ctx context.Context, ipfs api.IPFSID, filters ...interface{}) []api.PinInfo {
	var pinfos []api.PinInfo
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	ops := filterOpsMap(ctx, opt.operations, filters)
	for _, op := range ops {
		pinfo := opt.unsafePinInfo(ctx, op, ipfs)
		pinfos = append(pinfos, pinfo)
	}
	return pinfos
}

// filterOps returns a slice that only contains operations
// with the matching filter. Note, only supports
// filters of type OperationType or Phase, any other type
// will result in a nil slice being returned.
// Only used in tests right now.
func (opt *OperationTracker) filterOps(ctx context.Context, filters ...interface{}) []*Operation {
	var fltops []*Operation
	opt.mu.RLock()
	defer opt.mu.RUnlock()
	for _, op := range filterOpsMap(ctx, opt.operations, filters) {
		fltops = append(fltops, op)
	}
	return fltops
}

func filterOpsMap(ctx context.Context, ops map[api.Cid]*Operation, filters []interface{}) map[api.Cid]*Operation {
	fltops := make(map[api.Cid]*Operation)
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

func filter(ctx context.Context, in, out map[api.Cid]*Operation, filter interface{}) {
	for _, op := range in {
		switch filter.(type) {
		case OperationType:
			if op.Type() == filter {
				out[op.Cid()] = op
			}
		case Phase:
			if op.Phase() == filter {
				out[op.Cid()] = op
			}
		}
	}
}

func initializeMetrics(ctx context.Context) {
	stats.Record(ctx, observations.PinsPinError.M(0))
	stats.Record(ctx, observations.PinsQueued.M(0))
	stats.Record(ctx, observations.PinsPinning.M(0))
}

func (opt *OperationTracker) recordMetricUnsafe(op *Operation, val int64) {
	if opt == nil || op == nil {
		return
	}

	if op.opType == OperationPin {
		switch op.phase {
		case PhaseError:
			pinErrors := atomic.AddInt64(&opt.pinErrorCount, val)
			stats.Record(op.Context(), observations.PinsPinError.M(pinErrors))
		case PhaseQueued:
			pinQueued := atomic.AddInt64(&opt.pinQueuedCount, val)
			stats.Record(op.Context(), observations.PinsQueued.M(pinQueued))
		case PhaseInProgress:
			pinning := atomic.AddInt64(&opt.pinningCount, val)
			stats.Record(op.Context(), observations.PinsPinning.M(pinning))
		case PhaseDone:
			// we have no metric to log anything
		}
	}
}

func (opt *OperationTracker) recordMetric(op *Operation, val int64) {
	if op == nil {
		return
	}
	op.mu.RLock()
	{
		opt.recordMetricUnsafe(op, val)
	}
	op.mu.RUnlock()
}

// PinQueueSize returns the current number of items queued to pin.
func (opt *OperationTracker) PinQueueSize() int64 {
	return atomic.LoadInt64(&opt.pinQueuedCount)
}
