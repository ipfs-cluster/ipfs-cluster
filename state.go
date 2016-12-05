package ipfscluster

import (
	"sync"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

const (
	pinEverywhere = -1
)

const (
	Error = iota
	Pinned
	Pinning
	Unpinning
)

type Pin struct {
	Cid     string    `json:"cid"`
	PinMode pinMode   `json:ignore`
	Status  pinStatus `json:"status"`
}

type pinMode int
type pinStatus int

// MapState is a very simple pin map representation
// PinMap must be public as it will be serialized
type MapState struct {
	PinMap map[string]Pin
	rpcCh  chan ClusterRPC
	mux    sync.Mutex
	tag    string
}

func NewMapState(tag string) MapState {
	return MapState{
		PinMap: make(map[string]Pin),
		rpcCh:  make(chan ClusterRPC),
		tag:    tag,
	}
}

func (st MapState) Pinning(c *cid.Cid) error {
	st.mux.Lock()
	defer st.mux.Unlock()
	st.PinMap[c.String()] = Pin{
		Cid:     c.String(),
		PinMode: pinEverywhere,
		Status:  Pinning,
	}
	return nil
}

func (st MapState) Unpinning(c *cid.Cid) error {
	st.mux.Lock()
	defer st.mux.Unlock()
	st.PinMap[c.String()] = Pin{
		Cid:     c.String(),
		PinMode: pinEverywhere,
		Status:  Unpinning,
	}
	return nil
}

func (st MapState) Pinned(c *cid.Cid) error {
	st.mux.Lock()
	defer st.mux.Unlock()
	st.PinMap[c.String()] = Pin{
		Cid:     c.String(),
		PinMode: pinEverywhere,
		Status:  Pinned,
	}
	return nil
}

func (st MapState) PinError(c *cid.Cid) error {
	st.mux.Lock()
	defer st.mux.Unlock()
	st.PinMap[c.String()] = Pin{
		Cid:     c.String(),
		PinMode: pinEverywhere,
		Status:  Error,
	}
	return nil
}

func (st MapState) RmPin(c *cid.Cid) error {
	st.mux.Lock()
	defer st.mux.Unlock()
	delete(st.PinMap, c.String())
	return nil
}

func (st MapState) Exists(c *cid.Cid) bool {
	st.mux.Lock()
	defer st.mux.Unlock()
	_, ok := st.PinMap[c.String()]
	return ok
}

func (st MapState) ListPins() []Pin {
	st.mux.Lock()
	defer st.mux.Unlock()
	pins := make([]Pin, 0, len(st.PinMap))
	for _, v := range st.PinMap {
		pins = append(pins, v)
	}
	return pins
}

func (st MapState) ShouldPin(c *cid.Cid) bool {
	return true
}

func (st MapState) Shutdown() error {
	return nil
}

func (st MapState) RpcChan() <-chan ClusterRPC {
	return st.rpcCh
}
