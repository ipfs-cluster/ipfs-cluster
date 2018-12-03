package metrics

import (
	"github.com/ipfs/ipfs-cluster/api"

	peer "gx/ipfs/QmTRhk7cgjUf2gfQ3p2M9KPECNZEW9XUrmHcFCgog4cPgB/go-libp2p-peer"
)

// PeersetFilter removes all metrics not belonging to the given
// peerset
func PeersetFilter(metrics []api.Metric, peerset []peer.ID) []api.Metric {
	peerMap := make(map[peer.ID]struct{})
	for _, pid := range peerset {
		peerMap[pid] = struct{}{}
	}

	filtered := make([]api.Metric, 0, len(metrics))

	for _, metric := range metrics {
		_, ok := peerMap[metric.Peer]
		if !ok {
			continue
		}
		filtered = append(filtered, metric)
	}

	return filtered
}
