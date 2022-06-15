package raft

import (
	"context"
	"errors"

	"go.opencensus.io/tag"
	"go.opencensus.io/trace"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/state"

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
	SpanCtx   trace.SpanContext `codec:"s,omitempty"`
	TagCtx    []byte            `codec:"t,omitempty"`
	Cid       api.Pin           `codec:"c,omitempty"`
	Type      LogOpType         `codec:"p,omitempty"`
	consensus *Consensus        `codec:"-"`
	tracing   bool              `codec:"-"`
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

	pin := op.Cid

	switch op.Type {
	case LogOpPin:
		err = state.Add(ctx, pin)
		if err != nil {
			logger.Error(err)
			goto ROLLBACK
		}
		// Async, we let the PinTracker take care of any problems
		op.consensus.rpcClient.GoContext(
			ctx,
			"",
			"PinTracker",
			"Track",
			pin,
			&struct{}{},
			nil,
		)
	case LogOpUnpin:
		err = state.Rm(ctx, pin.Cid)
		if err != nil {
			logger.Error(err)
			goto ROLLBACK
		}
		// Async, we let the PinTracker take care of any problems
		op.consensus.rpcClient.GoContext(
			ctx,
			"",
			"PinTracker",
			"Untrack",
			pin,
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
