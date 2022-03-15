package sharding

import (
	"context"
	"fmt"

	"github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"

	humanize "github.com/dustin/go-humanize"
)

// a shard represents a set of blocks (or bucket) which have been assigned
// a peer to be block-put and will be part of the same shard in the
// cluster DAG.
type shard struct {
	rpc         *rpc.Client
	allocations []peer.ID
	pinOptions  api.PinOptions
	ba          *adder.BlockAdder
	// dagNode represents a node with links and will be converted
	// to Cbor.
	dagNode     map[string]cid.Cid
	currentSize uint64
	sizeLimit   uint64
}

func newShard(ctx context.Context, rpc *rpc.Client, opts api.PinOptions) (*shard, error) {
	allocs, err := adder.BlockAllocate(ctx, rpc, opts)
	if err != nil {
		return nil, err
	}

	if opts.ReplicationFactorMin > 0 && len(allocs) == 0 {
		// This would mean that the empty cid is part of the shared state somehow.
		panic("allocations for new shard cannot be empty without error")
	}

	if opts.ReplicationFactorMin < 0 {
		logger.Warn("Shard is set to replicate everywhere ,which doesn't make sense for sharding")
	}

	// TODO (hector): get latest metrics for allocations, adjust sizeLimit
	// to minimum. This can be done later.

	return &shard{
		rpc:         rpc,
		allocations: allocs,
		pinOptions:  opts,
		ba:          adder.NewBlockAdder(rpc, allocs),
		dagNode:     make(map[string]cid.Cid),
		currentSize: 0,
		sizeLimit:   opts.ShardSize,
	}, nil
}

// AddLink tries to add a new block to this shard if it's not full.
// Returns true if the block was added
func (sh *shard) AddLink(ctx context.Context, c cid.Cid, s uint64) {
	linkN := len(sh.dagNode)
	linkName := fmt.Sprintf("%d", linkN)
	logger.Debugf("shard: add link: %s", linkName)

	sh.dagNode[linkName] = c
	sh.currentSize += s
}

// Allocations returns the peer IDs on which blocks are put for this shard.
func (sh *shard) Allocations() []peer.ID {
	if len(sh.allocations) == 1 && sh.allocations[0] == "" {
		return nil
	}
	return sh.allocations
}

// Flush completes the allocation of this shard by building a CBOR node
// and adding it to IPFS, then pinning it in cluster. It returns the Cid of the
// shard.
func (sh *shard) Flush(ctx context.Context, shardN int, prev cid.Cid) (cid.Cid, error) {
	logger.Debugf("shard %d: flush", shardN)
	nodes, err := makeDAG(ctx, sh.dagNode)
	if err != nil {
		return cid.Undef, err
	}

	err = sh.ba.AddMany(ctx, nodes)
	if err != nil {
		return cid.Undef, err
	}

	rootCid := nodes[0].Cid()
	pin := api.PinWithOpts(rootCid, sh.pinOptions)
	pin.Name = fmt.Sprintf("%s-shard-%d", sh.pinOptions.Name, shardN)
	// this sets allocations as priority allocation
	pin.Allocations = sh.allocations
	pin.Type = api.ShardType
	pin.Reference = &prev
	pin.MaxDepth = 1
	pin.ShardSize = sh.Size()           // use current size, not the limit
	if len(nodes) > len(sh.dagNode)+1 { // using an indirect graph
		pin.MaxDepth = 2
	}

	logger.Infof("shard #%d (%s) completed. Total size: %s. Links: %d",
		shardN,
		rootCid,
		humanize.Bytes(sh.Size()),
		len(sh.dagNode),
	)

	return rootCid, adder.Pin(ctx, sh.rpc, pin)
}

// Size returns this shard's current size.
func (sh *shard) Size() uint64 {
	return sh.currentSize
}

// Size returns this shard's size limit.
func (sh *shard) Limit() uint64 {
	return sh.sizeLimit
}

// Last returns the last added link. When finishing sharding,
// the last link of the last shard is the data root for the
// full sharded DAG (the CID that would have resulted from
// adding the content to a single IPFS daemon).
func (sh *shard) LastLink() cid.Cid {
	l := len(sh.dagNode)
	lastLink := fmt.Sprintf("%d", l-1)
	return sh.dagNode[lastLink]
}
