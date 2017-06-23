// Package ascendalloc implements an ipfscluster.Allocator returns allocations
// based on sorting the metrics in ascending order. Thus, peers with smallest
// metrics are first in the list. This allocator can be used with a number
// of informers, as long as they provide a numeric metric value.
package ascendalloc

import (
	"sort"
	"strconv"

	"github.com/ipfs/ipfs-cluster/api"

	logging "gx/ipfs/QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52/go-log"
	rpc "gx/ipfs/QmYqnvVzUjjVddWPLGMAErUjNBqnyjoeeCgZUZFsAJeGHr/go-libp2p-gorpc"
	peer "gx/ipfs/QmZcUPvPhD1Xvk6mwijYF8AfR3mG31S1YsEfHG4khrFPRr/go-libp2p-peer"
	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

var logger = logging.Logger("ascendalloc")

// Allocator implements ipfscluster.Allocate.
type Allocator struct{}

// NewAllocator returns an initialized Allocator
func NewAllocator() *Allocator {
	return &Allocator{}
}

// SetClient does nothing in this allocator
func (alloc *Allocator) SetClient(c *rpc.Client) {}

// Shutdown does nothing in this allocator
func (alloc *Allocator) Shutdown() error { return nil }

// Allocate returns where to allocate a pin request based on metrics which
// carry a numeric value such as "used disk". We do not pay attention to
// the metrics of the currently allocated peers and we just sort the candidates
// based on their metric values (from smallest to largest).
func (alloc *Allocator) Allocate(c *cid.Cid, current, candidates map[peer.ID]api.Metric) ([]peer.ID, error) {
	// sort our metrics
	sortable := newMetricsSorter(candidates)
	sort.Sort(sortable)
	return sortable.peers, nil
}

// metricsSorter attaches sort.Interface methods to our metrics and sorts
// a slice of peers in the way that interest us
type metricsSorter struct {
	peers []peer.ID
	m     map[peer.ID]int
}

func newMetricsSorter(m map[peer.ID]api.Metric) *metricsSorter {
	vMap := make(map[peer.ID]int)
	peers := make([]peer.ID, 0, len(m))
	for k, v := range m {
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

	sorter := &metricsSorter{
		m:     vMap,
		peers: peers,
	}
	return sorter
}

// Len returns the number of metrics
func (s metricsSorter) Len() int {
	return len(s.peers)
}

// Less reports if the element in position i is less than the element in j
func (s metricsSorter) Less(i, j int) bool {
	peeri := s.peers[i]
	peerj := s.peers[j]

	x := s.m[peeri]
	y := s.m[peerj]

	return x < y
}

// Swap swaps the elements in positions i and j
func (s metricsSorter) Swap(i, j int) {
	temp := s.peers[i]
	s.peers[i] = s.peers[j]
	s.peers[j] = temp
}
