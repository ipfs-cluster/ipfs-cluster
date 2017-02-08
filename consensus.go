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
	ma "github.com/multiformats/go-multiaddr"
)

// Type of pin operation
const (
	LogOpPin = iota + 1
	LogOpUnpin
	LogOpAddPeer
	LogOpRmPeer
)

// LeaderTimeout specifies how long to wait before failing an operation
// because there is no leader
var LeaderTimeout = 15 * time.Second

// CommitRetries specifies how many times we retry a failed commit until
// we give up
var CommitRetries = 2

type clusterLogOpType int

// clusterLogOp represents an operation for the OpLogConsensus system.
// It implements the consensus.Op interface.
type clusterLogOp struct {
	Arg       string
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

	switch op.Type {
	case LogOpPin:
		c, err := cid.Decode(op.Arg)
		if err != nil {
			panic("could not decode a CID we ourselves encoded")
		}
		err = state.AddPin(c)
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
		c, err := cid.Decode(op.Arg)
		if err != nil {
			panic("could not decode a CID we ourselves encoded")
		}
		err = state.RmPin(c)
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
	case LogOpAddPeer:
		addr, err := ma.NewMultiaddr(op.Arg)
		if err != nil {
			panic("could not decode a multiaddress we ourselves encoded")
		}
		op.rpcClient.Call("",
			"Cluster",
			"PeerManagerAddPeer",
			MultiaddrToSerial(addr),
			&struct{}{})
		// TODO rebalance ops
	case LogOpRmPeer:
		pid, err := peer.IDB58Decode(op.Arg)
		if err != nil {
			panic("could not decode a PID we ourselves encoded")
		}
		op.rpcClient.Call("",
			"Cluster",
			"PeerManagerRmPeer",
			pid,
			&struct{}{})
		// TODO rebalance ops
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

		go cc.finishBootstrap()
		<-cc.shutdownCh
	}()
}

// WaitForSync waits for a leader and for the state to be up to date, then returns.
func (cc *Consensus) WaitForSync() error {
	leaderCtx, cancel := context.WithTimeout(cc.ctx, LeaderTimeout)
	defer cancel()
	err := cc.raft.WaitForLeader(leaderCtx)
	if err != nil {
		return errors.New("error waiting for leader: " + err.Error())
	}
	err = cc.raft.WaitForUpdates(cc.ctx)
	if err != nil {
		return errors.New("error waiting for consensus updates: " + err.Error())
	}
	return nil
}

// waits until there is a consensus leader and syncs the state
// to the tracker
func (cc *Consensus) finishBootstrap() {
	err := cc.WaitForSync()
	if err != nil {
		return
	}
	logger.Info("Consensus state is up to date")

	// While rpc is not ready we cannot perform a sync
	if cc.rpcClient == nil {
		select {
		case <-cc.ctx.Done():
			return
		case <-cc.rpcReady:
		}
	}

	st, err := cc.State()
	_ = st
	// only check sync if we have a state
	// avoid error on new running clusters
	if err != nil {
		logger.Debug("skipping state sync: ", err)
	} else {
		var pInfo []PinInfo
		cc.rpcClient.Go(
			"",
			"Cluster",
			"StateSync",
			struct{}{},
			&pInfo,
			nil)
	}
	cc.readyCh <- struct{}{}
	logger.Debug("consensus ready")
}

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

func (cc *Consensus) op(argi interface{}, t clusterLogOpType) *clusterLogOp {
	var arg string
	switch argi.(type) {
	case *cid.Cid:
		arg = argi.(*cid.Cid).String()
	case peer.ID:
		arg = peer.IDB58Encode(argi.(peer.ID))
	case ma.Multiaddr:
		arg = argi.(ma.Multiaddr).String()
	default:
		panic("bad type")
	}
	return &clusterLogOp{
		Arg:  arg,
		Type: t,
	}
}

// returns true if the operation was redirected to the leader
func (cc *Consensus) redirectToLeader(method string, arg interface{}) (bool, error) {
	leader, err := cc.Leader()
	if err != nil {
		rctx, cancel := context.WithTimeout(cc.ctx, LeaderTimeout)
		defer cancel()
		err := cc.raft.WaitForLeader(rctx)
		if err != nil {
			return false, err
		}
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

func (cc *Consensus) logOpCid(rpcOp string, opType clusterLogOpType, c *cid.Cid) error {
	var finalErr error
	for i := 0; i < CommitRetries; i++ {
		logger.Debugf("Try %d", i)
		redirected, err := cc.redirectToLeader(rpcOp, NewCidArg(c))
		if err != nil {
			finalErr = err
			continue
		}

		if redirected {
			return nil
		}

		// It seems WE are the leader.

		// Create pin operation for the log
		op := cc.op(c, opType)
		_, err = cc.consensus.CommitOp(op)
		if err != nil {
			// This means the op did not make it to the log
			finalErr = err
			time.Sleep(200 * time.Millisecond)
			continue
		}
		finalErr = nil
		break
	}
	if finalErr != nil {
		return finalErr
	}

	switch opType {
	case LogOpPin:
		logger.Infof("pin committed to global state: %s", c)
	case LogOpUnpin:
		logger.Infof("unpin committed to global state: %s", c)
	}
	return nil
}

// LogPin submits a Cid to the shared state of the cluster. It will forward
// the operation to the leader if this is not it.
func (cc *Consensus) LogPin(c *cid.Cid) error {
	return cc.logOpCid("ConsensusLogPin", LogOpPin, c)
}

// LogUnpin removes a Cid from the shared state of the cluster.
func (cc *Consensus) LogUnpin(c *cid.Cid) error {
	return cc.logOpCid("ConsensusLogUnpin", LogOpUnpin, c)
}

// LogAddPeer submits a new peer to the shared state of the cluster. It will
// forward the operation to the leader if this is not it.
func (cc *Consensus) LogAddPeer(addr ma.Multiaddr) error {
	var finalErr error
	for i := 0; i < CommitRetries; i++ {
		logger.Debugf("Try %d", i)
		redirected, err := cc.redirectToLeader("ConsensusLogAddPeer", MultiaddrToSerial(addr))
		if err != nil {
			finalErr = err
			continue
		}

		if redirected {
			return nil
		}

		// It seems WE are the leader.
		pid, _, err := multiaddrSplit(addr)
		if err != nil {
			return err
		}

		// Create pin operation for the log
		op := cc.op(addr, LogOpAddPeer)
		_, err = cc.consensus.CommitOp(op)
		if err != nil {
			// This means the op did not make it to the log
			finalErr = err
			time.Sleep(200 * time.Millisecond)
			continue
		}
		err = cc.raft.AddPeer(peer.IDB58Encode(pid))
		if err != nil {
			finalErr = err
			continue
		}
		finalErr = nil
		break
	}
	if finalErr != nil {
		return finalErr
	}
	logger.Infof("peer committed to global state: %s", addr)
	return nil
}

// LogRmPeer removes a peer from the shared state of the cluster. It will
// forward the operation to the leader if this is not it.
func (cc *Consensus) LogRmPeer(pid peer.ID) error {
	var finalErr error
	for i := 0; i < CommitRetries; i++ {
		logger.Debugf("Try %d", i)
		redirected, err := cc.redirectToLeader("ConsensusLogRmPeer", pid)
		if err != nil {
			finalErr = err
			continue
		}

		if redirected {
			return nil
		}

		// It seems WE are the leader.

		// Create pin operation for the log
		op := cc.op(pid, LogOpRmPeer)
		_, err = cc.consensus.CommitOp(op)
		if err != nil {
			// This means the op did not make it to the log
			finalErr = err
			continue
		}
		err = cc.raft.RemovePeer(peer.IDB58Encode(pid))
		if err != nil {
			finalErr = err
			time.Sleep(200 * time.Millisecond)
			continue
		}
		finalErr = nil
		break
	}
	if finalErr != nil {
		return finalErr
	}
	logger.Infof("peer removed from global state: %s", pid)
	return nil
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
