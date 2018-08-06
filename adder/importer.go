package adder

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/ipfs/ipfs-cluster/adder/ipfsadd"
	"github.com/ipfs/ipfs-cluster/api"

	"github.com/ipfs/go-ipfs-cmdkit/files"
)

// BlockHandler is a function used to process a block as is received by the
// Importer. Used in Importer.Run().
type BlockHandler func(ctx context.Context, n *api.NodeWithMeta) (string, error)

// Importer facilitates converting a file into a stream
// of chunked blocks.
type Importer struct {
	startedMux sync.Mutex
	started    bool

	files  files.File
	params *api.AddParams

	output chan *api.AddedOutput
	blocks chan *api.NodeWithMeta
	errors chan error
}

// NewImporter sets up an Importer ready to Go().
func NewImporter(f files.File, p *api.AddParams, out chan *api.AddedOutput) (*Importer, error) {
	blocks := make(chan *api.NodeWithMeta, 1)
	errors := make(chan error, 1)

	return &Importer{
		started: false,
		files:   f,
		params:  p,
		output:  out,
		blocks:  blocks,
		errors:  errors,
	}, nil
}

// Blocks returns a channel where each imported block is sent.
func (imp *Importer) Blocks() <-chan *api.NodeWithMeta {
	return imp.blocks
}

// Errors returns a channel to which any errors during the import
// process are sent.
func (imp *Importer) Errors() <-chan error {
	return imp.errors
}

func (imp *Importer) start() bool {
	imp.startedMux.Lock()
	defer imp.startedMux.Unlock()
	retVal := imp.started
	imp.started = true
	return !retVal
}

func (imp *Importer) addFile(ipfsAdder *ipfsadd.Adder) error {
	f, err := imp.files.NextFile()
	if err != nil {
		return err
	}

	logger.Debugf("ipfsAdder AddFile(%s)", f.FullPath())
	return ipfsAdder.AddFile(f)
}

func (imp *Importer) addFiles(ctx context.Context, ipfsAdder *ipfsadd.Adder) {
	defer close(imp.blocks)
	defer close(imp.errors)

	for {
		select {
		case <-ctx.Done():
			imp.errors <- ctx.Err()
			return
		default:
			err := imp.addFile(ipfsAdder)
			if err == io.EOF {
				goto FINALIZE
			}

			if err != nil {
				imp.errors <- err
				return
			}
		}
	}
FINALIZE:
	_, err := ipfsAdder.Finalize()
	if err != nil {
		imp.errors <- err
	}
}

// Go starts a goroutine which reads the blocks as outputted by the
// ipfsadd module called with the parameters of this importer. The blocks,
// errors and output are placed in the respective importer channels for
// further processing. When there are no more blocks, or an error happen,
// the channels will be closed.
func (imp *Importer) Go(ctx context.Context) error {
	if !imp.start() {
		return errors.New("importing process already started or finished")
	}

	dagsvc := newAdderDAGService(imp.blocks)

	ipfsAdder, err := ipfsadd.NewAdder(ctx, dagsvc)
	if err != nil {
		return err
	}

	ipfsAdder.Hidden = imp.params.Hidden
	ipfsAdder.Trickle = imp.params.Layout == "trickle"
	ipfsAdder.RawLeaves = imp.params.RawLeaves
	ipfsAdder.Wrap = true
	ipfsAdder.Chunker = imp.params.Chunker
	ipfsAdder.Out = imp.output

	go imp.addFiles(ctx, ipfsAdder)
	return nil
}

// Run triggers the importing process (calling Go) and  calls the given BlockHandler
// on every node read from the importer.
// It returns the value returned by the last-called BlockHandler.
func (imp *Importer) Run(ctx context.Context, blockF BlockHandler) (string, error) {
	var retVal string

	errors := imp.Errors()
	blocks := imp.Blocks()

	err := imp.Go(ctx)
	if err != nil {
		return retVal, err
	}

	for {
		select {
		case <-ctx.Done():
			return retVal, ctx.Err()
		case err, ok := <-errors:
			if ok {
				logger.Error(err)
				return retVal, err
			}
		case node, ok := <-blocks:
			if !ok {
				goto BREAK // finished importing
			}
			retVal, err = blockF(ctx, node)
			if err != nil {
				return retVal, err
			}
		}
	}
BREAK:
	// grab any last errors from errors if necessary
	// (this waits for errors to be closed)
	err = <-errors
	return retVal, err
}
