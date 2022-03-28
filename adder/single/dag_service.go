// Package single implements a ClusterDAGService that chunks and adds content
// to cluster without sharding, before pinning it.
package single

import (
	"context"

	adder "github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log/v2"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"
)

var logger = logging.Logger("singledags")
var _ = logger // otherwise unused

// DAGService is an implementation of an adder.ClusterDAGService which
// puts the added blocks directly in the peers allocated to them (without
// sharding).
type DAGService struct {
	adder.BaseDAGService

	ctx       context.Context
	rpcClient *rpc.Client

	dests     []peer.ID
	addParams api.AddParams
	local     bool

	bs           *adder.BlockStreamer
	blocks       chan api.NodeWithMeta
	recentBlocks *recentBlocks
}

// New returns a new Adder with the given rpc Client. The client is used
// to perform calls to IPFS.BlockStream and Pin content on Cluster.
func New(ctx context.Context, rpc *rpc.Client, opts api.AddParams, local bool) *DAGService {
	// ensure don't Add something and pin it in direct mode.
	opts.Mode = api.PinModeRecursive
	return &DAGService{
		ctx:          ctx,
		rpcClient:    rpc,
		dests:        nil,
		addParams:    opts,
		local:        local,
		blocks:       make(chan api.NodeWithMeta, 256),
		recentBlocks: &recentBlocks{},
	}
}

// Add puts the given node in the destination peers.
func (dgs *DAGService) Add(ctx context.Context, node ipld.Node) error {
	// Avoid adding the same node multiple times in a row.
	// This is done by the ipfsadd-er, because some nodes are added
	// via dagbuilder, then via MFS, and root nodes once more.
	if dgs.recentBlocks.Has(node) {
		return nil
	}

	// FIXME: can't this happen on initialization?  Perhaps the point here
	// is the adder only allocates and starts streaming when the first
	// block arrives and not on creation.
	if dgs.dests == nil {
		dests, err := adder.BlockAllocate(ctx, dgs.rpcClient, dgs.addParams.PinOptions)
		if err != nil {
			return err
		}

		hasLocal := false
		localPid := dgs.rpcClient.ID()
		for i, d := range dests {
			if d == localPid || d == "" {
				hasLocal = true
				// ensure our allocs do not carry an empty peer
				// mostly an issue with testing mocks
				dests[i] = localPid
			}
		}

		dgs.dests = dests

		if dgs.local {
			// If this is a local pin, make sure that the local
			// peer is among the allocations..
			// UNLESS user-allocations are defined!
			if !hasLocal && localPid != "" && len(dgs.addParams.UserAllocations) == 0 {
				// replace last allocation with local peer
				dgs.dests[len(dgs.dests)-1] = localPid
			}

			dgs.bs = adder.NewBlockStreamer(dgs.ctx, dgs.rpcClient, []peer.ID{localPid}, dgs.blocks)
		} else {
			dgs.bs = adder.NewBlockStreamer(dgs.ctx, dgs.rpcClient, dgs.dests, dgs.blocks)
		}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-dgs.ctx.Done():
		return ctx.Err()
	case dgs.blocks <- adder.IpldNodeToNodeWithMeta(node):
		dgs.recentBlocks.Add(node)
		return nil
	}
}

// Finalize pins the last Cid added to this DAGService.
func (dgs *DAGService) Finalize(ctx context.Context, root cid.Cid) (cid.Cid, error) {
	close(dgs.blocks)

	select {
	case <-dgs.ctx.Done():
		return root, ctx.Err()
	case <-ctx.Done():
		return root, ctx.Err()
	case <-dgs.bs.Done():
	}

	// If the streamer failed to put blocks.
	if err := dgs.bs.Err(); err != nil {
		return root, err
	}

	// Do not pin, just block put.
	// Why? Because some people are uploading CAR files with partial DAGs
	// and ideally they should be pinning only when the last partial CAR
	// is uploaded. This gives them that option.
	if dgs.addParams.NoPin {
		return root, nil
	}

	// Cluster pin the result
	rootPin := api.PinWithOpts(root, dgs.addParams.PinOptions)
	rootPin.Allocations = dgs.dests

	return root, adder.Pin(ctx, dgs.rpcClient, rootPin)
}

// Allocations returns the add destinations decided by the DAGService.
func (dgs *DAGService) Allocations() []peer.ID {
	// using rpc clients without a host results in an empty peer
	// which cannot be parsed to peer.ID on deserialization.
	if len(dgs.dests) == 1 && dgs.dests[0] == "" {
		return nil
	}
	return dgs.dests
}

// AddMany calls Add for every given node.
func (dgs *DAGService) AddMany(ctx context.Context, nodes []ipld.Node) error {
	for _, node := range nodes {
		err := dgs.Add(ctx, node)
		if err != nil {
			return err
		}
	}
	return nil
}

type recentBlocks struct {
	blocks [2]cid.Cid
	cur    int
}

func (rc *recentBlocks) Add(n ipld.Node) {
	rc.blocks[rc.cur] = n.Cid()
	rc.cur = (rc.cur + 1) % 2
}

func (rc *recentBlocks) Has(n ipld.Node) bool {
	c := n.Cid()
	return rc.blocks[0].Equals(c) || rc.blocks[1].Equals(c)
}
