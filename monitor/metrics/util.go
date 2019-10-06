package metrics

import (
	"github.com/ipfs/ipfs-cluster/api"

	peer "github.com/libp2p/go-libp2p-core/peer"
)

// PeersetFilter removes all metrics not belonging to the given
// peerset
func PeersetFilter(metrics []*api.Metric, peerset []peer.ID) []*api.Metric {
	peerMap := make(map[peer.ID]struct{})
	for _, pid := range peerset {
		peerMap[pid] = struct{}{}
	}

	metricsByPeer := make(map[peer.ID]*api.Metric)
	filtered := make([]*api.Metric, 0, len(metrics))

	for _, metric := range metrics {
		_, ok := peerMap[metric.Peer]
		if !ok {
			continue
		}
		metricsByPeer[metric.Peer] = metric
	}

	for _, peer := range peerset {
		metric, ok := metricsByPeer[peer]
		if !ok {
			continue
		}
		filtered = append(filtered, metric)
	}

	return filtered
}
