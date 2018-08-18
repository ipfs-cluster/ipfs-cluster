// Package adderutils provides some utilities for adding content to cluster.
package adderutils

import (
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"sync"

	"github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/adder/local"
	"github.com/ipfs/ipfs-cluster/adder/sharding"
	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
)

var logger = logging.Logger("adder")

// AddMultipartHTTPHandler is a helper function to add content
// uploaded using a multipart request.
func AddMultipartHTTPHandler(
	ctx context.Context,
	rpc *rpc.Client,
	params *api.AddParams,
	reader *multipart.Reader,
	w http.ResponseWriter,
) (*cid.Cid, error) {
	var dags adder.ClusterDAGService
	output := make(chan *api.AddedOutput, 200)
	flusher, flush := w.(http.Flusher)
	if params.Shard {
		dags = sharding.New(rpc, params.PinOptions, output)
	} else {
		dags = local.New(rpc, params.PinOptions)
	}

	enc := json.NewEncoder(w)
	// This must be application/json otherwise go-ipfs client
	// will break.
	w.Header().Add("Content-Type", "application/json")
	// Browsers should not cache when streaming content.
	w.Header().Add("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for v := range output {
			err := enc.Encode(v)
			if err != nil {
				logger.Error(err)
			}
			if flush {
				flusher.Flush()
			}
		}
	}()

	add := adder.New(dags, params, output)
	root, err := add.FromMultipart(ctx, reader)
	wg.Wait()
	return root, err
}
