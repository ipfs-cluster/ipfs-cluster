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

	rpcClient *rpc.Client

	dests     []peer.ID
	addParams api.AddParams
	local     bool

	ba *adder.BlockAdder
}

// New returns a new Adder with the given rpc Client. The client is used
// to perform calls to IPFS.BlockPut and Pin content on Cluster.
func New(rpc *rpc.Client, opts api.AddParams, local bool) *DAGService {
	// ensure don't Add something and pin it in direct mode.
	opts.Mode = api.PinModeRecursive
	return &DAGService{
		rpcClient: rpc,
		dests:     nil,
		addParams: opts,
		local:     local,
	}
}

// Add puts the given node in the destination peers.
func (dgs *DAGService) Add(ctx context.Context, node ipld.Node) error {
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

			dgs.ba = adder.NewBlockAdder(dgs.rpcClient, []peer.ID{localPid})
		} else {
			dgs.ba = adder.NewBlockAdder(dgs.rpcClient, dgs.dests)
		}
	}

	return dgs.ba.Add(ctx, node)
}

// Finalize pins the last Cid added to this DAGService.
func (dgs *DAGService) Finalize(ctx context.Context, root cid.Cid) (cid.Cid, error) {
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
