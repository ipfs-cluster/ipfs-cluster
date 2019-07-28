package ipfscluster

import (
	"time"

	peer "github.com/libp2p/go-libp2p-core/peer"
)

// TimeCache is an lru cache where entries are valid upto a specified
// duration.
type TimeCache struct {
	M    map[peer.ID]map[string]time.Time
	span time.Duration
}

// NewTimeCache returns a TimeCache give a time duration for which an
// entry should be valid.
func NewTimeCache(span time.Duration) *TimeCache {
	if span == 0 {
		return nil
	}

	return &TimeCache{
		M:    make(map[peer.ID]map[string]time.Time),
		span: span,
	}
}

// Add adds an entry to the cache.
func (tc *TimeCache) Add(peer peer.ID, s string) {
	_, ok := tc.M[peer]
	if !ok {
		tc.M[peer] = make(map[string]time.Time)
	}

	tc.M[peer][s] = time.Now()
}

// Has tells whether an entry is present in the cache.
func (tc *TimeCache) Has(peer peer.ID, s string) bool {
	m, ok := tc.M[peer]
	if !ok {
		return false
	}
	t, ok := m[s]
	if ok && time.Since(t) > tc.span {
		delete(m, s)
		return false
	}
	return true
}
