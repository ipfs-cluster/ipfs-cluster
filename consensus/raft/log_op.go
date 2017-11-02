package raft

import (
	"context"
	"errors"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/state"

	consensus "github.com/libp2p/go-libp2p-consensus"
	peer "github.com/libp2p/go-libp2p-peer"
)

// Type of consensus operation
const (
	LogOpPin = iota + 1
	LogOpUnpin
	LogOpAddPeer
	LogOpRmPeer
)

// LogOpType expresses the type of a consensus Operation
type LogOpType int

// LogOp represents an operation for the OpLogConsensus system.
// It implements the consensus.Op interface and it is used by the
// Consensus component.
type LogOp struct {
	Cid       api.PinSerial
	Peer      api.MultiaddrSerial
	Type      LogOpType
	consensus *Consensus
}

// ApplyTo applies the operation to the State
func (op *LogOp) ApplyTo(cstate consensus.State) (consensus.State, error) {
	state, ok := cstate.(state.State)
	var err error
	if !ok {
		// Should never be here
		panic("received unexpected state type")
	}

	switch op.Type {
	case LogOpPin:
		err = state.Add(op.Cid.ToPin())
		if err != nil {
			goto ROLLBACK
		}
		// Async, we let the PinTracker take care of any problems
		op.consensus.rpcClient.Go("",
			"Cluster",
			"Track",
			op.Cid,
			&struct{}{},
			nil)
	case LogOpUnpin:
		err = state.Rm(op.Cid.ToPin().Cid)
		if err != nil {
			goto ROLLBACK
		}
		// Async, we let the PinTracker take care of any problems
		op.consensus.rpcClient.Go("",
			"Cluster",
			"Untrack",
			op.Cid,
			&struct{}{},
			nil)
	case LogOpAddPeer:
		// pidstr := parsePIDFromMultiaddr(op.Peer.ToMultiaddr())

		op.consensus.rpcClient.Call("",
			"Cluster",
			"PeerManagerAddPeer",
			op.Peer,
			&struct{}{})

	case LogOpRmPeer:
		pidstr := parsePIDFromMultiaddr(op.Peer.ToMultiaddr())
		pid, err := peer.IDB58Decode(pidstr)
		if err != nil {
			panic("could not decode a PID we ourselves encoded")
		}

		// Asynchronously wait for peer to be removed from raft
		// and remove it from the peerset. Otherwise do nothing
		go func() {
			ctx, cancel := context.WithTimeout(op.consensus.ctx,
				10*time.Second)
			defer cancel()

			// Do not wait if we are being removed
			// as it may just hang waiting for a future.
			if pid != op.consensus.host.ID() {
				err = op.consensus.raft.WaitForPeer(ctx, pidstr, true)
				if err != nil {
					if err.Error() != errWaitingForSelf.Error() {
						logger.Warningf("Peer has not been removed from raft: %s: %s", pidstr, err)
					}
					return
				}
			}
			op.consensus.rpcClient.Call("",
				"Cluster",
				"PeerManagerRmPeer",
				pid,
				&struct{}{})
		}()

	default:
		logger.Error("unknown LogOp type. Ignoring")
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
