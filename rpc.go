package ipfscluster

import cid "github.com/ipfs/go-cid"

// RPC supported operations.
const (
	// Cluster API
	PinRPC = iota
	UnpinRPC
	PinListRPC
	VersionRPC
	MemberListRPC
	StatusRPC
	StatusCidRPC
	LocalSyncRPC
	LocalSyncCidRPC
	GlobalSyncRPC
	GlobalSyncCidRPC
	StateSyncRPC

	// Tracker
	TrackRPC
	UntrackRPC
	TrackerStatusRPC
	TrackerStatusCidRPC
	TrackerRecoverRPC

	// IPFS Connector
	IPFSPinRPC
	IPFSUnpinRPC
	IPFSIsPinnedRPC

	// Consensus
	ConsensusLogPinRPC
	ConsensusLogUnpinRPC

	// Special
	LeaderRPC
	BroadcastRPC
	RollbackRPC

	NoopRPC
)

// Supported RPC types for serialization
const (
	BaseRPCType = iota
	GenericRPCType
	CidRPCType
	WrappedRPCType
)

// RPCOp identifies which RPC supported operation we are trying to make
type RPCOp int

// RPCType identified which implementation of RPC we are using
type RPCType int

// RPC represents an internal RPC operation. It should be implemented
// by all RPC types.
type RPC interface {
	// Op indicates which operation should be performed
	Op() RPCOp
	// RType indicates which RPC implementation is used
	RType() RPCType
	// ResponseCh returns a channel to place the response for this RPC
	ResponseCh() chan RPCResponse
}

// baseRPC implements RPC and can be included as anonymous
// field in other types.
type baseRPC struct {
	Type     RPCType
	Method   RPCOp
	RespChan chan RPCResponse
}

// Op returns the RPC method for this request
func (brpc *baseRPC) Op() RPCOp {
	return brpc.Method
}

// ResponseCh returns a channel on which the result of the
// RPC operation can be sent.
func (brpc *baseRPC) ResponseCh() chan RPCResponse {
	return brpc.RespChan
}

func (brpc *baseRPC) RType() RPCType {
	return brpc.Type
}

// GenericRPC is a ClusterRPC with generic arguments.
type GenericRPC struct {
	baseRPC
	Argument interface{}
}

// CidRPC is a ClusterRPC whose only argument is a CID.
type CidRPC struct {
	baseRPC
	// Because CIDs are not a serializable object we need to carry
	// the string representation
	CIDStr string
}

func (crpc *CidRPC) CID() *cid.Cid {
	c, err := cid.Decode(crpc.CIDStr)
	if err != nil {
		panic("Bad CID in CidRPC")
	}
	return c
}

type WrappedRPC struct {
	baseRPC
	WRPC RPC
}

// RPC builds a RPC request. It will create a
// CidRPC if the arg is of type cid.Cid. Otherwise,
// a GenericRPC is returned.
func NewRPC(m RPCOp, arg interface{}) RPC {
	switch arg.(type) {
	case *cid.Cid:
		c := arg.(*cid.Cid)
		r := new(CidRPC)
		r.Method = m
		r.Type = CidRPCType
		r.CIDStr = c.String()
		r.RespChan = make(chan RPCResponse)
		return r
	case RPC:
		w := arg.(RPC)
		r := new(WrappedRPC)
		r.Method = m
		r.Type = WrappedRPCType
		r.WRPC = w
		r.RespChan = make(chan RPCResponse)
		return r
	default:
		r := new(GenericRPC)
		r.Method = m
		r.Argument = arg
		r.Type = GenericRPCType
		r.RespChan = make(chan RPCResponse)
		return r
	}
}

// RPCResponse carries the result of a RPC requested operation and/or
// an error to indicate if the operation was successful.
type RPCResponse struct {
	Data  interface{}
	Error *RPCError
}

// RPCError is a serializable implementation of error.
type RPCError struct {
	Msg string
}

func NewRPCError(m string) *RPCError {
	return &RPCError{m}
}

func CastRPCError(err error) *RPCError {
	if err != nil {
		return NewRPCError(err.Error())
	} else {
		return nil
	}
}

func (r *RPCError) Error() string {
	return r.Msg
}
