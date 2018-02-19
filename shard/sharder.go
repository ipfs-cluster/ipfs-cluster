package shard

import (
	"errors"
	"fmt"

	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
	mh "github.com/multiformats/go-multihash"
)

var logger = logging.Logger("shard")

// Sharder aggregates incident ipfs file dag nodes into a shard, or group of
// nodes.  The Sharder builds a reference node in the ipfs-cluster DAG to
// reference the nodes in a shard.  This component distributes shards among
// cluster peers based on the decisions of the cluster allocator
type Sharder struct {
	rpcClient *rpc.Client

	assignedPeer  peer.ID
	currentShard  shardObj
	byteCount     uint64
	byteThreshold uint64

	allocSize uint64
}

// NewSharder returns a new sharder for use by an ipfs-cluster.  In the future
// this may take in a shard-config
func NewSharder(cfg *Config) (*Sharder, error) {
	logger.Debugf("The alloc size provided: %d", cfg.AllocSize)
	return &Sharder{allocSize: cfg.AllocSize,
		currentShard: make(map[string]*cid.Cid),
	}, nil
}

func (s *Sharder) unInit() bool {
	a := len(s.currentShard) == 0
	b := s.assignedPeer == peer.ID("")
	c := s.byteCount == 0
	d := s.byteThreshold == 0
	logger.Debugf("a: %t b: %t c: %t d: %t", a, b, c, d)
	return a && b && c && d
}

// SetClient registers the rpcClient used by the Sharder to communicate with
// other components
func (s *Sharder) SetClient(c *rpc.Client) {
	s.rpcClient = c
}

// Shutdown shuts down the sharding component
func (s *Sharder) Shutdown() error {
	return nil
}

// Temporary storage of links to be serialized to ipld cbor once allocation is
// complete
type shardObj map[string]*cid.Cid

// clusterDAGCountBytes tracks the number of bytes in the serialized cluster
// DAG node used to track this shard.  For now ignoring these bytes
func (s *Sharder) clusterDAGCountBytes() uint64 {
	// link size * link bytes
	// + overhead
	return 0
}

// getAssignment returns the pid of a cluster peer that will allocate space
// for a new shard, along with a byte threshold determining the max size of
// this shard.
func (s *Sharder) getAssignment() (peer.ID, uint64, error) {
	// TODO: work out the relationship between shard allocations and
	// cluster pin allocations.  For now I'm assuming that none of the cids
	// within a shard are pinned elsewhere in the cluster, or if they are
	// it has no effect on the independent allocation of the shard

	// TODO: if a shard is redundant we won't know the root hash until
	// after adding the whole thing and we will effectively waste work.
	// We may not be able to avoid this, at least not in the case when
	// we are directly adding a file and not sharding the nodes of an
	// existing DAG with a root hash available at the start of sharding.
	// A single shard with a different byte threshold will totally change
	// the root hash and make this harder to catch too

	// RPC to get metrics
	var metrics []api.Metric
	err := s.rpcClient.Call("",
		"Cluster",
		"GetInformerMetrics",
		struct{}{},
		&metrics)
	if err != nil {
		return peer.ID(""), 0, err
	}
	candidates := make(map[peer.ID]api.Metric)
	for _, m := range metrics {
		switch {
		case m.Discard():
			// discard invalid metrics
			continue
		default:
			if m.Name != "freespace" {
				logger.Warningf("Metric type not freespace but %s", m.Name)
			}
			candidates[m.Peer] = m
		}
	}

	// RPC to get Allocations based on this
	// TODO: support for repl factor > 1.  Investigate best way
	// to build allocation abstractions (rpc endpoints, cluster
	// functions and allocator functions) to reduce code duplication
	// between this function and allocate.go functions.  Not too bad
	// for now though.
	allocs := make([]peer.ID, 0)
	err = s.rpcClient.Call("",
		"Cluster",
		"Allocate",
		candidates,
		&allocs)
	if err != nil {
		return peer.ID(""), 0, err
	}
	if len(allocs) == 0 {
		return peer.ID(""), 0, errors.New("allocation failed")
	}

	// For now allocSize is a config-dependent constant
	// TODO: we could configure with more complexity across
	// all file shards e.g.:
	//  allocSize = if TotalSize < C1 then max(f*TotalSize, C) else TotalSize
	// TODO: we could parametrize ipfs-cluster-ctl add --shard to take in
	// per-file sharding configurations
	// TODO: eventually all of this should plug into future complex extentsions
	// of allocation configuration, including things like data center regions
	// measured throughput of a node, or predictions of where the data will be
	// most accessed.  It is a good start to call into alloc, but we will need
	// to update as alloc develops.
	return allocs[0], s.allocSize, nil
}

// initShard gets a new allocation and updates sharder state accordingly
func (s *Sharder) initShard() error {
	var err error
	s.assignedPeer, s.byteThreshold, err = s.getAssignment()
	if err != nil {
		return err
	}
	s.byteCount = 0
	s.currentShard = make(map[string]*cid.Cid)
	logger.Debugf("Within initShard. thresh: %d", s.byteThreshold)
	return nil
}

// AddNode includes the provided node into a shard in the cluster DAG
// that tracks this node's graph
func (s *Sharder) AddNode(node ipld.Node) error {
	logger.Debug("adding node to shard")
	logger.Debugf("sharder size: %d---sharder thresh: %d", s.byteCount, s.byteThreshold)
	size, err := node.Size()
	if err != nil {
		return err
	}
	c := node.Cid()
	format, ok := cid.CodecToStr[c.Type()]
	if !ok {
		format = ""
		logger.Warning("unsupported cid type, treating as v0")
	}
	if c.Prefix().Version == 0 {
		format = "v0"
	}
	if s.unInit() {
		logger.Debug("initializing next shard of data")
		if err := s.initShard(); err != nil {
			return err
		}
		logger.Debugf("After first init. thresh: %d", s.byteThreshold)
	} else {
		if s.byteCount+size+s.clusterDAGCountBytes() > s.byteThreshold {
			logger.Debug("shard at capacity, pin cluster DAG node")
			if err := s.Flush(); err != nil {
				return err
			}
			if err := s.initShard(); err != nil {
				return err
			}
			logger.Debugf("After flushing. thresh: %d", s.byteThreshold)
		}
	}

	// Shard is initialized and can accommodate node by config-enforced
	// invariant that shard size is always greater than the ipfs block
	// max chunk size
	logger.Debugf("Adding size: %d to byteCount at: %d", size, s.byteCount)
	s.byteCount += size

	key := fmt.Sprintf("%d", len(s.currentShard))
	s.currentShard[key] = node.Cid()
	var retStr string
	b := api.BlockWithFormat{
		Data:   node.RawData(),
		Format: format,
	}
	return s.rpcClient.Call(s.assignedPeer, "Cluster", "IPFSBlockPut",
		b, &retStr)
}

// Flush completes the allocation of the current shard of a file by adding the
// clusterDAG shard node block to IPFS.  It clears sharder state in order to
// clear the cluster sharding component to shard new files in the case the last
// shard is being flushed.
func (s *Sharder) Flush() error {
	// Serialize shard node and reset state
	logger.Debugf("Flushing the current shard %v", s.currentShard)
	shardNode, err := cbor.WrapObject(s.currentShard, mh.SHA2_256, mh.DefaultLengths[mh.SHA2_256])
	if err != nil {
		return err
	}
	logger.Debugf("The dag cbor Node Links: %v", shardNode.Links())

	targetPeer := s.assignedPeer
	s.currentShard = make(map[string]*cid.Cid)
	s.assignedPeer = peer.ID("")
	s.byteThreshold = 0
	s.byteCount = 0
	var retStr string
	b := api.BlockWithFormat{
		Data:   shardNode.RawData(),
		Format: "cbor",
	}
	logger.Debugf("Here is the serialized ipld: %x", b.Data)
	err = s.rpcClient.Call(targetPeer, "Cluster", "IPFSBlockPut",
		b, &retStr)
	if err != nil {
		return err
	}

	// Track shard node as a normal pin within cluster state
	pinS := api.Pin{
		Cid:                  shardNode.Cid(),
		Allocations:          []peer.ID{targetPeer},
		ReplicationFactorMin: 1,
		ReplicationFactorMax: 1,
	}.ToSerial()

	return s.rpcClient.Call(targetPeer, "Cluster", "Pin", pinS, &struct{}{})
}

/* Open questions

1. getAssignment()
   How to decide how much space to give out?  There seems to be an infinity
   of different ways to do this so ultimately I see this being configurable
   perhaps keeping strategies in the allocator config.  Orthoganal to this
   is deciding WHICH peer gets the next shard.  Currently this is the choice
   the cluster pin allocator makes and of course there is a rabbit hole of
   work to get different properties and guarantees from this decision too.
   Perhaps these two decisions will be made as part of a more general
   decision incorporating other things as well?

   For now I am assuming the WHICH peer decision is made first (probably
   the peer with most open disk space) and want a way to answer HOW MUCH
   space do we take from this peer to store a shard?  A few basic ideas:

   A. All nodes offer up some fraction of their space (say 2/3) during each
      allocation.  The emptiest nodes are allocated first. The second
      alloc gets 2/3 of remaining 1/3 = 2/6 of the total empty disk space
      etc.
   B. Same as A but saturates in a finite number of steps.  First allocate
      2/3 of remaining. If peer is allocated again allocate remaining 1/3
   C. All nodes offer up min space that any node can afford.  All nodes
      are allocated before repeating allocations (this bleeds into the
      WHICH node decision)

  Note pseudo-code as it stands doesn't take into account the shard node
  when keeping block put data within threshold.  Size can't be determined
  beforehand because blocks may be of different sizes.

  We will probably be reusing the pin allocator.  Where is the best place for
  the HOW MUCH decision code, generally in an allocator or here in the
  sharder responding to information received from informer calls?

2. Shard node (cluster dag) format
   In the current go-ipfs landscape downloading resolvers for non-standard
   ipld formats (i.e. everything but unixfs, and I think dag-cbor) is
   a difficult requirement to ask from nodes in the cluster.

   For this reason I think we should use one of ipfs's standard ipld
   formats.  Is dag-cbor ok to use? Probably preferrable over unixfs
   protobuf if we can help it.

3. We'll need to think about serializing nodes across rpc.  This may be
   simple I need to dig into this.  Depending on the node format that
   files are imported to we might need multiple api types for multiple
   formats.  We might be able to guarantee one format from our importer
   though.


4. Safety
   There is a lot to think about here.  We have already discussed that
   there is a gc problem.  Using block put blocks are not pinned.  In a
   sense this is good because we can pin all blocks at once through the
   cluster shard after all blocks are inserted without having to unpin
   each individual.  However if a gc is triggered before the shard
   root is pinned then work will be lost.  If we pin each block individually
   using object put then we have the concern of unpinning all individual
   blocks to keep ipfs state clean.

   Is concern about pinning all blocks warranted?  We wouldn't be tracking
   these pins in ipfs state.  Maybe it is detrimental to have so many pins
   in ipfs pinset.  If we unpin a cluster shard from cluster maybe the blocks
   will stay which is annoying-unanticipated-bad-design behavior.

   Potentially we could have a --safe flag or config option to do slow
   pinning of every block and unpin after shard root is safely pinned.

   More generally, in the event something happens to a peer (network lost,
   shutdown) what do we do?  The way the pseudocode works each
   peer needs to be available for the entire time it takes to fullfill their
   assignment.  Block data is not stored on the ingesting cluster peer, it
   is sent over to the assigned peer immediately and only the link is
   recorded.  We COULD cache an entire peer's worth of data locally
   and then redo the transmission to a new assignee if it fails.  This though
   leads to problems when the ingesting peer has less capacity than
   the target assigned peer, particulary as the ingesting peer is keeping
   all data in memory.

   Likely cluster sharded adds will fail and we'll need to offer recourse
   to users.  How will they know which cids are remnants of the import if
   they want to remove them?  Should we track ALL block links before
   completion so that we can provide a command for automatic cleanup?
   We do have a distributed log primitive in cluster, perhaps our raft
   log should be more involved in the process coordinating file ingestion,
   after all we have this log to maintain consistency among cluster peers
   in the face of failures when they are trying to coordinate tasks together.

   I have a lot more ideas jumping off from the points made here, but let's
   start here.  After we have agreed on our desired guarantees, our desired
   UX for avoiding and handling bad failures and our acceptable solution
   space (ex: maybe we REALLY don't want to add ingestion management ops to
   the log) then we can dig into this more deeply.

*/
