package raft

import (
	"context"
	"errors"

	"go.opencensus.io/tag"
	"go.opencensus.io/trace"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/state"

	consensus "github.com/libp2p/go-libp2p-consensus"
)

// Type of consensus operation
const (
	LogOpPin = iota + 1
	LogOpUnpin
)

// LogOpType expresses the type of a consensus Operation
type LogOpType int

// LogOp represents an operation for the OpLogConsensus system.
// It implements the consensus.Op interface and it is used by the
// Consensus component.
type LogOp struct {
	SpanCtx   trace.SpanContext
	TagCtx    []byte
	Cid       api.PinSerial
	Type      LogOpType
	consensus *Consensus
	tracing   bool
}

// ApplyTo applies the operation to the State
func (op *LogOp) ApplyTo(cstate consensus.State) (consensus.State, error) {
	var err error
	ctx := context.Background()
	if op.tracing {
		tagmap, err := tag.Decode(op.TagCtx)
		if err != nil {
			logger.Error(err)
		}
		ctx = tag.NewContext(ctx, tagmap)
		var span *trace.Span
		ctx, span = trace.StartSpanWithRemoteParent(ctx, "consensus/raft/logop/ApplyTo", op.SpanCtx)
		defer span.End()
	}

	state, ok := cstate.(state.State)
	if !ok {
		// Should never be here
		panic("received unexpected state type")
	}

	// Copy the Cid. We are about to pass it to go-routines
	// that will make things with it (read its fields). However,
	// as soon as ApplyTo is done, the next operation will be deserealized
	// on top of "op". This can cause data races with the slices in
	// api.PinSerial, which don't get copied when passed.
	pinS := op.Cid.Clone()

	switch op.Type {
	case LogOpPin:
		err = state.Add(ctx, pinS.ToPin())
		if err != nil {
			goto ROLLBACK
		}
		// Async, we let the PinTracker take care of any problems
		op.consensus.rpcClient.GoContext(
			ctx,
			"",
			"Cluster",
			"Track",
			pinS,
			&struct{}{},
			nil,
		)
	case LogOpUnpin:
		err = state.Rm(ctx, pinS.DecodeCid())
		if err != nil {
			goto ROLLBACK
		}
		// Async, we let the PinTracker take care of any problems
		op.consensus.rpcClient.GoContext(
			ctx,
			"",
			"Cluster",
			"Untrack",
			pinS,
			&struct{}{},
			nil,
		)
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
