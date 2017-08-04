package util

import (
	"strconv"

	"github.com/ipfs/ipfs-cluster/api"

	peer "github.com/libp2p/go-libp2p-peer"
)

type MetricSorter struct {
	Peers   []peer.ID
	M       map[peer.ID]int
	Reverse bool
}

func NewMetricSorter(m map[peer.ID]api.Metric, reverse bool) *MetricSorter {
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

	sorter := &MetricSorter{
		M:       vMap,
		Peers:   peers,
		Reverse: reverse,
	}
	return sorter
}

// Len returns the number of metrics
func (s MetricSorter) Len() int {
	return len(s.Peers)
}

// Swap swaps the elements in positions i and j
func (s MetricSorter) Swap(i, j int) {
	temp := s.Peers[i]
	s.Peers[i] = s.Peers[j]
	s.Peers[j] = temp
}

// Less reports if the element in position i is less than the element in j
// (important to override this)
func (s MetricSorter) Less(i, j int) bool {
	peeri := s.Peers[i]
	peerj := s.Peers[j]

	x := s.M[peeri]
	y := s.M[peerj]

	if s.Reverse {
		return x > y
	}
	return x < y
}
