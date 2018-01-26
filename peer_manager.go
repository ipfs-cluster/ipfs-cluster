package ipfscluster

import (
	"context"
	"fmt"
	"time"

	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
)

// peerManager provides wrappers peerset control
type peerManager struct {
	host host.Host
	ctx  context.Context
}

func newPeerManager(h host.Host) *peerManager {
	return &peerManager{
		ctx:  context.Background(),
		host: h,
	}
}

func (pm *peerManager) addPeer(addr ma.Multiaddr, connect bool) error {
	logger.Debugf("adding peer address %s", addr)
	pid, decapAddr, err := multiaddrSplit(addr)
	if err != nil {
		return err
	}
	pm.host.Peerstore().AddAddr(pid, decapAddr, peerstore.PermanentAddrTTL)

	// dns multiaddresses need to be resolved because libp2p only does that
	// on explicit bhost.Connect().
	if madns.Matches(addr) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel()
		resolvedAddrs, err := madns.Resolve(ctx, addr)
		if err != nil {
			logger.Error(err)
			return err
		}
		pm.importAddresses(resolvedAddrs, connect)
	}
	if connect {
		pm.host.Network().DialPeer(pm.ctx, pid)
	}
	return nil
}

func (pm *peerManager) rmPeer(pid peer.ID) error {
	logger.Debugf("forgetting peer %s", pid.Pretty())
	pm.host.Peerstore().ClearAddrs(pid)
	return nil
}

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

func (pm *peerManager) importAddresses(addrs []ma.Multiaddr, connect bool) error {
	for _, a := range addrs {
		pm.addPeer(a, connect)
	}
	return nil
}
