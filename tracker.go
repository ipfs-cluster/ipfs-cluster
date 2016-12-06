package ipfscluster

import (
	"context"
	"sync"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

const (
	pinEverywhere = -1
)

const (
	PinError = iota
	UnPinError
	Pinned
	Pinning
	Unpinning
	Unpinned
	Pending
)

type Pin struct {
	Cid     string    `json:"cid"`
	PinMode PinMode   `json:ignore`
	Status  PinStatus `json:"status"`
}

type PinMode int
type PinStatus int

type MapPinTracker struct {
	status map[string]Pin
	rpcCh  chan ClusterRPC

	mux    sync.Mutex
	doneCh chan bool

	ctx    context.Context
	cancel context.CancelFunc
}

func NewMapPinTracker() *MapPinTracker {
	ctx, cancel := context.WithCancel(context.Background())
	mpt := &MapPinTracker{
		status: make(map[string]Pin),
		rpcCh:  make(chan ClusterRPC),
		doneCh: make(chan bool),
		ctx:    ctx,
		cancel: cancel,
	}
	go mpt.run()
	return mpt
}

func (mpt *MapPinTracker) run() {
	// Great plans for this thread
	select {
	case <-mpt.ctx.Done():
		close(mpt.doneCh)
	}
}

func (mpt *MapPinTracker) set(c *cid.Cid, s PinStatus) error {
	mpt.mux.Lock()
	defer mpt.mux.Unlock()
	mpt.status[c.String()] = Pin{
		Cid:     c.String(),
		PinMode: pinEverywhere,
		Status:  s,
	}
	return nil
}

func (mpt *MapPinTracker) Pinning(c *cid.Cid) error {
	return mpt.set(c, Pinning)
}

func (mpt *MapPinTracker) Unpinning(c *cid.Cid) error {
	return mpt.set(c, Unpinning)
}

func (mpt *MapPinTracker) Pinned(c *cid.Cid) error {
	return mpt.set(c, Pinned)
}

func (mpt *MapPinTracker) PinError(c *cid.Cid) error {
	return mpt.set(c, PinError)
}

func (mpt *MapPinTracker) UnpinError(c *cid.Cid) error {
	return mpt.set(c, UnpinError)
}

func (mpt *MapPinTracker) Unpinned(c *cid.Cid) error {
	mpt.mux.Lock()
	defer mpt.mux.Unlock()
	delete(mpt.status, c.String())
	return nil
}

func (mpt *MapPinTracker) GetPin(c *cid.Cid) (Pin, bool) {
	mpt.mux.Lock()
	defer mpt.mux.Unlock()
	p, ok := mpt.status[c.String()]
	return p, ok
}

func (mpt *MapPinTracker) ListPins() []Pin {
	mpt.mux.Lock()
	defer mpt.mux.Unlock()
	pins := make([]Pin, 0, len(mpt.status))
	for _, v := range mpt.status {
		pins = append(pins, v)
	}
	return pins
}

func (mpt *MapPinTracker) Shutdown() error {
	mpt.cancel()
	<-mpt.doneCh
	return nil
}

func (mpt *MapPinTracker) RpcChan() <-chan ClusterRPC {
	return mpt.rpcCh
}
