package ipfscluster

import (
	peer "github.com/libp2p/go-libp2p-peer"

	"go.opencensus.io/trace"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/rpcutil"
)

// ConnectGraph returns a description of which cluster peers and ipfs
// daemons are connected to each other
func (c *Cluster) ConnectGraph() (api.ConnectGraph, error) {
	ctx, span := trace.StartSpan(c.ctx, "cluster/ConnectGraph")
	defer span.End()

	cg := api.ConnectGraph{
		ClusterID:     c.host.ID(),
		IPFSLinks:     make(map[string][]peer.ID),
		ClusterLinks:  make(map[string][]peer.ID),
		ClustertoIPFS: make(map[string]peer.ID),
	}
	members, err := c.consensus.Peers(ctx)
	if err != nil {
		return cg, err
	}

	peers := make([][]*api.ID, len(members), len(members))

	ctxs, cancels := rpcutil.CtxsWithCancel(ctx, len(members))
	defer rpcutil.MultiCancel(cancels)

	errs := c.rpcClient.MultiCall(
		ctxs,
		members,
		"Cluster",
		"Peers",
		struct{}{},
		rpcutil.CopyIDSliceToIfaces(peers),
	)

	for i, err := range errs {
		p := peer.IDB58Encode(members[i])
		cg.ClusterLinks[p] = make([]peer.ID, 0)
		if err != nil { // Only setting cluster connections when no error occurs
			logger.Debugf("RPC error reaching cluster peer %s: %s", p, err.Error())
			continue
		}

		selfConnection, pID := c.recordClusterLinks(&cg, p, peers[i])

		// IPFS connections
		if !selfConnection {
			logger.Warningf("cluster peer %s not its own peer.  No ipfs info ", p)
			continue
		}
		c.recordIPFSLinks(&cg, pID)
	}

	return cg, nil
}

func (c *Cluster) recordClusterLinks(cg *api.ConnectGraph, p string, peers []*api.ID) (bool, *api.ID) {
	selfConnection := false
	var pID *api.ID
	for _, id := range peers {
		if id.Error != "" {
			logger.Debugf("Peer %s errored connecting to its peer %s", p, id.ID.Pretty())
			continue
		}
		if peer.IDB58Encode(id.ID) == p {
			selfConnection = true
			pID = id
		} else {
			cg.ClusterLinks[p] = append(cg.ClusterLinks[p], id.ID)
		}
	}
	return selfConnection, pID
}

func (c *Cluster) recordIPFSLinks(cg *api.ConnectGraph, pID *api.ID) {
	ipfsID := pID.IPFS.ID
	if pID.IPFS.Error != "" { // Only setting ipfs connections when no error occurs
		logger.Warningf("ipfs id: %s has error: %s. Skipping swarm connections", ipfsID.Pretty(), pID.IPFS.Error)
		return
	}

	pid := peer.IDB58Encode(pID.ID)
	ipfsPid := peer.IDB58Encode(ipfsID)

	if _, ok := cg.IPFSLinks[pid]; ok {
		logger.Warningf("ipfs id: %s already recorded, one ipfs daemon in use by multiple cluster peers", ipfsID.Pretty())
	}
	cg.ClustertoIPFS[pid] = ipfsID
	cg.IPFSLinks[ipfsPid] = make([]peer.ID, 0)
	var swarmPeers []peer.ID
	err := c.rpcClient.Call(
		pID.ID,
		"Cluster",
		"IPFSSwarmPeers",
		struct{}{},
		&swarmPeers,
	)
	if err != nil {
		return
	}
	cg.IPFSLinks[ipfsPid] = swarmPeers
}
