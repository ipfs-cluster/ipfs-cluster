// Package raft implements a Consensus component for IPFS Cluster which uses
// Raft (go-libp2p-raft).
package raft

import (
	"context"
	"errors"
	"fmt"
	"sort"
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

// NewConsensus builds a new ClusterConsensus component using Raft. The state
// is used to initialize the Consensus system, so any information
// in it is discarded once the raft state is loaded.
// The singlePeer parameter controls whether this Raft peer is be expected to
// join a cluster or it should run on its own.
func NewConsensus(
	host host.Host,
	cfg *Config,
	state state.State,
	staging bool, // this peer must not be bootstrapped if no state exists
) (*Consensus, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	baseOp := &LogOp{}

	logger.Debug("starting Consensus and waiting for a leader...")
	consensus := libp2praft.NewOpLog(state, baseOp)
	raft, err := newRaftWrapper(host, cfg, consensus.FSM(), staging)
	if err != nil {
		logger.Error("error creating raft: ", err)
		return nil, err
	}
	actor := libp2praft.NewActor(raft.raft)
	consensus.SetActor(actor)

	ctx, cancel := context.WithCancel(context.Background())

	cc := &Consensus{
		ctx:       ctx,
		cancel:    cancel,
		config:    cfg,
		host:      host,
		consensus: consensus,
		actor:     actor,
		baseOp:    baseOp,
		raft:      raft,
		rpcReady:  make(chan struct{}, 1),
		readyCh:   make(chan struct{}, 1),
	}

	baseOp.consensus = cc

	go cc.finishBootstrap()
	return cc, nil
}

// WaitForSync waits for a leader and for the state to be up to date, then returns.
func (cc *Consensus) WaitForSync() error {
	leaderCtx, cancel := context.WithTimeout(
		cc.ctx,
		cc.config.WaitForLeaderTimeout)
	defer cancel()

	// 1 - wait for leader
	// 2 - wait until we are a Voter
	// 3 - wait until last index is applied

	// From raft docs:

	// once a staging server receives enough log entries to be sufficiently
	// caught up to the leader's log, the leader will invoke a  membership
	// change to change the Staging server to a Voter

	// Thus, waiting to be a Voter is a guarantee that we have a reasonable
	// up to date state. Otherwise, we might return too early (see
	// https://github.com/ipfs/ipfs-cluster/issues/378)

	_, err := cc.raft.WaitForLeader(leaderCtx)
	if err != nil {
		return errors.New("error waiting for leader: " + err.Error())
	}

	err = cc.raft.WaitForVoter(cc.ctx)
	if err != nil {
		return errors.New("error waiting to become a Voter: " + err.Error())
	}

	err = cc.raft.WaitForUpdates(cc.ctx)
	if err != nil {
		return errors.New("error waiting for consensus updates: " + err.Error())
	}
	return nil
}

// waits until there is a consensus leader and syncs the state
// to the tracker. If errors happen, this will return and never
// signal the component as Ready.
func (cc *Consensus) finishBootstrap() {
	// wait until we have RPC to perform any actions.
	if cc.rpcClient == nil {
		select {
		case <-cc.ctx.Done():
			return
		case <-cc.rpcReady:
		}
	}

	// Sometimes bootstrap is a no-op. It only applies when
	// no state exists and staging=false.
	_, err := cc.raft.Bootstrap()
	if err != nil {
		return
	}

	err = cc.WaitForSync()
	if err != nil {
		return
	}
	logger.Debug("Raft state is now up to date")
	logger.Debug("consensus ready")
	cc.readyCh <- struct{}{}
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
	}

	if cc.config.hostShutdown {
		cc.host.Close()
	}

	cc.shutdown = true
	cc.cancel()
	close(cc.rpcReady)
	return nil
}

// SetClient makes the component ready to perform RPC requets
func (cc *Consensus) SetClient(c *rpc.Client) {
	cc.rpcClient = c
	cc.rpcReady <- struct{}{}
}

// Ready returns a channel which is signaled when the Consensus
// algorithm has finished bootstrapping and is ready to use
func (cc *Consensus) Ready() <-chan struct{} {
	return cc.readyCh
}

func (cc *Consensus) op(pin api.Pin, t LogOpType) *LogOp {
	return &LogOp{
		Cid:  pin.ToSerial(),
		Type: t,
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
			logger.Warning("there seems to be no leader. Waiting for one")
			rctx, cancel := context.WithTimeout(
				cc.ctx,
				cc.config.WaitForLeaderTimeout)
			defer cancel()
			pidstr, err := cc.raft.WaitForLeader(rctx)

			// means we timed out waiting for a leader
			// we don't retry in this case
			if err != nil {
				return false, fmt.Errorf("timed out waiting for leader: %s", err)
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

		logger.Debugf("redirecting %s to leader: %s", method, leader.Pretty())
		finalErr = cc.rpcClient.Call(
			leader,
			"Cluster",
			method,
			arg,
			&struct{}{})
		if finalErr != nil {
			logger.Error(finalErr)
			logger.Error("retrying to redirect request to leader")
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
		logger.Debugf("attempt #%d: committing %+v", i, op)

		// this means we are retrying
		if finalErr != nil {
			logger.Errorf("retrying upon failed commit (retry %d): %s ",
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
		cc.shutdownLock.Lock() // do not shut down while committing
		_, finalErr = cc.consensus.CommitOp(op)
		cc.shutdownLock.Unlock()
		if finalErr != nil {
			goto RETRY
		}

		switch op.Type {
		case LogOpPin:
			logger.Infof("pin committed to global state: %s", op.Cid.Cid)
		case LogOpUnpin:
			logger.Infof("unpin committed to global state: %s", op.Cid.Cid)
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
	return nil
}

// LogUnpin removes a Cid from the shared state of the cluster.
func (cc *Consensus) LogUnpin(pin api.Pin) error {
	op := cc.op(pin, LogOpUnpin)
	err := cc.commit(op, "ConsensusLogUnpin", pin.ToSerial())
	if err != nil {
		return err
	}
	return nil
}

// AddPeer adds a new peer to participate in this consensus. It will
// forward the operation to the leader if this is not it.
func (cc *Consensus) AddPeer(pid peer.ID) error {
	var finalErr error
	for i := 0; i <= cc.config.CommitRetries; i++ {
		logger.Debugf("attempt #%d: AddPeer %s", i, pid.Pretty())
		if finalErr != nil {
			logger.Errorf("retrying to add peer. Attempt #%d failed: %s", i, finalErr)
		}
		ok, err := cc.redirectToLeader("ConsensusAddPeer", pid)
		if err != nil || ok {
			return err
		}
		// Being here means we are the leader and can commit
		cc.shutdownLock.Lock() // do not shutdown while committing
		finalErr = cc.raft.AddPeer(peer.IDB58Encode(pid))
		cc.shutdownLock.Unlock()
		if finalErr != nil {
			time.Sleep(cc.config.CommitRetryDelay)
			continue
		}
		logger.Infof("peer added to Raft: %s", pid.Pretty())
		break
	}
	return finalErr
}

// RmPeer removes a peer from this consensus. It will
// forward the operation to the leader if this is not it.
func (cc *Consensus) RmPeer(pid peer.ID) error {
	var finalErr error
	for i := 0; i <= cc.config.CommitRetries; i++ {
		logger.Debugf("attempt #%d: RmPeer %s", i, pid.Pretty())
		if finalErr != nil {
			logger.Errorf("retrying to remove peer. Attempt #%d failed: %s", i, finalErr)
		}
		ok, err := cc.redirectToLeader("ConsensusRmPeer", pid)
		if err != nil || ok {
			return err
		}
		// Being here means we are the leader and can commit
		cc.shutdownLock.Lock() // do not shutdown while committing
		finalErr = cc.raft.RemovePeer(peer.IDB58Encode(pid))
		cc.shutdownLock.Unlock()
		if finalErr != nil {
			time.Sleep(cc.config.CommitRetryDelay)
			continue
		}
		logger.Infof("peer removed from Raft: %s", pid.Pretty())
		break
	}
	return finalErr
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

// Clean removes all raft data from disk. Next time
// a full new peer will be bootstrapped.
func (cc *Consensus) Clean() error {
	if !cc.shutdown {
		return errors.New("consensus component is not shutdown")
	}

	err := cc.raft.Clean()
	if err != nil {
		return err
	}
	return nil
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

// Peers return the current list of peers in the consensus.
// The list will be sorted alphabetically.
func (cc *Consensus) Peers() ([]peer.ID, error) {
	if cc.shutdown { // things hang a lot in this case
		return nil, errors.New("consensus is shutdown")
	}
	peers := []peer.ID{}
	raftPeers, err := cc.raft.Peers()
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve list of peers: %s", err)
	}

	sort.Strings(raftPeers)

	for _, p := range raftPeers {
		id, err := peer.IDB58Decode(p)
		if err != nil {
			panic("could not decode peer")
		}
		peers = append(peers, id)
	}
	return peers, nil
}

func parsePIDFromMultiaddr(addr ma.Multiaddr) string {
	pidstr, err := addr.ValueForProtocol(ma.P_IPFS)
	if err != nil {
		panic("peer badly encoded")
	}
	return pidstr
}
