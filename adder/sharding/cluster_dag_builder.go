package sharding

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/rpcutil"

	humanize "github.com/dustin/go-humanize"
	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
)

// A clusterDAGBuilder is in charge of creating a full cluster dag upon receiving
// a stream of blocks (NodeWithMeta)
type clusterDAGBuilder struct {
	ctx    context.Context
	cancel context.CancelFunc
	error  error

	pinOpts api.PinOptions

	rpc *rpc.Client

	blocks chan *api.NodeWithMeta

	// Current shard being built
	currentShard *shard

	// shard tracking
	shards map[string]*cid.Cid

	startTime time.Time
	totalSize uint64
}

func newClusterDAGBuilder(rpc *rpc.Client, opts api.PinOptions) *clusterDAGBuilder {
	ctx, cancel := context.WithCancel(context.Background())

	// By caching one node don't block sending something
	// to the channel.
	blocks := make(chan *api.NodeWithMeta, 0)

	cdb := &clusterDAGBuilder{
		ctx:       ctx,
		cancel:    cancel,
		rpc:       rpc,
		blocks:    blocks,
		pinOpts:   opts,
		shards:    make(map[string]*cid.Cid),
		startTime: time.Now(),
	}
	go cdb.ingestBlocks()
	return cdb
}

// Blocks returns a channel on which to send blocks to be processed by this
// clusterDAGBuilder (ingested). Close channel when done.
func (cdb *clusterDAGBuilder) Blocks() chan<- *api.NodeWithMeta {
	return cdb.blocks
}

// Done returns a channel that is closed when the clusterDAGBuilder has finished
// processing blocks. Use Err() to check for any errors after done.
func (cdb *clusterDAGBuilder) Done() <-chan struct{} {
	return cdb.ctx.Done()
}

// Err returns any error after the clusterDAGBuilder is Done().
// Err always returns nil if not Done().
func (cdb *clusterDAGBuilder) Err() error {
	select {
	case <-cdb.ctx.Done():
		return cdb.error
	default:
		return nil
	}
}

// Cancel cancels the clusterDAGBulder and all associated operations
func (cdb *clusterDAGBuilder) Cancel() {
	cdb.cancel()
}

// shortcut to pin something in Cluster
func (cdb *clusterDAGBuilder) pin(p api.Pin) error {
	return cdb.rpc.CallContext(
		cdb.ctx,
		"",
		"Cluster",
		"Pin",
		p.ToSerial(),
		&struct{}{},
	)
}

// finalize is used to signal that we need to wrap up this clusterDAG
//. It is called when the Blocks() channel is closed.
func (cdb *clusterDAGBuilder) finalize() error {
	lastShard := cdb.currentShard
	if lastShard == nil {
		return errors.New("cannot finalize a ClusterDAG with no shards")
	}

	lastShardCid, err := lastShard.Flush(cdb.ctx, len(cdb.shards))
	if err != nil {
		return err
	}

	cdb.totalSize += lastShard.Size()

	// Do not forget this shard
	cdb.shards[fmt.Sprintf("%d", len(cdb.shards))] = lastShardCid

	clusterDAGNodes, err := makeDAG(cdb.shards)
	if err != nil {
		return err
	}

	// PutDAG to ourselves
	err = putDAG(cdb.ctx, cdb.rpc, clusterDAGNodes, []peer.ID{""})
	if err != nil {
		return err
	}

	dataRootCid := lastShard.LastLink()
	clusterDAG := clusterDAGNodes[0].Cid()

	// Pin the ClusterDAG
	clusterDAGPin := api.PinCid(clusterDAG)
	clusterDAGPin.ReplicationFactorMin = -1
	clusterDAGPin.ReplicationFactorMax = -1
	clusterDAGPin.MaxDepth = 0 // pin direct
	clusterDAGPin.Name = fmt.Sprintf("%s-clusterDAG", cdb.pinOpts.Name)
	clusterDAGPin.Type = api.ClusterDAGType
	clusterDAGPin.Parents = cid.NewSet()
	clusterDAGPin.Parents.Add(dataRootCid)
	err = cdb.pin(clusterDAGPin)
	if err != nil {
		return err
	}

	// Pin the META pin
	metaPin := api.PinWithOpts(dataRootCid, cdb.pinOpts)
	metaPin.Type = api.MetaType
	metaPin.ClusterDAG = clusterDAG
	metaPin.MaxDepth = 0 // irrelevant. Meta-pins are not pinned
	err = cdb.pin(metaPin)
	if err != nil {
		return err
	}

	// Log some stats
	cdb.logStats(metaPin.Cid, clusterDAGPin.Cid)

	// Consider doing this? Seems like overkill
	//
	// // Ammend ShardPins to reference clusterDAG root hash as a Parent
	// shardParents := cid.NewSet()
	// shardParents.Add(clusterDAG)
	// for shardN, shard := range cdb.shardNodes {
	// 	pin := api.PinWithOpts(shard, cdb.pinOpts)
	// 	pin.Name := fmt.Sprintf("%s-shard-%s", pin.Name, shardN)
	// 	pin.Type = api.ShardType
	// 	pin.Parents = shardParents
	// 	// FIXME: We don't know anymore the shard pin maxDepth
	// 	err := cdb.pin(pin)
	// 	if err != nil {
	// 		return err
	// 	}
	// }

	return nil
}

func (cdb *clusterDAGBuilder) ingestBlocks() {
	// if this function returns, it means we are Done().
	// we auto-cancel ourselves in that case.
	// if it was due to an error, it will be in Err().
	defer cdb.Cancel()

	for {
		select {
		case <-cdb.ctx.Done(): // cancelled from outside
			return
		case n, ok := <-cdb.blocks:
			if !ok {
				err := cdb.finalize()
				if err != nil {
					logger.Error(err)
					cdb.error = err
				}
				return // will cancel on defer
			}
			err := cdb.ingestBlock(n)
			if err != nil {
				logger.Error(err)
				cdb.error = err
				return // will cancel on defer
			}
			// continue with next block
		}
	}
}

// ingests a block to the current shard. If it get's full, it
// Flushes the shard and retries with a new one.
func (cdb *clusterDAGBuilder) ingestBlock(n *api.NodeWithMeta) error {
	shard := cdb.currentShard

	// if we have no currentShard, create one
	if shard == nil {
		logger.Infof("new shard for '%s': #%d", cdb.pinOpts.Name, len(cdb.shards))
		var err error
		shard, err = newShard(cdb.ctx, cdb.rpc, cdb.pinOpts)
		if err != nil {
			return err
		}
		cdb.currentShard = shard
	}

	logger.Debugf("ingesting block %s in shard %d (%s)", n.Cid, len(cdb.shards), cdb.pinOpts.Name)

	c, err := cid.Decode(n.Cid)
	if err != nil {
		return err
	}

	// add the block to it if it fits and return
	if shard.Size()+n.Size() < shard.Limit() {
		shard.AddLink(c, n.Size())
		return cdb.putBlock(n, shard.Allocations())
	}

	logger.Debugf("shard %d full: block: %d. shard: %d. limit: %d",
		len(cdb.shards),
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

	// otherwise, shard considered full. Flush and pin result
	logger.Debugf("flushing shard %d", len(cdb.shards))
	shardCid, err := shard.Flush(cdb.ctx, len(cdb.shards))
	if err != nil {
		return err
	}
	cdb.totalSize += shard.Size()

	// Do not forget this shard
	cdb.shards[fmt.Sprintf("%d", len(cdb.shards))] = shardCid
	cdb.currentShard = nil
	return cdb.ingestBlock(n) // <-- retry ingest
}

// performs an IPFSBlockPut of this Node to the given destinations
func (cdb *clusterDAGBuilder) putBlock(n *api.NodeWithMeta, dests []peer.ID) error {
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

	ctxs, cancels := rpcutil.CtxsWithCancel(cdb.ctx, len(dests))
	defer rpcutil.MultiCancel(cancels)

	logger.Debugf("block put %s", n.Cid)
	errs := cdb.rpc.MultiCall(
		ctxs,
		dests,
		"Cluster",
		"IPFSBlockPut",
		*n,
		rpcutil.RPCDiscardReplies(len(dests)),
	)
	return rpcutil.CheckErrs(errs)
}

func (cdb *clusterDAGBuilder) logStats(metaPin, clusterDAGPin *cid.Cid) {
	duration := time.Since(cdb.startTime)
	seconds := uint64(duration) / uint64(time.Second)
	var rate string
	if seconds == 0 {
		rate = "∞ B"
	} else {
		rate = humanize.Bytes(cdb.totalSize / seconds)
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
		len(cdb.shards),
		humanize.Bytes(cdb.totalSize),
		duration,
		rate,
	)

}
