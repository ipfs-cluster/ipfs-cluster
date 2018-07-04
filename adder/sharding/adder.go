// package sharding implements a sharding adder that chunks and
// shards content while it's added, creating Cluster DAGs and
// pinning them.
package sharding

import (
	"context"
	"mime/multipart"

	"github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	"github.com/ipfs/go-ipfs-cmdkit/files"
	logging "github.com/ipfs/go-log"
)

var logger = logging.Logger("addshard")

type Adder struct {
	rpcClient *rpc.Client
}

func New(rpc *rpc.Client) *Adder {
	return &Adder{
		rpcClient: rpc,
	}
}

func (a *Adder) FromMultipart(ctx context.Context, r *multipart.Reader, p *adder.Params) error {
	f := &files.MultipartFile{
		Mediatype: "multipart/form-data",
		Reader:    r,
	}

	pinOpts := api.PinOptions{
		ReplicationFactorMin: p.ReplicationFactorMin,
		ReplicationFactorMax: p.ReplicationFactorMax,
		Name:                 p.Name,
		ShardSize:            p.ShardSize,
	}

	dagBuilder := newClusterDAGBuilder(a.rpcClient, pinOpts)
	// Always stop the builder
	defer dagBuilder.Cancel()

	blockSink := dagBuilder.Blocks()

	blockHandle := func(ctx context.Context, n *api.NodeWithMeta) (string, error) {
		blockSink <- n
		return "", nil
	}

	importer, err := adder.NewImporter(f, p)
	if err != nil {
		return err
	}

	_, err = importer.Run(ctx, blockHandle)
	if err != nil {
		return err
	}

	// Trigger shard finalize
	close(blockSink)

	<-dagBuilder.Done() // wait for the builder to finish
	return nil
}
