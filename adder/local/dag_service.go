// Package local implements a ClusterDAGService that chunks and adds content
// to a local peer, before pinning it.
package local

import (
	"context"
	"errors"

	adder "github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/api"

	cid "gx/ipfs/QmPSQnBKM9g7BaUcZCvswUJVscQ1ipjmwxN5PXCjkp9EQ7/go-cid"
	rpc "gx/ipfs/QmPYiV9nwnXPxdn9zDgY4d9yaHwTS414sUb1K6nvQVHqqo/go-libp2p-gorpc"
	ipld "gx/ipfs/QmR7TcHkR9nxkUorfi8XMTAMLUK7GiP64TWWBzY3aacc1o/go-ipld-format"
	peer "gx/ipfs/QmTRhk7cgjUf2gfQ3p2M9KPECNZEW9XUrmHcFCgog4cPgB/go-libp2p-peer"
	logging "gx/ipfs/QmZChCsSt8DctjceaL56Eibc29CVQq4dGKRXC5JRZ6Ppae/go-log"
)

var errNotFound = errors.New("dagservice: block not found")

var logger = logging.Logger("localdags")

// DAGService is an implementation of an adder.ClusterDAGService which
// puts the added blocks directly in the peers allocated to them (without
// sharding).
type DAGService struct {
	adder.BaseDAGService

	rpcClient *rpc.Client

	dests   []peer.ID
	pinOpts api.PinOptions
}

// New returns a new Adder with the given rpc Client. The client is used
// to perform calls to IPFSBlockPut and Pin content on Cluster.
func New(rpc *rpc.Client, opts api.PinOptions) *DAGService {
	return &DAGService{
		rpcClient: rpc,
		dests:     nil,
		pinOpts:   opts,
	}
}

// Add puts the given node in the destination peers.
func (dgs *DAGService) Add(ctx context.Context, node ipld.Node) error {
	if dgs.dests == nil {
		dests, err := adder.BlockAllocate(ctx, dgs.rpcClient, dgs.pinOpts)
		if err != nil {
			return err
		}
		dgs.dests = dests
	}

	size, err := node.Size()
	if err != nil {
		return err
	}
	nodeSerial := &api.NodeWithMeta{
		Cid:     node.Cid().String(),
		Data:    node.RawData(),
		CumSize: size,
	}

	return adder.PutBlock(ctx, dgs.rpcClient, nodeSerial, dgs.dests)
}

// Finalize pins the last Cid added to this DAGService.
func (dgs *DAGService) Finalize(ctx context.Context, root cid.Cid) (cid.Cid, error) {
	// Cluster pin the result
	rootPin := api.PinWithOpts(root, dgs.pinOpts)
	rootPin.Allocations = dgs.dests

	dgs.dests = nil

	return root, dgs.rpcClient.CallContext(
		ctx,
		"",
		"Cluster",
		"Pin",
		rootPin.ToSerial(),
		&struct{}{},
	)
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
