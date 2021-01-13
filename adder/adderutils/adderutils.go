// Package adderutils provides some utilities for adding content to cluster.
package adderutils

import (
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"sync"

	"github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/adder/sharding"
	"github.com/ipfs/ipfs-cluster/adder/single"
	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	rpc "github.com/libp2p/go-libp2p-gorpc"
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

	if params.Shard {
		dags = sharding.New(rpc, params.PinOptions, output)
	} else {
		dags = single.New(rpc, params.PinOptions, params.Local)
	}

	if outputTransform == nil {
		outputTransform = func(in *api.AddedOutput) interface{} { return in }
	}

	// This must be application/json otherwise go-ipfs client
	// will break.
	w.Header().Set("Content-Type", "application/json")
	// Browsers should not cache these responses.
	w.Header().Set("Cache-Control", "no-cache")
	// We need to ask the clients to close the connection
	// (no keep-alive) of things break badly when adding.
	// https://github.com/ipfs/go-ipfs-cmds/pull/116
	w.Header().Set("Connection", "close")

	var wg sync.WaitGroup
	if !params.StreamChannels {
		// in this case we buffer responses in memory and
		// return them as a valid JSON array.
		wg.Add(1)
		var bufOutput []interface{} // a slice of transformed AddedOutput
		go func() {
			defer wg.Done()
			bufOutput = buildOutput(output, outputTransform)
		}()

		enc := json.NewEncoder(w)
		add := adder.New(dags, params, output)
		root, err := add.FromMultipart(ctx, reader)
		if err != nil { // Send an error
			logger.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			errorResp := api.Error{
				Code:    http.StatusInternalServerError,
				Message: err.Error(),
			}

			if err := enc.Encode(errorResp); err != nil {
				logger.Error(err)
			}
			wg.Wait()
			return root, err
		}
		wg.Wait()
		w.WriteHeader(http.StatusOK)
		enc.Encode(bufOutput)
		return root, err
	}

	// handle stream-adding. This should be the default.

	// https://github.com/ipfs-shipyard/ipfs-companion/issues/600
	w.Header().Set("X-Chunked-Output", "1")
	// Used by go-ipfs to signal errors half-way through the stream.
	w.Header().Set("Trailer", "X-Stream-Error")
	w.WriteHeader(http.StatusOK)
	wg.Add(1)
	go func() {
		defer wg.Done()
		streamOutput(w, output, outputTransform)
	}()
	add := adder.New(dags, params, output)
	root, err := add.FromMultipart(ctx, reader)
	if err != nil {
		logger.Error(err)
		// Set trailer with error
		w.Header().Set("X-Stream-Error", err.Error())
	}
	wg.Wait()
	return root, err
}

func streamOutput(w http.ResponseWriter, output chan *api.AddedOutput, transform func(*api.AddedOutput) interface{}) {
	flusher, flush := w.(http.Flusher)
	enc := json.NewEncoder(w)
	for v := range output {
		err := enc.Encode(transform(v))
		if err != nil {
			logger.Error(err)
			break
		}
		if flush {
			flusher.Flush()
		}
	}
}

func buildOutput(output chan *api.AddedOutput, transform func(*api.AddedOutput) interface{}) []interface{} {
	var finalOutput []interface{}
	for v := range output {
		finalOutput = append(finalOutput, transform(v))
	}
	return finalOutput
}
