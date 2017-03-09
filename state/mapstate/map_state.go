package mapstate

import (
	"sync"

	"github.com/ipfs/ipfs-cluster/api"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

// Version is the map state Version. States with old versions should
// perform an upgrade before.
const Version = 1

// MapState is a very simple database to store the state of the system
// using a Go map. It is thread safe. It implements the State interface.
type MapState struct {
	pinMux  sync.RWMutex
	PinMap  map[string]api.PinSerial
	Version int
}

// NewMapState initializes the internal map and returns a new MapState object.
func NewMapState() *MapState {
	return &MapState{
		PinMap: make(map[string]api.PinSerial),
	}
}

// Add adds a Pin to the internal map.
func (st *MapState) Add(c api.Pin) error {
	st.pinMux.Lock()
	defer st.pinMux.Unlock()
	st.PinMap[c.Cid.String()] = c.ToSerial()
	return nil
}

// Rm removes a Cid from the internal map.
func (st *MapState) Rm(c *cid.Cid) error {
	st.pinMux.Lock()
	defer st.pinMux.Unlock()
	delete(st.PinMap, c.String())
	return nil
}

// Get returns Pin information for a CID.
func (st *MapState) Get(c *cid.Cid) api.Pin {
	st.pinMux.RLock()
	defer st.pinMux.RUnlock()
	pins, ok := st.PinMap[c.String()]
	if !ok { // make sure no panics
		return api.Pin{}
	}
	return pins.ToPin()
}

// Has returns true if the Cid belongs to the State.
func (st *MapState) Has(c *cid.Cid) bool {
	st.pinMux.RLock()
	defer st.pinMux.RUnlock()
	_, ok := st.PinMap[c.String()]
	return ok
}

// List provides the list of tracked Pins.
func (st *MapState) List() []api.Pin {
	st.pinMux.RLock()
	defer st.pinMux.RUnlock()
	cids := make([]api.Pin, 0, len(st.PinMap))
	for _, v := range st.PinMap {
		cids = append(cids, v.ToPin())
	}
	return cids
}
