// Package ascendalloc implements an ipfscluster.PinAllocator, which returns
// allocations based on sorting the metrics in ascending order. Thus, peers with
// smallest metrics are first in the list. This allocator can be used with a
// number of informers, as long as they provide a numeric metric value.
package ascendalloc

import (
	"context"

	"github.com/ipfs/ipfs-cluster/allocator/util"
	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p-peer"
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
func (alloc AscendAllocator) Shutdown(_ context.Context) error { return nil }

// Allocate returns where to allocate a pin request based on metrics which
// carry a numeric value such as "used disk". We do not pay attention to
// the metrics of the currently allocated peers and we just sort the
// candidates based on their metric values (smallest to largest).
func (alloc AscendAllocator) Allocate(
	ctx context.Context,
	c cid.Cid,
	current, candidates, priority map[peer.ID]api.Metric,
) ([]peer.ID, error) {
	// sort our metrics
	first := util.SortNumeric(priority, false)
	last := util.SortNumeric(candidates, false)
	return append(first, last...), nil
}
