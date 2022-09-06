package metrics

import (
	"sort"
	"sync"

	"github.com/ipfs-cluster/ipfs-cluster/api"

	peer "github.com/libp2p/go-libp2p/core/peer"
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

// RemovePeer removes all metrics related to a peer from the Store.
func (mtrs *Store) RemovePeer(pid peer.ID) {
	mtrs.mux.Lock()
	for _, metrics := range mtrs.byName {
		delete(metrics, pid)
	}
	mtrs.mux.Unlock()
}

// RemovePeerMetrics removes all metrics of a given name for a given peer ID.
func (mtrs *Store) RemovePeerMetrics(pid peer.ID, name string) {
	mtrs.mux.Lock()
	metrics := mtrs.byName[name]
	delete(metrics, pid)
	mtrs.mux.Unlock()
}

// LatestValid returns all the last known valid metrics of a given type. A metric
// is valid if it has not expired.
func (mtrs *Store) LatestValid(name string) []api.Metric {
	mtrs.mux.RLock()
	defer mtrs.mux.RUnlock()

	byPeer, ok := mtrs.byName[name]
	if !ok {
		return []api.Metric{}
	}

	metrics := make([]api.Metric, 0, len(byPeer))
	for _, window := range byPeer {
		m, err := window.Latest()
		// TODO(ajl): for accrual, does it matter if a ping has expired?
		if err != nil || m.Discard() {
			continue
		}
		metrics = append(metrics, m)
	}

	sortedMetrics := api.MetricSlice(metrics)
	sort.Stable(sortedMetrics)
	return sortedMetrics
}

// AllMetrics returns the latest metrics for all peers and metrics types.  It
// may return expired metrics.
func (mtrs *Store) AllMetrics() []api.Metric {
	mtrs.mux.RLock()
	defer mtrs.mux.RUnlock()

	result := make([]api.Metric, 0)

	for _, byPeer := range mtrs.byName {
		for _, window := range byPeer {
			metric, err := window.Latest()
			if err != nil || !metric.Valid {
				continue
			}
			result = append(result, metric)
		}
	}
	return result
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
		if err != nil || !metric.Valid {
			continue
		}
		result = append(result, metric)
	}
	return result
}

// PeerMetricAll returns all of a particular metrics for a
// particular peer.
func (mtrs *Store) PeerMetricAll(name string, pid peer.ID) []api.Metric {
	mtrs.mux.RLock()
	defer mtrs.mux.RUnlock()

	byPeer, ok := mtrs.byName[name]
	if !ok {
		return nil
	}

	window, ok := byPeer[pid]
	if !ok {
		return nil
	}
	ms := window.All()
	return ms
}

// PeerLatest returns the latest of a particular metric for a
// particular peer. It may return an expired metric.
func (mtrs *Store) PeerLatest(name string, pid peer.ID) api.Metric {
	mtrs.mux.RLock()
	defer mtrs.mux.RUnlock()

	byPeer, ok := mtrs.byName[name]
	if !ok {
		return api.Metric{}
	}

	window, ok := byPeer[pid]
	if !ok {
		return api.Metric{}
	}
	m, err := window.Latest()
	if err != nil {
		// ignoring error, as nil metric is indicative enough
		return api.Metric{}
	}
	return m
}

// MetricNames returns all the known metric names
func (mtrs *Store) MetricNames() []string {
	mtrs.mux.RLock()
	defer mtrs.mux.RUnlock()

	list := make([]string, 0, len(mtrs.byName))
	for k := range mtrs.byName {
		list = append(list, k)
	}
	return list
}
