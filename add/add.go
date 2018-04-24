package ipfscluster

import (
	"context"
	"errors"
	"mime/multipart"
	"net/url"
	"strconv"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/importer"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	"github.com/ipfs/go-ipfs-cmdkit/files"
	logging "github.com/ipfs/go-log"
)

// AddSession stores utilities of the calling component needed for the add
type AddSession struct {
	rpcClient *rpc.Client
	logger    logging.EventLogger
}

// NewAddSession creates a new AddSession for adding a file
func NewAddSession(rpcClient *rpc.Client, logger logging.EventLogger) *AddSession {
	return &AddSession{
		rpcClient: rpcClient,
		logger:    logger,
	}
}

func (a *AddSession) consumeLocalAdd(
	arg string,
	outObj *api.NodeWithMeta,
	replMin, replMax int,
) (string, error) {
	//TODO: when ipfs add starts supporting formats other than
	// v0 (v1.cbor, v1.protobuf) we'll need to update this
	outObj.Format = ""

	err := a.rpcClient.Call(
		"",
		"Cluster",
		"IPFSBlockPut",
		*outObj,
		&struct{}{},
	)
	
	return outObj.Cid, err // root node returned in case this is last call
}

func (a *AddSession) finishLocalAdd(rootCid string, replMin, replMax int) error {
	if rootCid == "" {
		return errors.New("no record of root to pin")
	}

	pinS := api.PinSerial{
		Cid:                  rootCid,
		Type:                 api.DataType,
		Recursive:            true,
		ReplicationFactorMin: replMin,
		ReplicationFactorMax: replMax,
	}
	return a.rpcClient.Call(
		"",
		"Cluster",
		"Pin",
		pinS,
		&struct{}{},
	)
}

func (a *AddSession) consumeShardAdd(
	shardID string,
	outObj *api.NodeWithMeta,
	replMin, replMax int,
) (string, error) {
	outObj.ID = shardID
	outObj.ReplMax = replMax
	outObj.ReplMin = replMin
	var retStr string
	err := a.rpcClient.Call(
		"",
		"Cluster",
		"SharderAddNode",
		*outObj,
		&retStr,
	)

	return retStr, err
}

func (a *AddSession) finishShardAdd(
	shardID string,
	replMin, replMax int,
) error {
	if shardID == "" {
		return errors.New("bad state: shardID passed incorrectly")
	}
	return a.rpcClient.Call(
		"",
		"Cluster",
		"SharderFinalize",
		shardID,
		&struct{}{},
	)
}

func (a *AddSession) consumeImport(ctx context.Context,
	outChan <-chan *api.NodeWithMeta,
	printChan <-chan *api.AddedOutput,
	errChan <-chan error,
	consume func(string, *api.NodeWithMeta, int, int) (string, error),
	finish func(string, int, int) error,
	replMin int, replMax int,
) ([]api.AddedOutput, error) {
	var err error
	openChs := 3
	toPrint := make([]api.AddedOutput, 0)
	arg := ""

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

			arg, err = consume(arg, outObj, replMin, replMax)
			if err != nil {
				return nil, err
			}
		}
	}

	if err := finish(arg, replMin, replMax); err != nil {
		return nil, err
	}
	a.logger.Debugf("succeeding sharding import")
	return toPrint, nil
}

// AddFile adds a file to the ipfs daemons of the cluster.  The ipfs importer
// pipeline is used to DAGify the file.  Depending on input parameters this
// DAG can be added locally to the calling cluster peer's ipfs repo, or
// sharded across the entire cluster.
func (a *AddSession) AddFile(ctx context.Context,
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
	hidden, _ := strconv.ParseBool(params.Get("hidden"))
	silent, _ := strconv.ParseBool(params.Get("silent"))

	f := &files.MultipartFile{
		Mediatype: "multipart/form-data",
		Reader:    reader,
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	printChan, outChan, errChan := importer.ToChannel(
		ctx,
		f,
		hidden,
		trickle,
		raw,
		silent,
		chunker,
	)

	shard := params.Get("shard")
	replMin, _ := strconv.Atoi(params.Get("repl_min"))
	replMax, _ := strconv.Atoi(params.Get("repl_max"))

	var consume func(string, *api.NodeWithMeta, int, int) (string, error)
	var finish func(string, int, int) error
	if shard == "true" {
		consume = a.consumeShardAdd
		finish = a.finishShardAdd
	} else {
		consume = a.consumeLocalAdd
		finish = a.finishLocalAdd

	}
	return a.consumeImport(
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
