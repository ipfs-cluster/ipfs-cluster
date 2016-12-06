package ipfscluster

import (
	"sync"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

// MapState is a very simple database to store
// the state of the system.
// PinMap is public because it is serialized
// and maintained by Raft.
type MapState struct {
	PinMap map[string]bool
	rpcCh  chan ClusterRPC
	mux    sync.Mutex
}

func NewMapState() MapState {
	return MapState{
		PinMap: make(map[string]bool),
		rpcCh:  make(chan ClusterRPC),
	}
}

func (st MapState) AddPin(c *cid.Cid) error {
	st.mux.Lock()
	defer st.mux.Unlock()
	st.PinMap[c.String()] = true
	return nil
}

func (st MapState) RmPin(c *cid.Cid) error {
	st.mux.Lock()
	defer st.mux.Unlock()
	delete(st.PinMap, c.String())
	return nil
}
