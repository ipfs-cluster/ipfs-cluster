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
	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
	protocol "github.com/libp2p/go-libp2p-protocol"

	"github.com/ipfs/ipfs-cluster/api"
)

// RPCProtocol is used to send libp2p messages between cluster peers
var RPCProtocol = protocol.ID("/ipfscluster/" + Version + "/rpc")

// Component represents a piece of ipfscluster. Cluster components
// usually run their own goroutines (a http server for example). They
// communicate with the main Cluster component and other components
// (both local and remote), using an instance of rpc.Client.
type Component interface {
	SetClient(*rpc.Client)
	Shutdown() error
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
	Pin(*cid.Cid) error
	Unpin(*cid.Cid) error
	PinLsCid(*cid.Cid) (api.IPFSPinStatus, error)
	PinLs(typeFilter string) (map[string]api.IPFSPinStatus, error)
}

// Peered represents a component which needs to be aware of the peers
// in the Cluster and of any changes to the peer set.
type Peered interface {
	AddPeer(p peer.ID)
	RmPeer(p peer.ID)
	//SetPeers(peers []peer.ID)
}

// State represents the shared state of the cluster and it
// is used by the Consensus component to keep track of
// objects which objects are pinned. This component should be thread safe.
type State interface {
	// Add adds a pin to the State
	Add(api.CidArg) error
	// Rm removes a pin from the State
	Rm(*cid.Cid) error
	// List lists all the pins in the state
	List() []api.CidArg
	// Has returns true if the state is holding information for a Cid
	Has(*cid.Cid) bool
	// Get returns the information attacthed to this pin
	Get(*cid.Cid) api.CidArg
}

// PinTracker represents a component which tracks the status of
// the pins in this cluster and ensures they are in sync with the
// IPFS daemon. This component should be thread safe.
type PinTracker interface {
	Component
	// Track tells the tracker that a Cid is now under its supervision
	// The tracker may decide to perform an IPFS pin.
	Track(api.CidArg) error
	// Untrack tells the tracker that a Cid is to be forgotten. The tracker
	// may perform an IPFS unpin operation.
	Untrack(*cid.Cid) error
	// StatusAll returns the list of pins with their local status.
	StatusAll() []api.PinInfo
	// Status returns the local status of a given Cid.
	Status(*cid.Cid) api.PinInfo
	// SyncAll makes sure that all tracked Cids reflect the real IPFS status.
	// It returns the list of pins which were updated by the call.
	SyncAll() ([]api.PinInfo, error)
	// Sync makes sure that the Cid status reflect the real IPFS status.
	// It returns the local status of the Cid.
	Sync(*cid.Cid) (api.PinInfo, error)
	// Recover retriggers a Pin/Unpin operation in Cids with error status.
	Recover(*cid.Cid) (api.PinInfo, error)
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
	// Pin content in oder of preference (from the most preferred to the
	// least). The "current" map contains valid metrics for peers
	// which are currently pinning the content. The candidates map
	// contains the metrics for all peers which are eligible for pinning
	// the content.
	Allocate(c *cid.Cid, current, candidates map[peer.ID]api.Metric) ([]peer.ID, error)
}

// PeerMonitor is a component in charge of monitoring the peers in the cluster
// and providing candidates to the PinAllocator when a pin request arrives.
type PeerMonitor interface {
	Component
	// LogMetric stores a metric. Metrics are pushed reguarly from each peer
	// to the active PeerMonitor.
	LogMetric(api.Metric)
	// LastMetrics returns a map with the latest metrics of matching name
	// for the current cluster peers.
	LastMetrics(name string) []api.Metric
	// Alerts delivers alerts generated when this peer monitor detects
	// a problem (i.e. metrics not arriving as expected). Alerts are used to
	// trigger rebalancing operations.
	Alerts() <-chan api.Alert
}
