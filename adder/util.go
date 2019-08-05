package adder

import (
	"context"
	"errors"
	"fmt"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/rpcutil"

	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"
)

// BlockAdder implements "github.com/ipfs/go-ipld-format".NodeAdder.
// It is efficient because it doesn't try failed peers again as long as
// block is stored with at least one peer.
type BlockAdder struct {
	dests     []peer.ID
	rpcClient *rpc.Client
}

// NewBlockAdder creates a BlockAdder given an rpc client and allocation peers.
func NewBlockAdder(rpcClient *rpc.Client, dests []peer.ID) *BlockAdder {
	return &BlockAdder{
		dests:     dests,
		rpcClient: rpcClient,
	}
}

// Add puts an ipld node to allocated destinations.
func (ba *BlockAdder) Add(ctx context.Context, node ipld.Node) error {
	size, err := node.Size()
	if err != nil {
		logger.Warning(err)
	}
	nodeSerial := &api.NodeWithMeta{
		Cid:     node.Cid(),
		Data:    node.RawData(),
		CumSize: size,
	}

	format, ok := cid.CodecToStr[nodeSerial.Cid.Type()]
	if !ok {
		format = ""
		logger.Warning("unsupported cid type, treating as v0")
	}
	if nodeSerial.Cid.Prefix().Version == 0 {
		format = "v0"
	}

	nodeSerial.Format = format
	ctxs, cancels := rpcutil.CtxsWithCancel(ctx, len(ba.dests))
	defer rpcutil.MultiCancel(cancels)

	logger.Debugf("block put %s to %s", nodeSerial.Cid, ba.dests)
	errs := ba.rpcClient.MultiCall(
		ctxs,
		ba.dests,
		"IPFSConnector",
		"BlockPut",
		nodeSerial,
		rpcutil.RPCDiscardReplies(len(ba.dests)),
	)

	var actDests []peer.ID
	for i, e := range errs {
		if rpc.IsAuthorizationError(e) || rpc.IsServerError(e) {
			continue
		}
		actDests = append(actDests, ba.dests[i])
	}

	if len(actDests) == 0 {
		// request to all peers failed
		return fmt.Errorf("could not put block on any peer: %s", rpcutil.CheckErrs(errs))
	}

	ba.dests = actDests
	return nil
}

// AddMany puts multiple ipld nodes to allocated destinations.
func (ba *BlockAdder) AddMany(ctx context.Context, nodes []ipld.Node) error {
	for _, node := range nodes {
		err := ba.Add(ctx, node)
		if err != nil {
			return err
		}
	}
	return nil
}

// BlockAllocate helps allocating blocks to peers.
func BlockAllocate(ctx context.Context, rpc *rpc.Client, pinOpts api.PinOptions) ([]peer.ID, error) {
	// Find where to allocate this file
	var allocsStr []peer.ID
	err := rpc.CallContext(
		ctx,
		"",
		"Cluster",
		"BlockAllocate",
		api.PinWithOpts(cid.Undef, pinOpts),
		&allocsStr,
	)
	return allocsStr, err
}

// Pin helps sending local RPC pin requests.
func Pin(ctx context.Context, rpc *rpc.Client, pin *api.Pin) error {
	if pin.ReplicationFactorMin < 0 {
		pin.Allocations = []peer.ID{}
	}
	logger.Debugf("adder pinning %+v", pin)
	var pinResp api.Pin
	return rpc.CallContext(
		ctx,
		"", // use ourself to pin
		"Cluster",
		"Pin",
		pin,
		&pinResp,
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
func (dag BaseDAGService) Get(ctx context.Context, key cid.Cid) (ipld.Node, error) {
	return nil, ErrDAGNotFound
}

// GetMany returns an output channel that always emits an error
func (dag BaseDAGService) GetMany(ctx context.Context, keys []cid.Cid) <-chan *ipld.NodeOption {
	out := make(chan *ipld.NodeOption, 1)
	out <- &ipld.NodeOption{Err: fmt.Errorf("failed to fetch all nodes")}
	close(out)
	return out
}

// Remove is a nop
func (dag BaseDAGService) Remove(ctx context.Context, key cid.Cid) error {
	return nil
}

// RemoveMany is a nop
func (dag BaseDAGService) RemoveMany(ctx context.Context, keys []cid.Cid) error {
	return nil
}
