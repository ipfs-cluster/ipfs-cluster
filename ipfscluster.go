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
	"time"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	crypto "github.com/libp2p/go-libp2p-crypto"
	peer "github.com/libp2p/go-libp2p-peer"
	protocol "github.com/libp2p/go-libp2p-protocol"
	ma "github.com/multiformats/go-multiaddr"
)

// RPCProtocol is used to send libp2p messages between cluster peers
var RPCProtocol = protocol.ID("/ipfscluster/" + Version + "/rpc")

// TrackerStatus values
const (
	// IPFSStatus should never take this value
	TrackerStatusBug = iota
	// The cluster node is offline or not responding
	TrackerStatusClusterError
	// An error occurred pinning
	TrackerStatusPinError
	// An error occurred unpinning
	TrackerStatusUnpinError
	// The IPFS daemon has pinned the item
	TrackerStatusPinned
	// The IPFS daemon is currently pinning the item
	TrackerStatusPinning
	// The IPFS daemon is currently unpinning the item
	TrackerStatusUnpinning
	// The IPFS daemon is not pinning the item
	TrackerStatusUnpinned
	// The IPFS deamon is not pinning the item but it is being tracked
	TrackerStatusRemotePin
)

// TrackerStatus represents the status of a tracked Cid in the PinTracker
type TrackerStatus int

// IPFSPinStatus values
const (
	IPFSPinStatusBug = iota
	IPFSPinStatusError
	IPFSPinStatusDirect
	IPFSPinStatusRecursive
	IPFSPinStatusIndirect
	IPFSPinStatusUnpinned
)

// IPFSPinStatus represents the status of a pin in IPFS (direct, recursive etc.)
type IPFSPinStatus int

// IsPinned returns true if the status is Direct or Recursive
func (ips IPFSPinStatus) IsPinned() bool {
	return ips == IPFSPinStatusDirect || ips == IPFSPinStatusRecursive
}

// GlobalPinInfo contains cluster-wide status information about a tracked Cid,
// indexed by cluster peer.
type GlobalPinInfo struct {
	Cid     *cid.Cid
	PeerMap map[peer.ID]PinInfo
}

// PinInfo holds information about local pins. PinInfo is
// serialized when requesting the Global status, therefore
// we cannot use *cid.Cid.
type PinInfo struct {
	CidStr string
	Peer   peer.ID
	Status TrackerStatus
	TS     time.Time
	Error  string
}

// String converts an IPFSStatus into a readable string.
func (st TrackerStatus) String() string {
	switch st {
	case TrackerStatusBug:
		return "bug"
	case TrackerStatusClusterError:
		return "cluster_error"
	case TrackerStatusPinError:
		return "pin_error"
	case TrackerStatusUnpinError:
		return "unpin_error"
	case TrackerStatusPinned:
		return "pinned"
	case TrackerStatusPinning:
		return "pinning"
	case TrackerStatusUnpinning:
		return "unpinning"
	case TrackerStatusUnpinned:
		return "unpinned"
	case TrackerStatusRemotePin:
		return "remote"
	default:
		return ""
	}
}

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
	ID() (IPFSID, error)
	Pin(*cid.Cid) error
	Unpin(*cid.Cid) error
	PinLsCid(*cid.Cid) (IPFSPinStatus, error)
	PinLs() (map[string]IPFSPinStatus, error)
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
	StatusAll() []PinInfo
	// Status returns the local status of a given Cid.
	Status(*cid.Cid) PinInfo
	// SyncAll makes sure that all tracked Cids reflect the real IPFS status.
	// It returns the list of pins which were updated by the call.
	SyncAll() ([]PinInfo, error)
	// Sync makes sure that the Cid status reflect the real IPFS status.
	// It returns the local status of the Cid.
	Sync(*cid.Cid) (PinInfo, error)
	// Recover retriggers a Pin/Unpin operation in Cids with error status.
	Recover(*cid.Cid) (PinInfo, error)
}

// IPFSID is used to store information about the underlying IPFS daemon
type IPFSID struct {
	ID        peer.ID
	Addresses []ma.Multiaddr
	Error     string
}

// IPFSIDSerial is the serializable IPFSID for RPC requests
type IPFSIDSerial struct {
	ID        string
	Addresses MultiaddrsSerial
	Error     string
}

// ToSerial converts IPFSID to a go serializable object
func (id *IPFSID) ToSerial() IPFSIDSerial {
	return IPFSIDSerial{
		ID:        peer.IDB58Encode(id.ID),
		Addresses: MultiaddrsToSerial(id.Addresses),
		Error:     id.Error,
	}
}

// ToID converts an IPFSIDSerial to IPFSID
// It will ignore any errors when parsing the fields.
func (ids *IPFSIDSerial) ToID() IPFSID {
	id := IPFSID{}
	if pID, err := peer.IDB58Decode(ids.ID); err == nil {
		id.ID = pID
	}
	id.Addresses = ids.Addresses.ToMultiaddrs()
	id.Error = ids.Error
	return id
}

// ID holds information about the Cluster peer
type ID struct {
	ID                 peer.ID
	PublicKey          crypto.PubKey
	Addresses          []ma.Multiaddr
	ClusterPeers       []ma.Multiaddr
	Version            string
	Commit             string
	RPCProtocolVersion protocol.ID
	Error              string
	IPFS               IPFSID
}

// IDSerial is the serializable ID counterpart for RPC requests
type IDSerial struct {
	ID                 string
	PublicKey          []byte
	Addresses          MultiaddrsSerial
	ClusterPeers       MultiaddrsSerial
	Version            string
	Commit             string
	RPCProtocolVersion string
	Error              string
	IPFS               IPFSIDSerial
}

// ToSerial converts an ID to its Go-serializable version
func (id ID) ToSerial() IDSerial {
	var pkey []byte
	if id.PublicKey != nil {
		pkey, _ = id.PublicKey.Bytes()
	}

	return IDSerial{
		ID:                 peer.IDB58Encode(id.ID),
		PublicKey:          pkey,
		Addresses:          MultiaddrsToSerial(id.Addresses),
		ClusterPeers:       MultiaddrsToSerial(id.ClusterPeers),
		Version:            id.Version,
		Commit:             id.Commit,
		RPCProtocolVersion: string(id.RPCProtocolVersion),
		Error:              id.Error,
		IPFS:               id.IPFS.ToSerial(),
	}
}

// ToID converts an IDSerial object to ID.
// It will ignore any errors when parsing the fields.
func (ids IDSerial) ToID() ID {
	id := ID{}
	if pID, err := peer.IDB58Decode(ids.ID); err == nil {
		id.ID = pID
	}
	if pkey, err := crypto.UnmarshalPublicKey(ids.PublicKey); err == nil {
		id.PublicKey = pkey
	}

	id.Addresses = ids.Addresses.ToMultiaddrs()
	id.ClusterPeers = ids.ClusterPeers.ToMultiaddrs()
	id.Version = ids.Version
	id.Commit = ids.Commit
	id.RPCProtocolVersion = protocol.ID(ids.RPCProtocolVersion)
	id.Error = ids.Error
	id.IPFS = ids.IPFS.ToID()
	return id
}

// MultiaddrSerial is a Multiaddress in a serializable form
type MultiaddrSerial []byte

// MultiaddrsSerial is an array of Multiaddresses in serializable form
type MultiaddrsSerial []MultiaddrSerial

// MultiaddrToSerial converts a Multiaddress to its serializable form
func MultiaddrToSerial(addr ma.Multiaddr) MultiaddrSerial {
	return addr.Bytes()
}

// ToMultiaddr converts a serializable Multiaddress to its original type.
// All errors are ignored.
func (addrS MultiaddrSerial) ToMultiaddr() ma.Multiaddr {
	a, _ := ma.NewMultiaddrBytes(addrS)
	return a
}

// MultiaddrsToSerial converts a slice of Multiaddresses to its
// serializable form.
func MultiaddrsToSerial(addrs []ma.Multiaddr) MultiaddrsSerial {
	addrsS := make([]MultiaddrSerial, len(addrs), len(addrs))
	for i, a := range addrs {
		addrsS[i] = MultiaddrToSerial(a)
	}
	return addrsS
}

// ToMultiaddrs converts MultiaddrsSerial back to a slice of Multiaddresses
func (addrsS MultiaddrsSerial) ToMultiaddrs() []ma.Multiaddr {
	addrs := make([]ma.Multiaddr, len(addrsS), len(addrsS))
	for i, addrS := range addrsS {
		addrs[i] = addrS.ToMultiaddr()
	}
	return addrs
}
