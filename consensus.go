package ipfscluster

import (
	"context"
	"errors"
	"sync"
	"time"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	consensus "github.com/libp2p/go-libp2p-consensus"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	libp2praft "github.com/libp2p/go-libp2p-raft"
)

// Type of pin operation
const (
	LogOpPin = iota + 1
	LogOpUnpin
)

// LeaderTimeout specifies how long to wait during initialization
// before failing for not having a leader.
var LeaderTimeout = 120 * time.Second

type clusterLogOpType int

// clusterLogOp represents an operation for the OpLogConsensus system.
// It implements the consensus.Op interface.
type clusterLogOp struct {
	Cid       string
	Type      clusterLogOpType
	ctx       context.Context
	rpcClient *rpc.Client
}

// ApplyTo applies the operation to the State
func (op *clusterLogOp) ApplyTo(cstate consensus.State) (consensus.State, error) {
	state, ok := cstate.(State)
	var err error
	if !ok {
		// Should never be here
		panic("received unexpected state type")
	}

	c, err := cid.Decode(op.Cid)
	if err != nil {
		// Should never be here
		panic("could not decode a CID we ourselves encoded")
	}

	switch op.Type {
	case LogOpPin:
		err := state.AddPin(c)
		if err != nil {
			goto ROLLBACK
		}
		// Async, we let the PinTracker take care of any problems
		op.rpcClient.Go("",
			"Cluster",
			"Track",
			NewCidArg(c),
			&struct{}{},
			nil)
	case LogOpUnpin:
		err := state.RmPin(c)
		if err != nil {
			goto ROLLBACK
		}
		// Async, we let the PinTracker take care of any problems
		op.rpcClient.Go("",
			"Cluster",
			"Untrack",
			NewCidArg(c),
			&struct{}{},
			nil)
	default:
		logger.Error("unknown clusterLogOp type. Ignoring")
	}
	return state, nil

ROLLBACK:
	// We failed to apply the operation to the state
	// and therefore we need to request a rollback to the
	// cluster to the previous state. This operation can only be performed
	// by the cluster leader.
	logger.Error("Rollbacks are not implemented")
	return nil, errors.New("a rollback may be necessary. Reason: " + err.Error())
}

// Consensus handles the work of keeping a shared-state between
// the peers of an IPFS Cluster, as well as modifying that state and
// applying any updates in a thread-safe manner.
type Consensus struct {
	ctx context.Context

	host host.Host

	consensus consensus.OpLogConsensus
	actor     consensus.Actor
	baseOp    *clusterLogOp
	raft      *Raft

	rpcClient *rpc.Client
	rpcReady  chan struct{}
	readyCh   chan struct{}

	shutdownLock sync.Mutex
	shutdown     bool
	shutdownCh   chan struct{}
	wg           sync.WaitGroup
}

// NewConsensus builds a new ClusterConsensus component. The state
// is used to initialize the Consensus system, so any information in it
// is discarded.
func NewConsensus(clusterPeers []peer.ID, host host.Host, dataFolder string, state State) (*Consensus, error) {
	ctx := context.Background()
	op := &clusterLogOp{
		ctx: context.Background(),
	}

	logger.Infof("starting Consensus and waiting for a leader...")
	consensus := libp2praft.NewOpLog(state, op)
	raft, err := NewRaft(clusterPeers, host, dataFolder, consensus.FSM())
	if err != nil {
		return nil, err
	}
	actor := libp2praft.NewActor(raft.raft)
	consensus.SetActor(actor)

	cc := &Consensus{
		ctx:        ctx,
		host:       host,
		consensus:  consensus,
		actor:      actor,
		baseOp:     op,
		raft:       raft,
		shutdownCh: make(chan struct{}, 1),
		rpcReady:   make(chan struct{}, 1),
		readyCh:    make(chan struct{}, 1),
	}

	cc.run()
	return cc, nil
}

func (cc *Consensus) run() {
	cc.wg.Add(1)
	go func() {
		defer cc.wg.Done()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cc.ctx = ctx
		cc.baseOp.ctx = ctx

		go func() {
			cc.finishBootstrap()
		}()
		<-cc.shutdownCh
	}()
}

// waits until there is a consensus leader and syncs the state
// to the tracker
func (cc *Consensus) finishBootstrap() {
	cc.raft.WaitForLeader(cc.ctx)
	cc.raft.WaitForUpdates(cc.ctx)
	logger.Info("Consensus state is up to date")

	// While rpc is not ready we cannot perform a sync
	select {
	case <-cc.ctx.Done():
		return
	case <-cc.rpcReady:
	}

	var pInfo []PinInfo

	_, err := cc.State()
	// only check sync if we have a state
	// avoid error on new running clusters
	if err != nil {
		logger.Debug("skipping state sync: ", err)
	} else {
		cc.rpcClient.Go(
			"",
			"Cluster",
			"StateSync",
			struct{}{},
			&pInfo,
			nil)
	}
	cc.readyCh <- struct{}{}
	logger.Debug("consensus ready") // not accurate if we are shutting down
}

// // raft stores peer add/rm operations. This is how to force a peer set.
// func (cc *Consensus) setPeers() {
// 	logger.Debug("forcefully setting Raft peers to known set")
// 	var peersStr []string
// 	var peers []peer.ID
// 	err := cc.rpcClient.Call("",
// 		"Cluster",
// 		"PeerManagerPeers",
// 		struct{}{},
// 		&peers)
// 	if err != nil {
// 		logger.Error(err)
// 		return
// 	}
// 	for _, p := range peers {
// 		peersStr = append(peersStr, p.Pretty())
// 	}
// 	cc.p2pRaft.raft.SetPeers(peersStr)
// }

// Shutdown stops the component so it will not process any
// more updates. The underlying consensus is permanently
// shutdown, along with the libp2p transport.
func (cc *Consensus) Shutdown() error {
	cc.shutdownLock.Lock()
	defer cc.shutdownLock.Unlock()

	if cc.shutdown {
		logger.Debug("already shutdown")
		return nil
	}

	logger.Info("stopping Consensus component")

	close(cc.rpcReady)
	cc.shutdownCh <- struct{}{}

	// Raft shutdown
	errMsgs := ""
	err := cc.raft.Snapshot()
	if err != nil {
		errMsgs += err.Error()
	}
	err = cc.raft.Shutdown()
	if err != nil {
		errMsgs += err.Error()
	}

	if errMsgs != "" {
		errMsgs += "Consensus shutdown unsuccessful"
		logger.Error(errMsgs)
		return errors.New(errMsgs)
	}
	cc.wg.Wait()
	cc.shutdown = true
	return nil
}

// SetClient makes the component ready to perform RPC requets
func (cc *Consensus) SetClient(c *rpc.Client) {
	cc.rpcClient = c
	cc.baseOp.rpcClient = c
	cc.rpcReady <- struct{}{}
}

// Ready returns a channel which is signaled when the Consensus
// algorithm has finished bootstrapping and is ready to use
func (cc *Consensus) Ready() <-chan struct{} {
	return cc.readyCh
}

func (cc *Consensus) op(c *cid.Cid, t clusterLogOpType) *clusterLogOp {
	return &clusterLogOp{
		Cid:  c.String(),
		Type: t,
	}
}

// returns true if the operation was redirected to the leader
func (cc *Consensus) redirectToLeader(method string, arg interface{}) (bool, error) {
	leader, err := cc.Leader()
	if err != nil {
		return false, err
	}
	if leader == cc.host.ID() {
		return false, nil
	}

	err = cc.rpcClient.Call(
		leader,
		"Cluster",
		method,
		arg,
		&struct{}{})
	return true, err
}

// LogPin submits a Cid to the shared state of the cluster. It will forward
// the operation to the leader if this is not it.
func (cc *Consensus) LogPin(c *cid.Cid) error {
	redirected, err := cc.redirectToLeader("ConsensusLogPin", NewCidArg(c))
	if err != nil || redirected {
		return err
	}

	// It seems WE are the leader.

	// Create pin operation for the log
	op := cc.op(c, LogOpPin)
	_, err = cc.consensus.CommitOp(op)
	if err != nil {
		// This means the op did not make it to the log
		return err
	}
	logger.Infof("pin committed to global state: %s", c)
	return nil
}

// LogUnpin removes a Cid from the shared state of the cluster.
func (cc *Consensus) LogUnpin(c *cid.Cid) error {
	redirected, err := cc.redirectToLeader("ConsensusLogUnpin", NewCidArg(c))
	if err != nil || redirected {
		return err
	}

	// It seems WE are the leader.

	// Create  unpin operation for the log
	op := cc.op(c, LogOpUnpin)
	_, err = cc.consensus.CommitOp(op)
	if err != nil {
		return err
	}
	logger.Infof("unpin committed to global state: %s", c)
	return nil
}

// AddPeer attempts to add a peer to the consensus.
func (cc *Consensus) AddPeer(p peer.ID) error {
	//redirected, err := cc.redirectToLeader("ConsensusAddPeer", p)
	//if err != nil || redirected {
	//		return err
	//	}

	return cc.raft.AddPeer(peer.IDB58Encode(p))
}

// RemovePeer attempts to remove a peer from the consensus.
func (cc *Consensus) RemovePeer(p peer.ID) error {
	//redirected, err := cc.redirectToLeader("ConsensusRemovePeer", p)
	//if err != nil || redirected {
	//	return err
	//}

	return cc.raft.RemovePeer(peer.IDB58Encode(p))
}

// State retrieves the current consensus State. It may error
// if no State has been agreed upon or the state is not
// consistent. The returned State is the last agreed-upon
// State known by this node.
func (cc *Consensus) State() (State, error) {
	st, err := cc.consensus.GetLogHead()
	if err != nil {
		return nil, err
	}
	state, ok := st.(State)
	if !ok {
		return nil, errors.New("wrong state type")
	}
	return state, nil
}

// Leader returns the peerID of the Leader of the
// cluster. It returns an error when there is no leader.
func (cc *Consensus) Leader() (peer.ID, error) {
	raftactor := cc.actor.(*libp2praft.Actor)
	return raftactor.Leader()
}

// Rollback replaces the current agreed-upon
// state with the state provided. Only the consensus leader
// can perform this operation.
func (cc *Consensus) Rollback(state State) error {
	return cc.consensus.Rollback(state)
}
