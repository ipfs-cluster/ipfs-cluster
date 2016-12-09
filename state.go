package ipfscluster

import (
	"sync"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

// MapState is a very simple database to store
// the state of the system.
type MapState struct {
	PinMap map[string]struct{}
	rpcCh  chan ClusterRPC
	mux    sync.Mutex
}

func NewMapState() *MapState {
	return &MapState{
		PinMap: make(map[string]struct{}),
		rpcCh:  make(chan ClusterRPC),
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

func (st *MapState) ListPins() []*cid.Cid {
	st.mux.Lock()
	defer st.mux.Unlock()
	cids := make([]*cid.Cid, 0, len(st.PinMap))
	for k, _ := range st.PinMap {
		c, _ := cid.Decode(k)
		cids = append(cids, c)
	}
	return cids
}
