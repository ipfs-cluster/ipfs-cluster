package importer

import (
	"context"
	"fmt"
	"io"
	gopath "path"
	"strconv"

	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	syncds "github.com/ipfs/go-datastore/sync"
	files "github.com/ipfs/go-ipfs-cmdkit/files"
	bstore "github.com/ipfs/go-ipfs/blocks/blockstore"
	bserv "github.com/ipfs/go-ipfs/blockservice"
	"github.com/ipfs/go-ipfs/exchange/offline"
	balanced "github.com/ipfs/go-ipfs/importer/balanced"
	"github.com/ipfs/go-ipfs/importer/chunk"
	ihelper "github.com/ipfs/go-ipfs/importer/helpers"
	trickle "github.com/ipfs/go-ipfs/importer/trickle"
	dag "github.com/ipfs/go-ipfs/merkledag"
	mfs "github.com/ipfs/go-ipfs/mfs"
	"github.com/ipfs/go-ipfs/pin"
	posinfo "github.com/ipfs/go-ipfs/thirdparty/posinfo"
	unixfs "github.com/ipfs/go-ipfs/unixfs"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("coreunix")

// how many bytes of progress to wait before sending a progress update message
const progressReaderIncrement = 1024 * 256

var liveCacheSize = uint64(256 << 10)

type hiddenFileError struct {
	fileName string
}

func (e *hiddenFileError) Error() string {
	return fmt.Sprintf("%s is a hidden file", e.fileName)
}

type ignoreFileError struct {
	fileName string
}

func (e *ignoreFileError) Error() string {
	return fmt.Sprintf("%s is an ignored file", e.fileName)
}

// NewAdder Returns a new Adder used for a file add operation.
func NewAdder(ctx context.Context, p pin.Pinner, bs bstore.GCBlockstore, ds ipld.DAGService) (*Adder, error) {
	return &Adder{
		ctx:        ctx,
		blockstore: bs,
		dagService: ds,
		Progress:   false,
		Hidden:     true,
		Trickle:    false,
		Wrap:       false,
		Chunker:    "",
	}, nil
}

// Adder holds the switches passed to the `add` command.
type Adder struct {
	ctx        context.Context
	blockstore bstore.GCBlockstore
	dagService ipld.DAGService
	Out        chan *api.AddedOutput
	Progress   bool
	Hidden     bool
	Trickle    bool
	RawLeaves  bool
	Silent     bool
	Wrap       bool
	NoCopy     bool
	Chunker    string
	root       ipld.Node
	mroot      *mfs.Root
	tempRoot   *cid.Cid
	Prefix     *cid.Prefix
	liveNodes  uint64
}

func (adder *Adder) mfsRoot() (*mfs.Root, error) {
	if adder.mroot != nil {
		return adder.mroot, nil
	}
	rnode := unixfs.EmptyDirNode()
	rnode.SetPrefix(adder.Prefix)
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
	chnk, err := chunk.FromString(reader, adder.Chunker)
	if err != nil {
		return nil, err
	}

	params := ihelper.DagBuilderParams{
		Dagserv:   adder.dagService,
		RawLeaves: adder.RawLeaves,
		Maxlinks:  ihelper.DefaultLinksPerBlock,
		NoCopy:    adder.NoCopy,
		Prefix:    adder.Prefix,
	}

	if adder.Trickle {
		return trickle.TrickleLayout(params.New(chnk))
	}

	return balanced.BalancedLayout(params.New(chnk))
}

// RootNode returns the root node of the Added.
func (adder *Adder) RootNode() (ipld.Node, error) {
	// for memoizing
	if adder.root != nil {
		return adder.root, nil
	}

	mr, err := adder.mfsRoot()
	if err != nil {
		return nil, err
	}
	root, err := mr.GetValue().GetNode()
	if err != nil {
		return nil, err
	}

	// if not wrapping, AND one root file, use that hash as root.
	if !adder.Wrap && len(root.Links()) == 1 {
		nd, err := root.Links()[0].GetNode(adder.ctx, adder.dagService)
		if err != nil {
			return nil, err
		}

		root = nd
	}

	adder.root = root
	return root, err
}

// Finalize flushes the mfs root directory and returns the mfs root node.
func (adder *Adder) Finalize() (ipld.Node, error) {
	mr, err := adder.mfsRoot()
	if err != nil {
		return nil, err
	}
	root := mr.GetValue()

	err = root.Flush()
	if err != nil {
		return nil, err
	}

	var name string
	if !adder.Wrap {
		children, err := root.(*mfs.Directory).ListNames(adder.ctx)
		if err != nil {
			return nil, err
		}

		if len(children) == 0 {
			return nil, fmt.Errorf("expected at least one child dir, got none")
		}

		name = children[0]

		mr, err := adder.mfsRoot()
		if err != nil {
			return nil, err
		}

		dir, ok := mr.GetValue().(*mfs.Directory)
		if !ok {
			return nil, fmt.Errorf("root is not a directory")
		}

		root, err = dir.Child(name)
		if err != nil {
			return nil, err
		}
	}

	err = adder.outputDirs(name, root)
	if err != nil {
		return nil, err
	}

	err = mr.Close()
	if err != nil {
		return nil, err
	}

	return root.GetNode()
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
				return err
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

		return outputDagnode(adder.Out, path, nd)
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
			Mkparents: true,
			Flush:     false,
			Prefix:    adder.Prefix,
		}
		if err := mfs.Mkdir(mr, dir, opts); err != nil {
			return err
		}
	}

	if err := mfs.PutNode(mr, path, node); err != nil {
		return err
	}

	if !adder.Silent {
		return outputDagnode(adder.Out, path, node)
	}
	return nil
}

// AddFile adds a file to the dagservice and outputs through the outchan
func (adder *Adder) AddFile(file files.File) error {
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

	if file.IsDirectory() {
		return adder.addDir(file)
	}

	// case for symlink
	if s, ok := file.(*files.Symlink); ok {
		sdata, err := unixfs.SymlinkData(s.Target)
		if err != nil {
			return err
		}

		dagnode := dag.NodeWithData(sdata)
		dagnode.SetPrefix(adder.Prefix)
		err = adder.dagService.Add(adder.ctx, dagnode)
		if err != nil {
			return err
		}

		return adder.addNode(dagnode, s.FileName())
	}

	// case for regular file
	// if the progress flag was specified, wrap the file so that we can send
	// progress updates to the client (over the output channel)
	var reader io.Reader = file
	if adder.Progress {
		rdr := &progressReader{file: file, out: adder.Out}
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
	return adder.addNode(dagnode, file.FileName())
}

func (adder *Adder) addDir(dir files.File) error {
	log.Infof("adding directory: %s", dir.FileName())

	mr, err := adder.mfsRoot()
	if err != nil {
		return err
	}
	err = mfs.Mkdir(mr, dir.FileName(), mfs.MkdirOpts{
		Mkparents: true,
		Flush:     false,
		Prefix:    adder.Prefix,
	})
	if err != nil {
		return err
	}

	for {
		file, err := dir.NextFile()
		if err != nil && err != io.EOF {
			return err
		}
		if file == nil {
			break
		}

		// Skip hidden files when adding recursively, unless Hidden is enabled.
		if files.IsHidden(file) && !adder.Hidden {
			log.Infof("%s is hidden, skipping", file.FileName())
			continue
		}
		err = adder.AddFile(file)
		if err != nil {
			return err
		}
	}

	return nil
}

// outputDagnode sends dagnode info over the output channel
func outputDagnode(out chan *api.AddedOutput, name string, dn ipld.Node) error {
	if out == nil {
		return nil
	}

	c := dn.Cid()
	s, err := dn.Size()
	if err != nil {
		return err
	}

	out <- &api.AddedOutput{
		Hash: c.String(),
		Name: name,
		Size: strconv.FormatUint(s, 10),
	}

	return nil
}

// NewMemoryDagService builds and returns a new mem-datastore.
func NewMemoryDagService() ipld.DAGService {
	// build mem-datastore for editor's intermediary nodes
	bs := bstore.NewBlockstore(syncds.MutexWrap(ds.NewMapDatastore()))
	bsrv := bserv.New(bs, offline.Exchange(bs))
	return dag.NewDAGService(bsrv)
}

type progressReader struct {
	file         files.File
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
			Name:  i.file.FileName(),
			Bytes: i.bytes,
		}
	}

	return n, err
}

type progressReader2 struct {
	*progressReader
	files.FileInfo
}
