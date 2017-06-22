// Package raft implements a Consensus component for IPFS Cluster which uses
// Raft (go-libp2p-raft).
package raft

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/state"

	logging "gx/ipfs/QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52/go-log"
	libp2praft "gx/ipfs/QmUqn6GHVKewa8H4L8mEwWFV27R4xb3s9LB9qxNxGGqxgy/go-libp2p-raft"
	host "gx/ipfs/QmUywuGNZoUKV8B9iyvup9bPkLiMrhTsyVMkeSXW5VxAfC/go-libp2p-host"
	consensus "gx/ipfs/QmZ88KbrvZMJpXaNwAGffswcYKz8EbeafzAFGMCA6MEZKt/go-libp2p-consensus"
	rpc "gx/ipfs/QmayPizdYNaSKGyFFxcjKf4ZkZ6kriQePqZkFwZQyvteDp/go-libp2p-gorpc"
	ma "gx/ipfs/QmcyqRMCAXVtYPS4DiBrA7sezL9rRGfW8Ctx7cywL4TXJj/go-multiaddr"
	peer "gx/ipfs/QmdS9KpbDyPrieswibZhkod1oXqRwZJrUPzxCofAMWpFGq/go-libp2p-peer"
)

var logger = logging.Logger("consensus")

// LeaderTimeout specifies how long to wait before failing an operation
// because there is no leader
var LeaderTimeout = 15 * time.Second

// CommitRetries specifies how many times we retry a failed commit until
// we give up
var CommitRetries = 2

// Consensus handles the work of keeping a shared-state between
// the peers of an IPFS Cluster, as well as modifying that state and
// applying any updates in a thread-safe manner.
type Consensus struct {
	ctx    context.Context
	cancel func()

	host host.Host

	consensus consensus.OpLogConsensus
	actor     consensus.Actor
	baseOp    *LogOp
	raft      *Raft

	rpcClient *rpc.Client
	rpcReady  chan struct{}
	readyCh   chan struct{}

	shutdownLock sync.Mutex
	shutdown     bool
}

// NewConsensus builds a new ClusterConsensus component. The state
// is used to initialize the Consensus system, so any information in it
// is discarded.
func NewConsensus(clusterPeers []peer.ID, host host.Host, dataFolder string, state state.State) (*Consensus, error) {
	op := &LogOp{
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

	ctx, cancel := context.WithCancel(context.Background())
	op.ctx = ctx

	cc := &Consensus{
		ctx:       ctx,
		cancel:    cancel,
		host:      host,
		consensus: consensus,
		actor:     actor,
		baseOp:    op,
		raft:      raft,
		rpcReady:  make(chan struct{}, 1),
		readyCh:   make(chan struct{}, 1),
	}

	go cc.finishBootstrap()
	return cc, nil
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
		var pInfoSerial []api.PinInfoSerial
		cc.rpcClient.Go(
			"",
			"Cluster",
			"StateSync",
			struct{}{},
			&pInfoSerial,
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

	cc.cancel()
	close(cc.rpcReady)

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

func (cc *Consensus) op(argi interface{}, t LogOpType) *LogOp {
	switch argi.(type) {
	case api.Pin:
		return &LogOp{
			Cid:  argi.(api.Pin).ToSerial(),
			Type: t,
		}
	case ma.Multiaddr:
		return &LogOp{
			Peer: api.MultiaddrToSerial(argi.(ma.Multiaddr)),
			Type: t,
		}
	default:
		panic("bad type")
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

func (cc *Consensus) logOpCid(rpcOp string, opType LogOpType, pin api.Pin) error {
	var finalErr error
	for i := 0; i < CommitRetries; i++ {
		logger.Debugf("Try %d", i)
		redirected, err := cc.redirectToLeader(
			rpcOp, pin.ToSerial())
		if err != nil {
			finalErr = err
			continue
		}

		if redirected {
			return nil
		}

		// It seems WE are the leader.

		op := cc.op(pin, opType)
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
		logger.Infof("pin committed to global state: %s", pin.Cid)
	case LogOpUnpin:
		logger.Infof("unpin committed to global state: %s", pin.Cid)
	}
	return nil
}

// LogPin submits a Cid to the shared state of the cluster. It will forward
// the operation to the leader if this is not it.
func (cc *Consensus) LogPin(c api.Pin) error {
	return cc.logOpCid("ConsensusLogPin", LogOpPin, c)
}

// LogUnpin removes a Cid from the shared state of the cluster.
func (cc *Consensus) LogUnpin(c api.Pin) error {
	return cc.logOpCid("ConsensusLogUnpin", LogOpUnpin, c)
}

// LogAddPeer submits a new peer to the shared state of the cluster. It will
// forward the operation to the leader if this is not it.
func (cc *Consensus) LogAddPeer(addr ma.Multiaddr) error {
	var finalErr error
	for i := 0; i < CommitRetries; i++ {
		logger.Debugf("Try %d", i)
		redirected, err := cc.redirectToLeader(
			"ConsensusLogAddPeer", api.MultiaddrToSerial(addr))
		if err != nil {
			finalErr = err
			continue
		}

		if redirected {
			return nil
		}

		// It seems WE are the leader.
		pidStr, err := addr.ValueForProtocol(ma.P_IPFS)
		if err != nil {
			return err
		}
		pid, err := peer.IDB58Decode(pidStr)
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
		addr, err := ma.NewMultiaddr("/ipfs/" + peer.IDB58Encode(pid))
		if err != nil {
			return err
		}
		op := cc.op(addr, LogOpRmPeer)
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
func (cc *Consensus) State() (state.State, error) {
	st, err := cc.consensus.GetLogHead()
	if err != nil {
		return nil, err
	}
	state, ok := st.(state.State)
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
func (cc *Consensus) Rollback(state state.State) error {
	return cc.consensus.Rollback(state)
}
