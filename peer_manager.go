package ipfscluster

import (
	"sync"
	"time"

	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"
)

type peerManager struct {
	cluster *Cluster
	ps      peerstore.Peerstore

	peerSet map[peer.ID]ma.Multiaddr
	mux     sync.Mutex
}

func newPeerManager(c *Cluster) *peerManager {
	pm := &peerManager{
		cluster: c,
		ps:      c.host.Peerstore(),
	}
	pm.resetPeers()
	return pm
}

func (pm *peerManager) addPeer(addr ma.Multiaddr) (peer.ID, error) {
	logger.Debugf("adding peer %s", addr)

	pid, decapAddr, err := multiaddrSplit(addr)
	if err != nil {
		return pid, err
	}

	if pm.isPeer(pid) {
		logger.Debugf("%s is already a peer", pid)
		return pid, nil
	}

	pm.mux.Lock()
	pm.peerSet[pid] = addr
	pm.mux.Unlock()
	pm.ps.AddAddr(pid, decapAddr, peerstore.PermanentAddrTTL)
	pm.cluster.config.addPeer(addr)
	if con := pm.cluster.consensus; con != nil {
		pm.cluster.consensus.AddPeer(pid)
	}
	if path := pm.cluster.config.path; path != "" {
		err := pm.cluster.config.Save(path)
		if err != nil {
			logger.Error(err)
		}
	}
	logger.Infof("new Cluster peer %s", addr.String())
	return pid, nil
}

func (pm *peerManager) rmPeer(pid peer.ID, selfShutdown bool) error {
	logger.Debugf("removing peer %s", pid.Pretty())

	if !pm.isPeer(pid) {
		return nil
	}

	pm.mux.Lock()
	delete(pm.peerSet, pid)
	pm.mux.Unlock()
	pm.cluster.host.Peerstore().ClearAddrs(pid)
	pm.cluster.config.rmPeer(pid)
	pm.cluster.consensus.RemovePeer(pid)

	// It's ourselves. This is not very graceful
	if pid == pm.cluster.host.ID() && selfShutdown {
		logger.Warning("this peer has been removed from the Cluster and will shutdown itself in 5 seconds")
		pm.cluster.config.emptyPeers()
		defer func() {
			go func() {
				time.Sleep(1 * time.Second)
				pm.cluster.consensus.Shutdown()
				time.Sleep(4 * time.Second)
				pm.cluster.Shutdown()
			}()
		}()
	}

	if path := pm.cluster.config.path; path != "" {
		pm.cluster.config.Save(path)
	}
	logger.Infof("removed peer %s", pid.Pretty())
	return nil
}

func (pm *peerManager) isPeer(p peer.ID) bool {
	pm.mux.Lock()
	_, ok := pm.peerSet[p]
	pm.mux.Unlock()
	return ok
}

// empty the peerstore
func (pm *peerManager) resetPeers() {
	pm.mux.Lock()
	defer pm.mux.Unlock()
	pm.peerSet = make(map[peer.ID]ma.Multiaddr)
	pm.peerSet[pm.cluster.host.ID()] = pm.cluster.config.ClusterAddr
}

func (pm *peerManager) peers() []peer.ID {
	pm.mux.Lock()
	defer pm.mux.Unlock()
	var peers []peer.ID
	for k := range pm.peerSet {
		peers = append(peers, k)
	}
	return peers
}

// cluster peer addresses (NOT including ourselves)
func (pm *peerManager) peersAddrs() []ma.Multiaddr {
	pm.mux.Lock()
	defer pm.mux.Unlock()

	var addrs []ma.Multiaddr
	for pid, addr := range pm.peerSet {
		if pid == pm.cluster.host.ID() {
			continue
		}
		addrs = append(addrs, addr)
	}
	return addrs
}

func (pm *peerManager) addFromConfig(cfg *Config) error {
	return pm.addFromMultiaddrs(cfg.ClusterPeers)
}

func (pm *peerManager) addFromMultiaddrs(addrs []ma.Multiaddr) error {
	for _, m := range addrs {
		_, err := pm.addPeer(m)
		if err != nil {
			logger.Error(err)
			return err
		}
	}

	return nil
}
