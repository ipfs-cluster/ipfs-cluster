package ipfscluster

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"
)

type peerManager struct {
	cluster *Cluster

	peerSetMux sync.RWMutex
	peerSet    map[peer.ID]struct{}
}

func newPeerManager(c *Cluster) *peerManager {
	pm := &peerManager{
		cluster: c,
	}
	pm.resetPeerSet()
	return pm
}

func (pm *peerManager) addPeer(addr ma.Multiaddr) (peer.ID, error) {
	logger.Debugf("adding peer %s", addr)

	pid, decapAddr, err := multiaddrSplit(addr)
	if err != nil {
		return pid, err
	}

	pm.peerSetMux.RLock()
	_, ok := pm.peerSet[pid]
	pm.peerSetMux.RUnlock()

	if ok {
		logger.Debugf("%s is already a peer", pid)
		return pid, nil
	}

	pm.peerSetMux.Lock()
	pm.peerSet[pid] = struct{}{}
	pm.peerSetMux.Unlock()

	knownAddrs := pm.cluster.host.Peerstore().Addrs(pid)
	if len(knownAddrs) > 0 { // we have it in the peerstore, trust that
		addr, _ = multiaddrJoin(knownAddrs[0], pid)
		logger.Debugf("peer address was in peerstore so we are using %s for the config", addr)
	} else {
		pm.cluster.host.Peerstore().AddAddr(pid, decapAddr, peerstore.PermanentAddrTTL)
	}

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
	logger.Infof("added peer %s", addr.String())
	return pid, nil
}

func (pm *peerManager) isPeer(p peer.ID) bool {
	pm.peerSetMux.RLock()
	defer pm.peerSetMux.RUnlock()
	_, ok := pm.peerSet[p]
	return ok
}

func (pm *peerManager) rmPeer(p peer.ID) error {
	logger.Debugf("removing peer %s", p.Pretty())
	pm.peerSetMux.RLock()
	_, ok := pm.peerSet[p]
	pm.peerSetMux.RUnlock()
	if !ok {
		return nil
	}
	pm.peerSetMux.Lock()
	delete(pm.peerSet, p)
	pm.peerSetMux.Unlock()
	pm.cluster.host.Peerstore().ClearAddrs(p)
	pm.cluster.config.rmPeer(p)
	pm.cluster.consensus.RemovePeer(p)

	// It's ourselves. This is not very graceful
	if p == pm.cluster.host.ID() {
		logger.Warning("this peer has been removed from the Cluster and will shutdown itself")
		pm.cluster.config.emptyPeers()
		defer func() {
			go func() {
				time.Sleep(time.Second)
				pm.cluster.consensus.Shutdown()
				pm.selfShutdown()
			}()
		}()
	}

	if path := pm.cluster.config.path; path != "" {
		pm.cluster.config.Save(path)
	}
	logger.Infof("removed peer %s", p.Pretty())
	return nil
}

func (pm *peerManager) selfShutdown() {
	err := pm.cluster.Shutdown()
	if err == nil {
		// If the shutdown worked correctly
		// (including snapshot) we can remove the Raft
		// database (which traces peers additions
		// and removals). It makes re-start of the peer
		// way less confusing for Raft while the state
		// kept in the snapshot.
		os.Remove(filepath.Join(pm.cluster.config.ConsensusDataFolder, "raft.db"))
	}
}

// empty the peerset and add ourselves only
func (pm *peerManager) resetPeerSet() {
	pm.peerSetMux.Lock()
	defer pm.peerSetMux.Unlock()
	pm.peerSet = make(map[peer.ID]struct{})
	pm.peerSet[pm.cluster.host.ID()] = struct{}{}
}

func (pm *peerManager) peers() []peer.ID {
	pm.peerSetMux.RLock()
	defer pm.peerSetMux.RUnlock()
	var pList []peer.ID
	for k := range pm.peerSet {
		pList = append(pList, k)
	}
	return pList
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
