// Package ipfscluster implements a wrapper for the IPFS deamon which
// allows to orchestrate pinning operations among several IPFS nodes.
//
// IPFS Cluster uses a go-libp2p-raft to keep a shared state between
// the different cluster peers. It also uses LibP2P to enable
// communication between its different components, which perform different
// tasks like managing the underlying IPFS daemons, or providing APIs for
// external control.
package ipfscluster

import (
	"context"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/state"

	cid "github.com/ipfs/go-cid"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p-peer"
)

// Component represents a piece of ipfscluster. Cluster components
// usually run their own goroutines (a http server for example). They
// communicate with the main Cluster component and other components
// (both local and remote), using an instance of rpc.Client.
type Component interface {
	SetClient(*rpc.Client)
	Shutdown(context.Context) error
}

// Consensus is a component which keeps a shared state in
// IPFS Cluster and triggers actions on updates to that state.
// Currently, Consensus needs to be able to elect/provide a
// Cluster Leader and the implementation is very tight to
// the Cluster main component.
type Consensus interface {
	Component
	// Returns a channel to signal that the consensus layer is ready
	// allowing the main component to wait for it during start.
	Ready(context.Context) <-chan struct{}
	// Logs a pin operation
	LogPin(ctx context.Context, c api.Pin) error
	// Logs an unpin operation
	LogUnpin(ctx context.Context, c api.Pin) error
	AddPeer(ctx context.Context, p peer.ID) error
	RmPeer(ctx context.Context, p peer.ID) error
	State(context.Context) (state.State, error)
	// Provide a node which is responsible to perform
	// specific tasks which must only run in 1 cluster peer
	Leader(context.Context) (peer.ID, error)
	// Only returns when the consensus state has all log
	// updates applied to it
	WaitForSync(context.Context) error
	// Clean removes all consensus data
	Clean(context.Context) error
	// Peers returns the peerset participating in the Consensus
	Peers(context.Context) ([]peer.ID, error)
}

// API is a component which offers an API for Cluster. This is
// a base component.
type API interface {
	Component
}

// IPFSConnector is a component which allows cluster to interact with
// an IPFS daemon. This is a base component.
type IPFSConnector interface {
	Component
	ID(context.Context) (api.IPFSID, error)
	Pin(context.Context, cid.Cid, int) error
	Unpin(context.Context, cid.Cid) error
	PinLsCid(context.Context, cid.Cid) (api.IPFSPinStatus, error)
	PinLs(ctx context.Context, typeFilter string) (map[string]api.IPFSPinStatus, error)
	// ConnectSwarms make sure this peer's IPFS daemon is connected to
	// other peers IPFS daemons.
	ConnectSwarms(context.Context) error
	// SwarmPeers returns the IPFS daemon's swarm peers
	SwarmPeers(context.Context) (api.SwarmPeers, error)
	// ConfigKey returns the value for a configuration key.
	// Subobjects are reached with keypaths as "Parent/Child/GrandChild...".
	ConfigKey(keypath string) (interface{}, error)
	// RepoStat returns the current repository size and max limit as
	// provided by "repo stat".
	RepoStat(context.Context) (api.IPFSRepoStat, error)
	// BlockPut directly adds a block of data to the IPFS repo
	BlockPut(context.Context, api.NodeWithMeta) error
	// BlockGet retrieves the raw data of an IPFS block
	BlockGet(context.Context, cid.Cid) ([]byte, error)
}

// Peered represents a component which needs to be aware of the peers
// in the Cluster and of any changes to the peer set.
type Peered interface {
	AddPeer(ctx context.Context, p peer.ID)
	RmPeer(ctx context.Context, p peer.ID)
	//SetPeers(peers []peer.ID)
}

// PinTracker represents a component which tracks the status of
// the pins in this cluster and ensures they are in sync with the
// IPFS daemon. This component should be thread safe.
type PinTracker interface {
	Component
	// Track tells the tracker that a Cid is now under its supervision
	// The tracker may decide to perform an IPFS pin.
	Track(context.Context, api.Pin) error
	// Untrack tells the tracker that a Cid is to be forgotten. The tracker
	// may perform an IPFS unpin operation.
	Untrack(context.Context, cid.Cid) error
	// StatusAll returns the list of pins with their local status.
	StatusAll(context.Context) []api.PinInfo
	// Status returns the local status of a given Cid.
	Status(context.Context, cid.Cid) api.PinInfo
	// SyncAll makes sure that all tracked Cids reflect the real IPFS status.
	// It returns the list of pins which were updated by the call.
	SyncAll(context.Context) ([]api.PinInfo, error)
	// Sync makes sure that the Cid status reflect the real IPFS status.
	// It returns the local status of the Cid.
	Sync(context.Context, cid.Cid) (api.PinInfo, error)
	// RecoverAll calls Recover() for all pins tracked.
	RecoverAll(context.Context) ([]api.PinInfo, error)
	// Recover retriggers a Pin/Unpin operation in a Cids with error status.
	Recover(context.Context, cid.Cid) (api.PinInfo, error)
}

// Informer provides Metric information from a peer. The metrics produced by
// informers are then passed to a PinAllocator which will use them to
// determine where to pin content. The metric is agnostic to the rest of
// Cluster.
type Informer interface {
	Component
	Name() string
	GetMetric(context.Context) api.Metric
}

// PinAllocator decides where to pin certain content. In order to make such
// decision, it receives the pin arguments, the peers which are currently
// allocated to the content and metrics available for all peers which could
// allocate the content.
type PinAllocator interface {
	Component
	// Allocate returns the list of peers that should be assigned to
	// Pin content in order of preference (from the most preferred to the
	// least). The "current" map contains valid metrics for peers
	// which are currently pinning the content. The candidates map
	// contains the metrics for all peers which are eligible for pinning
	// the content.
	Allocate(ctx context.Context, c cid.Cid, current, candidates, priority map[peer.ID]api.Metric) ([]peer.ID, error)
}

// PeerMonitor is a component in charge of publishing a peer's metrics and
// reading metrics from other peers in the cluster. The PinAllocator will
// use the metrics provided by the monitor as candidates for Pin allocations.
//
// The PeerMonitor component also provides an Alert channel which is signaled
// when a metric is no longer received and the monitor identifies it
// as a problem.
type PeerMonitor interface {
	Component
	// LogMetric stores a metric. It can be used to manually inject
	// a metric to a monitor.
	LogMetric(context.Context, api.Metric) error
	// PublishMetric sends a metric to the rest of the peers.
	// How to send it, and to who, is to be decided by the implementation.
	PublishMetric(context.Context, api.Metric) error
	// LatestMetrics returns a map with the latest metrics of matching name
	// for the current cluster peers.
	LatestMetrics(ctx context.Context, name string) []api.Metric
	// Alerts delivers alerts generated when this peer monitor detects
	// a problem (i.e. metrics not arriving as expected). Alerts can be used
	// to trigger self-healing measures or re-pinnings of content.
	Alerts() <-chan api.Alert
}

// Tracer implements Component as a way
// to shutdown and flush and remaining traces.
type Tracer interface {
	Component
}
