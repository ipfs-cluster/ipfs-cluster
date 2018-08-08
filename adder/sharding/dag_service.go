// Package sharding implements a sharding ClusterDAGService places
// content in different shards while it's being added, creating
// a final Cluster DAG and pinning it.
package sharding

import (
	"context"
	"errors"
	"fmt"

	"time"

	"github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/api"

	humanize "github.com/dustin/go-humanize"
	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
)

var errNotFound = errors.New("dagservice: block not found")
var logger = logging.Logger("shardingdags")

// DAGService is an implementation of a ClusterDAGService which
// shards content while adding among several IPFS Cluster peers,
// creating a Cluster DAG to track and pin that content selectively
// in the IPFS daemons allocated to it.
type DAGService struct {
	adder.BaseDAGService

	rpcClient *rpc.Client

	pinOpts api.PinOptions
	output  chan<- *api.AddedOutput

	addedSet *cid.Set

	// Current shard being built
	currentShard *shard
	// Last flushed shard CID
	previousShard *cid.Cid

	// shard tracking
	shards map[string]*cid.Cid

	startTime time.Time
	totalSize uint64
}

// New returns a new ClusterDAGService, which uses the given rpc client to perform
// Allocate, IPFSBlockPut and Pin requests to other cluster components.
func New(rpc *rpc.Client, opts api.PinOptions, out chan<- *api.AddedOutput) *DAGService {
	return &DAGService{
		rpcClient: rpc,
		pinOpts:   opts,
		output:    out,
		addedSet:  cid.NewSet(),
		shards:    make(map[string]*cid.Cid),
		startTime: time.Now(),
	}
}

// Add puts the given node in its corresponding shard and sends it to the
// destination peers.
func (dag *DAGService) Add(ctx context.Context, node ipld.Node) error {
	// FIXME: This will grow in memory
	if !dag.addedSet.Visit(node.Cid()) {
		return nil
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

	return dag.ingestBlock(ctx, nodeSerial)
}

// Finalize finishes sharding, creates the cluster DAG and pins it along
// with the meta pin for the root node of the content.
func (dag *DAGService) Finalize(ctx context.Context, dataRoot *cid.Cid) (*cid.Cid, error) {
	lastCid, err := dag.flushCurrentShard(ctx)
	if err != nil {
		return lastCid, err
	}

	if !lastCid.Equals(dataRoot) {
		logger.Warningf("the last added CID (%s) is not the IPFS data root (%s). This is only normal when adding a single file without wrapping in directory.", lastCid, dataRoot)
	}

	clusterDAGNodes, err := makeDAG(dag.shards)
	if err != nil {
		return dataRoot, err
	}

	// PutDAG to ourselves
	err = putDAG(ctx, dag.rpcClient, clusterDAGNodes, []peer.ID{""})
	if err != nil {
		return dataRoot, err
	}

	clusterDAG := clusterDAGNodes[0].Cid()

	dag.sendOutput(&api.AddedOutput{
		Name: fmt.Sprintf("%s-clusterDAG", dag.pinOpts.Name),
		Hash: clusterDAG.String(),
		Size: fmt.Sprintf("%d", dag.totalSize),
	})

	// Pin the ClusterDAG
	clusterDAGPin := api.PinCid(clusterDAG)
	clusterDAGPin.ReplicationFactorMin = -1
	clusterDAGPin.ReplicationFactorMax = -1
	clusterDAGPin.MaxDepth = 0 // pin direct
	clusterDAGPin.Name = fmt.Sprintf("%s-clusterDAG", dag.pinOpts.Name)
	clusterDAGPin.Type = api.ClusterDAGType
	clusterDAGPin.Reference = dataRoot
	err = adder.Pin(ctx, dag.rpcClient, clusterDAGPin)
	if err != nil {
		return dataRoot, err
	}

	// Pin the META pin
	metaPin := api.PinWithOpts(dataRoot, dag.pinOpts)
	metaPin.Type = api.MetaType
	metaPin.Reference = clusterDAG
	metaPin.MaxDepth = 0 // irrelevant. Meta-pins are not pinned
	err = adder.Pin(ctx, dag.rpcClient, metaPin)
	if err != nil {
		return dataRoot, err
	}

	// Log some stats
	dag.logStats(metaPin.Cid, clusterDAGPin.Cid)

	// Consider doing this? Seems like overkill
	//
	// // Ammend ShardPins to reference clusterDAG root hash as a Parent
	// shardParents := cid.NewSet()
	// shardParents.Add(clusterDAG)
	// for shardN, shard := range dag.shardNodes {
	// 	pin := api.PinWithOpts(shard, dag.pinOpts)
	// 	pin.Name := fmt.Sprintf("%s-shard-%s", pin.Name, shardN)
	// 	pin.Type = api.ShardType
	// 	pin.Parents = shardParents
	// 	// FIXME: We don't know anymore the shard pin maxDepth
	//      // so we'd need to get the pin first.
	// 	err := dag.pin(pin)
	// 	if err != nil {
	// 		return err
	// 	}
	// }

	return dataRoot, nil
}

// ingests a block to the current shard. If it get's full, it
// Flushes the shard and retries with a new one.
func (dag *DAGService) ingestBlock(ctx context.Context, n *api.NodeWithMeta) error {
	shard := dag.currentShard

	// if we have no currentShard, create one
	if shard == nil {
		logger.Infof("new shard for '%s': #%d", dag.pinOpts.Name, len(dag.shards))
		var err error
		shard, err = newShard(ctx, dag.rpcClient, dag.pinOpts)
		if err != nil {
			return err
		}
		dag.currentShard = shard
	}

	logger.Debugf("ingesting block %s in shard %d (%s)", n.Cid, len(dag.shards), dag.pinOpts.Name)

	c, err := cid.Decode(n.Cid)
	if err != nil {
		return err
	}

	// add the block to it if it fits and return
	if shard.Size()+n.Size() < shard.Limit() {
		shard.AddLink(c, n.Size())
		return adder.PutBlock(ctx, dag.rpcClient, n, shard.Allocations())
	}

	logger.Debugf("shard %d full: block: %d. shard: %d. limit: %d",
		len(dag.shards),
		n.Size(),
		shard.Size(),
		shard.Limit(),
	)

	// -------
	// Below: block DOES NOT fit in shard
	// Flush and retry

	// if shard is empty, error
	if shard.Size() == 0 {
		return errors.New("block doesn't fit in empty shard: shard size too small?")
	}

	_, err = dag.flushCurrentShard(ctx)
	if err != nil {
		return err
	}
	return dag.ingestBlock(ctx, n) // <-- retry ingest
}

func (dag *DAGService) logStats(metaPin, clusterDAGPin *cid.Cid) {
	duration := time.Since(dag.startTime)
	seconds := uint64(duration) / uint64(time.Second)
	var rate string
	if seconds == 0 {
		rate = "∞ B"
	} else {
		rate = humanize.Bytes(dag.totalSize / seconds)
	}
	logger.Infof(`sharding session sucessful:
CID: %s
ClusterDAG: %s
Total shards: %d
Total size: %s
Total time: %s
Ingest Rate: %s/s
`,
		metaPin,
		clusterDAGPin,
		len(dag.shards),
		humanize.Bytes(dag.totalSize),
		duration,
		rate,
	)

}

func (dag *DAGService) sendOutput(ao *api.AddedOutput) {
	if dag.output != nil {
		dag.output <- ao
	}
}

// flushes the dag.currentShard and returns the LastLink()
func (dag *DAGService) flushCurrentShard(ctx context.Context) (*cid.Cid, error) {
	shard := dag.currentShard
	if shard == nil {
		return nil, errors.New("cannot flush a nil shard")
	}

	lens := len(dag.shards)

	shardCid, err := shard.Flush(ctx, lens, dag.previousShard)
	if err != nil {
		return shardCid, err
	}
	dag.totalSize += shard.Size()
	dag.shards[fmt.Sprintf("%d", lens)] = shardCid
	dag.previousShard = shardCid
	dag.currentShard = nil
	dag.sendOutput(&api.AddedOutput{
		Name: fmt.Sprintf("shard-%d", lens),
		Hash: shardCid.String(),
		Size: fmt.Sprintf("%d", shard.Size()),
	})

	return shard.LastLink(), nil
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
