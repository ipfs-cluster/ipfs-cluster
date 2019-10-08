package metrics

import (
	"github.com/ipfs/ipfs-cluster/api"

	peer "github.com/libp2p/go-libp2p-core/peer"
)

// PeersetFilter removes all metrics not belonging to the given
// peerset and returns an array of metrics ordered as per peers in peerset.
// There will only be one metric per peer.
func PeersetFilter(metrics []*api.Metric, peerset []peer.ID) []*api.Metric {
	peerMap := make(map[peer.ID]*api.Metric)
	for _, pid := range peerset {
		peerMap[pid] = &api.Metric{}
	}

	filtered := make([]*api.Metric, 0, len(metrics))
	for _, metric := range metrics {
		_, ok := peerMap[metric.Peer]
		if !ok {
			continue
		}
		peerMap[metric.Peer] = metric
	}

	for _, peer := range peerset {
		metric := peerMap[peer]
		if (*metric == api.Metric{}) {
			continue
		}
		filtered = append(filtered, metric)
	}

	return filtered
}
