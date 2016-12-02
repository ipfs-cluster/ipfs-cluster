package ipfscluster

// ClusterRPC supported methods.
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
)

// RPCMethod identifies which RPC-supported operation we are trying to make
type RPCMethod int

// ClusterRPC is used to let Cluster perform operations as mandated by
// its ClusterComponents. The result is placed on the ResponseCh channel.
type ClusterRPC struct {
	Method     RPCMethod
	ResponseCh chan RPCResponse
	Arguments  interface{}
}

// RPC builds a ClusterRPC request.
func RPC(m RPCMethod, args interface{}) ClusterRPC {
	return ClusterRPC{
		Method:     m,
		Arguments:  args,
		ResponseCh: make(chan RPCResponse),
	}
}

// RPC response carries the result of an ClusterRPC-requested operation
type RPCResponse struct {
	Data  interface{}
	Error error
}
