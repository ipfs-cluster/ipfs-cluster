// Package allocator.util is a utility package used by the allocator
// implementations. This package provides the SortNumeric function, which may be
// used by an allocator to sort peers by their metric values (ascending or
// descending).
package util

import (
	"sort"
	"strconv"

	"github.com/ipfs/ipfs-cluster/api"

	peer "github.com/libp2p/go-libp2p-peer"
)

// SortNumeric returns a list of peers sorted by their metric values. If reverse
// is false (true), peers will be sorted from smallest to largest (largest to
// smallest) metric
func SortNumeric(candidates map[peer.ID]api.Metric, reverse bool) []peer.ID {
	vMap := make(map[peer.ID]int)
	peers := make([]peer.ID, 0, len(candidates))
	for k, v := range candidates {
		if v.Discard() {
			continue
		}
		val, err := strconv.Atoi(v.Value)
		if err != nil {
			continue
		}
		peers = append(peers, k)
		vMap[k] = val
	}

	sorter := &metricSorter{
		m:       vMap,
		peers:   peers,
		reverse: reverse,
	}
	sort.Sort(sorter)
	return sorter.peers
}

// metricSorter implements the sort.Sort interface
type metricSorter struct {
	peers   []peer.ID
	m       map[peer.ID]int
	reverse bool
}

// Len returns the number of metrics
func (s metricSorter) Len() int {
	return len(s.peers)
}

// Swap Swaps the elements in positions i and j
func (s metricSorter) Swap(i, j int) {
	temp := s.peers[i]
	s.peers[i] = s.peers[j]
	s.peers[j] = temp
}

// Less reports if the element in position i is Less than the element in j
// (important to override this)
func (s metricSorter) Less(i, j int) bool {
	peeri := s.peers[i]
	peerj := s.peers[j]

	x := s.m[peeri]
	y := s.m[peerj]

	if s.reverse {
		return x > y
	}
	return x < y
}
