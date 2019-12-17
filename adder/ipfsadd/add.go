// Package ipfsadd is a simplified copy of go-ipfs/core/coreunix/add.go
package ipfsadd

import (
	"context"
	"errors"
	"fmt"
	"io"
	gopath "path"
	"path/filepath"

	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	chunker "github.com/ipfs/go-ipfs-chunker"
	files "github.com/ipfs/go-ipfs-files"
	posinfo "github.com/ipfs/go-ipfs-posinfo"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	dag "github.com/ipfs/go-merkledag"
	mfs "github.com/ipfs/go-mfs"
	unixfs "github.com/ipfs/go-unixfs"
	balanced "github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
	trickle "github.com/ipfs/go-unixfs/importer/trickle"
)

var log = logging.Logger("coreunix")

// how many bytes of progress to wait before sending a progress update message
const progressReaderIncrement = 1024 * 256

var liveCacheSize = uint64(256 << 10)

// NewAdder Returns a new Adder used for a file add operation.
func NewAdder(ctx context.Context, ds ipld.DAGService) (*Adder, error) {
	// Cluster: we don't use pinner nor GCLocker.
	return &Adder{
		ctx:        ctx,
		dagService: ds,
		Progress:   false,
		Trickle:    false,
		Chunker:    "",
	}, nil
}

// Adder holds the switches passed to the `add` command.
type Adder struct {
	ctx        context.Context
	dagService ipld.DAGService
	Out        chan *api.AddedOutput
	Progress   bool
	Trickle    bool
	RawLeaves  bool
	Silent     bool
	NoCopy     bool
	Chunker    string
	mroot      *mfs.Root
	tempRoot   cid.Cid
	CidBuilder cid.Builder
	liveNodes  uint64
	lastFile   mfs.FSNode
	// Cluster: ipfs does a hack in commands/add.go to prefix the output
	// path of the events from this adder with the root folder name when
	// adding a folder.  We are going to emit the events with the right
	// file name directly here by allowing the caller to set this.
	OutputDirPrefix string
}

func (adder *Adder) mfsRoot() (*mfs.Root, error) {
	if adder.mroot != nil {
		return adder.mroot, nil
	}
	rnode := unixfs.EmptyDirNode()
	rnode.SetCidBuilder(adder.CidBuilder)
	mr, err := mfs.NewRoot(adder.ctx, adder.dagService, rnode, nil)
	if err != nil {
		return nil, err
	}
	adder.mroot = mr
	return adder.mroot, nil
}

// SetMfsRoot sets `r` as the root for Adder.
func (adder *Adder) SetMfsRoot(r *mfs.Root) {
	adder.mroot = r
}

// Constructs a node from reader's data, and adds it. Doesn't pin.
func (adder *Adder) add(reader io.Reader) (ipld.Node, error) {
	chnk, err := chunker.FromString(reader, adder.Chunker)
	if err != nil {
		return nil, err
	}

	// Cluster: we don't do batching/use BufferedDS.

	params := ihelper.DagBuilderParams{
		Dagserv:    adder.dagService,
		RawLeaves:  adder.RawLeaves,
		Maxlinks:   ihelper.DefaultLinksPerBlock,
		NoCopy:     adder.NoCopy,
		CidBuilder: adder.CidBuilder,
	}

	db, err := params.New(chnk)
	if err != nil {
		return nil, err
	}

	var nd ipld.Node
	if adder.Trickle {
		nd, err = trickle.Layout(db)
	} else {
		nd, err = balanced.Layout(db)
	}
	if err != nil {
		return nil, err
	}

	return nd, nil
}

// RootNode returns the mfs root node
func (adder *Adder) curRootNode() (ipld.Node, error) {
	mr, err := adder.mfsRoot()
	if err != nil {
		return nil, err
	}
	root, err := mr.GetDirectory().GetNode()
	if err != nil {
		return nil, err
	}

	// if one root file, use that hash as root.
	if len(root.Links()) == 1 {
		nd, err := root.Links()[0].GetNode(adder.ctx, adder.dagService)
		if err != nil {
			return nil, err
		}

		root = nd
	}

	return root, err
}

// PinRoot recursively pins the root node of Adder and
// writes the pin state to the backing datastore.
// Cluster: we don't pin. Former Finalize().
func (adder *Adder) PinRoot(root ipld.Node) error {
	rnk := root.Cid()

	err := adder.dagService.Add(adder.ctx, root)
	if err != nil {
		return err
	}

	if adder.tempRoot.Defined() {
		adder.tempRoot = rnk
	}

	return nil
}

func (adder *Adder) outputDirs(path string, fsn mfs.FSNode) error {
	switch fsn := fsn.(type) {
	case *mfs.File:
		return nil
	case *mfs.Directory:
		names, err := fsn.ListNames(adder.ctx)
		if err != nil {
			return err
		}

		for _, name := range names {
			child, err := fsn.Child(name)
			if err != nil {
				// This fails when Child is of type *mfs.File
				// because it tries to get them from the DAG
				// service (does not implement this and returns
				// a "not found" error)
				// *mfs.Files are ignored in the recursive call
				// anyway.
				// For Cluster, we just ignore errors here.
				continue
			}

			childpath := gopath.Join(path, name)
			err = adder.outputDirs(childpath, child)
			if err != nil {
				return err
			}

			fsn.Uncache(name)
		}
		nd, err := fsn.GetNode()
		if err != nil {
			return err
		}

		return adder.outputDagnode(adder.Out, path, nd)
	default:
		return fmt.Errorf("unrecognized fsn type: %#v", fsn)
	}
}

func (adder *Adder) addNode(node ipld.Node, path string) error {
	// patch it into the root
	if path == "" {
		path = node.Cid().String()
	}

	if pi, ok := node.(*posinfo.FilestoreNode); ok {
		node = pi.Node
	}

	mr, err := adder.mfsRoot()
	if err != nil {
		return err
	}
	dir := gopath.Dir(path)
	if dir != "." {
		opts := mfs.MkdirOpts{
			Mkparents:  true,
			Flush:      false,
			CidBuilder: adder.CidBuilder,
		}
		if err := mfs.Mkdir(mr, dir, opts); err != nil {
			return err
		}
	}

	if err := mfs.PutNode(mr, path, node); err != nil {
		return err
	}

	// Cluster: cache the last file added.
	// This avoids using the DAGService to get the first children
	// if the MFS root when not wrapping.
	lastFile, err := mfs.NewFile(path, node, nil, adder.dagService)
	if err != nil {
		return err
	}
	adder.lastFile = lastFile

	if !adder.Silent {
		return adder.outputDagnode(adder.Out, path, node)
	}
	return nil
}

// AddAllAndPin adds the given request's files and pin them.
// Cluster: we don'pin. Former AddFiles.
func (adder *Adder) AddAllAndPin(file files.Node) (ipld.Node, error) {
	if err := adder.addFileNode("", file, true); err != nil {
		return nil, err
	}

	// get root
	mr, err := adder.mfsRoot()
	if err != nil {
		return nil, err
	}
	var root mfs.FSNode
	rootdir := mr.GetDirectory()
	root = rootdir

	err = root.Flush()
	if err != nil {
		return nil, err
	}

	// if adding a file without wrapping, swap the root to it (when adding a
	// directory, mfs root is the directory)
	_, dir := file.(files.Directory)
	var name string
	if !dir {
		children, err := rootdir.ListNames(adder.ctx)
		if err != nil {
			return nil, err
		}

		if len(children) == 0 {
			return nil, fmt.Errorf("expected at least one child dir, got none")
		}

		// Replace root with the first child
		name = children[0]
		root, err = rootdir.Child(name)
		if err != nil {
			// Cluster: use the last file we added
			// if we have one.
			if adder.lastFile == nil {
				return nil, err
			}
			root = adder.lastFile
		}
	}

	err = mr.Close()
	if err != nil {
		return nil, err
	}

	nd, err := root.GetNode()
	if err != nil {
		return nil, err
	}

	// output directory events
	err = adder.outputDirs(name, root)
	if err != nil {
		return nil, err
	}

	// Cluster: call PinRoot which adds the root cid to the DAGService.
	// Unsure if this a bug in IPFS when not pinning. Or it would get added
	// twice.
	return nd, adder.PinRoot(nd)
}

// Cluster: we don't Pause for GC
func (adder *Adder) addFileNode(path string, file files.Node, toplevel bool) error {
	defer file.Close()

	if adder.liveNodes >= liveCacheSize {
		// TODO: A smarter cache that uses some sort of lru cache with an eviction handler
		mr, err := adder.mfsRoot()
		if err != nil {
			return err
		}
		if err := mr.FlushMemFree(adder.ctx); err != nil {
			return err
		}

		adder.liveNodes = 0
	}
	adder.liveNodes++

	switch f := file.(type) {
	case files.Directory:
		return adder.addDir(path, f, toplevel)
	case *files.Symlink:
		return adder.addSymlink(path, f)
	case files.File:
		return adder.addFile(path, f)
	default:
		return errors.New("unknown file type")
	}
}

func (adder *Adder) addSymlink(path string, l *files.Symlink) error {
	sdata, err := unixfs.SymlinkData(l.Target)
	if err != nil {
		return err
	}

	dagnode := dag.NodeWithData(sdata)
	dagnode.SetCidBuilder(adder.CidBuilder)
	err = adder.dagService.Add(adder.ctx, dagnode)
	if err != nil {
		return err
	}

	return adder.addNode(dagnode, path)
}

func (adder *Adder) addFile(path string, file files.File) error {
	// if the progress flag was specified, wrap the file so that we can send
	// progress updates to the client (over the output channel)
	var reader io.Reader = file
	if adder.Progress {
		rdr := &progressReader{file: reader, path: path, out: adder.Out}
		if fi, ok := file.(files.FileInfo); ok {
			reader = &progressReader2{rdr, fi}
		} else {
			reader = rdr
		}
	}

	dagnode, err := adder.add(reader)
	if err != nil {
		return err
	}

	// patch it into the root
	return adder.addNode(dagnode, path)
}

func (adder *Adder) addDir(path string, dir files.Directory, toplevel bool) error {
	log.Infof("adding directory: %s", path)

	if !(toplevel && path == "") {
		mr, err := adder.mfsRoot()
		if err != nil {
			return err
		}
		err = mfs.Mkdir(mr, path, mfs.MkdirOpts{
			Mkparents:  true,
			Flush:      false,
			CidBuilder: adder.CidBuilder,
		})
		if err != nil {
			return err
		}
	}

	it := dir.Entries()
	for it.Next() {
		fpath := gopath.Join(path, it.Name())
		err := adder.addFileNode(fpath, it.Node(), false)
		if err != nil {
			return err
		}
	}

	return it.Err()
}

// outputDagnode sends dagnode info over the output channel.
// Cluster: we use *api.AddedOutput instead of coreiface events
// and make this an adder method to be be able to prefix.
func (adder *Adder) outputDagnode(out chan *api.AddedOutput, name string, dn ipld.Node) error {
	if out == nil {
		return nil
	}

	s, err := dn.Size()
	if err != nil {
		return err
	}

	out <- &api.AddedOutput{
		Cid:  dn.Cid(),
		Name: filepath.Join(adder.OutputDirPrefix, name),
		Size: s,
	}

	return nil
}

type progressReader struct {
	file         io.Reader
	path         string
	out          chan *api.AddedOutput
	bytes        int64
	lastProgress int64
}

func (i *progressReader) Read(p []byte) (int, error) {
	n, err := i.file.Read(p)

	i.bytes += int64(n)
	if i.bytes-i.lastProgress >= progressReaderIncrement || err == io.EOF {
		i.lastProgress = i.bytes
		i.out <- &api.AddedOutput{
			Name:  i.path,
			Bytes: uint64(i.bytes),
		}
	}

	return n, err
}

type progressReader2 struct {
	*progressReader
	files.FileInfo
}

func (i *progressReader2) Read(p []byte) (int, error) {
	return i.progressReader.Read(p)
}
