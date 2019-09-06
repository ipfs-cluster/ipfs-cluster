package api

import (
	peer "github.com/libp2p/go-libp2p-core/peer"
)

// PeersToStrings IDB58Encodes a list of peers.
func PeersToStrings(peers []peer.ID) []string {
	strs := make([]string, len(peers))
	for i, p := range peers {
		if p != "" {
			strs[i] = peer.IDB58Encode(p)
		}
	}
	return strs
}

// StringsToPeers decodes peer.IDs from strings.
func StringsToPeers(strs []string) []peer.ID {
	peers := []peer.ID{}
	for _, p := range strs {
		pid, err := peer.IDB58Decode(p)
		if err != nil {
			logger.Debugf("'%s': %s", p, err)
			continue
		}
		peers = append(peers, pid)
	}
	return peers
}
