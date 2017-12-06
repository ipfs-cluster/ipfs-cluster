// Package api holds declarations for types used in ipfs-cluster APIs to make
// them re-usable across differen tools. This include RPC API "Serial[izable]"
// versions for types. The Go API uses natives types, while RPC API,
// REST APIs etc use serializable types (i.e. json format). Conversion methods
// exists between types.
//
// Note that all conversion methods ignore any parsing errors. All values must
// be validated first before initializing any of the types defined here.
package api

import (
	"time"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
	protocol "github.com/libp2p/go-libp2p-protocol"
	ma "github.com/multiformats/go-multiaddr"
)

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
	TrackerStatusRemote
)

// TrackerStatus represents the status of a tracked Cid in the PinTracker
type TrackerStatus int

var trackerStatusString = map[TrackerStatus]string{
	TrackerStatusBug:          "bug",
	TrackerStatusClusterError: "cluster_error",
	TrackerStatusPinError:     "pin_error",
	TrackerStatusUnpinError:   "unpin_error",
	TrackerStatusPinned:       "pinned",
	TrackerStatusPinning:      "pinning",
	TrackerStatusUnpinning:    "unpinning",
	TrackerStatusUnpinned:     "unpinned",
	TrackerStatusRemote:       "remote",
}

// String converts a TrackerStatus into a readable string.
func (st TrackerStatus) String() string {
	return trackerStatusString[st]
}

// TrackerStatusFromString parses a string and returns the matching
// TrackerStatus value.
func TrackerStatusFromString(str string) TrackerStatus {
	for k, v := range trackerStatusString {
		if v == str {
			return k
		}
	}
	return TrackerStatusBug
}

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

// IPFSPinStatusFromString parses a string and returns the matching
// IPFSPinStatus.
func IPFSPinStatusFromString(t string) IPFSPinStatus {
	// TODO: This is only used in the http_connector to parse
	// ipfs-daemon-returned values. Maybe it should be extended.
	switch {
	case t == "indirect":
		return IPFSPinStatusIndirect
	case t == "direct":
		return IPFSPinStatusDirect
	case t == "recursive":
		return IPFSPinStatusRecursive
	default:
		return IPFSPinStatusBug
	}
}

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

// GlobalPinInfoSerial is the serializable version of GlobalPinInfo.
type GlobalPinInfoSerial struct {
	Cid     string                   `json:"cid"`
	PeerMap map[string]PinInfoSerial `json:"peer_map"`
}

// ToSerial converts a GlobalPinInfo to its serializable version.
func (gpi GlobalPinInfo) ToSerial() GlobalPinInfoSerial {
	s := GlobalPinInfoSerial{}
	s.Cid = gpi.Cid.String()
	s.PeerMap = make(map[string]PinInfoSerial)
	for k, v := range gpi.PeerMap {
		s.PeerMap[peer.IDB58Encode(k)] = v.ToSerial()
	}
	return s
}

// ToGlobalPinInfo converts a GlobalPinInfoSerial to its native version.
func (gpis GlobalPinInfoSerial) ToGlobalPinInfo() GlobalPinInfo {
	c, _ := cid.Decode(gpis.Cid)
	gpi := GlobalPinInfo{
		Cid:     c,
		PeerMap: make(map[peer.ID]PinInfo),
	}
	for k, v := range gpis.PeerMap {
		p, _ := peer.IDB58Decode(k)
		gpi.PeerMap[p] = v.ToPinInfo()
	}
	return gpi
}

// PinInfo holds information about local pins.
type PinInfo struct {
	Cid    *cid.Cid
	Peer   peer.ID
	Status TrackerStatus
	TS     time.Time
	Error  string
}

// PinInfoSerial is a serializable version of PinInfo.
// information is marked as
type PinInfoSerial struct {
	Cid    string `json:"cid"`
	Peer   string `json:"peer"`
	Status string `json:"status"`
	TS     string `json:"timestamp"`
	Error  string `json:"error"`
}

// ToSerial converts a PinInfo to its serializable version.
func (pi PinInfo) ToSerial() PinInfoSerial {
	return PinInfoSerial{
		Cid:    pi.Cid.String(),
		Peer:   peer.IDB58Encode(pi.Peer),
		Status: pi.Status.String(),
		TS:     pi.TS.UTC().Format(time.RFC3339),
		Error:  pi.Error,
	}
}

// ToPinInfo converts a PinInfoSerial to its native version.
func (pis PinInfoSerial) ToPinInfo() PinInfo {
	c, _ := cid.Decode(pis.Cid)
	p, _ := peer.IDB58Decode(pis.Peer)
	ts, _ := time.Parse(time.RFC3339, pis.TS)
	return PinInfo{
		Cid:    c,
		Peer:   p,
		Status: TrackerStatusFromString(pis.Status),
		TS:     ts,
		Error:  pis.Error,
	}
}

// Version holds version information
type Version struct {
	Version string `json:"Version"`
}

// IPFSID is used to store information about the underlying IPFS daemon
type IPFSID struct {
	ID        peer.ID
	Addresses []ma.Multiaddr
	Error     string
}

// IPFSIDSerial is the serializable IPFSID for RPC requests
type IPFSIDSerial struct {
	ID        string           `json:"id"`
	Addresses MultiaddrsSerial `json:"addresses"`
	Error     string           `json:"error"`
}

// ToSerial converts IPFSID to a go serializable object
func (id *IPFSID) ToSerial() IPFSIDSerial {
	return IPFSIDSerial{
		ID:        peer.IDB58Encode(id.ID),
		Addresses: MultiaddrsToSerial(id.Addresses),
		Error:     id.Error,
	}
}

// ToIPFSID converts an IPFSIDSerial to IPFSID
func (ids *IPFSIDSerial) ToIPFSID() IPFSID {
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
	ID                    peer.ID
	Addresses             []ma.Multiaddr
	ClusterPeers          []peer.ID
	ClusterPeersAddresses []ma.Multiaddr
	Version               string
	Commit                string
	RPCProtocolVersion    protocol.ID
	Error                 string
	IPFS                  IPFSID
	Peername              string
	//PublicKey          crypto.PubKey
}

// IDSerial is the serializable ID counterpart for RPC requests
type IDSerial struct {
	ID                    string           `json:"id"`
	Addresses             MultiaddrsSerial `json:"addresses"`
	ClusterPeers          []string         `json:"cluster_peers"`
	ClusterPeersAddresses MultiaddrsSerial `json:"cluster_peers_addresses"`
	Version               string           `json:"version"`
	Commit                string           `json:"commit"`
	RPCProtocolVersion    string           `json:"rpc_protocol_version"`
	Error                 string           `json:"error"`
	IPFS                  IPFSIDSerial     `json:"ipfs"`
	Peername              string           `json:"peername"`
	//PublicKey          []byte
}

// ToSerial converts an ID to its Go-serializable version
func (id ID) ToSerial() IDSerial {
	//var pkey []byte
	//if id.PublicKey != nil {
	//	pkey, _ = id.PublicKey.Bytes()
	//}

	peers := make([]string, len(id.ClusterPeers), len(id.ClusterPeers))
	for i, p := range id.ClusterPeers {
		peers[i] = peer.IDB58Encode(p)
	}

	return IDSerial{
		ID: peer.IDB58Encode(id.ID),
		//PublicKey:          pkey,
		Addresses:             MultiaddrsToSerial(id.Addresses),
		ClusterPeers:          peers,
		ClusterPeersAddresses: MultiaddrsToSerial(id.ClusterPeersAddresses),
		Version:               id.Version,
		Commit:                id.Commit,
		RPCProtocolVersion:    string(id.RPCProtocolVersion),
		Error:                 id.Error,
		IPFS:                  id.IPFS.ToSerial(),
		Peername:              id.Peername,
	}
}

// ToID converts an IDSerial object to ID.
// It will ignore any errors when parsing the fields.
func (ids IDSerial) ToID() ID {
	id := ID{}
	p, _ := peer.IDB58Decode(ids.ID)
	id.ID = p

	//if pkey, err := crypto.UnmarshalPublicKey(ids.PublicKey); err == nil {
	//	id.PublicKey = pkey
	//}

	peers := make([]peer.ID, len(ids.ClusterPeers), len(ids.ClusterPeers))
	for i, p := range ids.ClusterPeers {
		peers[i], _ = peer.IDB58Decode(p)
	}

	id.Addresses = ids.Addresses.ToMultiaddrs()
	id.ClusterPeers = peers
	id.ClusterPeersAddresses = ids.ClusterPeersAddresses.ToMultiaddrs()
	id.Version = ids.Version
	id.Commit = ids.Commit
	id.RPCProtocolVersion = protocol.ID(ids.RPCProtocolVersion)
	id.Error = ids.Error
	id.IPFS = ids.IPFS.ToIPFSID()
	id.Peername = ids.Peername
	return id
}

// MultiaddrSerial is a Multiaddress in a serializable form
type MultiaddrSerial string

// MultiaddrsSerial is an array of Multiaddresses in serializable form
type MultiaddrsSerial []MultiaddrSerial

// MultiaddrToSerial converts a Multiaddress to its serializable form
func MultiaddrToSerial(addr ma.Multiaddr) MultiaddrSerial {
	return MultiaddrSerial(addr.String())
}

// ToMultiaddr converts a serializable Multiaddress to its original type.
// All errors are ignored.
func (addrS MultiaddrSerial) ToMultiaddr() ma.Multiaddr {
	a, _ := ma.NewMultiaddr(string(addrS))
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

// Pin is an argument that carries a Cid. It may carry more things in the
// future.
type Pin struct {
	Cid               *cid.Cid
	Name              string
	Allocations       []peer.ID
	ReplicationFactor int
}

// PinCid is a shorcut to create a Pin only with a Cid.
func PinCid(c *cid.Cid) Pin {
	return Pin{
		Cid: c,
	}
}

// PinSerial is a serializable version of Pin
type PinSerial struct {
	Cid               string   `json:"cid"`
	Name              string   `json:"name"`
	Allocations       []string `json:"allocations"`
	Everywhere        bool     `json:"everywhere,omitempty"` // legacy
	ReplicationFactor int      `json:"replication_factor"`
}

// ToSerial converts a Pin to PinSerial.
func (pin Pin) ToSerial() PinSerial {
	lenAllocs := len(pin.Allocations)
	allocs := make([]string, lenAllocs, lenAllocs)
	for i, p := range pin.Allocations {
		allocs[i] = peer.IDB58Encode(p)
	}

	return PinSerial{
		Cid:               pin.Cid.String(),
		Name:              pin.Name,
		Allocations:       allocs,
		ReplicationFactor: pin.ReplicationFactor,
	}
}

// ToPin converts a PinSerial to its native form.
func (pins PinSerial) ToPin() Pin {
	c, _ := cid.Decode(pins.Cid)
	lenAllocs := len(pins.Allocations)
	allocs := make([]peer.ID, lenAllocs, lenAllocs)
	for i, p := range pins.Allocations {
		allocs[i], _ = peer.IDB58Decode(p)
	}

	// legacy format management
	if pins.ReplicationFactor == 0 && pins.Everywhere {
		pins.ReplicationFactor = -1
	}

	return Pin{
		Cid:               c,
		Name:              pins.Name,
		Allocations:       allocs,
		ReplicationFactor: pins.ReplicationFactor,
	}
}

// Metric transports information about a peer.ID. It is used to decide
// pin allocations by a PinAllocator. IPFS cluster is agnostic to
// the Value, which should be interpreted by the PinAllocator.
type Metric struct {
	Name   string
	Peer   peer.ID // filled-in by Cluster.
	Value  string
	Expire string // RFC3339Nano
	Valid  bool   // if the metric is not valid it will be discarded
}

// SetTTL sets Metric to expire after the given seconds
func (m *Metric) SetTTL(seconds int) {
	d := time.Duration(seconds) * time.Second
	m.SetTTLDuration(d)
}

// SetTTLDuration sets Metric to expire after the given time.Duration
func (m *Metric) SetTTLDuration(d time.Duration) {
	exp := time.Now().Add(d)
	m.Expire = exp.UTC().Format(time.RFC3339Nano)
}

// GetTTL returns the time left before the Metric expires
func (m *Metric) GetTTL() time.Duration {
	if m.Expire == "" {
		return 0
	}
	exp, err := time.Parse(time.RFC3339Nano, m.Expire)
	if err != nil {
		panic(err)
	}
	return exp.Sub(time.Now())
}

// Expired returns if the Metric has expired
func (m *Metric) Expired() bool {
	if m.Expire == "" {
		return true
	}
	exp, err := time.Parse(time.RFC3339Nano, m.Expire)
	if err != nil {
		panic(err)
	}
	return time.Now().After(exp)
}

// Discard returns if the metric not valid or has expired
func (m *Metric) Discard() bool {
	return !m.Valid || m.Expired()
}

// Alert carries alerting information about a peer. WIP.
type Alert struct {
	Peer       peer.ID
	MetricName string
}

// Error can be used by APIs to return errors.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error implements the error interface and returns the error's message.
func (e Error) Error() string {
	return e.Message
}
