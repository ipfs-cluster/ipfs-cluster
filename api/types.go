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
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "github.com/ipfs/ipfs-cluster/api/pb"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	peer "github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-core/protocol"
	multiaddr "github.com/multiformats/go-multiaddr"

	// needed to parse /ws multiaddresses
	_ "github.com/libp2p/go-ws-transport"
	// needed to parse /dns* multiaddresses
	_ "github.com/multiformats/go-multiaddr-dns"

	"github.com/pkg/errors"
	proto "google.golang.org/protobuf/proto"
)

var logger = logging.Logger("apitypes")

var unixZero = time.Unix(0, 0)

func init() {
	// Use /p2p/ multiaddresses
	multiaddr.SwapToP2pMultiaddrs()

	// intialize trackerStatusString
	stringTrackerStatus = make(map[string]TrackerStatus)
	for k, v := range trackerStatusString {
		stringTrackerStatus[v] = k
	}
}

// TrackerStatus values
const (
	// IPFSStatus should never take this value.
	// When used as a filter. It means "all".
	TrackerStatusUndefined TrackerStatus = 0
	// The cluster node is offline or not responding
	TrackerStatusClusterError TrackerStatus = 1 << iota
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
	// The IPFS daemon is not pinning the item but it is being tracked
	TrackerStatusRemote
	// The item has been queued for pinning on the IPFS daemon
	TrackerStatusPinQueued
	// The item has been queued for unpinning on the IPFS daemon
	TrackerStatusUnpinQueued
	// The IPFS daemon is not pinning the item through this cid but it is
	// tracked in a cluster dag
	TrackerStatusSharded
)

// Composite TrackerStatus.
const (
	TrackerStatusError  = TrackerStatusClusterError | TrackerStatusPinError | TrackerStatusUnpinError
	TrackerStatusQueued = TrackerStatusPinQueued | TrackerStatusUnpinQueued
)

// TrackerStatus represents the status of a tracked Cid in the PinTracker
type TrackerStatus int

var trackerStatusString = map[TrackerStatus]string{
	TrackerStatusUndefined:    "undefined",
	TrackerStatusClusterError: "cluster_error",
	TrackerStatusPinError:     "pin_error",
	TrackerStatusUnpinError:   "unpin_error",
	TrackerStatusError:        "error",
	TrackerStatusPinned:       "pinned",
	TrackerStatusPinning:      "pinning",
	TrackerStatusUnpinning:    "unpinning",
	TrackerStatusUnpinned:     "unpinned",
	TrackerStatusRemote:       "remote",
	TrackerStatusPinQueued:    "pin_queued",
	TrackerStatusUnpinQueued:  "unpin_queued",
	TrackerStatusQueued:       "queued",
}

// values autofilled in init()
var stringTrackerStatus map[string]TrackerStatus

// String converts a TrackerStatus into a readable string.
// If the given TrackerStatus is a filter (with several
// bits set), it will return a comma-separated list.
func (st TrackerStatus) String() string {
	var values []string

	// simple and known composite values
	if v, ok := trackerStatusString[st]; ok {
		return v
	}

	// other filters
	for k, v := range trackerStatusString {
		if st&k > 0 {
			values = append(values, v)
		}
	}

	return strings.Join(values, ",")
}

// Match returns true if the tracker status matches the given filter.
// For example TrackerStatusPinError will match TrackerStatusPinError
// and TrackerStatusError
func (st TrackerStatus) Match(filter TrackerStatus) bool {
	return filter == 0 || st&filter > 0
}

// MarshalJSON uses the string representation of TrackerStatus for JSON
// encoding.
func (st TrackerStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(st.String())
}

// UnmarshalJSON sets a tracker status from its JSON representation.
func (st *TrackerStatus) UnmarshalJSON(data []byte) error {
	var v string
	err := json.Unmarshal(data, &v)
	if err != nil {
		return err
	}
	*st = TrackerStatusFromString(v)
	return nil
}

// TrackerStatusFromString parses a string and returns the matching
// TrackerStatus value. The string can be a comma-separated list
// representing a TrackerStatus filter. Unknown status names are
// ignored.
func TrackerStatusFromString(str string) TrackerStatus {
	values := strings.Split(strings.Replace(str, " ", "", -1), ",")
	var status TrackerStatus
	for _, v := range values {
		st, ok := stringTrackerStatus[v]
		if ok {
			status |= st
		}
	}
	return status
}

// TrackerStatusAll all known TrackerStatus values.
func TrackerStatusAll() []TrackerStatus {
	var list []TrackerStatus
	for k := range trackerStatusString {
		if k != TrackerStatusUndefined {
			list = append(list, k)
		}
	}

	return list
}

// IPFSPinStatus values
// FIXME include maxdepth
const (
	IPFSPinStatusBug IPFSPinStatus = iota
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
	// Since indirect statuses are of the form "indirect through <cid>"
	// use a prefix match

	switch {
	case strings.HasPrefix(t, "indirect"):
		return IPFSPinStatusIndirect
	case strings.HasPrefix(t, "recursive"):
		// FIXME: Maxdepth?
		return IPFSPinStatusRecursive
	case t == "direct":
		return IPFSPinStatusDirect
	default:
		return IPFSPinStatusBug
	}
}

// IsPinned returns true if the item is pinned as expected by the
// maxDepth parameter.
func (ips IPFSPinStatus) IsPinned(maxDepth int) bool {
	switch {
	case maxDepth < 0:
		return ips == IPFSPinStatusRecursive
	case maxDepth == 0:
		return ips == IPFSPinStatusDirect
	case maxDepth > 0:
		// FIXME: when we know how ipfs returns partial pins.
		return ips == IPFSPinStatusRecursive
	}
	return false
}

// ToTrackerStatus converts the IPFSPinStatus value to the
// appropriate TrackerStatus value.
func (ips IPFSPinStatus) ToTrackerStatus() TrackerStatus {
	return ipfsPinStatus2TrackerStatusMap[ips]
}

var ipfsPinStatus2TrackerStatusMap = map[IPFSPinStatus]TrackerStatus{
	IPFSPinStatusDirect:    TrackerStatusPinned,
	IPFSPinStatusRecursive: TrackerStatusPinned,
	IPFSPinStatusIndirect:  TrackerStatusUnpinned,
	IPFSPinStatusUnpinned:  TrackerStatusUnpinned,
	IPFSPinStatusBug:       TrackerStatusUndefined,
	IPFSPinStatusError:     TrackerStatusClusterError, //TODO(ajl): check suitability
}

// GlobalPinInfo contains cluster-wide status information about a tracked Cid,
// indexed by cluster peer.
type GlobalPinInfo struct {
	Cid cid.Cid `json:"cid" codec:"c"`
	// https://github.com/golang/go/issues/28827
	// Peer IDs are of string Kind(). We can't use peer IDs here
	// as Go ignores TextMarshaler.
	PeerMap map[string]*PinInfo `json:"peer_map" codec:"pm,omitempty"`
}

// String returns the string representation of a GlobalPinInfo.
func (gpi *GlobalPinInfo) String() string {
	str := fmt.Sprintf("Cid: %v\n", gpi.Cid.String())
	str = str + "Peer:\n"
	for _, p := range gpi.PeerMap {
		str = str + fmt.Sprintf("\t%+v\n", p)
	}
	return str
}

// PinInfo holds information about local pins.
type PinInfo struct {
	Cid      cid.Cid       `json:"cid" codec:"c"`
	Peer     peer.ID       `json:"peer" codec:"p,omitempty"`
	PeerName string        `json:"peername" codec:"pn,omitempty"`
	Status   TrackerStatus `json:"status" codec:"st,omitempty"`
	TS       time.Time     `json:"timestamp" codec:"ts,omitempty"`
	Error    string        `json:"error" codec:"e,omitempty"`
}

// Version holds version information
type Version struct {
	Version string `json:"version" codec:"v"`
}

// ConnectGraph holds information about the connectivity of the cluster To
// read, traverse the keys of ClusterLinks.  Each such id is one of the peers
// of the "ClusterID" peer running the query.  ClusterLinks[id] in turn lists
// the ids that peer "id" sees itself connected to.  It is possible that id is
// a peer of ClusterID, but ClusterID can not reach id over rpc, in which case
// ClusterLinks[id] == [], as id's view of its connectivity can not be
// retrieved.
//
// Iff there was an error reading the IPFSID of the peer then id will not be a
// key of ClustertoIPFS or IPFSLinks. Finally iff id is a key of ClustertoIPFS
// then id will be a key of IPFSLinks.  In the event of a SwarmPeers error
// IPFSLinks[id] == [].
type ConnectGraph struct {
	ClusterID    peer.ID           `json:"cluster_id" codec:"id"`
	IDtoPeername map[string]string `json:"id_to_peername" codec:"ip,omitempty"`
	// ipfs to ipfs links
	IPFSLinks map[string][]peer.ID `json:"ipfs_links" codec:"il,omitempty"`
	// cluster to cluster links
	ClusterLinks map[string][]peer.ID `json:"cluster_links" codec:"cl,omitempty"`
	// cluster trust links
	ClusterTrustLinks map[string]bool `json:"cluster_trust_links" codec:"ctl,omitempty"`
	// cluster to ipfs links
	ClustertoIPFS map[string]peer.ID `json:"cluster_to_ipfs" codec:"ci,omitempty"`
}

// Multiaddr is a concrete type to wrap a Multiaddress so that it knows how to
// serialize and deserialize itself.
type Multiaddr struct {
	multiaddr.Multiaddr
}

// NewMultiaddr returns a cluster Multiaddr wrapper creating the
// multiaddr.Multiaddr with the given string.
func NewMultiaddr(mstr string) (Multiaddr, error) {
	m, err := multiaddr.NewMultiaddr(mstr)
	return Multiaddr{Multiaddr: m}, err
}

// NewMultiaddrWithValue returns a new cluster Multiaddr wrapper using the
// given multiaddr.Multiaddr.
func NewMultiaddrWithValue(ma multiaddr.Multiaddr) Multiaddr {
	return Multiaddr{Multiaddr: ma}
}

// MarshalJSON returns a JSON-formatted multiaddress.
func (maddr Multiaddr) MarshalJSON() ([]byte, error) {
	return maddr.Multiaddr.MarshalJSON()
}

// UnmarshalJSON parses a cluster Multiaddr from the JSON representation.
func (maddr *Multiaddr) UnmarshalJSON(data []byte) error {
	maddr.Multiaddr, _ = multiaddr.NewMultiaddr("/ip4/127.0.0.1") // null multiaddresses not allowed
	return maddr.Multiaddr.UnmarshalJSON(data)
}

// MarshalBinary returs the bytes of the wrapped multiaddress.
func (maddr Multiaddr) MarshalBinary() ([]byte, error) {
	return maddr.Multiaddr.MarshalBinary()
}

// UnmarshalBinary casts some bytes as a multiaddress wraps it with
// the given cluster Multiaddr.
func (maddr *Multiaddr) UnmarshalBinary(data []byte) error {
	datacopy := make([]byte, len(data)) // This is super important
	copy(datacopy, data)
	maddr.Multiaddr, _ = multiaddr.NewMultiaddr("/ip4/127.0.0.1") // null multiaddresses not allowed
	return maddr.Multiaddr.UnmarshalBinary(datacopy)
}

// Value returns the wrapped multiaddr.Multiaddr.
func (maddr Multiaddr) Value() multiaddr.Multiaddr {
	return maddr.Multiaddr
}

// ID holds information about the Cluster peer
type ID struct {
	ID                    peer.ID     `json:"id" codec:"i,omitempty"`
	Addresses             []Multiaddr `json:"addresses" codec:"a,omitempty"`
	ClusterPeers          []peer.ID   `json:"cluster_peers" codec:"cp,omitempty"`
	ClusterPeersAddresses []Multiaddr `json:"cluster_peers_addresses" codec:"cpa,omitempty"`
	Version               string      `json:"version" codec:"v,omitempty"`
	Commit                string      `json:"commit" codec:"c,omitempty"`
	RPCProtocolVersion    protocol.ID `json:"rpc_protocol_version" codec:"rv,omitempty"`
	Error                 string      `json:"error" codec:"e,omitempty"`
	IPFS                  *IPFSID     `json:"ipfs,omitempty" codec:"ip,omitempty"`
	Peername              string      `json:"peername" codec:"pn,omitempty"`
	//PublicKey          crypto.PubKey
}

// IPFSID is used to store information about the underlying IPFS daemon
type IPFSID struct {
	ID        peer.ID     `json:"id,omitempty" codec:"i,omitempty"`
	Addresses []Multiaddr `json:"addresses" codec:"a,omitempty"`
	Error     string      `json:"error" codec:"e,omitempty"`
}

// PinType specifies which sort of Pin object we are dealing with.
// In practice, the PinType decides how a Pin object is treated by the
// PinTracker.
// See descriptions above.
// A sharded Pin would look like:
//
// [ Meta ] (not pinned on IPFS, only present in cluster state)
//   |
//   v
// [ Cluster DAG ] (pinned everywhere in "direct")
//   |      ..  |
//   v          v
// [Shard1] .. [ShardN] (allocated to peers and pinned with max-depth=1
// | | .. |    | | .. |
// v v .. v    v v .. v
// [][]..[]    [][]..[] Blocks (indirectly pinned on ipfs, not tracked in cluster)
//
//
type PinType uint64

// PinType values. See PinType documentation for further explanation.
const (
	// BadType type showing up anywhere indicates a bug
	BadType PinType = 1 << iota
	// DataType is a regular, non-sharded pin. It is pinned recursively.
	// It has no associated reference.
	DataType
	// MetaType tracks the original CID of a sharded DAG. Its Reference
	// points to the Cluster DAG CID.
	MetaType
	// ClusterDAGType pins carry the CID of the root node that points to
	// all the shard-root-nodes of the shards in which a DAG has been
	// divided. Its Reference carries the MetaType CID.
	// ClusterDAGType pins are pinned directly everywhere.
	ClusterDAGType
	// ShardType pins carry the root CID of a shard, which points
	// to individual blocks on the original DAG that the user is adding,
	// which has been sharded.
	// They carry a Reference to the previous shard.
	// ShardTypes are pinned with MaxDepth=1 (root and
	// direct children only).
	ShardType
)

// AllType is a PinType used for filtering all pin types
const AllType PinType = DataType | MetaType | ClusterDAGType | ShardType

// PinTypeFromString is the inverse of String.  It returns the PinType value
// corresponding to the input string
func PinTypeFromString(str string) PinType {
	switch str {
	case "pin":
		return DataType
	case "meta-pin":
		return MetaType
	case "clusterdag-pin":
		return ClusterDAGType
	case "shard-pin":
		return ShardType
	case "all":
		return AllType
	case "":
		return AllType
	default:
		return BadType
	}
}

// String returns a printable value to identify the PinType
func (pT PinType) String() string {
	switch pT {
	case DataType:
		return "pin"
	case MetaType:
		return "meta-pin"
	case ClusterDAGType:
		return "clusterdag-pin"
	case ShardType:
		return "shard-pin"
	case AllType:
		return "all"
	default:
		return "bad-type"
	}
}

var pinOptionsMetaPrefix = "meta-"

// PinOptions wraps user-defined options for Pins
type PinOptions struct {
	ReplicationFactorMin int               `json:"replication_factor_min" codec:"rn,omitempty"`
	ReplicationFactorMax int               `json:"replication_factor_max" codec:"rx,omitempty"`
	Name                 string            `json:"name" codec:"n,omitempty"`
	ShardSize            uint64            `json:"shard_size" codec:"s,omitempty"`
	UserAllocations      []peer.ID         `json:"user_allocations" codec:"ua,omitempty"`
	ExpireAt             time.Time         `json:"expire_at" codec:"e,omitempty"`
	Metadata             map[string]string `json:"metadata" codec:"m,omitempty"`
	PinUpdate            cid.Cid           `json:"pin_update,omitempty" codec:"pu,omitempty"`
}

// Equals returns true if two PinOption objects are equivalent. po and po2 may
// be nil.
func (po *PinOptions) Equals(po2 *PinOptions) bool {
	if po == nil && po2 != nil || po2 == nil && po != nil {
		return false
	}

	if po == po2 { // same as pin.Equals()
		return false
	}

	if po.Name != po2.Name {
		return false
	}

	if po.ReplicationFactorMax != po2.ReplicationFactorMax {
		return false
	}

	if po.ReplicationFactorMin != po2.ReplicationFactorMin {
		return false
	}

	if po.ShardSize != po2.ShardSize {
		return false
	}

	lenAllocs1 := len(po.UserAllocations)
	lenAllocs2 := len(po2.UserAllocations)
	if lenAllocs1 != lenAllocs2 {
		return false
	}

	// avoid side effects in the original objects
	allocs1 := PeersToStrings(po.UserAllocations)
	allocs2 := PeersToStrings(po2.UserAllocations)
	sort.Strings(allocs1)
	sort.Strings(allocs2)
	if strings.Join(allocs1, ",") != strings.Join(allocs2, ",") {
		return false
	}

	if !po.ExpireAt.Equal(po2.ExpireAt) {
		return false
	}

	for k, v := range po.Metadata {
		v2 := po2.Metadata[k]
		if k != "" && v != v2 {
			return false
		}
	}

	// deliberately ignore Update

	return true
}

// ToQuery returns the PinOption as query arguments.
func (po *PinOptions) ToQuery() (string, error) {
	q := url.Values{}
	q.Set("replication-min", fmt.Sprintf("%d", po.ReplicationFactorMin))
	q.Set("replication-max", fmt.Sprintf("%d", po.ReplicationFactorMax))
	q.Set("name", po.Name)
	q.Set("shard-size", fmt.Sprintf("%d", po.ShardSize))
	q.Set("user-allocations", strings.Join(PeersToStrings(po.UserAllocations), ","))
	if !po.ExpireAt.IsZero() {
		v, err := po.ExpireAt.MarshalText()
		if err != nil {
			return "", err
		}
		q.Set("expire-at", string(v))
	}
	for k, v := range po.Metadata {
		if k == "" {
			continue
		}
		q.Set(fmt.Sprintf("%s%s", pinOptionsMetaPrefix, k), v)
	}
	if po.PinUpdate != cid.Undef {
		q.Set("pin-update", po.PinUpdate.String())
	}
	return q.Encode(), nil
}

// FromQuery is the inverse of ToQuery().
func (po *PinOptions) FromQuery(q url.Values) error {
	po.Name = q.Get("name")
	rplStr := q.Get("replication")
	if rplStr != "" { // override
		q.Set("replication-min", rplStr)
		q.Set("replication-max", rplStr)
	}

	err := parseIntParam(q, "replication-min", &po.ReplicationFactorMin)
	if err != nil {
		return err
	}

	err = parseIntParam(q, "replication-max", &po.ReplicationFactorMax)
	if err != nil {
		return err
	}

	if v := q.Get("shard-size"); v != "" {
		shardSize, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return errors.New("parameter shard_size is invalid")
		}
		po.ShardSize = shardSize
	}

	if allocs := q.Get("user-allocations"); allocs != "" {
		po.UserAllocations = StringsToPeers(strings.Split(allocs, ","))
	}

	if v := q.Get("expire-at"); v != "" {
		var tm time.Time
		err := tm.UnmarshalText([]byte(v))
		if err != nil {
			return errors.Wrap(err, "expire-at cannot be parsed")
		}
		po.ExpireAt = tm
	} else if v = q.Get("expire-in"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return errors.Wrap(err, "expire-in cannot be parsed")
		}
		if d < time.Second {
			return errors.New("expire-in duration too short")
		}
		po.ExpireAt = time.Now().Add(d)
	}

	po.Metadata = make(map[string]string)
	for k := range q {
		if !strings.HasPrefix(k, pinOptionsMetaPrefix) {
			continue
		}
		metaKey := strings.TrimPrefix(k, pinOptionsMetaPrefix)
		if metaKey == "" {
			continue
		}
		po.Metadata[metaKey] = q.Get(k)
	}

	updateStr := q.Get("pin-update")
	if updateStr != "" {
		updateCid, err := cid.Decode(updateStr)
		if err != nil {
			return fmt.Errorf("error decoding update option parameter: %s", err)
		}
		po.PinUpdate = updateCid
	}
	return nil
}

// Pin carries all the information associated to a CID that is pinned
// in IPFS Cluster. It also carries transient information (that may not
// get protobuffed, like UserAllocations).
type Pin struct {
	PinOptions

	Cid cid.Cid `json:"cid" codec:"c"`

	// See PinType comments
	Type PinType `json:"type" codec:"t,omitempty"`

	// The peers to which this pin is allocated
	Allocations []peer.ID `json:"allocations" codec:"a,omitempty"`

	// MaxDepth associated to this pin. -1 means
	// recursive.
	MaxDepth int `json:"max_depth" codec:"d,omitempty"`

	// We carry a reference CID to this pin. For
	// ClusterDAGs, it is the MetaPin CID. For the
	// MetaPin it is the ClusterDAG CID. For Shards,
	// it is the previous shard CID.
	// When not needed the pointer is nil
	Reference *cid.Cid `json:"reference" codec:"r,omitempty"`
}

// String is a string representation of a Pin.
func (pin *Pin) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "cid: %s\n", pin.Cid.String())
	fmt.Fprintf(&b, "type: %s\n", pin.Type)
	fmt.Fprintf(&b, "allocations: %v\n", pin.Allocations)
	fmt.Fprintf(&b, "maxdepth: %d\n", pin.MaxDepth)
	if pin.Reference != nil {
		fmt.Fprintf(&b, "reference: %s\n", pin.Reference)
	}
	return b.String()
}

// PinPath is a wrapper for holding pin options and path of the content.
type PinPath struct {
	PinOptions
	Path string `json:"path"`
}

// PinCid is a shortcut to create a Pin only with a Cid.  Default is for pin to
// be recursive and the pin to be of DataType.
func PinCid(c cid.Cid) *Pin {
	return &Pin{
		Cid:         c,
		Type:        DataType,
		Allocations: []peer.ID{},
		MaxDepth:    -1,
	}
}

// PinWithOpts creates a new Pin calling PinCid(c) and then sets
// its PinOptions fields with the given options.
func PinWithOpts(c cid.Cid, opts PinOptions) *Pin {
	p := PinCid(c)
	p.PinOptions = opts
	return p
}

func convertPinType(t PinType) pb.Pin_PinType {
	var i pb.Pin_PinType
	for t != 1 {
		if t == 0 {
			return pb.Pin_BadType
		}
		t = t >> 1
		i++
	}
	return i
}

// ProtoMarshal marshals this Pin using probobuf.
func (pin *Pin) ProtoMarshal() ([]byte, error) {
	allocs := make([][]byte, len(pin.Allocations))
	for i, pid := range pin.Allocations {
		bs, err := pid.Marshal()
		if err != nil {
			return nil, err
		}
		allocs[i] = bs
	}

	var expireAtProto uint64
	// Only set the protobuf field with non-zero times.
	if !(pin.ExpireAt.IsZero() || pin.ExpireAt.Equal(unixZero)) {
		expireAtProto = uint64(pin.ExpireAt.Unix())
	}

	opts := &pb.PinOptions{
		ReplicationFactorMin: int32(pin.ReplicationFactorMin),
		ReplicationFactorMax: int32(pin.ReplicationFactorMax),
		Name:                 pin.Name,
		ShardSize:            pin.ShardSize,
		// UserAllocations:      pin.UserAllocations,
		Metadata:  pin.Metadata,
		PinUpdate: pin.PinUpdate.Bytes(),
		ExpireAt:  expireAtProto,
	}

	pbPin := &pb.Pin{
		Cid:         pin.Cid.Bytes(),
		Type:        convertPinType(pin.Type),
		Allocations: allocs,
		MaxDepth:    int32(pin.MaxDepth),
		Options:     opts,
	}
	if ref := pin.Reference; ref != nil {
		pbPin.Reference = ref.Bytes()
	}
	return proto.Marshal(pbPin)
}

// ProtoUnmarshal unmarshals this fields from protobuf-encoded bytes.
func (pin *Pin) ProtoUnmarshal(data []byte) error {
	pbPin := pb.Pin{}
	err := proto.Unmarshal(data, &pbPin)
	if err != nil {
		return err
	}
	ci, err := cid.Cast(pbPin.GetCid())
	if err != nil {
		pin.Cid = cid.Undef
	} else {
		pin.Cid = ci
	}

	pin.Type = 1 << uint64(pbPin.GetType())

	pbAllocs := pbPin.GetAllocations()
	lenAllocs := len(pbAllocs)
	allocs := make([]peer.ID, lenAllocs)
	for i, pidb := range pbAllocs {
		pid, err := peer.IDFromBytes(pidb)
		if err != nil {
			return err
		}
		allocs[i] = pid
	}

	pin.Allocations = allocs
	pin.MaxDepth = int(pbPin.GetMaxDepth())
	ref, err := cid.Cast(pbPin.GetReference())
	if err != nil {
		pin.Reference = nil

	} else {
		pin.Reference = &ref
	}

	opts := pbPin.GetOptions()
	pin.ReplicationFactorMin = int(opts.GetReplicationFactorMin())
	pin.ReplicationFactorMax = int(opts.GetReplicationFactorMax())
	pin.Name = opts.GetName()
	pin.ShardSize = opts.GetShardSize()
	// pin.UserAllocations = opts.GetUserAllocations()
	t := opts.GetExpireAt()
	if t > 0 {
		pin.ExpireAt = time.Unix(int64(t), 0)
	}
	pin.Metadata = opts.GetMetadata()
	pinUpdate, err := cid.Cast(opts.GetPinUpdate())
	if err == nil {
		pin.PinUpdate = pinUpdate
	}
	return nil
}

// Equals checks if two pins are the same (with the same allocations).
// If allocations are the same but in different order, they are still
// considered equivalent.
// pin or pin2 may be nil. If both are nil, Equals returns false.
func (pin *Pin) Equals(pin2 *Pin) bool {
	if pin == nil && pin2 != nil || pin2 == nil && pin != nil {
		return false
	}

	if pin == pin2 {
		// ask @lanzafame why this is not true
		// in any case, this is anomalous and we should
		// not be using this with two nils.
		return false
	}

	if !pin.Cid.Equals(pin2.Cid) {
		return false
	}

	if pin.Type != pin2.Type {
		return false
	}

	if pin.MaxDepth != pin2.MaxDepth {
		return false
	}

	if pin.Reference != nil && pin2.Reference == nil ||
		pin.Reference == nil && pin2.Reference != nil {
		return false
	}

	if pin.Reference != nil && pin2.Reference != nil &&
		!pin.Reference.Equals(*pin2.Reference) {
		return false
	}

	allocs1 := PeersToStrings(pin.Allocations)
	sort.Strings(allocs1)
	allocs2 := PeersToStrings(pin2.Allocations)
	sort.Strings(allocs2)

	if strings.Join(allocs1, ",") != strings.Join(allocs2, ",") {
		return false
	}

	return pin.PinOptions.Equals(&pin2.PinOptions)
}

// IsRemotePin determines whether a Pin's ReplicationFactor has
// been met, so as to either pin or unpin it from the peer.
func (pin *Pin) IsRemotePin(pid peer.ID) bool {
	if pin.ReplicationFactorMax < 0 || pin.ReplicationFactorMin < 0 {
		return false
	}

	for _, p := range pin.Allocations {
		if p == pid {
			return false
		}
	}
	return true
}

// ExpiredAt returns whether the pin has expired at the given time.
func (pin *Pin) ExpiredAt(t time.Time) bool {
	if pin.ExpireAt.IsZero() || pin.ExpireAt.Equal(unixZero) {
		return false
	}

	return pin.ExpireAt.Before(t)
}

// NodeWithMeta specifies a block of data and a set of optional metadata fields
// carrying information about the encoded ipld node
type NodeWithMeta struct {
	Data    []byte  `codec:"d,omitempty"`
	Cid     cid.Cid `codec:"c,omitempty"`
	CumSize uint64  `codec:"s,omitempty"` // Cumulative size
}

// Size returns how big is the block. It is different from CumSize, which
// records the size of the underlying tree.
func (n *NodeWithMeta) Size() uint64 {
	return uint64(len(n.Data))
}

// Metric transports information about a peer.ID. It is used to decide
// pin allocations by a PinAllocator. IPFS cluster is agnostic to
// the Value, which should be interpreted by the PinAllocator.
// The ReceivedAt value is a timestamp representing when a peer has received
// the metric value.
type Metric struct {
	Name       string  `json:"name" codec:"n,omitempty"`
	Peer       peer.ID `json:"peer" codec:"p,omitempty"`
	Value      string  `json:"value" codec:"v,omitempty"`
	Expire     int64   `json:"expire" codec:"e,omitempty"`
	Valid      bool    `json:"valid" codec:"d,omitempty"`
	ReceivedAt int64   `json:"received_at" codec:"t,omitempty"` // ReceivedAt contains a UnixNano timestamp
}

// SetTTL sets Metric to expire after the given time.Duration
func (m *Metric) SetTTL(d time.Duration) {
	exp := time.Now().Add(d)
	m.Expire = exp.UnixNano()
}

// GetTTL returns the time left before the Metric expires
func (m *Metric) GetTTL() time.Duration {
	expDate := time.Unix(0, m.Expire)
	return time.Until(expDate)
}

// Expired returns if the Metric has expired
func (m *Metric) Expired() bool {
	expDate := time.Unix(0, m.Expire)
	return time.Now().After(expDate)
}

// Discard returns if the metric not valid or has expired
func (m *Metric) Discard() bool {
	return !m.Valid || m.Expired()
}

// MetricSlice is a sortable Metric array.
type MetricSlice []*Metric

func (es MetricSlice) Len() int      { return len(es) }
func (es MetricSlice) Swap(i, j int) { es[i], es[j] = es[j], es[i] }
func (es MetricSlice) Less(i, j int) bool {
	if es[i].Peer == es[j].Peer {
		return es[i].Expire < es[j].Expire
	}
	return es[i].Peer < es[j].Peer
}

// Alert carries alerting information about a peer. WIP.
type Alert struct {
	Peer       peer.ID
	MetricName string
}

// Error can be used by APIs to return errors.
type Error struct {
	Code    int    `json:"code" codec:"o,omitempty"`
	Message string `json:"message" codec:"m,omitempty"`
}

// Error implements the error interface and returns the error's message.
func (e *Error) Error() string {
	return fmt.Sprintf("%s (%d)", e.Message, e.Code)
}

// IPFSRepoStat wraps information about the IPFS repository.
type IPFSRepoStat struct {
	RepoSize   uint64 `codec:"r,omitempty"`
	StorageMax uint64 `codec:"s, omitempty"`
}

// IPFSRepoGC represents the streaming response sent from repo gc API of IPFS.
type IPFSRepoGC struct {
	Key   cid.Cid `json:"key,omitempty" codec:"k,omitempty"`
	Error string  `json:"error,omitempty" codec:"e,omitempty"`
}

// RepoGC contains garbage collected CIDs from a cluster peer's IPFS daemon.
type RepoGC struct {
	Peer     peer.ID      `json:"peer" codec:"p,omitempty"` // the Cluster peer ID
	Peername string       `json:"peername" codec:"pn,omitempty"`
	Keys     []IPFSRepoGC `json:"keys" codec:"k"`
	Error    string       `json:"error,omitempty" codec:"e,omitempty"`
}

// GlobalRepoGC contains cluster-wide information about garbage collected CIDs
// from IPFS.
type GlobalRepoGC struct {
	PeerMap map[string]*RepoGC `json:"peer_map" codec:"pm,omitempty"`
}
