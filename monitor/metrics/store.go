package metrics

import (
	"sync"

	"github.com/elastos/Elastos.NET.Hive.Cluster/api"

	peer "github.com/libp2p/go-libp2p-peer"
)

// PeerMetrics maps a peer IDs to a metrics window.
type PeerMetrics map[peer.ID]*Window

// Store can be used to store and access metrics.
type Store struct {
	mux    sync.RWMutex
	byName map[string]PeerMetrics
}

// NewStore can be used to create a Store.
func NewStore() *Store {
	return &Store{
		byName: make(map[string]PeerMetrics),
	}
}

// Add inserts a new metric in Metrics.
func (mtrs *Store) Add(m api.Metric) {
	mtrs.mux.Lock()
	defer mtrs.mux.Unlock()

	name := m.Name
	peer := m.Peer
	mbyp, ok := mtrs.byName[name]
	if !ok {
		mbyp = make(PeerMetrics)
		mtrs.byName[name] = mbyp
	}
	window, ok := mbyp[peer]
	if !ok {
		// We always lock the outer map, so we can use unsafe
		// Window.
		window = NewWindow(DefaultWindowCap)
		mbyp[peer] = window
	}

	window.Add(m)
}

// Latest returns all the last known valid metrics. A metric is valid
// if it has not expired.
func (mtrs *Store) Latest(name string) []api.Metric {
	mtrs.mux.RLock()
	defer mtrs.mux.RUnlock()

	byPeer, ok := mtrs.byName[name]
	if !ok {
		return []api.Metric{}
	}

	metrics := make([]api.Metric, 0, len(byPeer))
	for _, window := range byPeer {
		m, err := window.Latest()
		if err != nil || m.Discard() {
			continue
		}
		metrics = append(metrics, m)
	}
	return metrics
}

// PeerMetrics returns the latest metrics for a given peer ID for
// all known metrics types. It may return expired metrics.
func (mtrs *Store) PeerMetrics(pid peer.ID) []api.Metric {
	mtrs.mux.RLock()
	defer mtrs.mux.RUnlock()

	result := make([]api.Metric, 0)

	for _, byPeer := range mtrs.byName {
		window, ok := byPeer[pid]
		if !ok {
			continue
		}
		metric, err := window.Latest()
		if err != nil {
			continue
		}
		result = append(result, metric)
	}
	return result
}
