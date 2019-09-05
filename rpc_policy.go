package ipfscluster

// This file can be generated with rpcutil/policygen.

// DefaultRPCPolicy associates all rpc endpoints offered by cluster peers to an
// endpoint type. See rpcutil/policygen.go as a quick way to generate this
// without missing any endpoint.
var DefaultRPCPolicy = map[string]RPCEndpointType{
	// Cluster methods
	"Cluster.BlockAllocate":      RPCClosed,
	"Cluster.ConnectGraph":       RPCClosed,
	"Cluster.ID":                 RPCOpen,
	"Cluster.Join":               RPCClosed,
	"Cluster.PeerAdd":            RPCOpen, // Used by Join()
	"Cluster.PeerRemove":         RPCTrusted,
	"Cluster.Peers":              RPCTrusted, // Used by ConnectGraph()
	"Cluster.Pin":                RPCClosed,
	"Cluster.PinGet":             RPCClosed,
	"Cluster.PinPath":            RPCClosed,
	"Cluster.Pins":               RPCClosed, // Used in stateless tracker, ipfsproxy, restapi
	"Cluster.Recover":            RPCClosed,
	"Cluster.RecoverAllLocal":    RPCClosed,
	"Cluster.RecoverLocal":       RPCClosed,
	"Cluster.SendInformerMetric": RPCClosed,
	"Cluster.Status":             RPCClosed,
	"Cluster.StatusAll":          RPCClosed,
	"Cluster.StatusAllLocal":     RPCClosed,
	"Cluster.StatusLocal":        RPCClosed,
	"Cluster.Sync":               RPCClosed,
	"Cluster.SyncAll":            RPCClosed,
	"Cluster.SyncAllLocal":       RPCTrusted, // Called in broadcast from SyncAll()
	"Cluster.SyncLocal":          RPCTrusted, // Called in broadcast from Sync()
	"Cluster.Unpin":              RPCClosed,
	"Cluster.UnpinPath":          RPCClosed,
	"Cluster.Version":            RPCOpen,

	// PinTracker methods
	"PinTracker.Recover":    RPCTrusted, // Called in broadcast from Recover()
	"PinTracker.RecoverAll": RPCClosed,  // Broadcast in RecoverAll unimplemented
	"PinTracker.Status":     RPCTrusted,
	"PinTracker.StatusAll":  RPCTrusted,
	"PinTracker.Track":      RPCClosed,
	"PinTracker.Untrack":    RPCClosed,

	// IPFSConnector methods
	"IPFSConnector.BlockGet":   RPCClosed,
	"IPFSConnector.BlockPut":   RPCTrusted, // Called from Add()
	"IPFSConnector.ConfigKey":  RPCClosed,
	"IPFSConnector.Pin":        RPCClosed,
	"IPFSConnector.PinLs":      RPCClosed,
	"IPFSConnector.PinLsCid":   RPCClosed,
	"IPFSConnector.RepoStat":   RPCTrusted, // Called in broadcast from proxy/repo/stat
	"IPFSConnector.Resolve":    RPCClosed,
	"IPFSConnector.SwarmPeers": RPCTrusted, // Called in ConnectGraph
	"IPFSConnector.Unpin":      RPCClosed,

	// Consensus methods
	"Consensus.AddPeer":  RPCTrusted, // Called by Raft/redirect to leader
	"Consensus.LogPin":   RPCTrusted, // Called by Raft/redirect to leader
	"Consensus.LogUnpin": RPCTrusted, // Called by Raft/redirect to leader
	"Consensus.Peers":    RPCClosed,
	"Consensus.RmPeer":   RPCTrusted, // Called by Raft/redirect to leader

	// PeerMonitor methods
	"PeerMonitor.LatestMetrics": RPCClosed,
	"PeerMonitor.MetricNames":   RPCClosed,
}
