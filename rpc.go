package ipfscluster

import cid "github.com/ipfs/go-cid"

// RPC supported operations.
const (
	PinRPC = iota
	UnpinRPC
	PinListRPC
	IPFSPinRPC
	IPFSUnpinRPC
	IPFSIsPinnedRPC
	ConsensusAddPinRPC
	ConsensusRmPinRPC
	VersionRPC
	MemberListRPC
	RollbackRPC
	LeaderRPC
	BroadcastRPC
	LocalSyncRPC
	LocalSyncCidRPC
	GlobalSyncRPC
	GlobalSyncCidRPC
	StatusRPC
	StatusCidRPC

	NoopRPC
)

// RPCMethod identifies which RPC supported operation we are trying to make
type RPCOp int

// RPC represents an internal RPC operation. It should be implemented
// by all RPC types.
type RPC interface {
	Op() RPCOp
	ResponseCh() chan RPCResponse
}

// baseRPC implements RPC and can be included as anonymous
// field in other types.
type baseRPC struct {
	method     RPCOp
	responseCh chan RPCResponse
}

// Op returns the RPC method for this request
func (brpc *baseRPC) Op() RPCOp {
	return brpc.method
}

// ResponseCh returns a channel on which the result of the
// RPC operation can be sent.
func (brpc *baseRPC) ResponseCh() chan RPCResponse {
	return brpc.responseCh
}

// GenericRPC is a ClusterRPC with generic arguments.
type GenericRPC struct {
	baseRPC
	Argument interface{}
}

// CidRPC is a ClusterRPC whose only argument is a CID.
type CidRPC struct {
	baseRPC
	CID *cid.Cid
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
		r.method = m
		r.CID = c
		r.responseCh = make(chan RPCResponse)
		return r
	case RPC:
		w := arg.(RPC)
		r := new(WrappedRPC)
		r.method = m
		r.WRPC = w
		r.responseCh = make(chan RPCResponse)
		return r
	default:
		r := new(GenericRPC)
		r.method = m
		r.Argument = arg
		r.responseCh = make(chan RPCResponse)
		return r
	}
}

// RPCResponse carries the result of a RPC requested operation and/or
// an error to indicate if the operation was successful.
type RPCResponse struct {
	Data  interface{}
	Error error
}
