// Package ascendalloc implements an ipfscluster.PinAllocator, which returns
// allocations based on sorting the metrics in ascending order. Thus, peers with
// smallest metrics are first in the list. This allocator can be used with a
// number of informers, as long as they provide a numeric metric value.
package ascendalloc

import (
	"github.com/ipfs/ipfs-cluster/allocator/util"
	"github.com/ipfs/ipfs-cluster/api"

	cid "gx/ipfs/QmPSQnBKM9g7BaUcZCvswUJVscQ1ipjmwxN5PXCjkp9EQ7/go-cid"
	rpc "gx/ipfs/QmPYiV9nwnXPxdn9zDgY4d9yaHwTS414sUb1K6nvQVHqqo/go-libp2p-gorpc"
	peer "gx/ipfs/QmTRhk7cgjUf2gfQ3p2M9KPECNZEW9XUrmHcFCgog4cPgB/go-libp2p-peer"
	logging "gx/ipfs/QmZChCsSt8DctjceaL56Eibc29CVQq4dGKRXC5JRZ6Ppae/go-log"
)

var logger = logging.Logger("ascendalloc")

// AscendAllocator extends the SimpleAllocator
type AscendAllocator struct{}

// NewAllocator returns an initialized AscendAllocator
func NewAllocator() AscendAllocator {
	return AscendAllocator{}
}

// SetClient does nothing in this allocator
func (alloc AscendAllocator) SetClient(c *rpc.Client) {}

// Shutdown does nothing in this allocator
func (alloc AscendAllocator) Shutdown() error { return nil }

// Allocate returns where to allocate a pin request based on metrics which
// carry a numeric value such as "used disk". We do not pay attention to
// the metrics of the currently allocated peers and we just sort the
// candidates based on their metric values (smallest to largest).
func (alloc AscendAllocator) Allocate(c cid.Cid, current,
	candidates, priority map[peer.ID]api.Metric) ([]peer.ID, error) {
	// sort our metrics
	first := util.SortNumeric(priority, false)
	last := util.SortNumeric(candidates, false)
	return append(first, last...), nil
}
