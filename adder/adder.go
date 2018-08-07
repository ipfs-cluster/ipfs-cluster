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
// method. ClusterDAGServices can be used to create Adders with different
// add implementations.
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
	output chan *api.AddedOutput
}

// New returns a new Adder with the given ClusterDAGService, add optios and a
// channel to send updates during the adding process.
//
// An Adder may only be used once.
func New(ctx context.Context, ds ClusterDAGService, p *api.AddParams, out chan *api.AddedOutput) *Adder {
	ctx2, cancel := context.WithCancel(ctx)

	// Discard all output
	if out == nil {
		out = make(chan *api.AddedOutput, 100)
		go func() {
			for range out {
			}
		}()
	}

	return &Adder{
		ctx:    ctx2,
		cancel: cancel,
		dags:   ds,
		params: p,
		output: out,
	}
}

// FromMultipart adds content from a multipart.Reader. The adder will
// no longer be usable after calling this method.
func (a *Adder) FromMultipart(r *multipart.Reader) (*cid.Cid, error) {
	logger.Debugf("adding from multipart with params: %+v", a.params)

	f := &files.MultipartFile{
		Mediatype: "multipart/form-data",
		Reader:    r,
	}
	defer f.Close()
	return a.FromFiles(f)
}

// FromFiles adds content from a files.File. The adder will no longer
// be usable after calling this method.
func (a *Adder) FromFiles(f files.File) (*cid.Cid, error) {
	logger.Debugf("adding from files")
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
	ipfsAdder.Wrap = true
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
