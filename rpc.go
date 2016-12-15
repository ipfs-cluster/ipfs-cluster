package ipfscluster

import cid "github.com/ipfs/go-cid"

// ClusterRPC supported operations.
const (
	PinRPC = iota
	UnpinRPC
	PinListRPC
	IPFSPinRPC
	IPFSUnpinRPC
	IPFSIsPinnedRPC
	VersionRPC
	MemberListRPC
	RollbackRPC
	LeaderRPC
	SyncRPC

	NoopRPC
)

// RPCMethod identifies which RPC supported operation we are trying to make
type RPCOp int

// ClusterRPC represents an internal RPC operation. It should be implemented
// by all RPC types.
type ClusterRPC interface {
	Op() RPCOp
	ResponseCh() chan RPCResponse
}

// baseRPC implements ClusterRPC and can be included as anonymous
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

// GenericClusterRPC is a ClusterRPC with generic arguments.
type GenericClusterRPC struct {
	baseRPC
	Argument interface{}
}

// CidClusterRPC is a ClusterRPC whose only argument is a CID.
type CidClusterRPC struct {
	baseRPC
	CID *cid.Cid
}

// RPC builds a ClusterRPC request. It will create a
// CidClusterRPC if the arg is of type cid.Cid. Otherwise,
// a GenericClusterRPC is returned.
func RPC(m RPCOp, arg interface{}) ClusterRPC {
	c, ok := arg.(*cid.Cid)
	if ok { // Its a CID
		r := new(CidClusterRPC)
		r.method = m
		r.CID = c
		r.responseCh = make(chan RPCResponse)
		return r
	}
	// Its not a cid, make a generic
	r := new(GenericClusterRPC)
	r.method = m
	r.Argument = arg
	r.responseCh = make(chan RPCResponse)
	return r
}

// RPCResponse carries the result of a ClusterRPC requested operation and/or
// an error to indicate if the operation was successful.
type RPCResponse struct {
	Data  interface{}
	Error error
}
