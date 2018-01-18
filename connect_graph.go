package ipfscluster

import (
	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
)

// ConnectGraph returns a description of which cluster peers and ipfs
// daemons are connected to each other
func (c *Cluster) ConnectGraph() (api.ConnectGraph, error) {
	cg := api.ConnectGraph{
		IPFSLinks:     make(map[peer.ID][]peer.ID),
		ClusterLinks:  make(map[peer.ID][]peer.ID),
		ClustertoIPFS: make(map[peer.ID]peer.ID),
	}
	members, err := c.consensus.Peers()
	if err != nil {
		return cg, err
	}

	peersSerials := make([][]api.IDSerial, len(members), len(members))
	errs := c.multiRPC(members, "Cluster", "Peers", struct{}{},
		copyIDSerialSliceToIfaces(peersSerials))

	for i, err := range errs {
		p := members[i]
		cg.ClusterLinks[p] = make([]peer.ID, 0)
		if err != nil { // Only setting cluster connections when no error occurs
			logger.Debugf("RPC error reaching cluster peer %s: %s", p.Pretty(), err.Error())
			continue
		}

		selfConnection, pID := c.recordClusterLinks(&cg, p, peersSerials[i])

		// IPFS connections
		if !selfConnection {
			logger.Warningf("cluster peer %s not its own peer.  No ipfs info ", p.Pretty())
			continue
		}
		c.recordIPFSLinks(&cg, pID)
	}

	return cg, nil
}

func (c *Cluster) recordClusterLinks(cg *api.ConnectGraph, p peer.ID, sPeers []api.IDSerial) (bool, api.ID) {
	selfConnection := false
	var pID api.ID
	for _, sID := range sPeers {
		id := sID.ToID()
		if id.Error != "" {
			logger.Debugf("Peer %s errored connecting to its peer %s", p.Pretty(), id.ID.Pretty())
			continue
		}
		if id.ID == p {
			selfConnection = true
			pID = id
		} else {
			cg.ClusterLinks[p] = append(cg.ClusterLinks[p], id.ID)
		}
	}
	return selfConnection, pID
}

func (c *Cluster) recordIPFSLinks(cg *api.ConnectGraph, pID api.ID) {
	ipfsID := pID.IPFS.ID
	if pID.IPFS.Error != "" { // Only setting ipfs connections when no error occurs
		logger.Warningf("ipfs id: %s has error: %s. Skipping swarm connections", ipfsID.Pretty(), pID.IPFS.Error)
		return
	}
	if _, ok := cg.IPFSLinks[pID.ID]; ok {
		logger.Warningf("ipfs id: %s already recorded, one ipfs daemon in use by multiple cluster peers", ipfsID.Pretty())
	}
	cg.ClustertoIPFS[pID.ID] = ipfsID
	cg.IPFSLinks[ipfsID] = make([]peer.ID, 0)
	var swarmPeersS api.SwarmPeersSerial
	err := c.rpcClient.Call(pID.ID,
		"Cluster",
		"IPFSSwarmPeers",
		struct{}{},
		&swarmPeersS,
	)
	if err != nil {
		return
	}
	swarmPeers := swarmPeersS.ToSwarmPeers()
	cg.IPFSLinks[ipfsID] = swarmPeers
}
