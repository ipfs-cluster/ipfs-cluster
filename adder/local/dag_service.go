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
	lastCid *cid.Cid
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
func (dag *DAGService) Add(ctx context.Context, node ipld.Node) error {
	if dag.dests == nil {
		// Find where to allocate this file
		var allocsStr []string
		err := dag.rpcClient.CallContext(
			ctx,
			"",
			"Cluster",
			"Allocate",
			api.PinWithOpts(nil, dag.pinOpts).ToSerial(),
			&allocsStr,
		)
		if err != nil {
			return err
		}

		dag.dests = api.StringsToPeers(allocsStr)
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

	dag.lastCid = node.Cid()

	return adder.PutBlock(ctx, dag.rpcClient, nodeSerial, dag.dests)
}

// Finalize pins the last Cid added to this DAGService.
func (dag *DAGService) Finalize(ctx context.Context) (*cid.Cid, error) {
	if dag.lastCid == nil {
		return nil, errors.New("nothing was added")
	}

	// Cluster pin the result
	pinS := api.PinSerial{
		Cid:        dag.lastCid.String(),
		Type:       int(api.DataType),
		MaxDepth:   -1,
		PinOptions: dag.pinOpts,
	}

	dag.dests = nil

	return dag.lastCid, dag.rpcClient.CallContext(
		ctx,
		"",
		"Cluster",
		"Pin",
		pinS,
		&struct{}{},
	)
}

// AddMany calls Add for every given node.
func (dag *DAGService) AddMany(ctx context.Context, nodes []ipld.Node) error {
	for _, node := range nodes {
		err := dag.Add(ctx, node)
		if err != nil {
			return err
		}
	}
	return nil
}
