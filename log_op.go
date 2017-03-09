package ipfscluster

import (
	"context"
	"errors"

	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	consensus "github.com/libp2p/go-libp2p-consensus"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
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
	state, ok := cstate.(State)
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
