package ipfscluster

import (
	"sync"

	cid "github.com/ipfs/go-cid"
)

// MapState is a very simple database to store
// the state of the system.
type MapState struct {
	mux    sync.RWMutex
	PinMap map[string]struct{}

	rpcCh chan RPC
}

func NewMapState() *MapState {
	return &MapState{
		PinMap: make(map[string]struct{}),
		rpcCh:  make(chan RPC),
	}
}

func (st *MapState) AddPin(c *cid.Cid) error {
	st.mux.Lock()
	defer st.mux.Unlock()
	var a struct{}
	st.PinMap[c.String()] = a
	return nil
}

func (st *MapState) RmPin(c *cid.Cid) error {
	st.mux.Lock()
	defer st.mux.Unlock()
	delete(st.PinMap, c.String())
	return nil
}

func (st *MapState) HasPin(c *cid.Cid) bool {
	st.mux.RLock()
	defer st.mux.RUnlock()
	_, ok := st.PinMap[c.String()]
	return ok
}

func (st *MapState) ListPins() []*cid.Cid {
	st.mux.RLock()
	defer st.mux.RUnlock()
	cids := make([]*cid.Cid, 0, len(st.PinMap))
	for k, _ := range st.PinMap {
		c, _ := cid.Decode(k)
		cids = append(cids, c)
	}
	return cids
}
