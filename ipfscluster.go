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
	// AddPin adds a pin to the State
	AddPin(*cid.Cid) error
	// RmPin removes a pin from the State
	RmPin(*cid.Cid) error
	// ListPins lists all the pins in the state
	ListPins() []*cid.Cid
	// HasPin returns true if the state is holding a Cid
	HasPin(*cid.Cid) bool
	// AddPeer adds a peer to the shared state
}

// PinTracker represents a component which tracks the status of
// the pins in this cluster and ensures they are in sync with the
// IPFS daemon. This component should be thread safe.
type PinTracker interface {
	Component
	// Track tells the tracker that a Cid is now under its supervision
	// The tracker may decide to perform an IPFS pin.
	Track(*cid.Cid) error
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
