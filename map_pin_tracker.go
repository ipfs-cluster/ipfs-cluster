package ipfscluster

import (
	"context"
	"sync"
	"time"

	peer "github.com/libp2p/go-libp2p-peer"

	cid "github.com/ipfs/go-cid"
)

// A Pin or Unpin operation will be considered failed
// if the Cid has stayed in Pinning or Unpinning state
// for longer than these values.
var (
	PinningTimeout   = 15 * time.Minute
	UnpinningTimeout = 10 * time.Second
)

const (
	PinError = iota
	UnpinError
	Pinned
	Pinning
	Unpinning
	Unpinned
	RemotePin
)

type GlobalPinInfo struct {
	Cid    *cid.Cid
	Status map[peer.ID]PinInfo
}

// PinInfo holds information about local pins. PinInfo is
// serialized when requesting the Global status, therefore
// we cannot use *cid.Cid.
type PinInfo struct {
	CidStr string
	Peer   peer.ID
	IPFS   IPFSStatus
	TS     time.Time
}

type IPFSStatus int

func (st IPFSStatus) String() string {
	switch st {
	case PinError:
		return "pin_error"
	case UnpinError:
		return "unpin_error"
	case Pinned:
		return "pinned"
	case Pinning:
		return "pinning"
	case Unpinning:
		return "unpinning"
	case Unpinned:
		return "unpinned"
	}
	return ""
}

type MapPinTracker struct {
	mux    sync.RWMutex
	status map[string]PinInfo

	ctx    context.Context
	rpcCh  chan RPC
	peerID peer.ID

	shutdownLock sync.Mutex
	shutdown     bool
	shutdownCh   chan struct{}
	wg           sync.WaitGroup
}

func NewMapPinTracker(cfg *Config) *MapPinTracker {
	ctx := context.Background()

	pID, err := peer.IDB58Decode(cfg.ID)
	if err != nil {
		panic(err)
	}

	mpt := &MapPinTracker{
		ctx:        ctx,
		status:     make(map[string]PinInfo),
		rpcCh:      make(chan RPC, RPCMaxQueue),
		peerID:     pID,
		shutdownCh: make(chan struct{}),
	}
	logger.Info("starting MapPinTracker")
	mpt.run()
	return mpt
}

func (mpt *MapPinTracker) run() {
	mpt.wg.Add(1)
	go func() {
		defer mpt.wg.Done()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		mpt.ctx = ctx
		<-mpt.shutdownCh
	}()
}

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
	ctx, cancel := context.WithCancel(mpt.ctx)
	defer cancel()

	mpt.set(c, Pinning)
	resp := MakeRPC(ctx, mpt.rpcCh, NewRPC(IPFSPinRPC, c), true)
	if resp.Error != nil {
		mpt.set(c, PinError)
		return resp.Error
	}
	mpt.set(c, Pinned)
	return nil
}

func (mpt *MapPinTracker) unpin(c *cid.Cid) error {
	ctx, cancel := context.WithCancel(mpt.ctx)
	defer cancel()

	mpt.set(c, Unpinning)
	resp := MakeRPC(ctx, mpt.rpcCh, NewRPC(IPFSUnpinRPC, c), true)
	if resp.Error != nil {
		mpt.set(c, UnpinError)
		return resp.Error
	}
	mpt.set(c, Unpinned)
	return nil
}

func (mpt *MapPinTracker) Track(c *cid.Cid) error {
	return mpt.pin(c)
}

func (mpt *MapPinTracker) Untrack(c *cid.Cid) error {
	return mpt.unpin(c)
}

func (mpt *MapPinTracker) StatusCid(c *cid.Cid) PinInfo {
	return mpt.get(c)
}

func (mpt *MapPinTracker) Status() []PinInfo {
	mpt.mux.Lock()
	defer mpt.mux.Unlock()
	pins := make([]PinInfo, 0, len(mpt.status))
	for _, v := range mpt.status {
		pins = append(pins, v)
	}
	return pins
}

func (mpt *MapPinTracker) Sync(c *cid.Cid) bool {
	ctx, cancel := context.WithCancel(mpt.ctx)
	defer cancel()

	p := mpt.get(c)
	resp := MakeRPC(ctx, mpt.rpcCh, NewRPC(IPFSIsPinnedRPC, c), true)
	if resp.Error != nil {
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

	ipfsPinned, ok := resp.Data.(bool)
	if !ok {
		logger.Error("wrong type of IPFSIsPinnedRPC response")
		return false
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

func (mpt *MapPinTracker) RpcChan() <-chan RPC {
	return mpt.rpcCh
}
