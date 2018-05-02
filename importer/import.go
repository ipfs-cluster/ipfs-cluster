package importer

import (
	"context"
	"io"

	"github.com/ipfs/ipfs-cluster/api"

	"github.com/ipfs/go-ipfs-cmdkit/files"
)

func shouldIgnore(err error) bool {
	if err == errNotFound {
		return true
	}
	return false
}

// ToChannel imports file to ipfs ipld nodes, outputting nodes on the
// provided channel
func ToChannel(ctx context.Context, f files.File, hidden bool,
	trickle bool, raw bool, chunker string) (<-chan *api.AddedOutput,
	<-chan *api.NodeWithMeta, <-chan error) {

	printChan := make(chan *api.AddedOutput)
	errChan := make(chan error)
	outChan := make(chan *api.NodeWithMeta)

	dserv := &outDAGService{
		membership: make(map[string]struct{}),
		outChan:    outChan,
	}

	fileAdder, err := NewAdder(ctx, nil, dserv)
	if err != nil {
		go func() {
			errChan <- err
		}()
		return printChan, outChan, errChan
	}
	fileAdder.Hidden = hidden
	fileAdder.Trickle = trickle
	fileAdder.RawLeaves = raw
	// Files added in one session are wrapped.  This is because if files
	// are sharded together then the share one logical clusterDAG root hash
	fileAdder.Wrap = true
	fileAdder.Chunker = chunker
	fileAdder.Out = printChan

	go func() {
		defer close(printChan)
		defer close(outChan)
		defer close(errChan)
		// add all files under the root, as in ipfs
		for {
			file, err := f.NextFile()

			if err == io.EOF {
				break
			} else if err != nil {
				errChan <- err
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
			if err := fileAdder.AddFile(file); err != nil {
				errChan <- err
				return
			}
		}

		_, err := fileAdder.Finalize()
		if err != nil && !shouldIgnore(err) {
			errChan <- err
		}
	}()
	return printChan, outChan, errChan
}
