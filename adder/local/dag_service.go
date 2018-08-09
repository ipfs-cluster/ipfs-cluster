// Package local implements a ClusterDAGService that chunks and adds content
// to a local peer, before pinning it.
package local

import (
	"context"
	"errors"

	adder "github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
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
func (dgs *DAGService) Finalize(ctx context.Context, root *cid.Cid) (*cid.Cid, error) {
	// Cluster pin the result
	pinS := api.PinSerial{
		Cid:        root.String(),
		Type:       int(api.DataType),
		MaxDepth:   -1,
		PinOptions: dgs.pinOpts,
	}

	dgs.dests = nil

	return root, dgs.rpcClient.CallContext(
		ctx,
		"",
		"Cluster",
		"Pin",
		pinS,
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
