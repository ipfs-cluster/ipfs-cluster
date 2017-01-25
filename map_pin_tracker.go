package ipfscluster

import (
	"context"
	"errors"
	"sync"
	"time"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
)

// A Pin or Unpin operation will be considered failed
// if the Cid has stayed in Pinning or Unpinning state
// for longer than these values.
var (
	PinningTimeout   = 15 * time.Minute
	UnpinningTimeout = 10 * time.Second
)

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
	status map[string]PinInfo

	ctx       context.Context
	rpcClient *rpc.Client
	rpcReady  chan struct{}
	peerID    peer.ID

	shutdownLock sync.Mutex
	shutdown     bool
	shutdownCh   chan struct{}
	wg           sync.WaitGroup
}

// NewMapPinTracker returns a new object which has been correcly
// initialized with the given configuration.
func NewMapPinTracker(cfg *Config) *MapPinTracker {
	ctx := context.Background()

	mpt := &MapPinTracker{
		ctx:        ctx,
		status:     make(map[string]PinInfo),
		rpcReady:   make(chan struct{}, 1),
		peerID:     cfg.ID,
		shutdownCh: make(chan struct{}),
	}
	mpt.run()
	return mpt
}

// run does nothing other than give MapPinTracker a cancellable context.
func (mpt *MapPinTracker) run() {
	mpt.wg.Add(1)
	go func() {
		defer mpt.wg.Done()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		mpt.ctx = ctx
		<-mpt.rpcReady
		logger.Info("PinTracker ready")
		<-mpt.shutdownCh
	}()
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
	mpt.shutdownCh <- struct{}{}
	mpt.wg.Wait()
	mpt.shutdown = true
	return nil
}

func (mpt *MapPinTracker) set(c *cid.Cid, s TrackerStatus) {
	mpt.mux.Lock()
	defer mpt.mux.Unlock()
	mpt.unsafeSet(c, s)
}

func (mpt *MapPinTracker) unsafeSet(c *cid.Cid, s TrackerStatus) {
	if s == TrackerStatusUnpinned {
		delete(mpt.status, c.String())
		return
	}

	mpt.status[c.String()] = PinInfo{
		//		cid:    c,
		CidStr: c.String(),
		Peer:   mpt.peerID,
		Status: s,
		TS:     time.Now(),
		Error:  "",
	}
}

func (mpt *MapPinTracker) get(c *cid.Cid) PinInfo {
	mpt.mux.RLock()
	defer mpt.mux.RUnlock()
	return mpt.unsafeGet(c)
}

func (mpt *MapPinTracker) unsafeGet(c *cid.Cid) PinInfo {
	p, ok := mpt.status[c.String()]
	if !ok {
		return PinInfo{
			CidStr: c.String(),
			Peer:   mpt.peerID,
			Status: TrackerStatusUnpinned,
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
	case TrackerStatusPinned, TrackerStatusPinning, TrackerStatusPinError:
		mpt.status[c.String()] = PinInfo{
			CidStr: c.String(),
			Peer:   mpt.peerID,
			Status: TrackerStatusPinError,
			TS:     time.Now(),
			Error:  err.Error(),
		}
	case TrackerStatusUnpinned, TrackerStatusUnpinning, TrackerStatusUnpinError:
		mpt.status[c.String()] = PinInfo{
			CidStr: c.String(),
			Peer:   mpt.peerID,
			Status: TrackerStatusUnpinError,
			TS:     time.Now(),
			Error:  err.Error(),
		}
	}
}

func (mpt *MapPinTracker) pin(c *cid.Cid) error {
	mpt.set(c, TrackerStatusPinning)
	err := mpt.rpcClient.Call("",
		"Cluster",
		"IPFSPin",
		NewCidArg(c),
		&struct{}{})

	if err != nil {
		mpt.setError(c, err)
		return err
	}
	mpt.set(c, TrackerStatusPinned)
	return nil
}

func (mpt *MapPinTracker) unpin(c *cid.Cid) error {
	mpt.set(c, TrackerStatusUnpinning)
	err := mpt.rpcClient.Call("",
		"Cluster",
		"IPFSUnpin",
		NewCidArg(c),
		&struct{}{})
	if err != nil {
		mpt.setError(c, err)
		return err
	}
	mpt.set(c, TrackerStatusUnpinned)
	return nil
}

// Track tells the MapPinTracker to start managing a Cid,
// possibly trigerring Pin operations on the IPFS daemon.
func (mpt *MapPinTracker) Track(c *cid.Cid) error {
	return mpt.pin(c)
}

// Untrack tells the MapPinTracker to stop managing a Cid.
// If the Cid is pinned locally, it will be unpinned.
func (mpt *MapPinTracker) Untrack(c *cid.Cid) error {
	return mpt.unpin(c)
}

// StatusCid returns information for a Cid tracked by this
// MapPinTracker.
func (mpt *MapPinTracker) StatusCid(c *cid.Cid) PinInfo {
	return mpt.get(c)
}

// Status returns information for all Cids tracked by this
// MapPinTracker.
func (mpt *MapPinTracker) Status() []PinInfo {
	mpt.mux.Lock()
	defer mpt.mux.Unlock()
	pins := make([]PinInfo, 0, len(mpt.status))
	for _, v := range mpt.status {
		pins = append(pins, v)
	}
	return pins
}

// SyncCid verifies that the status of a Cid matches that of
// the IPFS daemon. If not, it will be transitioned
// to PinError or UnpinError.
//
// SyncCid returns the updated local status for the given Cid.
// Pins in error states can be recovered with Recover().
// An error is returned if we are unable to contact
// the IPFS daemon.
func (mpt *MapPinTracker) SyncCid(c *cid.Cid) (PinInfo, error) {
	var ips IPFSPinStatus
	err := mpt.rpcClient.Call("",
		"Cluster",
		"IPFSPinLsCid",
		NewCidArg(c),
		&ips)
	if err != nil {
		mpt.setError(c, err)
		return mpt.get(c), err
	}
	return mpt.syncStatus(c, ips), nil
}

// Sync verifies that the statuses of all tracked Cids match the
// one reported by the IPFS daemon. If not, they will be transitioned
// to PinError or UnpinError.
//
// Sync returns the list of local status for all tracked Cids which
// were updated or have errors. Cids in error states can be recovered
// with Recover().
// An error is returned if we are unable to contact the IPFS daemon.
func (mpt *MapPinTracker) Sync() ([]PinInfo, error) {
	var ipsMap map[string]IPFSPinStatus
	var pInfos []PinInfo
	err := mpt.rpcClient.Call("",
		"Cluster",
		"IPFSPinLs",
		struct{}{},
		&ipsMap)
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

	status := mpt.Status()
	for _, pInfoOrig := range status {
		c, err := cid.Decode(pInfoOrig.CidStr)
		if err != nil { // this should not happen but let's play safe
			return pInfos, err
		}
		var pInfoNew PinInfo
		ips, ok := ipsMap[pInfoOrig.CidStr]
		if !ok {
			pInfoNew = mpt.syncStatus(c, IPFSPinStatusUnpinned)
		} else {
			pInfoNew = mpt.syncStatus(c, ips)
		}

		if pInfoOrig.Status != pInfoNew.Status ||
			pInfoNew.Status == TrackerStatusUnpinError ||
			pInfoNew.Status == TrackerStatusPinError {
			pInfos = append(pInfos, pInfoNew)
		}
	}
	return pInfos, nil
}

func (mpt *MapPinTracker) syncStatus(c *cid.Cid, ips IPFSPinStatus) PinInfo {
	p := mpt.get(c)
	if ips.IsPinned() {
		switch p.Status {
		case TrackerStatusPinned: // nothing
		case TrackerStatusPinning, TrackerStatusPinError:
			mpt.set(c, TrackerStatusPinned)
		case TrackerStatusUnpinning:
			if time.Since(p.TS) > UnpinningTimeout {
				mpt.setError(c, errUnpinningTimeout)
			}
		case TrackerStatusUnpinned:
			mpt.setError(c, errPinned)
		case TrackerStatusUnpinError: // nothing, keep error as it was
		default:
		}
	} else {
		switch p.Status {
		case TrackerStatusPinned:

			mpt.setError(c, errUnpinned)
		case TrackerStatusPinError: // nothing, keep error as it was
		case TrackerStatusPinning:
			if time.Since(p.TS) > PinningTimeout {
				mpt.setError(c, errPinningTimeout)
			}
		case TrackerStatusUnpinning, TrackerStatusUnpinError:
			mpt.set(c, TrackerStatusUnpinned)
		case TrackerStatusUnpinned: // nothing
		default:
		}
	}
	return mpt.get(c)
}

// Recover will re-track or re-untrack a Cid in error state,
// possibly retriggering an IPFS pinning operation and returning
// only when it is done.
func (mpt *MapPinTracker) Recover(c *cid.Cid) error {
	p := mpt.get(c)
	if p.Status != TrackerStatusPinError &&
		p.Status != TrackerStatusUnpinError {
		return nil
	}
	logger.Infof("Recovering %s", c)
	var err error
	switch p.Status {
	case TrackerStatusPinError:
		err = mpt.Track(c)
	case TrackerStatusUnpinError:
		err = mpt.Untrack(c)
	}
	if err != nil {
		logger.Errorf("error recovering %s: %s", c, err)
		return err
	}
	return nil
}

// SetClient makes the MapPinTracker ready to perform RPC requests to
// other components.
func (mpt *MapPinTracker) SetClient(c *rpc.Client) {
	mpt.rpcClient = c
	mpt.rpcReady <- struct{}{}
}
