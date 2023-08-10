// Package adder implements functionality to add content to IPFS daemons
// managed by the Cluster.
package adder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"strings"

	"github.com/ipfs-cluster/ipfs-cluster/adder/ipfsadd"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs/boxo/ipld/unixfs"
	"github.com/ipld/go-car"
	peer "github.com/libp2p/go-libp2p/core/peer"

	files "github.com/ipfs/boxo/files"
	merkledag "github.com/ipfs/boxo/ipld/merkledag"
	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	ipldlegacy "github.com/ipfs/go-ipld-legacy"
	logging "github.com/ipfs/go-log/v2"
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/codec/raw"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/multicodec"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	multihash "github.com/multiformats/go-multihash"
)

var logger = logging.Logger("adder")

var ipldDecoder *ipldlegacy.Decoder

// create an ipld registry specific to this package
func init() {
	mcReg := multicodec.Registry{}
	mcReg.RegisterDecoder(cid.DagProtobuf, dagpb.Decode)
	mcReg.RegisterDecoder(cid.Raw, raw.Decode)
	mcReg.RegisterDecoder(cid.DagCBOR, dagcbor.Decode)
	ls := cidlink.LinkSystemUsingMulticodecRegistry(mcReg)

	ipldDecoder = ipldlegacy.NewDecoderWithLS(ls)
	ipldDecoder.RegisterCodec(cid.DagProtobuf, dagpb.Type.PBNode, merkledag.ProtoNodeConverter)
	ipldDecoder.RegisterCodec(cid.Raw, basicnode.Prototype.Bytes, merkledag.RawNodeConverter)
}

// ClusterDAGService is an implementation of ipld.DAGService plus a Finalize
// method. ClusterDAGServices can be used to provide Adders with a different
// add implementation.
type ClusterDAGService interface {
	ipld.DAGService
	// Finalize receives the IPFS content root CID as
	// returned by the ipfs adder.
	Finalize(ctx context.Context, ipfsRoot api.Cid) (api.Cid, error)
	// Close performs any necessary cleanups and should be called
	// whenever the DAGService is not going to be used anymore.
	Close() error
	// Allocations returns the allocations made by the cluster DAG service
	// for the added content.
	Allocations() []peer.ID
}

// A dagFormatter can create dags from files.Node. It can keep state
// to add several files to the same dag.
type dagFormatter interface {
	Add(name string, f files.Node) (api.Cid, error)
}

// Adder is used to add content to IPFS Cluster using an implementation of
// ClusterDAGService.
type Adder struct {
	ctx    context.Context
	cancel context.CancelFunc

	dgs ClusterDAGService

	params api.AddParams

	// AddedOutput updates are placed on this channel
	// whenever a block is processed. They contain information
	// about the block, the CID, the Name etc. and are mostly
	// meant to be streamed back to the user.
	output chan api.AddedOutput
}

// New returns a new Adder with the given ClusterDAGService, add options and a
// channel to send updates during the adding process.
//
// An Adder may only be used once.
func New(ds ClusterDAGService, p api.AddParams, out chan api.AddedOutput) *Adder {
	// Discard all progress update output as the caller has not provided
	// a channel for them to listen on.
	if out == nil {
		out = make(chan api.AddedOutput, 100)
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
func (a *Adder) FromMultipart(ctx context.Context, r *multipart.Reader) (api.Cid, error) {
	logger.Debugf("adding from multipart with params: %+v", a.params)

	f, err := files.NewFileFromPartReader(r, "multipart/form-data")
	if err != nil {
		return api.CidUndef, err
	}
	defer f.Close()
	return a.FromFiles(ctx, f)
}

// FromFiles adds content from a files.Directory. The adder will no longer
// be usable after calling this method.
func (a *Adder) FromFiles(ctx context.Context, f files.Directory) (api.Cid, error) {
	logger.Debug("adding from files")
	a.setContext(ctx)

	if a.ctx.Err() != nil { // don't allow running twice
		return api.CidUndef, a.ctx.Err()
	}

	defer a.cancel()
	defer close(a.output)

	var dagFmtr dagFormatter
	var err error
	switch a.params.Format {
	case "", "unixfs":
		dagFmtr, err = newIpfsAdder(ctx, a.dgs, a.params, a.output)

	case "car":
		dagFmtr, err = newCarAdder(ctx, a.dgs, a.params, a.output)
	default:
		err = errors.New("bad dag formatter option")
	}
	if err != nil {
		return api.CidUndef, err
	}

	// setup wrapping
	if a.params.Wrap {
		f = files.NewSliceDirectory(
			[]files.DirEntry{files.FileEntry("", f)},
		)
	}

	it := f.Entries()
	var adderRoot api.Cid
	for it.Next() {
		select {
		case <-a.ctx.Done():
			return api.CidUndef, a.ctx.Err()
		default:
			logger.Debugf("ipfsAdder AddFile(%s)", it.Name())

			adderRoot, err = dagFmtr.Add(it.Name(), it.Node())
			if err != nil {
				logger.Error("error adding to cluster: ", err)
				return api.CidUndef, err
			}
		}
		// TODO (hector): We can only add a single CAR file for the
		// moment.
		if a.params.Format == "car" {
			break
		}
	}
	if it.Err() != nil {
		return api.CidUndef, it.Err()
	}

	clusterRoot, err := a.dgs.Finalize(a.ctx, adderRoot)
	if err != nil {
		logger.Error("error finalizing adder:", err)
		return api.CidUndef, err
	}
	logger.Infof("%s successfully added to cluster", clusterRoot)
	return clusterRoot, nil
}

// A wrapper around the ipfsadd.Adder to satisfy the dagFormatter interface.
type ipfsAdder struct {
	*ipfsadd.Adder
}

func newIpfsAdder(ctx context.Context, dgs ClusterDAGService, params api.AddParams, out chan api.AddedOutput) (*ipfsAdder, error) {
	iadder, err := ipfsadd.NewAdder(ctx, dgs, dgs.Allocations)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	iadder.Trickle = params.Layout == "trickle"
	iadder.RawLeaves = params.RawLeaves
	iadder.Chunker = params.Chunker
	iadder.Out = out
	iadder.Progress = params.Progress
	iadder.NoCopy = params.NoCopy

	// Set up prefi
	prefix, err := merkledag.PrefixForCidVersion(params.CidVersion)
	if err != nil {
		return nil, fmt.Errorf("bad CID Version: %s", err)
	}

	hashFunCode, ok := multihash.Names[strings.ToLower(params.HashFun)]
	if !ok {
		return nil, errors.New("hash function name not known")
	}
	prefix.MhType = hashFunCode
	prefix.MhLength = -1
	iadder.CidBuilder = &prefix
	return &ipfsAdder{
		Adder: iadder,
	}, nil
}

func (ia *ipfsAdder) Add(name string, f files.Node) (api.Cid, error) {
	// In order to set the AddedOutput names right, we use
	// OutputPrefix:
	//
	// When adding a folder, this is the root folder name which is
	// prepended to the addedpaths.  When adding a single file,
	// this is the name of the file which overrides the empty
	// AddedOutput name.
	//
	// After coreunix/add.go was refactored in go-ipfs and we
	// followed suit, it no longer receives the name of the
	// file/folder being added and does not emit AddedOutput
	// events with the right names. We addressed this by adding
	// OutputPrefix to our version. go-ipfs modifies emitted
	// events before sending to user).
	ia.OutputPrefix = name

	nd, err := ia.AddAllAndPin(f)
	if err != nil {
		return api.CidUndef, err
	}
	return api.NewCid(nd.Cid()), nil
}

// An adder to add CAR files. It is at the moment very basic, and can
// add a single CAR file with a single root. Ideally, it should be able to
// add more complex, or several CARs by wrapping them with a single root.
// But for that we would need to keep state and track an MFS root similarly to
// what the ipfsadder does.
type carAdder struct {
	ctx    context.Context
	dgs    ClusterDAGService
	params api.AddParams
	output chan api.AddedOutput
}

func newCarAdder(ctx context.Context, dgs ClusterDAGService, params api.AddParams, out chan api.AddedOutput) (*carAdder, error) {
	return &carAdder{
		ctx:    ctx,
		dgs:    dgs,
		params: params,
		output: out,
	}, nil
}

// Add takes a node which should be a CAR file and nothing else and
// adds its blocks using the ClusterDAGService.
func (ca *carAdder) Add(name string, fn files.Node) (api.Cid, error) {
	if ca.params.Wrap {
		return api.CidUndef, errors.New("cannot wrap a CAR file upload")
	}

	f, ok := fn.(files.File)
	if !ok {
		return api.CidUndef, errors.New("expected CAR file is not of type file")
	}
	carReader, err := car.NewCarReader(f)
	if err != nil {
		return api.CidUndef, err
	}

	if len(carReader.Header.Roots) != 1 {
		return api.CidUndef, errors.New("only CAR files with a single root are supported")
	}

	root := carReader.Header.Roots[0]
	bytes := uint64(0)
	size := uint64(0)

	for {
		block, err := carReader.Next()
		if err != nil && err != io.EOF {
			return api.CidUndef, err
		} else if block == nil {
			break
		}

		bytes += uint64(len(block.RawData()))

		nd, err := ipldDecoder.DecodeNode(context.TODO(), block)
		if err != nil {
			return api.CidUndef, err
		}

		// If the root is in the CAR and the root is a UnixFS
		// node, then set the size in the output object.
		if nd.Cid().Equals(root) {
			ufs, err := unixfs.ExtractFSNode(nd)
			if err == nil {
				size = ufs.FileSize()
			}
		}

		err = ca.dgs.Add(ca.ctx, nd)
		if err != nil {
			return api.CidUndef, err
		}
	}

	ca.output <- api.AddedOutput{
		Name:        name,
		Cid:         api.NewCid(root),
		Bytes:       bytes,
		Size:        size,
		Allocations: ca.dgs.Allocations(),
	}

	return api.NewCid(root), nil
}
