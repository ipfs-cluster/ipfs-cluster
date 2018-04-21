package ipfscluster

import (
	"context"
	"errors"
	"mime/multipart"
	"net/url"
	"strconv"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/importer"

	"github.com/ipfs/go-ipfs-cmdkit/files"
)

func (c *Cluster) consumeLocalAdd(
	args map[string]string,
	outObj *api.NodeWithMeta,
	replMin, replMax int,
) error {
	//TODO: when ipfs add starts supporting formats other than
	// v0 (v1.cbor, v1.protobuf) we'll need to update this
	outObj.Format = ""
	args["cid"] = outObj.Cid // root node stored on last call
	var hash string
	err := c.rpcClient.Call(
		"",
		"Cluster",
		"IPFSBlockPut",
		*outObj,
		&hash)
	if outObj.Cid != hash {
		logger.Warningf("mismatch. node cid: %s\nrpc cid: %s", outObj.Cid, hash)
	}
	return err
}

func (c *Cluster) finishLocalAdd(
	args map[string]string,
	replMin, replMax int,
) error {
	rootCid, ok := args["cid"]
	if !ok {
		return errors.New("no record of root to pin")
	}

	pinS := api.PinSerial{
		Cid:                  rootCid,
		Type:                 api.DataType,
		Recursive:            true,
		ReplicationFactorMin: replMin,
		ReplicationFactorMax: replMax,
	}
	return c.rpcClient.Call(
		"",
		"Cluster",
		"Pin",
		pinS,
		&struct{}{},
	)
}

func (c *Cluster) consumeShardAdd(
	args map[string]string,
	outObj *api.NodeWithMeta,
	replMin, replMax int,
) error {

	var shardID string
	shardID, ok := args["id"]
	outObj.ID = shardID
	outObj.ReplMax = replMax
	outObj.ReplMin = replMin
	var retStr string
	err := c.rpcClient.Call(
		"",
		"Cluster",
		"SharderAddNode",
		*outObj,
		&retStr)
	if !ok {
		args["id"] = retStr
	}
	return err
}

func (c *Cluster) finishShardAdd(
	args map[string]string,
	replMin, replMax int,
) error {
	shardID, ok := args["id"]
	if !ok {
		return errors.New("bad state: shardID passed incorrectly")
	}
	err := c.rpcClient.Call(
		"",
		"Cluster",
		"SharderFinalize",
		shardID,
		&struct{}{},
	)
	return err
}

func (c *Cluster) consumeImport(ctx context.Context,
	outChan <-chan *api.NodeWithMeta,
	printChan <-chan *api.AddedOutput,
	errChan <-chan error,
	consume func(map[string]string, *api.NodeWithMeta, int, int) error,
	finish func(map[string]string, int, int) error,
	replMin int, replMax int,
) ([]api.AddedOutput, error) {
	var err error
	openChs := 3
	toPrint := make([]api.AddedOutput, 0)
	args := make(map[string]string)

	for {
		if openChs == 0 {
			break
		}

		// Consume signals from importer.  Errors resulting from
		select {
		// Ensure we terminate reading from importer after cancellation
		// but do not block
		case <-ctx.Done():
			err = errors.New("context timeout terminated add")
			return nil, err
		// Terminate session when importer throws an error
		case err, ok := <-errChan:
			if !ok {
				openChs--
				errChan = nil
				continue
			}
			return nil, err

		// Send status information to client for user output
		case printObj, ok := <-printChan:
			//TODO: if we support progress bar we must update this
			if !ok {
				openChs--
				printChan = nil
				continue
			}
			toPrint = append(toPrint, *printObj)
		// Consume ipld node output by importer
		case outObj, ok := <-outChan:
			if !ok {
				openChs--
				outChan = nil
				continue
			}

			if err := consume(args, outObj, replMin, replMax); err != nil {
				return nil, err
			}
		}
	}

	if err := finish(args, replMin, replMax); err != nil {
		return nil, err
	}
	logger.Debugf("succeeding sharding import")
	return toPrint, nil
}

// AddFile adds a file to the ipfs daemons of the cluster.  The ipfs importer
// pipeline is used to DAGify the file.  Depending on input parameters this
// DAG can be added locally to the calling cluster peer's ipfs repo, or
// sharded across the entire cluster.
func (c *Cluster) AddFile(
	reader *multipart.Reader,
	params url.Values,
) ([]api.AddedOutput, error) {
	layout := params.Get("layout")
	trickle := false
	if layout == "trickle" {
		trickle = true
	}
	chunker := params.Get("chunker")
	raw, _ := strconv.ParseBool(params.Get("raw"))
	wrap, _ := strconv.ParseBool(params.Get("wrap"))
	progress, _ := strconv.ParseBool(params.Get("progress"))
	hidden, _ := strconv.ParseBool(params.Get("hidden"))
	silent, _ := strconv.ParseBool(params.Get("silent"))

	f := &files.MultipartFile{
		Mediatype: "multipart/form-data",
		Reader:    reader,
	}
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()
	printChan, outChan, errChan := importer.ToChannel(
		ctx,
		f,
		progress,
		hidden,
		trickle,
		raw,
		silent,
		wrap,
		chunker,
	)

	shard := params.Get("shard")
	replMin, _ := strconv.Atoi(params.Get("repl_min"))
	replMax, _ := strconv.Atoi(params.Get("repl_max"))

	var consume func(map[string]string, *api.NodeWithMeta, int, int) error
	var finish func(map[string]string, int, int) error
	if shard == "true" {
		consume = c.consumeShardAdd
		finish = c.finishShardAdd
	} else {
		consume = c.consumeLocalAdd
		finish = c.finishLocalAdd

	}
	return c.consumeImport(
		ctx,
		outChan,
		printChan,
		errChan,
		consume,
		finish,
		replMin,
		replMax,
	)
}
