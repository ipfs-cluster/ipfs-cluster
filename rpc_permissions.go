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
	raft       string = "raft"
	crdtStrict string = "crdt_strict"
	crdtSoft   string = "crdt_soft"
)

type permissionPolicy map[peerKind]map[string]bool
type authorizer struct {
	policyName string
	policy     permissionPolicy
	trusted    []peer.ID
}

func newAuthorizer(policyName string, trusted []peer.ID) (*authorizer, error) {
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
	policyName := a.policyName
	policy := a.policy

	return func(pid peer.ID, svc string, method string) bool {
		if policyName == raft {
			return policy[all][svc+"."+method]
		}

		return false
	}
}

func (a *authorizer) addPeers(peer peer.ID) {
	a.trusted = append(a.trusted, peer)
}

func getPermissionPolicy(name string) (permissionPolicy, error) {
	switch name {
	case raft:
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
				"Cluster.IPFSBlockPut":      true,
				"Cluster.IPFSSwarmPeers":    true,

				"Cluster.ConsensusAddPeer":  true,
				"Cluster.ConsensusRmPeer":   true,
				"Cluster.ConsensusLogPin":   true,
				"Cluster.ConsensusLogUnpin": true,
			},
		}, nil
	// TODO(Kishan): Add policies `crdt_soft` and `crdt_strict`
	default:
		return permissionPolicy{}, errors.New("invalid permission policy name")
	}
}
