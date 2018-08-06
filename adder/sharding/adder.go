// Package sharding implements a sharding adder that chunks and
// shards content while it's added, creating Cluster DAGs and
// pinning them.
package sharding

import (
	"context"
	"fmt"
	"mime/multipart"

	"github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-cmdkit/files"
	logging "github.com/ipfs/go-log"
)

var logger = logging.Logger("addshard")
var outputBuffer = 200

// Adder is an implementation of IPFS Cluster's Adder interface which
// shards content while adding among several IPFS Cluster peers,
// creating a Cluster DAG to track and pin that content selectively
// in the IPFS daemons allocated to it.
type Adder struct {
	rpcClient *rpc.Client

	output chan *api.AddedOutput
}

// New returns a new Adder, which uses the given rpc client to perform
// Allocate, IPFSBlockPut and Pin requests to other cluster components.
func New(rpc *rpc.Client, discardOutput bool) *Adder {
	output := make(chan *api.AddedOutput, outputBuffer)
	if discardOutput {
		go func() {
			for range output {
			}
		}()
	}

	return &Adder{
		rpcClient: rpc,
		output:    output,
	}
}

// Output returns a channel for output updates during the adding process.
func (a *Adder) Output() <-chan *api.AddedOutput {
	return a.output
}

// FromMultipart allows to add (and shard) a file encoded as multipart.
func (a *Adder) FromMultipart(ctx context.Context, r *multipart.Reader, p *api.AddParams) (*cid.Cid, error) {
	logger.Debugf("adding from multipart with params: %+v", p)

	f := &files.MultipartFile{
		Mediatype: "multipart/form-data",
		Reader:    r,
	}
	defer close(a.output)
	defer f.Close()

	ctxRun, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	pinOpts := api.PinOptions{
		ReplicationFactorMin: p.ReplicationFactorMin,
		ReplicationFactorMax: p.ReplicationFactorMax,
		Name:                 p.Name,
		ShardSize:            p.ShardSize,
	}

	dagBuilder := newClusterDAGBuilder(a.rpcClient, pinOpts, a.output)
	// Always stop the builder
	defer dagBuilder.Cancel()

	blockHandle := func(ctx context.Context, n *api.NodeWithMeta) (string, error) {
		logger.Debugf("handling block %s (size %d)", n.Cid, n.Size())
		select {
		case <-dagBuilder.Done():
			return "", dagBuilder.Err()
		case <-ctx.Done():
			return "", ctx.Err()
		case dagBuilder.Blocks() <- n:
			return n.Cid, nil
		}
	}

	logger.Debug("creating importer")
	importer, err := adder.NewImporter(f, p, a.output)
	if err != nil {
		return nil, err
	}

	logger.Infof("importing file to Cluster (name '%s')", p.Name)
	rootCidStr, err := importer.Run(ctxRun, blockHandle)
	if err != nil {
		cancelRun()
		logger.Error("Importing aborted: ", err)
		return nil, err
	}

	// Trigger shard finalize
	close(dagBuilder.Blocks())

	select {
	case <-dagBuilder.Done(): // wait for the builder to finish
		err = dagBuilder.Err()
	case <-ctx.Done():
		err = ctx.Err()
	}

	if err != nil {
		logger.Info("import process finished with error: ", err)
		return nil, err
	}

	rootCid, err := cid.Decode(rootCidStr)
	if err != nil {
		return nil, fmt.Errorf("bad root cid: %s", err)
	}

	logger.Info("import process finished successfully")
	return rootCid, nil
}
