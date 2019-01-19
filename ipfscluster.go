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

	"github.com/elastos/Elastos.NET.Hive.Cluster/api"
	"github.com/elastos/Elastos.NET.Hive.Cluster/state"

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
	Shutdown() error
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
	Ready() <-chan struct{}
	// Logs a pin operation
	LogPin(c api.Pin) error
	// Logs an unpin operation
	LogUnpin(c api.Pin) error
	AddPeer(p peer.ID) error
	RmPeer(p peer.ID) error
	State() (state.State, error)
	// Provide a node which is responsible to perform
	// specific tasks which must only run in 1 cluster peer
	Leader() (peer.ID, error)
	// Only returns when the consensus state has all log
	// updates applied to it
	WaitForSync() error
	// Clean removes all consensus data
	Clean() error
	// Peers returns the peerset participating in the Consensus
	Peers() ([]peer.ID, error)
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
	ID() (api.IPFSID, error)
	Pin(context.Context, cid.Cid, int) error
	Unpin(context.Context, cid.Cid) error
	PinLsCid(context.Context, cid.Cid) (api.IPFSPinStatus, error)
	PinLs(ctx context.Context, typeFilter string) (map[string]api.IPFSPinStatus, error)
	// ConnectSwarms make sure this peer's IPFS daemon is connected to
	// other peers IPFS daemons.
	ConnectSwarms() error
	// SwarmPeers returns the IPFS daemon's swarm peers
	SwarmPeers() (api.SwarmPeers, error)
	// ConfigKey returns the value for a configuration key.
	// Subobjects are reached with keypaths as "Parent/Child/GrandChild...".
	ConfigKey(keypath string) (interface{}, error)
	// RepoStat returns the current repository size and max limit as
	// provided by "repo stat".
	RepoStat() (api.IPFSRepoStat, error)
	// BlockPut directly adds a block of data to the IPFS repo
	BlockPut(api.NodeWithMeta) error
	// BlockGet retrieves the raw data of an IPFS block
	BlockGet(cid.Cid) ([]byte, error)
	// UidNew registers a uid in hive cluster
	UidNew(name string) (api.UIDSecret, error)
	// UidLogIn is used to change uid
	UidLogIn([]string) (api.UIDLogIn, error)
	// FileGet downloads file from ipfs service
	FileGet(fg []string) ([]byte, error)
	// FilesCp is used to copy file
	FilesCp([]string) error
	// FilesFlush is used to change uid
	FilesFlush([]string) error
	// FilesLs is used to list files
	FilesLs([]string) (api.FilesLs, error)
	// FilesMkdir creates directory in IFPS peer
	FilesMkdir([]string) error
	// FilesMv moves file in IPFS peer
	FilesMv([]string) error
	// FilesRead reads file
	FilesRead([]string) ([]byte, error)
	// FilesRm remove directory from IPFS peer
	FilesRm([]string) error
	// FilesStat fetchs file statistics
	FilesStat([]string) (api.FilesStat, error)
	// FilesWrite writes file
	FilesWrite(api.FilesWrite) error
	// NamePublish publish ipfs path with uid
	NamePublish(np []string) (api.NamePublish, error)
}

// Peered represents a component which needs to be aware of the peers
// in the Cluster and of any changes to the peer set.
type Peered interface {
	AddPeer(p peer.ID)
	RmPeer(p peer.ID)
	//SetPeers(peers []peer.ID)
}

// PinTracker represents a component which tracks the status of
// the pins in this cluster and ensures they are in sync with the
// IPFS daemon. This component should be thread safe.
type PinTracker interface {
	Component
	// Track tells the tracker that a Cid is now under its supervision
	// The tracker may decide to perform an IPFS pin.
	Track(api.Pin) error
	// Untrack tells the tracker that a Cid is to be forgotten. The tracker
	// may perform an IPFS unpin operation.
	Untrack(cid.Cid) error
	// StatusAll returns the list of pins with their local status.
	StatusAll() []api.PinInfo
	// Status returns the local status of a given Cid.
	Status(cid.Cid) api.PinInfo
	// SyncAll makes sure that all tracked Cids reflect the real IPFS status.
	// It returns the list of pins which were updated by the call.
	SyncAll() ([]api.PinInfo, error)
	// Sync makes sure that the Cid status reflect the real IPFS status.
	// It returns the local status of the Cid.
	Sync(cid.Cid) (api.PinInfo, error)
	// RecoverAll calls Recover() for all pins tracked.
	RecoverAll() ([]api.PinInfo, error)
	// Recover retriggers a Pin/Unpin operation in a Cids with error status.
	Recover(cid.Cid) (api.PinInfo, error)
}

// Informer provides Metric information from a peer. The metrics produced by
// informers are then passed to a PinAllocator which will use them to
// determine where to pin content. The metric is agnostic to the rest of
// Cluster.
type Informer interface {
	Component
	Name() string
	GetMetric() api.Metric
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
	Allocate(c cid.Cid, current, candidates, priority map[peer.ID]api.Metric) ([]peer.ID, error)
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
	LogMetric(api.Metric) error
	// PublishMetric sends a metric to the rest of the peers.
	// How to send it, and to who, is to be decided by the implementation.
	PublishMetric(api.Metric) error
	// LatestMetrics returns a map with the latest metrics of matching name
	// for the current cluster peers.
	LatestMetrics(name string) []api.Metric
	// Alerts delivers alerts generated when this peer monitor detects
	// a problem (i.e. metrics not arriving as expected). Alerts can be used
	// to trigger self-healing measures or re-pinnings of content.
	Alerts() <-chan api.Alert
}
