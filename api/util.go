package api

import (
	"fmt"

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
	peers := make([]peer.ID, len(strs))
	for i, p := range strs {
		var err error
		peers[i], err = peer.IDB58Decode(p)
		if err != nil {
			logger.Debugf("'%s': %s", p, err)
		}
	}
	return peers
}

// Libp2pMultiaddrSplit takes a LibP2P multiaddress (/<multiaddr>/ipfs/<peerID>)
// and decapsulates it, parsing the peer ID. Returns an error if there is
// any problem (for example, the provided address not being a Libp2p one).
func Libp2pMultiaddrSplit(addr ma.Multiaddr) (peer.ID, ma.Multiaddr, error) {
	pid, err := addr.ValueForProtocol(ma.P_IPFS)
	if err != nil {
		err = fmt.Errorf("invalid peer multiaddress: %s: %s", addr, err)
		logger.Error(err)
		return "", nil, err
	}

	ipfs, _ := ma.NewMultiaddr("/ipfs/" + pid)
	decapAddr := addr.Decapsulate(ipfs)

	peerID, err := peer.IDB58Decode(pid)
	if err != nil {
		err = fmt.Errorf("invalid peer ID in multiaddress: %s: %s", pid, err)
		logger.Error(err)
		return "", nil, err
	}
	return peerID, decapAddr, nil
}

// MustLibp2pMultiaddrJoin takes a LibP2P multiaddress and a peer ID and
// encapsulates a new /ipfs/<peerID> address. It will panic if the given
// peer ID is bad.
func MustLibp2pMultiaddrJoin(addr ma.Multiaddr, p peer.ID) ma.Multiaddr {
	pidAddr, err := ma.NewMultiaddr("/ipfs/" + peer.IDB58Encode(p))
	// let this break badly
	if err != nil {
		panic("called MustLibp2pMultiaddrJoin with bad peer!")
	}
	return addr.Encapsulate(pidAddr)
}
