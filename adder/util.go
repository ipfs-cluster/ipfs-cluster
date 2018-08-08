package adder

import (
	"context"
	"errors"
	"fmt"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/rpcutil"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	peer "github.com/libp2p/go-libp2p-peer"
)

// PutBlock sends a NodeWithMeta to the given destinations.
func PutBlock(ctx context.Context, rpc *rpc.Client, n *api.NodeWithMeta, dests []peer.ID) error {
	c, err := cid.Decode(n.Cid)
	if err != nil {
		return err
	}

	format, ok := cid.CodecToStr[c.Type()]
	if !ok {
		format = ""
		logger.Warning("unsupported cid type, treating as v0")
	}
	if c.Prefix().Version == 0 {
		format = "v0"
	}
	n.Format = format

	ctxs, cancels := rpcutil.CtxsWithCancel(ctx, len(dests))
	defer rpcutil.MultiCancel(cancels)

	logger.Debugf("block put %s to %s", n.Cid, dests)
	errs := rpc.MultiCall(
		ctxs,
		dests,
		"Cluster",
		"IPFSBlockPut",
		*n,
		rpcutil.RPCDiscardReplies(len(dests)),
	)
	return rpcutil.CheckErrs(errs)
}

// BlockAllocate helps allocating blocks to peers.
func BlockAllocate(ctx context.Context, rpc *rpc.Client, pinOpts api.PinOptions) ([]peer.ID, error) {
	// Find where to allocate this file
	var allocsStr []string
	err := rpc.CallContext(
		ctx,
		"",
		"Cluster",
		"BlockAllocate",
		api.PinWithOpts(nil, pinOpts).ToSerial(),
		&allocsStr,
	)
	return api.StringsToPeers(allocsStr), err
}

// Pin helps sending local RPC pin requests.
func Pin(ctx context.Context, rpc *rpc.Client, pin api.Pin) error {
	if pin.ReplicationFactorMin < 0 {
		pin.Allocations = []peer.ID{}
	}
	logger.Debugf("adder pinning %+v", pin)
	return rpc.CallContext(
		ctx,
		"", // use ourself to pin
		"Cluster",
		"Pin",
		pin.ToSerial(),
		&struct{}{},
	)
}

// ErrDAGNotFound is returned whenever we try to get a block from the DAGService.
var ErrDAGNotFound = errors.New("dagservice: block not found")

// BaseDAGService partially implements an ipld.DAGService.
// It provides the methods which are not needed by ClusterDAGServices
// (Get*, Remove*) so that they can save adding this code.
type BaseDAGService struct {
}

// Get always returns errNotFound
func (dag BaseDAGService) Get(ctx context.Context, key *cid.Cid) (ipld.Node, error) {
	return nil, ErrDAGNotFound
}

// GetMany returns an output channel that always emits an error
func (dag BaseDAGService) GetMany(ctx context.Context, keys []*cid.Cid) <-chan *ipld.NodeOption {
	out := make(chan *ipld.NodeOption, 1)
	out <- &ipld.NodeOption{Err: fmt.Errorf("failed to fetch all nodes")}
	close(out)
	return out
}

// Remove is a nop
func (dag BaseDAGService) Remove(ctx context.Context, key *cid.Cid) error {
	return nil
}

// RemoveMany is a nop
func (dag BaseDAGService) RemoveMany(ctx context.Context, keys []*cid.Cid) error {
	return nil
}
