package ipfscluster

import cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"

// ClusterRPC supported operations.
const (
	PinRPC = iota
	UnpinRPC
	PinListRPC
	IPFSPinRPC
	IPFSUnpinRPC
	VersionRPC
	MemberListRPC
	RollbackRPC
	LeaderRPC
	StatusPinnedRPC
	StatusPinningRPC
	StatusUnpinnedRPC
	StatusUnpinningRPC
	StatusPinErrorRPC
	StatusUnPinErrorRPC
)

// RPCMethod identifies which RPC-supported operation we are trying to make
type RPCOp int

type ClusterRPC interface {
	Op() RPCOp
	ResponseCh() chan RPCResponse
}

type baseRPC struct {
	method     RPCOp
	responseCh chan RPCResponse
}

// Method returns the RPC method for this request
func (brpc *baseRPC) Op() RPCOp {
	return brpc.method
}

// ResponseCh returns a channel to send the RPCResponse
func (brpc *baseRPC) ResponseCh() chan RPCResponse {
	return brpc.responseCh
}

// GenericClusterRPC is used to let Cluster perform operations as mandated by
// its ClusterComponents. The result is placed on the ResponseCh channel.
type GenericClusterRPC struct {
	baseRPC
	Arguments interface{}
}

type CidClusterRPC struct {
	baseRPC
	CID *cid.Cid
}

// RPC builds a ClusterRPC request.
func RPC(m RPCOp, args interface{}) ClusterRPC {
	c, ok := args.(*cid.Cid)
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
	r.Arguments = args
	r.responseCh = make(chan RPCResponse)
	return r
}

// RPC response carries the result of an ClusterRPC-requested operation
type RPCResponse struct {
	Data  interface{}
	Error error
}
