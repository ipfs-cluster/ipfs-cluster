package ipfscluster

import (
	"context"
	"errors"
	"fmt"
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
	mux    sync.Mutex
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

func (mpt *MapPinTracker) get(c *cid.Cid) PinInfo {
	mpt.mux.Lock()
	defer mpt.mux.Unlock()
	p, ok := mpt.status[c.String()]
	if !ok {
		return PinInfo{
			//			cid:    c,
			CidStr: c.String(),
			Peer:   mpt.peerID,
			IPFS:   Unpinned,
			TS:     time.Now(),
		}
	}
	return p
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
		mpt.set(c, PinError)
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

func (mpt *MapPinTracker) LocalStatusCid(c *cid.Cid) PinInfo {
	return mpt.get(c)
}

func (mpt *MapPinTracker) LocalStatus() []PinInfo {
	mpt.mux.Lock()
	defer mpt.mux.Unlock()
	pins := make([]PinInfo, 0, len(mpt.status))
	for _, v := range mpt.status {
		pins = append(pins, v)
	}
	return pins
}

func (mpt *MapPinTracker) GlobalStatus() ([]GlobalPinInfo, error) {
	ctx, cancel := context.WithCancel(mpt.ctx)
	defer cancel()

	rpc := NewRPC(TrackerLocalStatusRPC, nil)
	wrpc := NewRPC(BroadcastRPC, rpc)
	resp := MakeRPC(ctx, mpt.rpcCh, wrpc, true)
	if resp.Error != nil {
		return nil, resp.Error
	}

	responses, ok := resp.Data.([]RPCResponse)
	if !ok {
		return nil, errors.New("unexpected responses format")
	}
	fullMap := make(map[string]GlobalPinInfo)

	mergePins := func(pins []PinInfo) {
		for _, p := range pins {
			item, ok := fullMap[p.CidStr]
			c, _ := cid.Decode(p.CidStr)
			if !ok {
				fullMap[p.CidStr] = GlobalPinInfo{
					Cid: c,
					Status: map[peer.ID]PinInfo{
						p.Peer: p,
					},
				}
			} else {
				item.Status[p.Peer] = p
			}
		}
	}
	for _, r := range responses {
		if r.Error != nil {
			logger.Error("error in one of the broadcast responses: ", r.Error)
			continue
		}
		pins, ok := r.Data.([]PinInfo)
		if !ok {
			return nil, fmt.Errorf("unexpected response format: %+v", r.Data)
		}
		mergePins(pins)
	}

	status := make([]GlobalPinInfo, 0, len(fullMap))
	for _, v := range fullMap {
		status = append(status, v)
	}
	return status, nil
}

func (mpt *MapPinTracker) GlobalStatusCid(c *cid.Cid) (GlobalPinInfo, error) {
	ctx, cancel := context.WithCancel(mpt.ctx)
	defer cancel()

	pin := GlobalPinInfo{
		Cid:    c,
		Status: make(map[peer.ID]PinInfo),
	}

	rpc := NewRPC(TrackerLocalStatusCidRPC, c)
	wrpc := NewRPC(BroadcastRPC, rpc)
	resp := MakeRPC(ctx, mpt.rpcCh, wrpc, true)
	if resp.Error != nil {
		return pin, resp.Error
	}

	responses, ok := resp.Data.([]RPCResponse)
	if !ok {
		return pin, errors.New("unexpected responses format")
	}

	for _, r := range responses {
		if r.Error != nil {
			return pin, r.Error
		}
		info, ok := r.Data.(PinInfo)
		if !ok {
			return pin, errors.New("unexpected response format")
		}
		pin.Status[info.Peer] = info
	}

	return pin, nil
}

func (mpt *MapPinTracker) Sync(c *cid.Cid) bool {
	ctx, cancel := context.WithCancel(mpt.ctx)
	defer cancel()

	p := mpt.get(c)

	// We assume errors will need a Recover() so we return true
	if p.IPFS == PinError || p.IPFS == UnpinError {
		return true
	}

	resp := MakeRPC(ctx, mpt.rpcCh, NewRPC(IPFSIsPinnedRPC, c), true)
	if resp.Error != nil {
		if p.IPFS == Pinned || p.IPFS == Pinning {
			mpt.set(c, PinError)
			return true
		}
		if p.IPFS == Unpinned || p.IPFS == Unpinning {
			mpt.set(c, UnpinError)
			return true
		}
		return false
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
		case Pinning:
			mpt.set(c, Pinned)
			return true
		case Unpinning:
			if time.Since(p.TS) > UnpinningTimeout {
				mpt.set(c, UnpinError)
				return true
			}
			return false
		case Unpinned:
			mpt.set(c, UnpinError)
			return true
		}
	} else {
		switch p.IPFS {
		case Pinned:
			mpt.set(c, PinError)
			return true
		case Pinning:
			if time.Since(p.TS) > PinningTimeout {
				mpt.set(c, PinError)
				return true
			}
			return false
		case Unpinning:
			mpt.set(c, Unpinned)
			return true
		case Unpinned:
			return false
		}
	}
	return false
}

func (mpt *MapPinTracker) Recover(c *cid.Cid) error {
	p := mpt.get(c)
	if p.IPFS != PinError && p.IPFS != UnpinError {
		return nil
	}

	if p.IPFS == PinError {
		mpt.pin(c)
	}
	if p.IPFS == UnpinError {
		mpt.unpin(c)
	}
	return nil
}

func (mpt *MapPinTracker) SyncAll() []PinInfo {
	var changedPins []PinInfo
	pins := mpt.LocalStatus()
	for _, p := range pins {
		c, _ := cid.Decode(p.CidStr)
		changed := mpt.Sync(c)
		if changed {
			changedPins = append(changedPins, p)
		}
	}
	return changedPins
}

func (mpt *MapPinTracker) SyncState(cState State) []*cid.Cid {
	clusterPins := cState.ListPins()
	clusterMap := make(map[string]struct{})
	// Make a map for faster lookup
	for _, c := range clusterPins {
		var a struct{}
		clusterMap[c.String()] = a
	}
	var toRemove []*cid.Cid
	var toAdd []*cid.Cid
	var changed []*cid.Cid
	mpt.mux.Lock()

	// Collect items in the State not in the tracker
	for _, c := range clusterPins {
		_, ok := mpt.status[c.String()]
		if !ok {
			toAdd = append(toAdd, c)
		}
	}

	// Collect items in the tracker not in the State
	for _, p := range mpt.status {
		c, _ := cid.Decode(p.CidStr)
		_, ok := clusterMap[p.CidStr]
		if !ok {
			toRemove = append(toRemove, c)
		}
	}

	// Update new items and mark them as pinning error
	for _, c := range toAdd {
		mpt.unsafeSet(c, PinError)
		changed = append(changed, c)
	}

	// Mark items that need to be removed as unpin error
	for _, c := range toRemove {
		mpt.unsafeSet(c, UnpinError)
		changed = append(changed, c)
	}
	mpt.mux.Unlock()
	return changed
}

func (mpt *MapPinTracker) RpcChan() <-chan RPC {
	return mpt.rpcCh
}
