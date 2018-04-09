// Package maptracker implements a PinTracker component for IPFS Cluster. It
// uses a map to keep track of the state of tracked pins.
package maptracker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
)

var logger = logging.Logger("pintracker")

var (
	errUnpinningTimeout = errors.New("unpinning operation is taking too long")
	errPinningTimeout   = errors.New("pinning operation is taking too long")
	errPinned           = errors.New("the item is unexpectedly pinned on IPFS")
	errUnpinned         = errors.New("the item is unexpectedly not pinned on IPFS")
)

// MapPinTracker is a PinTracker implementation which uses a Go map
// to store the status of the tracked Cids. This component is thread-safe.
type MapPinTracker struct {
	mux    sync.RWMutex
	status map[string]api.PinInfo
	config *Config

	ctx    context.Context
	cancel func()

	rpcClient *rpc.Client
	rpcReady  chan struct{}

	peerID  peer.ID
	pinCh   chan api.Pin
	unpinCh chan api.Pin

	shutdownLock sync.Mutex
	shutdown     bool
	wg           sync.WaitGroup
}

// NewMapPinTracker returns a new object which has been correcly
// initialized with the given configuration.
func NewMapPinTracker(cfg *Config, pid peer.ID) *MapPinTracker {
	ctx, cancel := context.WithCancel(context.Background())

	mpt := &MapPinTracker{
		ctx:      ctx,
		cancel:   cancel,
		status:   make(map[string]api.PinInfo),
		config:   cfg,
		rpcReady: make(chan struct{}, 1),
		peerID:   pid,
		pinCh:    make(chan api.Pin, cfg.MaxPinQueueSize),
		unpinCh:  make(chan api.Pin, cfg.MaxPinQueueSize),
	}
	for i := 0; i < mpt.config.ConcurrentPins; i++ {
		go mpt.pinWorker()
	}
	go mpt.unpinWorker()
	return mpt
}

// reads the queue and makes pins to the IPFS daemon one by one
func (mpt *MapPinTracker) pinWorker() {
	for {
		select {
		case p := <-mpt.pinCh:
			//TODO(ajl):
			mpt.pin(p)
		case <-mpt.ctx.Done():
			return
		}
	}
}

// reads the queue and makes unpin requests to the IPFS daemon
func (mpt *MapPinTracker) unpinWorker() {
	for {
		select {
		case p := <-mpt.unpinCh:
			mpt.unpin(p)
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

func (mpt *MapPinTracker) set(c *cid.Cid, s api.TrackerStatus) {
	mpt.mux.Lock()
	defer mpt.mux.Unlock()
	mpt.unsafeSet(c, s)
}

func (mpt *MapPinTracker) unsafeSet(c *cid.Cid, s api.TrackerStatus) {
	if s == api.TrackerStatusUnpinned {
		delete(mpt.status, c.String())
		return
	}

	mpt.status[c.String()] = api.PinInfo{
		Cid:    c,
		Peer:   mpt.peerID,
		Status: s,
		TS:     time.Now(),
		Error:  "",
	}
}

func (mpt *MapPinTracker) get(c *cid.Cid) api.PinInfo {
	mpt.mux.RLock()
	defer mpt.mux.RUnlock()
	return mpt.unsafeGet(c)
}

func (mpt *MapPinTracker) unsafeGet(c *cid.Cid) api.PinInfo {
	p, ok := mpt.status[c.String()]
	if !ok {
		return api.PinInfo{
			Cid:    c,
			Peer:   mpt.peerID,
			Status: api.TrackerStatusUnpinned,
			TS:     time.Now(),
			Error:  "",
		}
	}
	return p
}

// sets a Cid in error state
func (mpt *MapPinTracker) setError(c *cid.Cid, err error) {
	mpt.mux.Lock()
	defer mpt.mux.Unlock()
	mpt.unsafeSetError(c, err)
}

func (mpt *MapPinTracker) unsafeSetError(c *cid.Cid, err error) {
	p := mpt.unsafeGet(c)
	switch p.Status {
	case api.TrackerStatusPinned, api.TrackerStatusPinning, api.TrackerStatusPinError:
		mpt.status[c.String()] = api.PinInfo{
			Cid:    c,
			Peer:   mpt.peerID,
			Status: api.TrackerStatusPinError,
			TS:     time.Now(),
			Error:  err.Error(),
		}
	case api.TrackerStatusUnpinned, api.TrackerStatusUnpinning, api.TrackerStatusUnpinError:
		mpt.status[c.String()] = api.PinInfo{
			Cid:    c,
			Peer:   mpt.peerID,
			Status: api.TrackerStatusUnpinError,
			TS:     time.Now(),
			Error:  err.Error(),
		}
	}
}

func (mpt *MapPinTracker) isRemote(c api.Pin) bool {
	if c.ReplicationFactorMax < 0 {
		return false
	}
	if c.ReplicationFactorMax == 0 {
		logger.Errorf("Pin with replication factor 0! %+v", c)
	}

	for _, p := range c.Allocations {
		if p == mpt.peerID {
			return false
		}
	}
	return true
}

func (mpt *MapPinTracker) pin(c api.Pin) error {
	logger.Debugf("issuing pin call for %s", c.Cid)
	mpt.set(c.Cid, api.TrackerStatusPinning)
	err := mpt.rpcClient.Call(
		"",
		"Cluster",
		"IPFSPin",
		c.ToSerial(),
		&struct{}{},
	)
	if err != nil {
		mpt.setError(c.Cid, err)
		return err
	}

	mpt.set(c.Cid, api.TrackerStatusPinned)
	return nil
}

func (mpt *MapPinTracker) unpin(c api.Pin) error {
	logger.Debugf("issuing unpin call for %s", c.Cid)
	mpt.set(c.Cid, api.TrackerStatusUnpinning)
	err := mpt.rpcClient.Call(
		"",
		"Cluster",
		"IPFSUnpin",
		c.ToSerial(),
		&struct{}{},
	)
	if err != nil {
		mpt.setError(c.Cid, err)
		return err
	}

	mpt.set(c.Cid, api.TrackerStatusUnpinned)
	return nil
}

// Track tells the MapPinTracker to start managing a Cid,
// possibly triggering Pin operations on the IPFS daemon.
func (mpt *MapPinTracker) Track(c api.Pin) error {
	logger.Debugf("tracking %s", c.Cid)
	if mpt.isRemote(c) {
		if mpt.get(c.Cid).Status == api.TrackerStatusPinned {
			mpt.unpin(c)
		}
		mpt.set(c.Cid, api.TrackerStatusRemote)
		return nil
	}

	mpt.set(c.Cid, api.TrackerStatusPinning)
	select {
	case mpt.pinCh <- c:
	default:
		err := errors.New("pin queue is full")
		mpt.setError(c.Cid, err)
		logger.Error(err.Error())
		return err
	}
	return nil
}

// Untrack tells the MapPinTracker to stop managing a Cid.
// If the Cid is pinned locally, it will be unpinned.
func (mpt *MapPinTracker) Untrack(c *cid.Cid) error {
	logger.Debugf("untracking %s", c)
	mpt.set(c, api.TrackerStatusUnpinning)
	select {
	case mpt.unpinCh <- api.PinCid(c):
	default:
		err := errors.New("unpin queue is full")
		mpt.setError(c, err)
		logger.Error(err.Error())
		return err
	}
	return nil
}

// Status returns information for a Cid tracked by this
// MapPinTracker.
func (mpt *MapPinTracker) Status(c *cid.Cid) api.PinInfo {
	return mpt.get(c)
}

// StatusAll returns information for all Cids tracked by this
// MapPinTracker.
func (mpt *MapPinTracker) StatusAll() []api.PinInfo {
	mpt.mux.Lock()
	defer mpt.mux.Unlock()
	pins := make([]api.PinInfo, 0, len(mpt.status))
	for _, v := range mpt.status {
		pins = append(pins, v)
	}
	return pins
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
		mpt.setError(c, err)
		return mpt.get(c), err
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
	var pInfos []api.PinInfo
	err := mpt.rpcClient.Call(
		"",
		"Cluster",
		"IPFSPinLs",
		"recursive",
		&ipsMap,
	)
	if err != nil {
		mpt.mux.Lock()
		for k := range mpt.status {
			c, _ := cid.Decode(k)
			mpt.unsafeSetError(c, err)
			pInfos = append(pInfos, mpt.unsafeGet(c))
		}
		mpt.mux.Unlock()
		return pInfos, err
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
			pInfos = append(pInfos, pInfoNew)
		}
	}
	return pInfos, nil
}

func (mpt *MapPinTracker) syncStatus(c *cid.Cid, ips api.IPFSPinStatus) api.PinInfo {
	p := mpt.get(c)
	if ips.IsPinned() {
		switch p.Status {
		case api.TrackerStatusPinned: // nothing
		case api.TrackerStatusPinning, api.TrackerStatusPinError:
			mpt.set(c, api.TrackerStatusPinned)
		case api.TrackerStatusUnpinning:
			if time.Since(p.TS) > mpt.config.UnpinningTimeout {
				mpt.setError(c, errUnpinningTimeout)
			}
		case api.TrackerStatusUnpinned:
			mpt.setError(c, errPinned)
		case api.TrackerStatusUnpinError: // nothing, keep error as it was
		default: //remote
		}
	} else {
		switch p.Status {
		case api.TrackerStatusPinned:
			mpt.setError(c, errUnpinned)
		case api.TrackerStatusPinError: // nothing, keep error as it was
		case api.TrackerStatusPinning:
			if time.Since(p.TS) > mpt.config.PinningTimeout {
				mpt.setError(c, errPinningTimeout)
			}
		case api.TrackerStatusUnpinning, api.TrackerStatusUnpinError:
			mpt.set(c, api.TrackerStatusUnpinned)
		case api.TrackerStatusUnpinned: // nothing
		default: // remote
		}
	}
	return mpt.get(c)
}

// Recover will re-track or re-untrack a Cid in error state,
// possibly retriggering an IPFS pinning operation and returning
// only when it is done. The pinning/unpinning operation happens
// synchronously, jumping the queues.
func (mpt *MapPinTracker) Recover(c *cid.Cid) (api.PinInfo, error) {
	p := mpt.get(c)
	logger.Infof("Attempting to recover %s", c)
	var err error
	switch p.Status {
	case api.TrackerStatusPinError:
		// FIXME: This always recovers recursive == true
		// but sharding will bring direct-pin objects
		err = mpt.pin(api.PinCid(c))
	case api.TrackerStatusUnpinError:
		err = mpt.unpin(api.PinCid(c))
	default:
		logger.Warningf("%s does not need recovery. Try syncing first", c)
		return p, nil
	}
	if err != nil {
		logger.Errorf("error recovering %s: %s", c, err)
	}
	return mpt.get(c), err
}

// RecoverAll attempts to recover all items tracked by this peer.
func (mpt *MapPinTracker) RecoverAll() ([]api.PinInfo, error) {
	statuses := mpt.StatusAll()
	resp := make([]api.PinInfo, 0)
	for _, st := range statuses {
		r, err := mpt.Recover(st.Cid)
		if err != nil {
			return resp, err
		}
		resp = append(resp, r)
	}
	return resp, nil
}

// SetClient makes the MapPinTracker ready to perform RPC requests to
// other components.
func (mpt *MapPinTracker) SetClient(c *rpc.Client) {
	mpt.rpcClient = c
	mpt.rpcReady <- struct{}{}
}
