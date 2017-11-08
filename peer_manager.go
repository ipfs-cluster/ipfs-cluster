package ipfscluster

import (
	"fmt"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"
)

// peerManager provides wrappers peerset control
type peerManager struct {
	host host.Host
}

func newPeerManager(h host.Host) *peerManager {
	return &peerManager{h}
}

func (pm *peerManager) addPeer(addr ma.Multiaddr) error {
	logger.Debugf("adding peer address %s", addr)
	pid, decapAddr, err := multiaddrSplit(addr)
	if err != nil {
		return err
	}
	pm.host.Peerstore().AddAddr(pid, decapAddr, peerstore.PermanentAddrTTL)
	return nil
}

func (pm *peerManager) rmPeer(pid peer.ID) error {
	logger.Debugf("forgetting peer %s", pid.Pretty())
	pm.host.Peerstore().ClearAddrs(pid)
	return nil
}

// func (pm *peerManager) savePeers() {
// 	peers := pm.peersAddrs()
// 	logger.Debugf("saving peers: %s", peers)
// 	pm.cluster.config.Peers = peers
// 	pm.cluster.config.NotifySave()
// }

// func (pm *peerManager) isPeer(p peer.ID) bool {
// 	if p == pm.cluster.id {
// 		return true
// 	}

// 	peers := pm.cluster.consensus.Peers()
// 	for _, pid := range peers {
// 		if p == pid {
// 			return true
// 		}
// 	}
// 	return false
// }

// cluster peer addresses (NOT including ourselves)
func (pm *peerManager) addresses(peers []peer.ID) []ma.Multiaddr {
	addrs := []ma.Multiaddr{}
	if peers == nil {
		return addrs
	}

	for _, p := range peers {
		if p == pm.host.ID() {
			continue
		}
		peerAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", peer.IDB58Encode(p)))
		for _, a := range pm.host.Peerstore().Addrs(p) {
			addrs = append(addrs, a.Encapsulate(peerAddr))
		}
	}
	return addrs
}

func (pm *peerManager) importAddresses(addrs []ma.Multiaddr) error {
	for _, a := range addrs {
		pm.addPeer(a)
	}
	return nil
}
