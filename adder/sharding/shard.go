package sharding

import (
	"context"
	"errors"
	"fmt"

	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
)

// a shard represents a set of blocks (or bucket) which have been assigned
// a peer to be block-put and will be part of the same shard in the
// cluster DAG.
type shard struct {
	rpc      *rpc.Client
	destPeer peer.ID
	// dagNode represents a node with links and will be converted
	// to Cbor.
	dagNode     map[string]*cid.Cid
	currentSize uint64
	sizeLimit   uint64
}

func newShard(ctx context.Context, rpc *rpc.Client, sizeLimit uint64) (*shard, error) {
	var allocs []string
	// TODO: before it figured out how much freespace there is in the node
	// and set the maximum shard size to that.
	// I have dropped that functionality.
	// It would involve getting metrics manually.
	err := rpc.CallContext(
		ctx,
		"",
		"Cluster",
		"Allocate",
		api.PinSerial{
			Cid: "",
			PinOptions: api.PinOptions{
				ReplicationFactorMin: 1,
				ReplicationFactorMax: int(^uint(0) >> 1), //max int
			},
		},
		&allocs,
	)
	if err != nil {
		return nil, err
	}

	if len(allocs) < 1 { // redundant
		return nil, errors.New("cannot allocate new shard")
	}

	return &shard{
		rpc:         rpc,
		destPeer:    api.StringsToPeers(allocs)[0],
		dagNode:     make(map[string]*cid.Cid),
		currentSize: 0,
		sizeLimit:   sizeLimit,
	}, nil
}

// AddLink tries to add a new block to this shard if it's not full.
// Returns true if the block was added
func (sh *shard) AddLink(c *cid.Cid, s uint64) {
	linkN := len(sh.dagNode)
	linkName := fmt.Sprintf("%d", linkN)
	sh.dagNode[linkName] = c
	sh.currentSize += s
}

// DestPeer returns the peer ID on which blocks are put for this shard.
func (sh *shard) DestPeer() peer.ID {
	return sh.destPeer
}

// Flush completes the allocation of this shard by building a CBOR node
// and adding it to IPFS, then pinning it in cluster. It returns the Cid of the
// shard.
func (sh *shard) Flush(ctx context.Context, opts api.PinOptions, shardN int) (*cid.Cid, error) {
	logger.Debug("flushing shard")
	nodes, err := makeDAG(sh.dagNode)
	if err != nil {
		return nil, err
	}

	putDAG(ctx, sh.rpc, nodes, sh.destPeer)

	rootCid := nodes[0].Cid()
	pin := api.PinWithOpts(rootCid, opts)
	pin.Name = fmt.Sprintf("%s-shard-%d", opts.Name, shardN)
	// this sets dest peer as priority allocation
	pin.Allocations = []peer.ID{sh.destPeer}
	pin.Type = api.ShardType
	pin.MaxDepth = 1
	if len(nodes) > len(sh.dagNode)+1 { // using an indirect graph
		pin.MaxDepth = 2
	}

	err = sh.rpc.Call(
		sh.destPeer,
		"Cluster",
		"Pin",
		pin.ToSerial(),
		&struct{}{},
	)
	return rootCid, err
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
func (sh *shard) LastLink() *cid.Cid {
	l := len(sh.dagNode)
	lastLink := fmt.Sprintf("%d", l-1)
	return sh.dagNode[lastLink]
}
