package ipfscluster

import (
	"context"
	"sync"
	"time"

	rpc "github.com/hsanjuan/go-libp2p-rpc"
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
	logger.Info("starting MapPinTracker")
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
		//<-mpt.rpcReady
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

func (mpt *MapPinTracker) unsafeSet(c *cid.Cid, s IPFSStatus) {
	if s == Unpinned {
		delete(mpt.status, c.String())
		return
	}

	mpt.status[c.String()] = PinInfo{
		//		cid:    c,
		CidStr: c.String(),
		Peer:   mpt.peerID,
		IPFS:   s,
		TS:     time.Now(),
	}
}

func (mpt *MapPinTracker) set(c *cid.Cid, s IPFSStatus) {
	mpt.mux.Lock()
	defer mpt.mux.Unlock()
	mpt.unsafeSet(c, s)
}

func (mpt *MapPinTracker) unsafeGet(c *cid.Cid) PinInfo {
	p, ok := mpt.status[c.String()]
	if !ok {
		return PinInfo{
			CidStr: c.String(),
			Peer:   mpt.peerID,
			IPFS:   Unpinned,
			TS:     time.Now(),
		}
	}
	return p
}

func (mpt *MapPinTracker) get(c *cid.Cid) PinInfo {
	mpt.mux.RLock()
	defer mpt.mux.RUnlock()
	return mpt.unsafeGet(c)
}

func (mpt *MapPinTracker) pin(c *cid.Cid) error {
	mpt.set(c, Pinning)
	err := mpt.rpcClient.Call("",
		"Cluster",
		"IPFSPin",
		NewCidArg(c),
		&struct{}{})

	if err != nil {
		mpt.set(c, PinError)
		return err
	}
	mpt.set(c, Pinned)
	return nil
}

func (mpt *MapPinTracker) unpin(c *cid.Cid) error {
	mpt.set(c, Unpinning)
	err := mpt.rpcClient.Call("",
		"Cluster",
		"IPFSUnpin",
		NewCidArg(c),
		&struct{}{})
	if err != nil {
		mpt.set(c, UnpinError)
		return err
	}
	mpt.set(c, Unpinned)
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

// Sync verifies that the status of a Cid matches the status
// of it in the IPFS daemon. If not, it will be transitioned
// to Pin or Unpin error. Sync returns true if the status was
// modified or the status is error. Pins in error states can be
// recovered with Recover().
func (mpt *MapPinTracker) Sync(c *cid.Cid) bool {
	var ipfsPinned bool
	p := mpt.get(c)
	err := mpt.rpcClient.Call("",
		"Cluster",
		"IPFSIsPinned",
		NewCidArg(c),
		&ipfsPinned)

	if err != nil {
		switch p.IPFS {
		case Pinned, Pinning:
			mpt.set(c, PinError)
			return true
		case Unpinned, Unpinning:
			mpt.set(c, UnpinError)
			return true
		case PinError, UnpinError:
			return true
		default:
			return false
		}
	}

	if ipfsPinned {
		switch p.IPFS {
		case Pinned:
			return false
		case Pinning, PinError:
			mpt.set(c, Pinned)
			return true
		case Unpinning:
			if time.Since(p.TS) > UnpinningTimeout {
				mpt.set(c, UnpinError)
				return true
			}
			return false
		case Unpinned, UnpinError:
			mpt.set(c, UnpinError)
			return true
		default:
			return false
		}
	} else {
		switch p.IPFS {
		case Pinned, PinError:
			mpt.set(c, PinError)
			return true
		case Pinning:
			if time.Since(p.TS) > PinningTimeout {
				mpt.set(c, PinError)
				return true
			}
			return false
		case Unpinning, UnpinError:
			mpt.set(c, Unpinned)
			return true
		case Unpinned:
			return false
		default:
			return false
		}
	}
}

// Recover will re-track or re-untrack a Cid in error state,
// possibly retriggering an IPFS pinning operation and returning
// only when it is done.
func (mpt *MapPinTracker) Recover(c *cid.Cid) error {
	p := mpt.get(c)
	if p.IPFS != PinError && p.IPFS != UnpinError {
		return nil
	}
	logger.Infof("Recovering %s", c)
	var err error
	if p.IPFS == PinError {
		err = mpt.Track(c)
	}
	if p.IPFS == UnpinError {
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
