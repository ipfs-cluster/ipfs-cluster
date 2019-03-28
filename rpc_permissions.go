package ipfscluster

import (
	"errors"

	peer "github.com/libp2p/go-libp2p-peer"
)

type peerKind int

const (
	all peerKind = iota
	trusted
)

const (
	raftPolicy string = "raft"
	crdtStrict string = "crdt_strict"
	crdtSoft   string = "crdt_soft"
)

type permissionPolicy map[peerKind]map[string]bool
type authorizer struct {
	policyName string
	policy     permissionPolicy
	trusted    map[peer.ID]bool
}

func newAuthorizer(policyName string, trusted map[peer.ID]bool) (*authorizer, error) {
	policy, err := getPermissionPolicy(policyName)
	if err != nil {
		return nil, err
	}

	return &authorizer{
		policyName: policyName,
		policy:     policy,
		trusted:    trusted,
	}, nil
}

func (a *authorizer) authorizeFunc() func(pid peer.ID, svc string, method string) bool {
	return func(pid peer.ID, svc string, method string) bool {
		if a.trusted[pid] {
			return a.policy[trusted][svc+"."+method]
		}

		return a.policy[all][svc+"."+method]
	}
}

func (a *authorizer) addPeers(peers []peer.ID) {
	if a.trusted == nil {
		a.trusted = make(map[peer.ID]bool)
	}

	for _, p := range peers {
		a.trusted[p] = true
	}
}

func getPermissionPolicy(name string) (permissionPolicy, error) {
	switch name {
	case raftPolicy:
		return permissionPolicy{
			all: {
				"Cluster.ID":      true,
				"Cluster.PeerAdd": true,
				"Cluster.Peers":   true,

				"Cluster.TrackerStatusAll": true,
				"Cluster.TrackerStatus":    true,
				"Cluster.TrackerRecover":   true,

				"Cluster.SyncAllLocal": true,
				"Cluster.SyncLocal":    true,

				"Cluster.IPFSConnectSwarms": true,
				"Cluster.IPFSSwarmPeers":    true,
				"Cluster.IPFSBlockPut":      true,

				"Cluster.ConsensusAddPeer":  true,
				"Cluster.ConsensusRmPeer":   true,
				"Cluster.ConsensusLogPin":   true,
				"Cluster.ConsensusLogUnpin": true,
			},
		}, nil
	case crdtStrict:
		return permissionPolicy{
			all: {
				"Cluster.ID":      true,
				"Cluster.PeerAdd": true,

				"Cluster.PeerMonitorLogMetric": true,

				"Cluster.IPFSConnectSwarms": true,
			},
			trusted: {
				"Cluster.ID":      true,
				"Cluster.PeerAdd": true,

				"Cluster.TrackerStatusAll": true,
				"Cluster.TrackerStatus":    true,

				"Cluster.PeerMonitorLogMetric": true,

				"Cluster.IPFSConnectSwarms": true,
				"Cluster.IPFSSwarmPeers":    true,
			},
		}, nil
	case crdtSoft:
		return permissionPolicy{
			all: {
				"Cluster.ID":      true,
				"Cluster.PeerAdd": true,

				"Cluster.PeerMonitorLogMetric": true,

				"Cluster.IPFSConnectSwarms": true,
			},
			trusted: {
				"Cluster.ID":      true,
				"Cluster.PeerAdd": true,
				"Cluster.Peers":   true,

				"Cluster.TrackerStatusAll":  true,
				"Cluster.TrackerStatus":     true,
				"Cluster.TrackerRecover":    true,
				"Cluster.TrackerRecoverAll": true,

				"Cluster.Sync":         true,
				"Cluster.SyncAll":      true,
				"Cluster.SyncLocal":    true,
				"Cluster.SyncAllLocal": true,

				"Cluster.PeerMonitorLogMetric": true,

				"Cluster.IPFSConnectSwarms": true,
				"Cluster.IPFSSwarmPeers":    true,
				"Cluster.IPFSBlockPut":      true,
			},
		}, nil
	default:
		return permissionPolicy{}, errors.New("invalid permission policy name")
	}
}
