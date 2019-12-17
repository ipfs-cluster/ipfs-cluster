// Package adder implements functionality to add content to IPFS daemons
// managed by the Cluster.
package adder

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"io/ioutil"
	"mime/multipart"

	"github.com/ipfs/ipfs-cluster/adder/ipfsadd"
	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
)

var logger = logging.Logger("adder")

// ClusterDAGService is an implementation of ipld.DAGService plus a Finalize
// method. ClusterDAGServices can be used to provide Adders with a different
// add implementation.
type ClusterDAGService interface {
	ipld.DAGService
	// Finalize receives the IPFS content root CID as
	// returned by the ipfs adder.
	Finalize(ctx context.Context, ipfsRoot cid.Cid) (cid.Cid, error)
}

// Adder is used to add content to IPFS Cluster using an implementation of
// ClusterDAGService.
type Adder struct {
	ctx    context.Context
	cancel context.CancelFunc

	dgs ClusterDAGService

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
		dgs:    ds,
		params: p,
		output: out,
	}
}

func (a *Adder) setContext(ctx context.Context) {
	if a.ctx == nil { // only allows first context
		ctxc, cancel := context.WithCancel(ctx)
		a.ctx = ctxc
		a.cancel = cancel
	}
}

// FromMultipart adds content from a multipart.Reader. The adder will
// no longer be usable after calling this method.
func (a *Adder) FromMultipart(ctx context.Context, r *multipart.Reader) (cid.Cid, error) {
	logger.Debugf("adding from multipart with params: %+v", a.params)

	var f files.Directory
	var err error
	if a.params.Untar {
		// TODO: If tar contains individual files instead of files wrapped in dir,
		// pin those individual files. (currently it doesn't pin)
		pipeReader, pipeWriter := io.Pipe()

		go func() {
			for {
				p, err := r.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					logger.Error(err)
					return
				}
				zr, err := gzip.NewReader(p)
				if err != nil {
					logger.Error(err)
					return
				}

				slurp, err := ioutil.ReadAll(zr)
				if err != nil {
					logger.Error(err)
					return
				}
				_, err = pipeWriter.Write(slurp)
				if err != nil {
					logger.Error(err)
					return
				}
			}
		}()

		tr := tar.NewReader(pipeReader)
		f, err = tarToSliceDirectory(tr)
	} else {
		f, err = files.NewFileFromPartReader(r, "multipart/form-data")
	}
	if err != nil {
		return cid.Undef, err
	}
	defer f.Close()

	return a.FromFiles(ctx, f)
}

// FromFiles adds content from a files.Directory. The adder will no longer
// be usable after calling this method.
func (a *Adder) FromFiles(ctx context.Context, f files.Directory) (cid.Cid, error) {
	logger.Debug("adding from files")
	a.setContext(ctx)

	if a.ctx.Err() != nil { // don't allow running twice
		return cid.Undef, a.ctx.Err()
	}

	defer a.cancel()
	defer close(a.output)

	ipfsAdder, err := ipfsadd.NewAdder(a.ctx, a.dgs, a.params)
	if err != nil {
		logger.Error(err)
		return cid.Undef, err
	}

	ipfsAdder.Out = a.output

	// setup wrapping
	if a.params.Wrap {
		f = files.NewSliceDirectory(
			[]files.DirEntry{files.FileEntry("", f)},
		)
	}

	it := f.Entries()
	var adderRoot ipld.Node
	for it.Next() {
		select {
		case <-a.ctx.Done():
			return cid.Undef, a.ctx.Err()
		default:
			logger.Debugf("ipfsAdder AddFile(%s)", it.Name())

			adderRoot, err = ipfsAdder.AddAllAndPin(it.Node())
			if err != nil {
				logger.Error("error adding to cluster: ", err)
				return cid.Undef, err
			}
		}
	}
	if it.Err() != nil {
		return cid.Undef, it.Err()
	}

	clusterRoot, err := a.dgs.Finalize(a.ctx, adderRoot.Cid())
	if err != nil {
		logger.Error("error finalizing adder:", err)
		return cid.Undef, err
	}
	logger.Infof("%s successfully added to cluster", clusterRoot)
	return clusterRoot, nil
}
