package api

import (
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
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

// MustLibp2pMultiaddrJoin takes a LibP2P multiaddress and a peer ID and
// encapsulates a new /ipfs/<peerID> address. It will panic if the given
// peer ID is bad.
func MustLibp2pMultiaddrJoin(addr Multiaddr, p peer.ID) Multiaddr {
	v := addr.Value()
	pidAddr, err := ma.NewMultiaddr("/ipfs/" + peer.IDB58Encode(p))
	// let this break badly
	if err != nil {
		panic("called MustLibp2pMultiaddrJoin with bad peer!")
	}
	return Multiaddr{Multiaddr: v.Encapsulate(pidAddr)}
}
