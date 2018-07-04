package sharding

import (
	"context"
	"errors"
	"fmt"

	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
)

// A clusterDAGBuilder is in charge of creating a full cluster dag upon receiving
// a stream of blocks (NodeWithMeta)
type clusterDAGBuilder struct {
	ctx    context.Context
	cancel context.CancelFunc

	pinOpts api.PinOptions

	rpc *rpc.Client

	blocks chan *api.NodeWithMeta

	// Current shard being built
	currentShard *shard

	// shard tracking
	shards map[string]*cid.Cid
}

func newClusterDAGBuilder(rpc *rpc.Client, opts api.PinOptions) *clusterDAGBuilder {
	ctx, cancel := context.WithCancel(context.Background())

	// By caching one node don't block sending something
	// to the channel.
	blocks := make(chan *api.NodeWithMeta, 1)

	cdb := &clusterDAGBuilder{
		ctx:     ctx,
		cancel:  cancel,
		rpc:     rpc,
		blocks:  blocks,
		pinOpts: opts,
		shards:  make(map[string]*cid.Cid),
	}
	go cdb.ingestBlocks()
	return cdb
}

// Blocks returns a channel on which to send blocks to be processed by this
// clusterDAGBuilder (ingested). Close channel when done.
func (cdb *clusterDAGBuilder) Blocks() chan<- *api.NodeWithMeta {
	return cdb.blocks
}

func (cdb *clusterDAGBuilder) Done() <-chan struct{} {
	return cdb.ctx.Done()
}

func (cdb *clusterDAGBuilder) Cancel() {
	cdb.cancel()
}

// shortcut to pin something
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

	rootCid, err := lastShard.Flush(cdb.ctx, cdb.pinOpts, len(cdb.shards))
	if err != nil {
		return err
	}

	// Do not forget this shard
	cdb.shards[fmt.Sprintf("%d", len(cdb.shards))] = rootCid

	shardNodes, err := makeDAG(cdb.shards)
	if err != nil {
		return err
	}

	err = putDAG(cdb.ctx, cdb.rpc, shardNodes, "")
	if err != nil {
		return err
	}

	dataRootCid := lastShard.LastLink()
	clusterDAG := shardNodes[0].Cid()

	// Pin the META pin
	metaPin := api.PinWithOpts(dataRootCid, cdb.pinOpts)
	metaPin.Type = api.MetaType
	metaPin.ClusterDAG = clusterDAG
	metaPin.MaxDepth = 0 // irrelevant. Meta-pins are not pinned
	err = cdb.pin(metaPin)
	if err != nil {
		return err
	}

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

	cdb.cancel() // auto-cancel the builder. We're done.
	return nil
}

func (cdb *clusterDAGBuilder) ingestBlocks() {
	// TODO: handle errors somehow
	for {
		select {
		case <-cdb.ctx.Done():
			return
		case n, ok := <-cdb.blocks:
			if !ok {
				err := cdb.finalize()
				if err != nil {
					logger.Error(err)
					// TODO: handle
				}
				return
			}
			err := cdb.ingestBlock(n)
			if err != nil {
				logger.Error(err)
				// TODO: handle
			}
		}
	}
}

// ingests a block to the current shard. If it get's full, it
// Flushes the shard and retries with a new one.
func (cdb *clusterDAGBuilder) ingestBlock(n *api.NodeWithMeta) error {
	shard := cdb.currentShard

	// if we have no currentShard, create one
	if shard == nil {
		var err error
		shard, err = newShard(cdb.ctx, cdb.rpc, cdb.pinOpts.ShardSize)
		if err != nil {
			return err
		}
		cdb.currentShard = shard
	}

	c, err := cid.Decode(n.Cid)
	if err != nil {
		return err
	}

	// add the block to it if it fits and return
	if shard.Size()+n.Size < shard.Limit() {
		shard.AddLink(c, n.Size)
		return cdb.putBlock(n, shard.DestPeer())
	}

	// block doesn't fit in shard

	// if shard is empty, error
	if shard.Size() == 0 {
		return errors.New("block doesn't fit in empty shard")
	}

	// otherwise, shard considered full. Flush and pin result
	rootCid, err := shard.Flush(cdb.ctx, cdb.pinOpts, len(cdb.shards))
	if err != nil {
		return err
	}
	// Do not forget this shard
	cdb.shards[fmt.Sprintf("%d", len(cdb.shards))] = rootCid
	cdb.currentShard = nil
	return cdb.ingestBlock(n) // <-- retry ingest
}

// performs an IPFSBlockPut
func (cdb *clusterDAGBuilder) putBlock(n *api.NodeWithMeta, dest peer.ID) error {
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
	return cdb.rpc.CallContext(
		cdb.ctx,
		dest,
		"Cluster",
		"IPFSBlockPut",
		n,
		&struct{}{},
	)
}
