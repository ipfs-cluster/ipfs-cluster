package shard

import (
	//	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
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
	currentShard  ipld.Node
	byteCount     int
	byteThreshold int

	allocSize int
}

// NewSharder returns a new sharder for use by an ipfs-cluster.  In the future
// this may take in a shard-config
func NewSharder(cfg *Config) (*Sharder, error) {
	return &Sharder{allocSize: cfg.AllocSize}, nil
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

/*  what we really need to do is track links in a cluster dag
proto object, and only after we have tallied them all up do we
call WrapObject to serialize go slice of links into a cbor node*/
func newClusterDAGNode() (*cbor.Node, error) {
	return cbor.WrapObject(struct{}{}, mh.SHA2_256, mh.DefaultLengths[mh.SHA2_256])
}

func clusterDAGAppend(clusterNode *cbor.Node, child ipld.Node) error {
	lnk, err := ipld.MakeLink(child)
	if err != nil {
		return err
	}

}

// AddNode
func (s *Sharder) AddNode(node ipld.Node) error {

	// If this node puts us over the byte Threshold and peer is assigned
	// send off the cluster dag node to complete this shard
	//    rpcClient.Call(assignedPeer, "Cluster",
	//      "IPFSBlockPut", s.currentShard.RawData(), &api.PinSerial{})
	//    /* Actually the above should be an object put (requires new
	//       ipfs connector endpoint) so that all shard data is pinned*/
	// If assignedPeer is empty OR we went over the byte Threshold, get a
	// new assignment and reset temporary variables
	//    s.assignedPeer, s.byteThreshold = s.getAssignment()
	//    s.byteCount = 0; s.currentShard = newClusterGraphShardNode()

	// Now we can add the provided block to the assigned node
	// First we track this block in the cluster graph
	//    s.currentShard.AddNodeLinkClean(node) // This is assuming the shard is an ipfs dag proto node or has the same method
	//    byteCount = byteCount + node.Size()

	// Finish by streaming the node's block over to the assigned peer
	//  return rpcClient.Call(assignedPeer, "Cluster",
	//    "IPFSBlockPut", node.RawData(), &api.PinSerial{})

	return nil
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
