package raft

import (
	"context"
	"errors"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/state"

	consensus "gx/ipfs/QmZ88KbrvZMJpXaNwAGffswcYKz8EbeafzAFGMCA6MEZKt/go-libp2p-consensus"
	rpc "gx/ipfs/QmayPizdYNaSKGyFFxcjKf4ZkZ6kriQePqZkFwZQyvteDp/go-libp2p-gorpc"
	ma "gx/ipfs/QmcyqRMCAXVtYPS4DiBrA7sezL9rRGfW8Ctx7cywL4TXJj/go-multiaddr"
	peer "gx/ipfs/QmdS9KpbDyPrieswibZhkod1oXqRwZJrUPzxCofAMWpFGq/go-libp2p-peer"
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
	ctx       context.Context
	rpcClient *rpc.Client
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
		arg := op.Cid.ToPin()
		err = state.Add(arg)
		if err != nil {
			goto ROLLBACK
		}
		// Async, we let the PinTracker take care of any problems
		op.rpcClient.Go("",
			"Cluster",
			"Track",
			arg.ToSerial(),
			&struct{}{},
			nil)
	case LogOpUnpin:
		arg := op.Cid.ToPin()
		err = state.Rm(arg.Cid)
		if err != nil {
			goto ROLLBACK
		}
		// Async, we let the PinTracker take care of any problems
		op.rpcClient.Go("",
			"Cluster",
			"Untrack",
			arg.ToSerial(),
			&struct{}{},
			nil)
	case LogOpAddPeer:
		addr := op.Peer.ToMultiaddr()
		op.rpcClient.Call("",
			"Cluster",
			"PeerManagerAddPeer",
			api.MultiaddrToSerial(addr),
			&struct{}{})
		// TODO rebalance ops
	case LogOpRmPeer:
		addr := op.Peer.ToMultiaddr()
		pidstr, err := addr.ValueForProtocol(ma.P_IPFS)
		if err != nil {
			panic("peer badly encoded")
		}
		pid, err := peer.IDB58Decode(pidstr)
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
