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

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	logging "github.com/ipfs/go-log"
	consensus "github.com/libp2p/go-libp2p-consensus"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	libp2praft "github.com/libp2p/go-libp2p-raft"
	ma "github.com/multiformats/go-multiaddr"
)

var logger = logging.Logger("consensus")

// Consensus handles the work of keeping a shared-state between
// the peers of an IPFS Cluster, as well as modifying that state and
// applying any updates in a thread-safe manner.
type Consensus struct {
	ctx    context.Context
	cancel func()
	config *Config

	host host.Host

	consensus consensus.OpLogConsensus
	actor     consensus.Actor
	baseOp    *LogOp
	raft      *raftWrapper

	rpcClient *rpc.Client
	rpcReady  chan struct{}
	readyCh   chan struct{}

	shutdownLock sync.Mutex
	shutdown     bool
}

// NewConsensus builds a new ClusterConsensus component. The state
// is used to initialize the Consensus system, so any information in it
// is discarded.
func NewConsensus(clusterPeers []peer.ID, host host.Host, cfg *Config, state state.State) (*Consensus, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	op := &LogOp{
		ctx: context.Background(),
	}

	logger.Infof("starting Consensus and waiting for a leader...")
	consensus := libp2praft.NewOpLog(state, op)
	raft, err := newRaftWrapper(clusterPeers, host, cfg, consensus.FSM())
	if err != nil {
		logger.Error("error creating raft: ", err)
		return nil, err
	}
	actor := libp2praft.NewActor(raft.raft)
	consensus.SetActor(actor)

	ctx, cancel := context.WithCancel(context.Background())
	op.ctx = ctx

	cc := &Consensus{
		ctx:       ctx,
		cancel:    cancel,
		config:    cfg,
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
	leaderCtx, cancel := context.WithTimeout(
		cc.ctx,
		cc.config.WaitForLeaderTimeout)
	defer cancel()
	_, err := cc.raft.WaitForLeader(leaderCtx)
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

	// Raft Shutdown
	err := cc.raft.Shutdown()
	if err != nil {
		logger.Error(err)
		return err
	}
	cc.shutdown = true
	cc.cancel()
	close(cc.rpcReady)
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
// note that if the leader just dissappeared, the rpc call will
// fail because we haven't heard that it's gone.
func (cc *Consensus) redirectToLeader(method string, arg interface{}) (bool, error) {
	var finalErr error

	// Retry redirects
	for i := 0; i <= cc.config.CommitRetries; i++ {
		logger.Debugf("redirect try %d", i)
		leader, err := cc.Leader()

		// No leader, wait for one
		if err != nil {
			logger.Warningf("there seems to be no leader. Waiting for one")
			rctx, cancel := context.WithTimeout(
				cc.ctx,
				cc.config.WaitForLeaderTimeout)
			defer cancel()
			pidstr, err := cc.raft.WaitForLeader(rctx)

			// means we timed out waiting for a leader
			// we don't retry in this case
			if err != nil {
				return false, errors.New("timed out waiting for leader")
			}
			leader, err = peer.IDB58Decode(pidstr)
			if err != nil {
				return false, err
			}
		}

		// We are the leader. Do not redirect
		if leader == cc.host.ID() {
			return false, nil
		}

		logger.Debugf("redirecting to leader: %s", leader)
		finalErr = cc.rpcClient.Call(
			leader,
			"Cluster",
			method,
			arg,
			&struct{}{})
		if finalErr != nil {
			logger.Error(finalErr)
			logger.Info("retrying to redirect request to leader")
			time.Sleep(2 * cc.config.RaftConfig.HeartbeatTimeout)
			continue
		}
		break
	}

	// We tried to redirect, but something happened
	return true, finalErr
}

// commit submits a cc.consensus commit. It retries upon failures.
func (cc *Consensus) commit(op *LogOp, rpcOp string, redirectArg interface{}) error {
	var finalErr error
	for i := 0; i <= cc.config.CommitRetries; i++ {
		logger.Debug("attempt #%d: committing %+v", i, op)

		// this means we are retrying
		if finalErr != nil {
			logger.Error("retrying upon failed commit (retry %d): ",
				i, finalErr)
		}

		// try to send it to the leader
		// redirectToLeader has it's own retry loop. If this fails
		// we're done here.
		ok, err := cc.redirectToLeader(rpcOp, redirectArg)
		if err != nil || ok {
			return err
		}

		// Being here means we are the LEADER. We can commit.

		// now commit the changes to our state
		_, finalErr := cc.consensus.CommitOp(op)
		if finalErr != nil {
			goto RETRY
		}

		// addPeer and rmPeer need to apply the change to Raft directly.
		switch op.Type {
		case LogOpAddPeer:
			pidstr := parsePIDFromMultiaddr(op.Peer.ToMultiaddr())
			finalErr = cc.raft.AddPeer(pidstr)
			if finalErr != nil {
				goto RETRY
			}
			logger.Infof("peer committed to global state: %s", pidstr)
		case LogOpRmPeer:
			pidstr := parsePIDFromMultiaddr(op.Peer.ToMultiaddr())
			finalErr = cc.raft.RemovePeer(pidstr)
			if finalErr != nil {
				goto RETRY
			}
			logger.Infof("peer removed from global state: %s", pidstr)
		}
		break

	RETRY:
		time.Sleep(cc.config.CommitRetryDelay)
	}
	return finalErr
}

// LogPin submits a Cid to the shared state of the cluster. It will forward
// the operation to the leader if this is not it.
func (cc *Consensus) LogPin(pin api.Pin) error {
	op := cc.op(pin, LogOpPin)
	err := cc.commit(op, "ConsensusLogPin", pin.ToSerial())
	if err != nil {
		return err
	}
	logger.Infof("pin committed to global state: %s", pin.Cid)
	return nil
}

// LogUnpin removes a Cid from the shared state of the cluster.
func (cc *Consensus) LogUnpin(pin api.Pin) error {
	op := cc.op(pin, LogOpUnpin)
	err := cc.commit(op, "ConsensusLogUnpin", pin.ToSerial())
	if err != nil {
		return err
	}
	logger.Infof("unpin committed to global state: %s", pin.Cid)
	return nil
}

// LogAddPeer submits a new peer to the shared state of the cluster. It will
// forward the operation to the leader if this is not it.
func (cc *Consensus) LogAddPeer(addr ma.Multiaddr) error {
	addrS := api.MultiaddrToSerial(addr)
	op := cc.op(addr, LogOpAddPeer)
	return cc.commit(op, "ConsensusLogAddPeer", addrS)
}

// LogRmPeer removes a peer from the shared state of the cluster. It will
// forward the operation to the leader if this is not it.
func (cc *Consensus) LogRmPeer(pid peer.ID) error {
	// Create rmPeer operation for the log
	addr, err := ma.NewMultiaddr("/ipfs/" + peer.IDB58Encode(pid))
	if err != nil {
		return err
	}
	op := cc.op(addr, LogOpRmPeer)
	return cc.commit(op, "ConsensusLogRmPeer", pid)
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
	// Note the hard-dependency on raft here...
	raftactor := cc.actor.(*libp2praft.Actor)
	return raftactor.Leader()
}

// Rollback replaces the current agreed-upon
// state with the state provided. Only the consensus leader
// can perform this operation.
func (cc *Consensus) Rollback(state state.State) error {
	// This is unused. It *might* be used for upgrades.
	// There is rather untested magic in libp2p-raft's FSM()
	// to make this possible.
	return cc.consensus.Rollback(state)
}

func parsePIDFromMultiaddr(addr ma.Multiaddr) string {
	pidstr, err := addr.ValueForProtocol(ma.P_IPFS)
	if err != nil {
		panic("peer badly encoded")
	}
	return pidstr
}
