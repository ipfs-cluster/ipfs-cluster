package importer

import (
	"context"
	"io"
	"strings"

	"github.com/ipfs/go-ipfs-cmdkit/files"
	"github.com/ipfs/go-ipfs/core/coreunix"
	ipld "github.com/ipfs/go-ipld-format"
)

// ToPrint is a first step towards a streaming importer.  Verify that we
// can hijack the blockstore abstraction to redirect blocks as they arrive
// Closely follows go-ipfs/core/commands/add.go: Run func
func ToPrint(f files.File) error {
	dserv := &pDAGService{
		membership: make(map[string]bool),
	}

	ctx := context.Background() // using background for now, should upgrade later
	fileAdder, err := coreunix.NewAdder(ctx, nil, nil, dserv)
	if err != nil {
		return err
	}
	fileAdder.Pin = false // This way we can be honest that blockstore doesn't exist

	// add all files under the root, as in ipfs
	for {
		file, err := f.NextFile()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		if err := fileAdder.AddFile(file); err != nil {
			return err
		}
	}

	// Without this call all of the directory nodes (stored in MFS) do not get
	// written through to the dagservice and its blockstore
	_, err = fileAdder.Finalize()
	// ignore errors caused by printing-blockstore Get not finding blocks
	if err != nil && !strings.Contains(err.Error(), "dagservice: block not found") {
		return err
	}
	return nil
}

// ToChannel imports file to ipfs ipld nodes, outputting nodes on the
// provided channel
func ToChannel(f files.File, outChan chan<- *ipld.Node, ctx context.Context) error {
	dserv := &outDAGService{
		membership: make(map[string]bool),
		outChan:    outChan,
	}

	fileAdder, err := coreunix.NewAdder(ctx, nil, nil, dserv)
	if err != nil {
		return err
	}
	fileAdder.Pin = false

	// add all files under the root, as in ipfs
	for {
		file, err := f.NextFile()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		if err := fileAdder.AddFile(file); err != nil {
			return err
		}
	}

	_, err = fileAdder.Finalize()
	if !strings.Contains(err.Error(), "dagservice: block not found") {
		return err
	}
	close(outChan)
	return nil
}
