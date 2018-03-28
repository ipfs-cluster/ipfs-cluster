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
	replMin    int
	replMax    int
	dataRoot   *cid.Cid
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

// getAssignment returns the pid of a cluster peer that will allocate space
// for a new shard, along with a byte threshold determining the max size of
// this shard.
func (s *Sharder) getAssignment() (peer.ID, uint64, error) {
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
	allocInfo := api.AllocateInfo{
		Cid:        "",
		Current:    nil,
		Candidates: candidates,
		Priority:   nil,
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
func (s *Sharder) AddNode(
	size uint64,
	data []byte,
	cidserial string,
	id string,
	replMin, replMax int) (string, error) {
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
			replMin:    replMin,
			replMax:    replMax,
		}
		logger.Debug("Initializing first shard")
		if err := s.initShard(); err != nil {
			logger.Debug("Error initializing shard")
			delete(s.idToSession, id) // never map to uninit session
			return id, err
		}
		session = s.idToSession[id]
	} else { // Data exceeds shard threshold, flush and start a new shard
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
	return id, s.rpcClient.Call(
		session.assignedPeer,
		"Cluster",
		"IPFSBlockPut",
		b,
		&retStr,
	)
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
	if len(session.currentShard) > 0 {
		if err := s.flush(); err != nil {
			return err
		}
	}
	if session.dataRoot == nil {
		return errors.New("finalize called before adding any data")
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

	// Link dataDAG hash to clusterDAG root hash
	cdagCid := shardRootNodes[0].Cid()
	metaPinS := api.Pin{
		Cid:                  session.dataRoot,
		ReplicationFactorMin: session.replMin,
		ReplicationFactorMax: session.replMax,
		Type:                 api.MetaType,
		Clusterdag:           cdagCid,
	}.ToSerial()
	err = s.rpcClient.Call(
		"",
		"Cluster",
		"Pin",
		metaPinS,
		&struct{}{},
	)
	if err != nil {
		return err
	}

	// Pin root node of rootDAG everywhere non recursively
	cdagPinS := api.Pin{
		Cid:                  cdagCid,
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
		Type:                 api.CdagType,
		Recursive:            false,
		Parents:              []*cid.Cid{session.dataRoot},
	}.ToSerial()
	err = s.rpcClient.Call(
		"",
		"Cluster",
		"Pin",
		cdagPinS,
		&struct{}{},
	)
	if err != nil {
		return err
	}

	// Ammend ShardPins to reference clusterDAG root hash as a Parent
	for _, c := range session.shardNodes {
		pinS := api.Pin{
			Cid:                  c,
			ReplicationFactorMin: session.replMin,
			ReplicationFactorMax: session.replMax,
			Type:                 api.ShardType,
			Recursive:            true,
			Parents:              []*cid.Cid{cdagCid},
		}.ToSerial()

		err = s.rpcClient.Call(
			"",
			"Cluster",
			"Pin",
			pinS,
			&struct{}{},
		)
		if err != nil {
			return err
		}
	}

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
	// Track latest data hash
	key := fmt.Sprintf("%d", len(session.currentShard)-1)
	session.dataRoot = session.currentShard[key]

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
		err = s.rpcClient.Call(
			targetPeer,
			"Cluster",
			"IPFSBlockPut",
			b,
			&retStr,
		)
		if err != nil {
			return err
		}
	}

	// Track shardNodeDAG root within clusterDAG
	key = fmt.Sprintf("%d", len(session.shardNodes))
	c := shardNodes[0].Cid()
	session.shardNodes[key] = c

	// Track shard node as a shard pin within cluster state
	pinS := api.Pin{
		Cid:                  c,
		Allocations:          []peer.ID{targetPeer},
		ReplicationFactorMin: session.replMin,
		ReplicationFactorMax: session.replMax,
		Type:                 api.ShardType,
		Recursive:            true,
	}.ToSerial()

	return s.rpcClient.Call(
		targetPeer,
		"Cluster",
		"Pin",
		pinS,
		&struct{}{},
	)
}
