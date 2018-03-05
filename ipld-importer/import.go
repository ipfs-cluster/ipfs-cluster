package importer

import (
	"context"
	"io"
	"strings"

	"github.com/ipfs/ipfs-cluster/api"

	"github.com/ipfs/go-ipfs-cmdkit/files"
)

// ToChannel imports file to ipfs ipld nodes, outputting nodes on the
// provided channel
func ToChannel(ctx context.Context, f files.File, progress bool, hidden bool,
	trickle bool, raw bool, silent bool, wrap bool,
	chunker string) (<-chan *api.AddedOutput, <-chan *api.NodeWithMeta, <-chan error) {

	printChan := make(chan *api.AddedOutput)
	errChan := make(chan error)
	outChan := make(chan *api.NodeWithMeta)

	dserv := &outDAGService{
		membership: make(map[string]struct{}),
		outChan:    outChan,
	}

	fileAdder, err := NewAdder(ctx, nil, nil, dserv)
	if err != nil {
		go func() {
			errChan <- err
		}()
		return printChan, outChan, errChan
	}
	//	fileAdder.Progress = progress //TODO get progress working eventually.  dont need complexity right now
	fileAdder.Hidden = hidden
	fileAdder.Trickle = trickle
	fileAdder.RawLeaves = raw
	fileAdder.Silent = silent
	fileAdder.Wrap = wrap
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
		if err != nil && !strings.Contains(err.Error(), "dagservice: block not found") {
			errChan <- err
		}
	}()
	return printChan, outChan, errChan
}
