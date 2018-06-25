// Package maptracker implements a PinTracker component for IPFS Cluster. It
// uses a map to keep track of the state of tracked pins.
package maptracker

import (
	"context"
	"errors"
	"sync"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/pintracker/optracker"
	"github.com/ipfs/ipfs-cluster/pintracker/util"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
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
func NewMapPinTracker(cfg *Config, pid peer.ID) *MapPinTracker {
	ctx, cancel := context.WithCancel(context.Background())

	mpt := &MapPinTracker{
		ctx:       ctx,
		cancel:    cancel,
		config:    cfg,
		optracker: optracker.NewOperationTracker(ctx, pid),
		rpcReady:  make(chan struct{}, 1),
		peerID:    pid,
		pinCh:     make(chan *optracker.Operation, cfg.MaxPinQueueSize),
		unpinCh:   make(chan *optracker.Operation, cfg.MaxPinQueueSize),
	}

	for i := 0; i < mpt.config.ConcurrentPins; i++ {
		go mpt.opWorker(mpt.pin, mpt.pinCh)
	}
	go mpt.opWorker(mpt.unpin, mpt.unpinCh)
	return mpt
}

// receives a pin Function (pin or unpin) and a channel.
// Used for both pinning and unpinning
func (mpt *MapPinTracker) opWorker(pinF func(*optracker.Operation) error, opChan chan *optracker.Operation) {
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
				mpt.optracker.Clean(op)
			}
		case <-mpt.ctx.Done():
			return
		}
	}
}

// Shutdown finishes the services provided by the MapPinTracker and cancels
// any active context.
func (mpt *MapPinTracker) Shutdown() error {
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
	logger.Debugf("issuing pin call for %s", op.Cid())
	err := mpt.rpcClient.CallContext(
		op.Context(),
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
	logger.Debugf("issuing unpin call for %s", op.Cid())
	err := mpt.rpcClient.CallContext(
		op.Context(),
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
func (mpt *MapPinTracker) enqueue(c api.Pin, typ optracker.OperationType, ch chan *optracker.Operation) error {
	op := mpt.optracker.TrackNewOperation(c, typ, optracker.PhaseQueued)
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
func (mpt *MapPinTracker) Track(c api.Pin) error {
	logger.Debugf("tracking %s", c.Cid)

	// Trigger unpin whenever something remote is tracked
	// Note, IPFSConn checks with pin/ls before triggering
	// pin/rm.
	if util.IsRemotePin(c, mpt.peerID) {
		op := mpt.optracker.TrackNewOperation(c, optracker.OperationRemote, optracker.PhaseInProgress)
		if op == nil {
			return nil // ongoing unpin
		}
		err := mpt.unpin(op)
		op.Cancel()
		if err != nil {
			op.SetError(err)
		} else {
			op.SetPhase(optracker.PhaseDone)
		}
		return nil
	}

	return mpt.enqueue(c, optracker.OperationPin, mpt.pinCh)
}

// Untrack tells the MapPinTracker to stop managing a Cid.
// If the Cid is pinned locally, it will be unpinned.
func (mpt *MapPinTracker) Untrack(c *cid.Cid) error {
	logger.Debugf("untracking %s", c)
	return mpt.enqueue(api.PinCid(c), optracker.OperationUnpin, mpt.unpinCh)
}

// Status returns information for a Cid tracked by this
// MapPinTracker.
func (mpt *MapPinTracker) Status(c *cid.Cid) api.PinInfo {
	return mpt.optracker.Get(c)
}

// StatusAll returns information for all Cids tracked by this
// MapPinTracker.
func (mpt *MapPinTracker) StatusAll() []api.PinInfo {
	return mpt.optracker.GetAll()
}

// Sync verifies that the status of a Cid matches that of
// the IPFS daemon. If not, it will be transitioned
// to PinError or UnpinError.
//
// Sync returns the updated local status for the given Cid.
// Pins in error states can be recovered with Recover().
// An error is returned if we are unable to contact
// the IPFS daemon.
func (mpt *MapPinTracker) Sync(c *cid.Cid) (api.PinInfo, error) {
	var ips api.IPFSPinStatus
	err := mpt.rpcClient.Call(
		"",
		"Cluster",
		"IPFSPinLsCid",
		api.PinCid(c).ToSerial(),
		&ips,
	)

	if err != nil {
		mpt.optracker.SetError(c, err)
		return mpt.optracker.Get(c), nil
	}

	return mpt.syncStatus(c, ips), nil
}

// SyncAll verifies that the statuses of all tracked Cids match the
// one reported by the IPFS daemon. If not, they will be transitioned
// to PinError or UnpinError.
//
// SyncAll returns the list of local status for all tracked Cids which
// were updated or have errors. Cids in error states can be recovered
// with Recover().
// An error is returned if we are unable to contact the IPFS daemon.
func (mpt *MapPinTracker) SyncAll() ([]api.PinInfo, error) {
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
		// set everything as error
		pInfos := mpt.optracker.GetAll()
		for _, pInfo := range pInfos {
			mpt.optracker.SetError(pInfo.Cid, err)
			results = append(results, mpt.optracker.Get(pInfo.Cid))
		}
		return results, nil
	}

	status := mpt.StatusAll()
	for _, pInfoOrig := range status {
		var pInfoNew api.PinInfo
		c := pInfoOrig.Cid
		ips, ok := ipsMap[c.String()]
		if !ok {
			pInfoNew = mpt.syncStatus(c, api.IPFSPinStatusUnpinned)
		} else {
			pInfoNew = mpt.syncStatus(c, ips)
		}

		if pInfoOrig.Status != pInfoNew.Status ||
			pInfoNew.Status == api.TrackerStatusUnpinError ||
			pInfoNew.Status == api.TrackerStatusPinError {
			results = append(results, pInfoNew)
		}
	}
	return results, nil
}

func (mpt *MapPinTracker) syncStatus(c *cid.Cid, ips api.IPFSPinStatus) api.PinInfo {
	status, ok := mpt.optracker.Status(c)
	if !ok {
		status = api.TrackerStatusUnpinned
	}

	if ips.IsPinned() {
		switch status {
		case api.TrackerStatusPinError:
			// If an item that we wanted to pin is pinned, we mark it so
			mpt.optracker.TrackNewOperation(
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
				api.PinCid(c),
				optracker.OperationUnpin,
				optracker.PhaseDone,
			)
			if op != nil {
				mpt.optracker.Clean(op)
			}

		case api.TrackerStatusPinned:
			// not pinned in IPFS but we think it should be: mark as error
			mpt.optracker.SetError(c, errUnpinned)
		default:
			// 1. Pinning phases
			// -> do nothing
		}
	}
	return mpt.optracker.Get(c)
}

// Recover will re-queue a Cid in error state for the failed operation,
// possibly retriggering an IPFS pinning operation.
func (mpt *MapPinTracker) Recover(c *cid.Cid) (api.PinInfo, error) {
	logger.Infof("Attempting to recover %s", c)
	pInfo := mpt.optracker.Get(c)
	var err error

	switch pInfo.Status {
	case api.TrackerStatusPinError:
		err = mpt.enqueue(api.PinCid(c), optracker.OperationPin, mpt.pinCh)
	case api.TrackerStatusUnpinError:
		err = mpt.enqueue(api.PinCid(c), optracker.OperationUnpin, mpt.unpinCh)
	}
	return mpt.optracker.Get(c), err
}

// RecoverAll attempts to recover all items tracked by this peer.
func (mpt *MapPinTracker) RecoverAll() ([]api.PinInfo, error) {
	pInfos := mpt.optracker.GetAll()
	var results []api.PinInfo
	for _, pInfo := range pInfos {
		res, err := mpt.Recover(pInfo.Cid)
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
func (mpt *MapPinTracker) OpContext(c *cid.Cid) context.Context {
	return mpt.optracker.OpContext(c)
}
