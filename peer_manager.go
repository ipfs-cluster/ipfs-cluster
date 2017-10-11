package ipfscluster

import (
	"sync"
	"time"

	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"
)

// peerManager is our own local peerstore
type peerManager struct {
	cluster *Cluster
	ps      peerstore.Peerstore
	self    peer.ID

	peermap map[peer.ID]ma.Multiaddr
	m       sync.RWMutex
}

func newPeerManager(c *Cluster) *peerManager {
	pm := &peerManager{
		cluster: c,
		ps:      c.host.Peerstore(),
		self:    c.host.ID(),
	}
	pm.resetPeers()
	return pm
}

func (pm *peerManager) addPeer(addr ma.Multiaddr) error {
	logger.Debugf("adding peer %s", addr)
	pid, decapAddr, err := multiaddrSplit(addr)
	if err != nil {
		return err
	}
	pm.ps.AddAddr(pid, decapAddr, peerstore.PermanentAddrTTL)

	if !pm.isPeer(pid) {
		logger.Infof("new Cluster peer %s", addr.String())
	}

	pm.m.Lock()
	pm.peermap[pid] = addr
	pm.m.Unlock()

	return nil
}

func (pm *peerManager) rmPeer(pid peer.ID, selfShutdown bool) error {
	logger.Debugf("removing peer %s", pid.Pretty())

	if pm.isPeer(pid) {
		logger.Infof("removing Cluster peer %s", pid.Pretty())
	}

	pm.m.Lock()
	delete(pm.peermap, pid)
	pm.m.Unlock()

	// It's ourselves. This is not very graceful
	if pid == pm.self && selfShutdown {
		logger.Warning("this peer has been removed from the Cluster and will shutdown itself in 5 seconds")
		defer func() {
			go func() {
				time.Sleep(1 * time.Second)
				pm.cluster.consensus.Shutdown()
				pm.cluster.config.Bootstrap = pm.peersAddrs()
				pm.cluster.config.Shadow()
				pm.cluster.config.Save("")
				pm.resetPeers()
				time.Sleep(4 * time.Second)
				pm.cluster.Shutdown()
			}()
		}()
	}

	return nil
}

func (pm *peerManager) savePeers() {
	pm.cluster.config.ClusterPeers = pm.peersAddrs()
	pm.cluster.config.Save("")
}

func (pm *peerManager) resetPeers() {
	pm.m.Lock()
	pm.peermap = make(map[peer.ID]ma.Multiaddr)
	pm.peermap[pm.self] = pm.cluster.config.ClusterAddr
	pm.m.Unlock()
}

func (pm *peerManager) isPeer(p peer.ID) bool {
	if p == pm.self {
		return true
	}

	pm.m.RLock()
	_, ok := pm.peermap[p]
	pm.m.RUnlock()
	return ok
}

// peers including ourselves
func (pm *peerManager) peers() []peer.ID {
	pm.m.RLock()
	defer pm.m.RUnlock()
	var peers []peer.ID
	for k := range pm.peermap {
		peers = append(peers, k)
	}
	return peers
}

// cluster peer addresses (NOT including ourselves)
func (pm *peerManager) peersAddrs() []ma.Multiaddr {
	pm.m.RLock()
	defer pm.m.RUnlock()
	var addrs []ma.Multiaddr
	for k, addr := range pm.peermap {
		if k != pm.self {
			addrs = append(addrs, addr)
		}
	}
	return addrs
}

// func (pm *peerManager) addFromConfig(cfg *Config) error {
// 	return pm.addFromMultiaddrs(cfg.ClusterPeers)
// }

func (pm *peerManager) addFromMultiaddrs(addrs []ma.Multiaddr) error {
	for _, m := range addrs {
		err := pm.addPeer(m)
		if err != nil {
			logger.Error(err)
			return err
		}
	}
	return nil
}
