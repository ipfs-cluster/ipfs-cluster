// Package maptracker implements a PinTracker component for IPFS Cluster. It
// uses a map to keep track of the state of tracked pins.
package maptracker

import (
	"context"
	"errors"
	"sync"

	"go.opencensus.io/trace"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/pintracker/optracker"
	"github.com/ipfs/ipfs-cluster/pintracker/util"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p-peer"
)

var logger = logging.Logger("pintracker")

var (
	errUnpinned = errors.New("the item is unexpectedly not pinned on IPFS")
)

// MapPinTracker is a PinTracker implementation which uses a Go map
// to store the status of the tracked Cids. This component is thread-safe.
type MapPinTracker struct {
	config *Config

	optracker *optracker.OperationTracker

	ctx    context.Context
	cancel func()

	rpcClient *rpc.Client
	rpcReady  chan struct{}

	peerID  peer.ID
	pinCh   chan *optracker.Operation
	unpinCh chan *optracker.Operation

	shutdownLock sync.Mutex
	shutdown     bool
	wg           sync.WaitGroup
}

// NewMapPinTracker returns a new object which has been correcly
// initialized with the given configuration.
func NewMapPinTracker(cfg *Config, pid peer.ID, peerName string) *MapPinTracker {
	ctx, cancel := context.WithCancel(context.Background())

	mpt := &MapPinTracker{
		ctx:       ctx,
		cancel:    cancel,
		config:    cfg,
		optracker: optracker.NewOperationTracker(ctx, pid, peerName),
		rpcReady:  make(chan struct{}, 1),
		peerID:    pid,
		pinCh:     make(chan *optracker.Operation, cfg.MaxPinQueueSize),
		unpinCh:   make(chan *optracker.Operation, cfg.MaxPinQueueSize),
	}

	for i := 0; i < mpt.config.ConcurrentPins; i++ {
		go mpt.opWorker(ctx, mpt.pin, mpt.pinCh)
	}
	go mpt.opWorker(ctx, mpt.unpin, mpt.unpinCh)
	return mpt
}

// receives a pin Function (pin or unpin) and a channel.
// Used for both pinning and unpinning
func (mpt *MapPinTracker) opWorker(ctx context.Context, pinF func(*optracker.Operation) error, opChan chan *optracker.Operation) {
	for {
		select {
		case op := <-opChan:
			if op.Cancelled() {
				// operation was cancelled. Move on.
				// This saves some time, but not 100% needed.
				continue
			}
			op.SetPhase(optracker.PhaseInProgress)
			err := pinF(op) // call pin/unpin
			if err != nil {
				if op.Cancelled() {
					// there was an error because
					// we were cancelled. Move on.
					continue
				}
				op.SetError(err)
				op.Cancel()
				continue
			}
			op.SetPhase(optracker.PhaseDone)
			op.Cancel()

			// We keep all pinned things in the tracker,
			// only clean unpinned things.
			if op.Type() == optracker.OperationUnpin {
				mpt.optracker.Clean(ctx, op)
			}
		case <-mpt.ctx.Done():
			return
		}
	}
}

// Shutdown finishes the services provided by the MapPinTracker and cancels
// any active context.
func (mpt *MapPinTracker) Shutdown(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "tracker/map/Shutdown")
	defer span.End()

	mpt.shutdownLock.Lock()
	defer mpt.shutdownLock.Unlock()

	if mpt.shutdown {
		logger.Debug("already shutdown")
		return nil
	}

	logger.Info("stopping MapPinTracker")
	mpt.cancel()
	close(mpt.rpcReady)
	mpt.wg.Wait()
	mpt.shutdown = true
	return nil
}

func (mpt *MapPinTracker) pin(op *optracker.Operation) error {
	ctx, span := trace.StartSpan(op.Context(), "tracker/map/pin")
	defer span.End()

	logger.Debugf("issuing pin call for %s", op.Cid())
	err := mpt.rpcClient.CallContext(
		ctx,
		"",
		"Cluster",
		"IPFSPin",
		op.Pin().ToSerial(),
		&struct{}{},
	)
	if err != nil {
		return err
	}
	return nil
}

func (mpt *MapPinTracker) unpin(op *optracker.Operation) error {
	ctx, span := trace.StartSpan(op.Context(), "tracker/map/unpin")
	defer span.End()

	logger.Debugf("issuing unpin call for %s", op.Cid())
	err := mpt.rpcClient.CallContext(
		ctx,
		"",
		"Cluster",
		"IPFSUnpin",
		op.Pin().ToSerial(),
		&struct{}{},
	)
	if err != nil {
		return err
	}
	return nil
}

// puts a new operation on the queue, unless ongoing exists
func (mpt *MapPinTracker) enqueue(ctx context.Context, c api.Pin, typ optracker.OperationType, ch chan *optracker.Operation) error {
	ctx, span := trace.StartSpan(ctx, "tracker/map/enqueue")
	defer span.End()

	op := mpt.optracker.TrackNewOperation(ctx, c, typ, optracker.PhaseQueued)
	if op == nil {
		return nil // ongoing pin operation.
	}

	select {
	case ch <- op:
	default:
		err := errors.New("queue is full")
		op.SetError(err)
		op.Cancel()
		logger.Error(err.Error())
		return err
	}
	return nil
}

// Track tells the MapPinTracker to start managing a Cid,
// possibly triggering Pin operations on the IPFS daemon.
func (mpt *MapPinTracker) Track(ctx context.Context, c api.Pin) error {
	ctx, span := trace.StartSpan(ctx, "tracker/map/Track")
	defer span.End()

	logger.Debugf("tracking %s", c.Cid)

	// Sharded pins are never pinned. A sharded pin cannot turn into
	// something else or viceversa like it happens with Remote pins so
	// we just track them.
	if c.Type == api.MetaType {
		mpt.optracker.TrackNewOperation(ctx, c, optracker.OperationShard, optracker.PhaseDone)
		return nil
	}

	// Trigger unpin whenever something remote is tracked
	// Note, IPFSConn checks with pin/ls before triggering
	// pin/rm, so this actually does not always trigger unpin
	// to ipfs.
	if util.IsRemotePin(c, mpt.peerID) {
		op := mpt.optracker.TrackNewOperation(ctx, c, optracker.OperationRemote, optracker.PhaseInProgress)
		if op == nil {
			return nil // Ongoing operationRemote / PhaseInProgress
		}
		err := mpt.unpin(op) // unpin all the time, even if not pinned
		op.Cancel()
		if err != nil {
			op.SetError(err)
		} else {
			op.SetPhase(optracker.PhaseDone)
		}
		return nil
	}

	return mpt.enqueue(ctx, c, optracker.OperationPin, mpt.pinCh)
}

// Untrack tells the MapPinTracker to stop managing a Cid.
// If the Cid is pinned locally, it will be unpinned.
func (mpt *MapPinTracker) Untrack(ctx context.Context, c cid.Cid) error {
	ctx, span := trace.StartSpan(ctx, "tracker/map/Untrack")
	defer span.End()

	logger.Debugf("untracking %s", c)
	return mpt.enqueue(ctx, api.PinCid(c), optracker.OperationUnpin, mpt.unpinCh)
}

// Status returns information for a Cid tracked by this
// MapPinTracker.
func (mpt *MapPinTracker) Status(ctx context.Context, c cid.Cid) api.PinInfo {
	ctx, span := trace.StartSpan(mpt.ctx, "tracker/map/Status")
	defer span.End()

	return mpt.optracker.Get(ctx, c)
}

// StatusAll returns information for all Cids tracked by this
// MapPinTracker.
func (mpt *MapPinTracker) StatusAll(ctx context.Context) []api.PinInfo {
	ctx, span := trace.StartSpan(mpt.ctx, "tracker/map/StatusAll")
	defer span.End()

	return mpt.optracker.GetAll(ctx)
}

// Sync verifies that the status of a Cid matches that of
// the IPFS daemon. If not, it will be transitioned
// to PinError or UnpinError.
//
// Sync returns the updated local status for the given Cid.
// Pins in error states can be recovered with Recover().
// An error is returned if we are unable to contact
// the IPFS daemon.
func (mpt *MapPinTracker) Sync(ctx context.Context, c cid.Cid) (api.PinInfo, error) {
	ctx, span := trace.StartSpan(mpt.ctx, "tracker/map/Sync")
	defer span.End()

	var ips api.IPFSPinStatus
	err := mpt.rpcClient.Call(
		"",
		"Cluster",
		"IPFSPinLsCid",
		api.PinCid(c).ToSerial(),
		&ips,
	)

	if err != nil {
		mpt.optracker.SetError(ctx, c, err)
		return mpt.optracker.Get(ctx, c), nil
	}

	return mpt.syncStatus(ctx, c, ips), nil
}

// SyncAll verifies that the statuses of all tracked Cids match the
// one reported by the IPFS daemon. If not, they will be transitioned
// to PinError or UnpinError.
//
// SyncAll returns the list of local status for all tracked Cids which
// were updated or have errors. Cids in error states can be recovered
// with Recover().
// An error is returned if we are unable to contact the IPFS daemon.
func (mpt *MapPinTracker) SyncAll(ctx context.Context) ([]api.PinInfo, error) {
	ctx, span := trace.StartSpan(mpt.ctx, "tracker/map/SyncAll")
	defer span.End()

	var ipsMap map[string]api.IPFSPinStatus
	var results []api.PinInfo
	err := mpt.rpcClient.Call(
		"",
		"Cluster",
		"IPFSPinLs",
		"recursive",
		&ipsMap,
	)
	if err != nil {
		// set pinning or unpinning ops to error, since we can't
		// verify them
		pInfos := mpt.optracker.GetAll(ctx)
		for _, pInfo := range pInfos {
			op, _ := optracker.TrackerStatusToOperationPhase(pInfo.Status)
			if op == optracker.OperationPin || op == optracker.OperationUnpin {
				mpt.optracker.SetError(ctx, pInfo.Cid, err)
				results = append(results, mpt.optracker.Get(ctx, pInfo.Cid))
			} else {
				results = append(results, pInfo)
			}
		}
		return results, nil
	}

	status := mpt.StatusAll(ctx)
	for _, pInfoOrig := range status {
		var pInfoNew api.PinInfo
		c := pInfoOrig.Cid
		ips, ok := ipsMap[c.String()]
		if !ok {
			pInfoNew = mpt.syncStatus(ctx, c, api.IPFSPinStatusUnpinned)
		} else {
			pInfoNew = mpt.syncStatus(ctx, c, ips)
		}

		if pInfoOrig.Status != pInfoNew.Status ||
			pInfoNew.Status == api.TrackerStatusUnpinError ||
			pInfoNew.Status == api.TrackerStatusPinError {
			results = append(results, pInfoNew)
		}
	}
	return results, nil
}

func (mpt *MapPinTracker) syncStatus(ctx context.Context, c cid.Cid, ips api.IPFSPinStatus) api.PinInfo {
	status, ok := mpt.optracker.Status(ctx, c)
	if !ok {
		status = api.TrackerStatusUnpinned
	}

	// TODO(hector): for sharding, we may need to check that a shard
	// is pinned to the right depth. For now, we assumed that if it's pinned
	// in some way, then it must be right (including direct).
	pinned := func(i api.IPFSPinStatus) bool {
		switch i {
		case api.IPFSPinStatusRecursive:
			return i.IsPinned(-1)
		case api.IPFSPinStatusDirect:
			return i.IsPinned(0)
		default:
			return i.IsPinned(1) // Pinned with depth 1 or more.
		}
	}

	if pinned(ips) {
		switch status {
		case api.TrackerStatusPinError:
			// If an item that we wanted to pin is pinned, we mark it so
			mpt.optracker.TrackNewOperation(
				ctx,
				api.PinCid(c),
				optracker.OperationPin,
				optracker.PhaseDone,
			)
		default:
			// 1. Unpinning phases
			// 2. Pinned in ipfs but we are not tracking
			// -> do nothing
		}
	} else {
		switch status {
		case api.TrackerStatusUnpinError:
			// clean
			op := mpt.optracker.TrackNewOperation(
				ctx,
				api.PinCid(c),
				optracker.OperationUnpin,
				optracker.PhaseDone,
			)
			if op != nil {
				mpt.optracker.Clean(ctx, op)
			}

		case api.TrackerStatusPinned:
			// not pinned in IPFS but we think it should be: mark as error
			mpt.optracker.SetError(ctx, c, errUnpinned)
		default:
			// 1. Pinning phases
			// -> do nothing
		}
	}
	return mpt.optracker.Get(ctx, c)
}

// Recover will re-queue a Cid in error state for the failed operation,
// possibly retriggering an IPFS pinning operation.
func (mpt *MapPinTracker) Recover(ctx context.Context, c cid.Cid) (api.PinInfo, error) {
	ctx, span := trace.StartSpan(mpt.ctx, "tracker/map/Recover")
	defer span.End()

	logger.Infof("Attempting to recover %s", c)
	pInfo := mpt.optracker.Get(ctx, c)
	var err error

	switch pInfo.Status {
	case api.TrackerStatusPinError:
		err = mpt.enqueue(ctx, api.PinCid(c), optracker.OperationPin, mpt.pinCh)
	case api.TrackerStatusUnpinError:
		err = mpt.enqueue(ctx, api.PinCid(c), optracker.OperationUnpin, mpt.unpinCh)
	}
	return mpt.optracker.Get(ctx, c), err
}

// RecoverAll attempts to recover all items tracked by this peer.
func (mpt *MapPinTracker) RecoverAll(ctx context.Context) ([]api.PinInfo, error) {
	ctx, span := trace.StartSpan(mpt.ctx, "tracker/map/RecoverAll")
	defer span.End()

	pInfos := mpt.optracker.GetAll(ctx)
	var results []api.PinInfo
	for _, pInfo := range pInfos {
		res, err := mpt.Recover(ctx, pInfo.Cid)
		results = append(results, res)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

// SetClient makes the MapPinTracker ready to perform RPC requests to
// other components.
func (mpt *MapPinTracker) SetClient(c *rpc.Client) {
	mpt.rpcClient = c
	mpt.rpcReady <- struct{}{}
}

// OpContext exports the internal optracker's OpContext method.
// For testing purposes only.
func (mpt *MapPinTracker) OpContext(ctx context.Context, c cid.Cid) context.Context {
	return mpt.optracker.OpContext(ctx, c)
}
