// Package local implements an ipfs-cluster Adder that chunks and adds content
// to a local peer, before pinning it.
package local

import (
	"context"
	"errors"
	"mime/multipart"

	"github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/rpcutil"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-cmdkit/files"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
)

var logger = logging.Logger("addlocal")
var outputBuffer = 200

// Adder is an implementation of IPFS Cluster Adder interface,
// which allows adding content directly to IPFS daemons attached
// to the Cluster (without sharding).
type Adder struct {
	rpcClient *rpc.Client

	output chan *api.AddedOutput
}

// New returns a new Adder with the given rpc Client. The client is used
// to perform calls to IPFSBlockPut and Pin content on Cluster.
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

func (a *Adder) putBlock(ctx context.Context, n *api.NodeWithMeta, dests []peer.ID) error {
	logger.Debugf("put block: %s", n.Cid)
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

	ctxs, cancels := rpcutil.CtxsWithCancel(ctx, len(dests))
	defer rpcutil.MultiCancel(cancels)

	logger.Debugf("block put %s", n.Cid)
	errs := a.rpcClient.MultiCall(
		ctxs,
		dests,
		"Cluster",
		"IPFSBlockPut",
		*n,
		rpcutil.RPCDiscardReplies(len(dests)),
	)
	return rpcutil.CheckErrs(errs)
}

// FromMultipart allows to add a file encoded as multipart.
func (a *Adder) FromMultipart(ctx context.Context, r *multipart.Reader, p *api.AddParams) (*cid.Cid, error) {
	f := &files.MultipartFile{
		Mediatype: "multipart/form-data",
		Reader:    r,
	}
	defer close(a.output)
	defer f.Close()

	ctxRun, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	var allocsStr []string
	err := a.rpcClient.CallContext(
		ctx,
		"",
		"Cluster",
		"Allocate",
		api.PinWithOpts(nil, p.PinOptions).ToSerial(),
		&allocsStr,
	)
	if err != nil {
		return nil, err
	}

	allocations := api.StringsToPeers(allocsStr)

	localBlockPut := func(ctx context.Context, n *api.NodeWithMeta) (string, error) {
		retVal := n.Cid
		return retVal, a.putBlock(ctx, n, allocations)
	}

	importer, err := adder.NewImporter(f, p, a.output)
	if err != nil {
		return nil, err
	}

	lastCidStr, err := importer.Run(ctxRun, localBlockPut)
	if err != nil {
		cancelRun()
		return nil, err
	}

	lastCid, err := cid.Decode(lastCidStr)
	if err != nil {
		return nil, errors.New("nothing imported: invalid Cid")
	}

	// Finally, cluster pin the result
	pinS := api.PinSerial{
		Cid:      lastCidStr,
		Type:     int(api.DataType),
		MaxDepth: -1,
		PinOptions: api.PinOptions{
			ReplicationFactorMin: p.ReplicationFactorMin,
			ReplicationFactorMax: p.ReplicationFactorMax,
			Name:                 p.Name,
		},
	}
	err = a.rpcClient.CallContext(
		ctx,
		"",
		"Cluster",
		"Pin",
		pinS,
		&struct{}{},
	)
	return lastCid, err
}
