// Package descendalloc implements an ipfscluster.PinAllocator returns
// allocations based on sorting the metrics in descending order. Thus, peers
// with largest metrics are first in the list. This allocator can be used with a
// number of informers, as long as they provide a numeric metric value.
package descendalloc

import (
	"github.com/ipfs/ipfs-cluster/allocator/util"
	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
)

var logger = logging.Logger("descendalloc")

// DescendAllocator extends the SimpleAllocator
type DescendAllocator struct{}

// NewAllocator returns an initialized DescendAllocator
func NewAllocator() DescendAllocator {
	return DescendAllocator{}
}

// SetClient does nothing in this allocator
func (alloc DescendAllocator) SetClient(c *rpc.Client) {}

// Shutdown does nothing in this allocator
func (alloc DescendAllocator) Shutdown() error { return nil }

// Allocate returns where to allocate a pin request based on metrics which
// carry a numeric value such as "used disk". We do not pay attention to
// the metrics of the currently allocated peers and we just sort the
// candidates based on their metric values (largest to smallest).
func (alloc DescendAllocator) Allocate(c *cid.Cid, current, candidates, priority map[peer.ID]api.Metric) ([]peer.ID, error) {
	// sort our metrics
	first := util.SortNumeric(priority, true)
	last := util.SortNumeric(candidates, true)
	return append(first, last...), nil
}
