// Package mapstate implements the State interface for IPFS Cluster by using
// a map to keep track of the consensus-shared state.
package mapstate

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"sync"

	"github.com/ipfs/ipfs-cluster/api"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

// Version is the map state Version. States with old versions should
// perform an upgrade before.
const Version = 2

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
		PinMap:  make(map[string]api.PinSerial),
		Version: Version,
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
		if v.Cid == "" {
			continue
		}
		cids = append(cids, v.ToPin())
	}
	return cids
}

// Snapshot dumps the MapState to the given writer, in pretty json
// format.
func (st *MapState) Snapshot(w io.Writer) error {
	st.pinMux.RLock()
	defer st.pinMux.RUnlock()
	enc := json.NewEncoder(w)
	enc.SetIndent("", "    ")
	return enc.Encode(st)
}

// Restore takes a reader and restores a snapshot. It should migrate
// the format if it is not compatible with the current version.
func (st *MapState) Restore(r io.Reader) error {
	snap, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	var vonly struct{ Version int }
	err = json.Unmarshal(snap, &vonly)
	if err != nil {
		return err
	}
	if vonly.Version == Version {
		// we are good
		err := json.Unmarshal(snap, st)
		return err
	}
	return st.migrateFrom(vonly.Version, snap)
}
