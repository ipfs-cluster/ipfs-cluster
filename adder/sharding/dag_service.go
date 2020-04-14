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
	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log/v2"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"
)

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
	previousShard cid.Cid

	// shard tracking
	shards map[string]cid.Cid

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
		shards:    make(map[string]cid.Cid),
		startTime: time.Now(),
	}
}

// Add puts the given node in its corresponding shard and sends it to the
// destination peers.
func (dgs *DAGService) Add(ctx context.Context, node ipld.Node) error {
	// FIXME: This will grow in memory
	if !dgs.addedSet.Visit(node.Cid()) {
		return nil
	}

	return dgs.ingestBlock(ctx, node)
}

// Finalize finishes sharding, creates the cluster DAG and pins it along
// with the meta pin for the root node of the content.
func (dgs *DAGService) Finalize(ctx context.Context, dataRoot cid.Cid) (cid.Cid, error) {
	lastCid, err := dgs.flushCurrentShard(ctx)
	if err != nil {
		return lastCid, err
	}

	if !lastCid.Equals(dataRoot) {
		logger.Warnf("the last added CID (%s) is not the IPFS data root (%s). This is only normal when adding a single file without wrapping in directory.", lastCid, dataRoot)
	}

	clusterDAGNodes, err := makeDAG(ctx, dgs.shards)
	if err != nil {
		return dataRoot, err
	}

	// PutDAG to ourselves
	err = adder.NewBlockAdder(dgs.rpcClient, []peer.ID{""}).AddMany(ctx, clusterDAGNodes)
	if err != nil {
		return dataRoot, err
	}

	clusterDAG := clusterDAGNodes[0].Cid()

	dgs.sendOutput(&api.AddedOutput{
		Name: fmt.Sprintf("%s-clusterDAG", dgs.pinOpts.Name),
		Cid:  clusterDAG,
		Size: dgs.totalSize,
	})

	// Pin the ClusterDAG
	clusterDAGPin := api.PinWithOpts(clusterDAG, dgs.pinOpts)
	clusterDAGPin.ReplicationFactorMin = -1
	clusterDAGPin.ReplicationFactorMax = -1
	clusterDAGPin.MaxDepth = 0 // pin direct
	clusterDAGPin.Name = fmt.Sprintf("%s-clusterDAG", dgs.pinOpts.Name)
	clusterDAGPin.Type = api.ClusterDAGType
	clusterDAGPin.Reference = &dataRoot
	err = adder.Pin(ctx, dgs.rpcClient, clusterDAGPin)
	if err != nil {
		return dataRoot, err
	}

	// Pin the META pin
	metaPin := api.PinWithOpts(dataRoot, dgs.pinOpts)
	metaPin.Type = api.MetaType
	metaPin.Reference = &clusterDAG
	metaPin.MaxDepth = 0 // irrelevant. Meta-pins are not pinned
	err = adder.Pin(ctx, dgs.rpcClient, metaPin)
	if err != nil {
		return dataRoot, err
	}

	// Log some stats
	dgs.logStats(metaPin.Cid, clusterDAGPin.Cid)

	// Consider doing this? Seems like overkill
	//
	// // Amend ShardPins to reference clusterDAG root hash as a Parent
	// shardParents := cid.NewSet()
	// shardParents.Add(clusterDAG)
	// for shardN, shard := range dgs.shardNodes {
	// 	pin := api.PinWithOpts(shard, dgs.pinOpts)
	// 	pin.Name := fmt.Sprintf("%s-shard-%s", pin.Name, shardN)
	// 	pin.Type = api.ShardType
	// 	pin.Parents = shardParents
	// 	// FIXME: We don't know anymore the shard pin maxDepth
	//      // so we'd need to get the pin first.
	// 	err := dgs.pin(pin)
	// 	if err != nil {
	// 		return err
	// 	}
	// }

	return dataRoot, nil
}

// ingests a block to the current shard. If it get's full, it
// Flushes the shard and retries with a new one.
func (dgs *DAGService) ingestBlock(ctx context.Context, n ipld.Node) error {
	shard := dgs.currentShard

	// if we have no currentShard, create one
	if shard == nil {
		logger.Infof("new shard for '%s': #%d", dgs.pinOpts.Name, len(dgs.shards))
		var err error
		shard, err = newShard(ctx, dgs.rpcClient, dgs.pinOpts)
		if err != nil {
			return err
		}
		dgs.currentShard = shard
	}

	logger.Debugf("ingesting block %s in shard %d (%s)", n.Cid(), len(dgs.shards), dgs.pinOpts.Name)

	// this is not same as n.Size()
	size := uint64(len(n.RawData()))

	// add the block to it if it fits and return
	if shard.Size()+size < shard.Limit() {
		shard.AddLink(ctx, n.Cid(), size)
		return dgs.currentShard.ba.Add(ctx, n)
	}

	logger.Debugf("shard %d full: block: %d. shard: %d. limit: %d",
		len(dgs.shards),
		size,
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

	_, err := dgs.flushCurrentShard(ctx)
	if err != nil {
		return err
	}
	return dgs.ingestBlock(ctx, n) // <-- retry ingest
}

func (dgs *DAGService) logStats(metaPin, clusterDAGPin cid.Cid) {
	duration := time.Since(dgs.startTime)
	seconds := uint64(duration) / uint64(time.Second)
	var rate string
	if seconds == 0 {
		rate = "∞ B"
	} else {
		rate = humanize.Bytes(dgs.totalSize / seconds)
	}

	statsFmt := `sharding session successful:
CID: %s
ClusterDAG: %s
Total shards: %d
Total size: %s
Total time: %s
Ingest Rate: %s/s
`

	logger.Infof(
		statsFmt,
		metaPin,
		clusterDAGPin,
		len(dgs.shards),
		humanize.Bytes(dgs.totalSize),
		duration,
		rate,
	)

}

func (dgs *DAGService) sendOutput(ao *api.AddedOutput) {
	if dgs.output != nil {
		dgs.output <- ao
	}
}

// flushes the dgs.currentShard and returns the LastLink()
func (dgs *DAGService) flushCurrentShard(ctx context.Context) (cid.Cid, error) {
	shard := dgs.currentShard
	if shard == nil {
		return cid.Undef, errors.New("cannot flush a nil shard")
	}

	lens := len(dgs.shards)

	shardCid, err := shard.Flush(ctx, lens, dgs.previousShard)
	if err != nil {
		return shardCid, err
	}
	dgs.totalSize += shard.Size()
	dgs.shards[fmt.Sprintf("%d", lens)] = shardCid
	dgs.previousShard = shardCid
	dgs.currentShard = nil
	dgs.sendOutput(&api.AddedOutput{
		Name: fmt.Sprintf("shard-%d", lens),
		Cid:  shardCid,
		Size: shard.Size(),
	})

	return shard.LastLink(), nil
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
