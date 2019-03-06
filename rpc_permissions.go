package ipfscluster

import (
	peer "github.com/libp2p/go-libp2p-peer"
)

type peerKind int

const (
	all peerKind = iota
	trusted
)

type permissionPolicy map[peerKind]map[string]bool

func (c *Cluster) authorizeWithPolicy() func(pid peer.ID, svc string, method string) bool {
	policyName := c.config.PermissionPolicy
	policy := getPermissionPolicy(policyName)
	return func(pid peer.ID, svc string, method string) bool {
		if policy == nil || policyName != DefaultPermissionPolicy {
			return false
		}

		return policy[all][svc+"."+method]
	}
}

func getPermissionMap() map[string]permissionPolicy {
	return map[string]permissionPolicy{
		"raft": permissionPolicy{
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

				"Cluster.ConsensusAddPeer": true,
				"Cluster.ConsensusRmPeer":  true,
			},
		},
	}
}

func getPermissionPolicy(name string) permissionPolicy {
	return getPermissionMap()[name]
}
