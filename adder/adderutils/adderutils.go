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
// uploaded using a multipart request. The outputTransform parameter
// allows to customize the http response output format to something
// else than api.AddedOutput objects.
func AddMultipartHTTPHandler(
	ctx context.Context,
	rpc *rpc.Client,
	params *api.AddParams,
	reader *multipart.Reader,
	w http.ResponseWriter,
	outputTransform func(*api.AddedOutput) interface{},
) (cid.Cid, error) {
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
	w.Header().Set("Content-Type", "application/json")
	// Browsers should not cache when streaming content.
	w.Header().Set("Cache-Control", "no-cache")
	// Custom header which breaks js-ipfs-api if not set
	// https://github.com/ipfs-shipyard/ipfs-companion/issues/600
	w.Header().Set("X-Chunked-Output", "1")

	// Used by go-ipfs to signal errors half-way through the stream.
	w.Header().Set("Trailer", "X-Stream-Error")

	// We need to ask the clients to close the connection
	// (no keep-alive) of things break badly when adding.
	// https://github.com/ipfs/go-ipfs-cmds/pull/116
	w.Header().Set("Connection", "close")
	w.WriteHeader(http.StatusOK)

	if outputTransform == nil {
		outputTransform = func(in *api.AddedOutput) interface{} { return in }
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for v := range output {
			err := enc.Encode(outputTransform(v))
			if err != nil {
				logger.Error(err)
				break
			}
			if flush {
				flusher.Flush()
			}
		}
	}()

	add := adder.New(dags, params, output)
	root, err := add.FromMultipart(ctx, reader)
	if err != nil {
		// Set trailer with error
		w.Header().Set("X-Stream-Error", err.Error())
	}
	wg.Wait()
	return root, err
}
