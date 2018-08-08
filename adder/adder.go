package adder

import (
	"context"
	"io"
	"mime/multipart"

	"github.com/ipfs/ipfs-cluster/adder/ipfsadd"
	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-cmdkit/files"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
)

var logger = logging.Logger("adder")

// ClusterDAGService is an implementation of ipld.DAGService plus a Finalize
// method. ClusterDAGServices can be used to provide Adders with a different
// add implementation.
type ClusterDAGService interface {
	ipld.DAGService
	Finalize(context.Context) (*cid.Cid, error)
}

// Adder is used to add content to IPFS Cluster using an implementation of
// ClusterDAGService.
type Adder struct {
	ctx    context.Context
	cancel context.CancelFunc

	dags ClusterDAGService

	params *api.AddParams

	// AddedOutput updates are placed on this channel
	// whenever a block is processed. They contain information
	// about the block, the CID, the Name etc. and are mostly
	// meant to be streamed back to the user.
	output chan *api.AddedOutput
}

// New returns a new Adder with the given ClusterDAGService, add options and a
// channel to send updates during the adding process.
//
// An Adder may only be used once.
func New(ds ClusterDAGService, p *api.AddParams, out chan *api.AddedOutput) *Adder {
	// Discard all progress update output as the caller has not provided
	// a channel for them to listen on.
	if out == nil {
		out = make(chan *api.AddedOutput, 100)
		go func() {
			for range out {
			}
		}()
	}

	return &Adder{
		dags:   ds,
		params: p,
		output: out,
	}
}

func (a *Adder) setContext(ctx context.Context) {
	ctxc, cancel := context.WithCancel(ctx)
	a.ctx = ctxc
	a.cancel = cancel
}

// FromMultipart adds content from a multipart.Reader. The adder will
// no longer be usable after calling this method.
func (a *Adder) FromMultipart(ctx context.Context, r *multipart.Reader) (*cid.Cid, error) {
	logger.Debugf("adding from multipart with params: %+v", a.params)

	f := &files.MultipartFile{
		Mediatype: "multipart/form-data",
		Reader:    r,
	}
	defer f.Close()
	return a.FromFiles(ctx, f)
}

// FromFiles adds content from a files.File. The adder will no longer
// be usable after calling this method.
func (a *Adder) FromFiles(ctx context.Context, f files.File) (*cid.Cid, error) {
	logger.Debugf("adding from files")
	a.setContext(ctx)

	if a.ctx.Err() != nil { // don't allow running twice
		return nil, a.ctx.Err()
	}

	defer a.cancel()
	defer close(a.output)

	ipfsAdder, err := ipfsadd.NewAdder(a.ctx, a.dags)
	if err != nil {
		return nil, err
	}

	ipfsAdder.Hidden = a.params.Hidden
	ipfsAdder.Trickle = a.params.Layout == "trickle"
	ipfsAdder.RawLeaves = a.params.RawLeaves
	ipfsAdder.Wrap = a.params.Wrap
	ipfsAdder.Chunker = a.params.Chunker
	ipfsAdder.Out = a.output

	for {
		select {
		case <-a.ctx.Done():
			return nil, a.ctx.Err()
		default:
			err := addFile(f, ipfsAdder)
			if err == io.EOF {
				goto FINALIZE
			}
			if err != nil {
				return nil, err
			}
		}
	}

FINALIZE:
	_, err = ipfsAdder.Finalize()
	if err != nil {
		return nil, err
	}

	return a.dags.Finalize(a.ctx)
}

func addFile(fs files.File, ipfsAdder *ipfsadd.Adder) error {
	f, err := fs.NextFile()
	if err != nil {
		return err
	}

	logger.Debugf("ipfsAdder AddFile(%s)", f.FullPath())
	return ipfsAdder.AddFile(f)
}
