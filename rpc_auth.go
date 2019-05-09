package ipfscluster

// RPC endpoint types w.r.t. trust level
const (
	// RPCClosed endpoints can only be called by the local cluster peer
	// on itself.
	RPCClosed RPCEndpointType = iota
	// RPCTrusted endpoints can be called by "trusted" peers.
	// It depends which peers are considered trusted. For example,
	// in "raft" mode, Cluster will all peers as "trusted". In "crdt" mode,
	// trusted peers are those specified in the configuration.
	RPCTrusted
	// RPCOpen endpoints can be called by any peer in the Cluster swarm.
	RPCOpen
)

// RPCEndpointType controls how access is granted to an RPC endpoint
type RPCEndpointType int

// A trick to find where something is used (i.e. Cluster.Pin):
// grep -R -B 3 '"Pin"' | grep -C 1 '"Cluster"'.
// This does not cover globalPinInfo*(...) broadcasts nor redirects to leader
// in Raft.

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
	"Cluster.Peers":              RPCTrusted, // Used by ConnectGraph
	"Cluster.Pin":                RPCClosed,
	"Cluster.PinGet":             RPCClosed,
	"Cluster.PinPath":            RPCClosed,
	"Cluster.Pins":               RPCClosed, // stateless, proxy, api
	"Cluster.Recover":            RPCClosed,
	"Cluster.RecoverAllLocal":    RPCClosed,
	"Cluster.RecoverLocal":       RPCClosed,
	"Cluster.SendInformerMetric": RPCClosed, // Local use in ipfshttp
	"Cluster.Status":             RPCClosed,
	"Cluster.StatusAll":          RPCClosed,
	"Cluster.StatusAllLocal":     RPCClosed,
	"Cluster.StatusLocal":        RPCClosed,
	"Cluster.Sync":               RPCClosed,
	"Cluster.SyncAll":            RPCClosed,
	"Cluster.SyncAllLocal":       RPCTrusted, // Bcast from SyncAll()
	"Cluster.SyncLocal":          RPCTrusted, // Bcast from Sync()
	"Cluster.Unpin":              RPCClosed,
	"Cluster.UnpinPath":          RPCClosed,
	"Cluster.Version":            RPCOpen,

	// PinTracker methods
	"PinTracker.Recover":    RPCTrusted, // Bcast from Cluster.Recover()
	"PinTracker.RecoverAll": RPCClosed,  // Bcast unimplemented
	"PinTracker.Status":     RPCTrusted, // Bcast from Cluster.Status()
	"PinTracker.StatusAll":  RPCTrusted, // Bcast from Cluster.StatusAll()
	"PinTracker.Track":      RPCClosed,
	"PinTracker.Untrack":    RPCClosed,

	// IPFSConnector methods
	"IPFSConnector.BlockGet":      RPCClosed,
	"IPFSConnector.BlockPut":      RPCTrusted, // Add() allocations
	"IPFSConnector.ConfigKey":     RPCClosed,
	"IPFSConnector.ConnectSwarms": RPCClosed,
	"IPFSConnector.Pin":           RPCClosed,
	"IPFSConnector.PinLs":         RPCClosed,
	"IPFSConnector.PinLsCid":      RPCClosed,
	"IPFSConnector.RepoStat":      RPCTrusted, // ipfsproxy: repo/stat broadcast
	"IPFSConnector.Resolve":       RPCClosed,
	"IPFSConnector.SwarmPeers":    RPCTrusted, // connectGraph.
	"IPFSConnector.Unpin":         RPCClosed,

	// Consensus methods
	"Consensus.AddPeer":  RPCTrusted, // Raft redirect to leader
	"Consensus.LogPin":   RPCTrusted, // Raft redirect to leader
	"Consensus.LogUnpin": RPCTrusted, // Raft redirect to leader
	"Consensus.Peers":    RPCClosed,
	"Consensus.RmPeer":   RPCTrusted, // Raft redirect to leader

	// PeerMonitor methods
	"PeerMonitor.LatestMetrics": RPCClosed,
}
