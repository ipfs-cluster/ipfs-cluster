package sharder

import (
	"errors"
	"fmt"

	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
	uuid "github.com/satori/go.uuid"
)

var logger = logging.Logger("sharder")

type sessionState struct {
	// State of current shard
	assignedPeer  peer.ID
	currentShard  shardObj
	byteCount     uint64
	byteThreshold uint64

	// Global session state
	shardNodes shardObj
}

// Sharder aggregates incident ipfs file dag nodes into a shard, or group of
// nodes.  The Sharder builds a reference node in the ipfs-cluster DAG to
// reference the nodes in a shard.  This component distributes shards among
// cluster peers based on the decisions of the cluster allocator
type Sharder struct {
	rpcClient   *rpc.Client
	idToSession map[string]*sessionState
	allocSize   uint64
	currentID   string
}

// 		currentShard: make(map[string]*cid.Cid),

// NewSharder returns a new sharder for use by an ipfs-cluster.  In the future
// this may take in a shard-config
func NewSharder(cfg *Config) (*Sharder, error) {
	logger.Debugf("The alloc size provided: %d", cfg.AllocSize)
	return &Sharder{
		allocSize:   cfg.AllocSize,
		idToSession: make(map[string]*sessionState),
	}, nil
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

// TODO: decide on whether this is worth including
// see clusterDAG.go:byteCount for more thoughts on this
// metaDataBytes tracks the number of bytes in the serialized cluster
// DAG node used to track this shard.  As this is called to determine
// whether a new links can be added, metaDataBytes takes into account
// the bytes that would be added with a new link added to the shard node
/*func (s *sessionState) metaDataBytes() uint64 {
	current := byteCount(s.currentShard)
	new := deltaByteCount(s.currentShard)
	return current + new
}*/

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
	allocInfo := api.AllocateInfo{
		Cid:        "",
		Current:    nil,
		Candidates: candidates,
	}

	allocs := make([]peer.ID, 0)
	err = s.rpcClient.Call("",
		"Cluster",
		"Allocate",
		allocInfo,
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
	session, ok := s.idToSession[s.currentID]
	if !ok {
		return errors.New("session ID not set on entry call")
	}
	session.assignedPeer, session.byteThreshold, err = s.getAssignment()
	if err != nil {
		return err
	}
	session.byteCount = 0
	session.currentShard = make(map[string]*cid.Cid)
	return nil
}

// AddNode includes the provided node into a shard in the cluster DAG
// that tracks this node's graph
func (s *Sharder) AddNode(size uint64, data []byte, cidserial string, id string) (string, error) {
	if id == "" {
		u, err := uuid.NewV4()
		if err != nil {
			return id, err
		}
		id = u.String()
	}
	s.currentID = id
	c, err := cid.Decode(cidserial)
	if err != nil {
		return id, err
	}
	blockSize := uint64(len(data))

	logger.Debug("adding node to shard")
	// Sharding session for this file is uninit
	session, ok := s.idToSession[id]
	if !ok {
		logger.Debugf("Initializing sharding session for id: %s", id)
		s.idToSession[id] = &sessionState{
			shardNodes: make(map[string]*cid.Cid),
		}
		logger.Debug("Initializing first shard")
		if err := s.initShard(); err != nil {
			logger.Debug("Error initializing shard")
			delete(s.idToSession, id) // never map to uninit session
			return id, err
		}
		session = s.idToSession[id]
	} else { // Data exceeds shard threshold, flush and start a new shard
		// TODO: evaluate whether we should count metadata bytes here too
		if session.byteCount+blockSize > session.byteThreshold {
			logger.Debug("shard at capacity, pin cluster DAG node")
			if err := s.flush(); err != nil {
				return id, err
			}
			if err := s.initShard(); err != nil {
				return id, err
			}
		}
	}

	// Shard is initialized and can accommodate node by config-enforced
	// invariant that shard size is always greater than the ipfs block
	// max chunk size
	logger.Debugf("Adding size: %d to byteCount at: %d", size, session.byteCount)
	session.byteCount += size

	key := fmt.Sprintf("%d", len(session.currentShard))
	session.currentShard[key] = c
	format, ok := cid.CodecToStr[c.Type()]
	if !ok {
		format = ""
		logger.Warning("unsupported cid type, treating as v0")
	}
	if c.Prefix().Version == 0 {
		format = "v0"
	}
	b := api.NodeWithMeta{
		Data:   data,
		Format: format,
	}
	var retStr string
	return id, s.rpcClient.Call(session.assignedPeer, "Cluster", "IPFSBlockPut",
		b, &retStr)
}

// Finalize completes a sharding session.  It flushes the final shard to the
// cluster, stores the root of the cluster DAG referencing all shard node roots
// and frees session state
func (s *Sharder) Finalize(id string) error {
	s.currentID = id
	session, ok := s.idToSession[id]
	if !ok {
		return errors.New("cannot finalize untracked id")
	}
	// call flush
	if err := s.flush(); err != nil {
		return err
	}

	// construct cluster DAG root
	shardRootNodes, err := makeDAG(session.shardNodes)
	if err != nil {
		return err
	}

	for _, shardRoot := range shardRootNodes {
		// block put the cluster DAG root nodes in local node
		b := api.NodeWithMeta{
			Data:   shardRoot.RawData(),
			Format: "cbor",
		}
		logger.Debugf("The serialized shard root cid: %s", shardRoot.Cid().String())
		var retStr string
		err = s.rpcClient.Call("", "Cluster", "IPFSBlockPut", b, &retStr)
		if err != nil {
			return err
		}
	}

	// Pin root node of rootDAG everywhere non recursively
	//    right now not sure if un-recursive pins exists for cluster pin
	//    I think we need to implement this into the Pin RPCs (can add to CLI too)

	// clear session state from sharder component
	delete(s.idToSession, s.currentID)
	return nil
}

// flush completes the allocation of the current shard of a session by adding the
// clusterDAG shard node block to IPFS. It must only be called by an entrypoint
// that has already set the currentID
func (s *Sharder) flush() error {
	// Serialize shard node and reset state
	session, ok := s.idToSession[s.currentID]
	if !ok {
		return errors.New("session ID not set on entry call")
	}
	logger.Debugf("flushing the current shard %v", session.currentShard)
	shardNodes, err := makeDAG(session.currentShard)
	if err != nil {
		return err
	}
	targetPeer := session.assignedPeer
	session.currentShard = make(map[string]*cid.Cid)
	session.assignedPeer = peer.ID("")
	session.byteThreshold = 0
	session.byteCount = 0

	for _, shardNode := range shardNodes {
		logger.Debugf("The dag cbor Node Links: %v", shardNode.Links())
		var retStr string
		b := api.NodeWithMeta{
			Data:   shardNode.RawData(),
			Format: "cbor",
		}
		logger.Debugf("Here is the serialized ipld: %x", b.Data)
		err = s.rpcClient.Call(targetPeer, "Cluster", "IPFSBlockPut",
			b, &retStr)
		if err != nil {
			return err
		}
	}

	// Track shardNodeDAG root within clusterDAG
	key := fmt.Sprintf("%d", len(session.shardNodes))
	c := shardNodes[0].Cid()
	session.shardNodes[key] = c

	// Track shard node as a normal pin within cluster state
	pinS := api.Pin{
		Cid:                  shardNodes[0].Cid(),
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
