package crdt

import (
	"context"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	cid "github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	ipld "github.com/ipfs/go-ipld-format"
)

// A DAGSyncer component implementation using ipfs-lite.
type liteDAGSyncer struct {
	getter ipld.NodeGetter
	dags   ipld.DAGService

	ipfs       *ipfslite.Peer
	blockstore blockstore.Blockstore
}

func newLiteDAGSyncer(ctx context.Context, ipfs *ipfslite.Peer) *liteDAGSyncer {
	return &liteDAGSyncer{
		getter:     ipfs,
		dags:       ipfs,
		ipfs:       ipfs,
		blockstore: ipfs.BlockStore(),
	}
}

func (lds *liteDAGSyncer) HasBlock(c cid.Cid) (bool, error) {
	return lds.ipfs.HasBlock(c)
}

func (lds *liteDAGSyncer) Get(ctx context.Context, c cid.Cid) (ipld.Node, error) {
	return lds.getter.Get(ctx, c)
}

func (lds *liteDAGSyncer) GetMany(ctx context.Context, cs []cid.Cid) <-chan *ipld.NodeOption {
	return lds.getter.GetMany(ctx, cs)
}

func (lds *liteDAGSyncer) Add(ctx context.Context, n ipld.Node) error {
	return lds.dags.Add(ctx, n)
}

func (lds *liteDAGSyncer) AddMany(ctx context.Context, nds []ipld.Node) error {
	return lds.dags.AddMany(ctx, nds)

}

func (lds *liteDAGSyncer) Remove(ctx context.Context, c cid.Cid) error {
	return lds.dags.Remove(ctx, c)
}

func (lds *liteDAGSyncer) RemoveMany(ctx context.Context, cs []cid.Cid) error {
	return lds.dags.RemoveMany(ctx, cs)
}
