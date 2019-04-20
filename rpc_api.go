package ipfscluster

import (
	"context"

	peer "github.com/libp2p/go-libp2p-peer"

	"go.opencensus.io/trace"

	cid "github.com/ipfs/go-cid"

	"github.com/ipfs/ipfs-cluster/api"
)

// RPCAPI is a go-libp2p-gorpc service which provides the internal ipfs-cluster
// API, which enables components and cluster peers to communicate and
// request actions from each other.
//
// The RPC API methods are usually redirects to the actual methods in
// the different components of ipfs-cluster, with very little added logic.
// Refer to documentation on those methods for details on their behaviour.
type RPCAPI struct {
	c *Cluster
}

/*
   Cluster components methods
*/

// ID runs Cluster.ID()
func (rpcapi *RPCAPI) ID(ctx context.Context, in struct{}, out *api.ID) error {
	id := rpcapi.c.ID(ctx)
	*out = *id
	return nil
}

// Pin runs Cluster.Pin().
func (rpcapi *RPCAPI) Pin(ctx context.Context, in *api.Pin, out *struct{}) error {
	return rpcapi.c.Pin(ctx, in)
}

// Unpin runs Cluster.Unpin().
func (rpcapi *RPCAPI) Unpin(ctx context.Context, in *api.Pin, out *struct{}) error {
	return rpcapi.c.Unpin(ctx, in.Cid)
}

// PinPath resolves path into a cid and runs Cluster.Pin().
func (rpcapi *RPCAPI) PinPath(ctx context.Context, in *api.PinPath, out *api.Pin) error {
	pin, err := rpcapi.c.PinPath(ctx, in)
	if err != nil {
		return err
	}
	*out = *pin
	return nil
}

// UnpinPath resolves path into a cid and runs Cluster.Unpin().
func (rpcapi *RPCAPI) UnpinPath(ctx context.Context, in *api.PinPath, out *api.Pin) error {
	pin, err := rpcapi.c.UnpinPath(ctx, in.Path)
	if err != nil {
		return err
	}
	*out = *pin
	return nil
}

// Pins runs Cluster.Pins().
func (rpcapi *RPCAPI) Pins(ctx context.Context, in struct{}, out *[]*api.Pin) error {
	cidList := rpcapi.c.Pins(ctx)
	*out = cidList
	return nil
}

// PinGet runs Cluster.PinGet().
func (rpcapi *RPCAPI) PinGet(ctx context.Context, in cid.Cid, out *api.Pin) error {
	pin, err := rpcapi.c.PinGet(ctx, in)
	if err != nil {
		return err
	}
	*out = *pin
	return nil
}

// Version runs Cluster.Version().
func (rpcapi *RPCAPI) Version(ctx context.Context, in struct{}, out *api.Version) error {
	*out = api.Version{
		Version: rpcapi.c.Version(),
	}
	return nil
}

// Peers runs Cluster.Peers().
func (rpcapi *RPCAPI) Peers(ctx context.Context, in struct{}, out *[]*api.ID) error {
	*out = rpcapi.c.Peers(ctx)
	return nil
}

// PeerAdd runs Cluster.PeerAdd().
func (rpcapi *RPCAPI) PeerAdd(ctx context.Context, in peer.ID, out *api.ID) error {
	id, err := rpcapi.c.PeerAdd(ctx, in)
	if err != nil {
		return err
	}
	*out = *id
	return nil
}

// ConnectGraph runs Cluster.GetConnectGraph().
func (rpcapi *RPCAPI) ConnectGraph(ctx context.Context, in struct{}, out *api.ConnectGraph) error {
	graph, err := rpcapi.c.ConnectGraph()
	if err != nil {
		return err
	}
	*out = graph
	return nil
}

// PeerRemove runs Cluster.PeerRm().
func (rpcapi *RPCAPI) PeerRemove(ctx context.Context, in peer.ID, out *struct{}) error {
	return rpcapi.c.PeerRemove(ctx, in)
}

// Join runs Cluster.Join().
func (rpcapi *RPCAPI) Join(ctx context.Context, in api.Multiaddr, out *struct{}) error {
	return rpcapi.c.Join(ctx, in.Value())
}

// StatusAll runs Cluster.StatusAll().
func (rpcapi *RPCAPI) StatusAll(ctx context.Context, in struct{}, out *[]*api.GlobalPinInfo) error {
	pinfos, err := rpcapi.c.StatusAll(ctx)
	if err != nil {
		return err
	}
	*out = pinfos
	return nil
}

// StatusAllLocal runs Cluster.StatusAllLocal().
func (rpcapi *RPCAPI) StatusAllLocal(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	pinfos := rpcapi.c.StatusAllLocal(ctx)
	*out = pinfos
	return nil
}

// Status runs Cluster.Status().
func (rpcapi *RPCAPI) Status(ctx context.Context, in cid.Cid, out *api.GlobalPinInfo) error {
	pinfo, err := rpcapi.c.Status(ctx, in)
	if err != nil {
		return err
	}
	*out = *pinfo
	return nil
}

// StatusLocal runs Cluster.StatusLocal().
func (rpcapi *RPCAPI) StatusLocal(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	pinfo := rpcapi.c.StatusLocal(ctx, in)
	*out = *pinfo
	return nil
}

// SyncAll runs Cluster.SyncAll().
func (rpcapi *RPCAPI) SyncAll(ctx context.Context, in struct{}, out *[]*api.GlobalPinInfo) error {
	pinfos, err := rpcapi.c.SyncAll(ctx)
	if err != nil {
		return err
	}
	*out = pinfos
	return nil
}

// SyncAllLocal runs Cluster.SyncAllLocal().
func (rpcapi *RPCAPI) SyncAllLocal(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	pinfos, err := rpcapi.c.SyncAllLocal(ctx)
	if err != nil {
		return err
	}
	*out = pinfos
	return nil
}

// Sync runs Cluster.Sync().
func (rpcapi *RPCAPI) Sync(ctx context.Context, in cid.Cid, out *api.GlobalPinInfo) error {
	pinfo, err := rpcapi.c.Sync(ctx, in)
	if err != nil {
		return err
	}
	*out = *pinfo
	return nil
}

// SyncLocal runs Cluster.SyncLocal().
func (rpcapi *RPCAPI) SyncLocal(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	pinfo, err := rpcapi.c.SyncLocal(ctx, in)
	if err != nil {
		return err
	}
	*out = *pinfo
	return nil
}

// RecoverAllLocal runs Cluster.RecoverAllLocal().
func (rpcapi *RPCAPI) RecoverAllLocal(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	pinfos, err := rpcapi.c.RecoverAllLocal(ctx)
	if err != nil {
		return err
	}
	*out = pinfos
	return nil
}

// Recover runs Cluster.Recover().
func (rpcapi *RPCAPI) Recover(ctx context.Context, in cid.Cid, out *api.GlobalPinInfo) error {
	pinfo, err := rpcapi.c.Recover(ctx, in)
	if err != nil {
		return err
	}
	*out = *pinfo
	return nil
}

// RecoverLocal runs Cluster.RecoverLocal().
func (rpcapi *RPCAPI) RecoverLocal(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	pinfo, err := rpcapi.c.RecoverLocal(ctx, in)
	if err != nil {
		return err
	}
	*out = *pinfo
	return nil
}

// BlockAllocate returns allocations for blocks. This is used in the adders.
// It's different from pin allocations when ReplicationFactor < 0.
func (rpcapi *RPCAPI) BlockAllocate(ctx context.Context, in *api.Pin, out *[]peer.ID) error {
	err := rpcapi.c.setupPin(ctx, in)
	if err != nil {
		return err
	}

	// Return the current peer list.
	if in.ReplicationFactorMin < 0 {
		// Returned metrics are Valid and belong to current
		// Cluster peers.
		metrics := rpcapi.c.monitor.LatestMetrics(ctx, pingMetricName)
		peers := make([]peer.ID, len(metrics), len(metrics))
		for i, m := range metrics {
			peers[i] = m.Peer
		}

		*out = peers
		return nil
	}

	allocs, err := rpcapi.c.allocate(
		ctx,
		in.Cid,
		in.ReplicationFactorMin,
		in.ReplicationFactorMax,
		[]peer.ID{}, // blacklist
		[]peer.ID{}, // prio list
	)

	if err != nil {
		return err
	}

	*out = allocs
	return nil
}

// SendInformerMetric runs Cluster.sendInformerMetric().
func (rpcapi *RPCAPI) SendInformerMetric(ctx context.Context, in struct{}, out *api.Metric) error {
	m, err := rpcapi.c.sendInformerMetric(ctx)
	if err != nil {
		return err
	}
	*out = *m
	return nil
}

/*
   Tracker component methods
*/

// Track runs PinTracker.Track().
func (rpcapi *RPCAPI) Track(ctx context.Context, in *api.Pin, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/Track")
	defer span.End()
	return rpcapi.c.tracker.Track(ctx, in)
}

// Untrack runs PinTracker.Untrack().
func (rpcapi *RPCAPI) Untrack(ctx context.Context, in *api.Pin, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/Untrack")
	defer span.End()
	return rpcapi.c.tracker.Untrack(ctx, in.Cid)
}

// TrackerStatusAll runs PinTracker.StatusAll().
func (rpcapi *RPCAPI) TrackerStatusAll(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/StatusAll")
	defer span.End()
	*out = rpcapi.c.tracker.StatusAll(ctx)
	return nil
}

// TrackerStatus runs PinTracker.Status().
func (rpcapi *RPCAPI) TrackerStatus(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/Status")
	defer span.End()
	pinfo := rpcapi.c.tracker.Status(ctx, in)
	*out = *pinfo
	return nil
}

// TrackerRecoverAll runs PinTracker.RecoverAll().f
func (rpcapi *RPCAPI) TrackerRecoverAll(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/RecoverAll")
	defer span.End()
	pinfos, err := rpcapi.c.tracker.RecoverAll(ctx)
	if err != nil {
		return err
	}
	*out = pinfos
	return nil
}

// TrackerRecover runs PinTracker.Recover().
func (rpcapi *RPCAPI) TrackerRecover(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/Recover")
	defer span.End()
	pinfo, err := rpcapi.c.tracker.Recover(ctx, in)
	*out = *pinfo
	return err
}

/*
   IPFS Connector component methods
*/

// IPFSPin runs IPFSConnector.Pin().
func (rpcapi *RPCAPI) IPFSPin(ctx context.Context, in *api.Pin, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/ipfsconn/IPFSPin")
	defer span.End()
	return rpcapi.c.ipfs.Pin(ctx, in.Cid, in.MaxDepth)
}

// IPFSUnpin runs IPFSConnector.Unpin().
func (rpcapi *RPCAPI) IPFSUnpin(ctx context.Context, in *api.Pin, out *struct{}) error {
	return rpcapi.c.ipfs.Unpin(ctx, in.Cid)
}

// IPFSPinLsCid runs IPFSConnector.PinLsCid().
func (rpcapi *RPCAPI) IPFSPinLsCid(ctx context.Context, in cid.Cid, out *api.IPFSPinStatus) error {
	b, err := rpcapi.c.ipfs.PinLsCid(ctx, in)
	if err != nil {
		return err
	}
	*out = b
	return nil
}

// IPFSPinLs runs IPFSConnector.PinLs().
func (rpcapi *RPCAPI) IPFSPinLs(ctx context.Context, in string, out *map[string]api.IPFSPinStatus) error {
	m, err := rpcapi.c.ipfs.PinLs(ctx, in)
	if err != nil {
		return err
	}
	*out = m
	return nil
}

// IPFSConnectSwarms runs IPFSConnector.ConnectSwarms().
func (rpcapi *RPCAPI) IPFSConnectSwarms(ctx context.Context, in struct{}, out *struct{}) error {
	err := rpcapi.c.ipfs.ConnectSwarms(ctx)
	return err
}

// IPFSConfigKey runs IPFSConnector.ConfigKey().
func (rpcapi *RPCAPI) IPFSConfigKey(ctx context.Context, in string, out *interface{}) error {
	res, err := rpcapi.c.ipfs.ConfigKey(in)
	if err != nil {
		return err
	}
	*out = res
	return nil
}

// IPFSRepoStat runs IPFSConnector.RepoStat().
func (rpcapi *RPCAPI) IPFSRepoStat(ctx context.Context, in struct{}, out *api.IPFSRepoStat) error {
	res, err := rpcapi.c.ipfs.RepoStat(ctx)
	if err != nil {
		return err
	}
	*out = *res
	return err
}

// IPFSRepoGC runs IPFSConnector.RepoGC()
func (rpcapi *RPCAPI) IPFSRepoGC(ctx context.Context, in struct{}, out *api.IPFSRepoGC) error {
	//TODO: Need some clarity on this.
	res, err := rpcapi.c.ipfs.RepoGC(ctx)
	if err != nil {
		return err
	}
	*out = *res
	return err
}

// IPFSSwarmPeers runs IPFSConnector.SwarmPeers().
func (rpcapi *RPCAPI) IPFSSwarmPeers(ctx context.Context, in struct{}, out *[]peer.ID) error {
	res, err := rpcapi.c.ipfs.SwarmPeers(ctx)
	if err != nil {
		return err
	}
	*out = res
	return nil
}

// IPFSBlockPut runs IPFSConnector.BlockPut().
func (rpcapi *RPCAPI) IPFSBlockPut(ctx context.Context, in *api.NodeWithMeta, out *struct{}) error {
	return rpcapi.c.ipfs.BlockPut(ctx, in)
}

// IPFSBlockGet runs IPFSConnector.BlockGet().
func (rpcapi *RPCAPI) IPFSBlockGet(ctx context.Context, in cid.Cid, out *[]byte) error {
	res, err := rpcapi.c.ipfs.BlockGet(ctx, in)
	if err != nil {
		return err
	}
	*out = res
	return nil
}

/*
   Consensus component methods
*/

// ConsensusLogPin runs Consensus.LogPin().
func (rpcapi *RPCAPI) ConsensusLogPin(ctx context.Context, in *api.Pin, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/consensus/LogPin")
	defer span.End()
	return rpcapi.c.consensus.LogPin(ctx, in)
}

// ConsensusLogUnpin runs Consensus.LogUnpin().
func (rpcapi *RPCAPI) ConsensusLogUnpin(ctx context.Context, in *api.Pin, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/consensus/LogUnpin")
	defer span.End()
	return rpcapi.c.consensus.LogUnpin(ctx, in)
}

// ConsensusAddPeer runs Consensus.AddPeer().
func (rpcapi *RPCAPI) ConsensusAddPeer(ctx context.Context, in peer.ID, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/consensus/AddPeer")
	defer span.End()
	return rpcapi.c.consensus.AddPeer(ctx, in)
}

// ConsensusRmPeer runs Consensus.RmPeer().
func (rpcapi *RPCAPI) ConsensusRmPeer(ctx context.Context, in peer.ID, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/consensus/RmPeer")
	defer span.End()
	return rpcapi.c.consensus.RmPeer(ctx, in)
}

// ConsensusPeers runs Consensus.Peers().
func (rpcapi *RPCAPI) ConsensusPeers(ctx context.Context, in struct{}, out *[]peer.ID) error {
	peers, err := rpcapi.c.consensus.Peers(ctx)
	if err != nil {
		return err
	}
	*out = peers
	return nil
}

/*
   PeerMonitor
*/

// PeerMonitorLogMetric runs PeerMonitor.LogMetric().
func (rpcapi *RPCAPI) PeerMonitorLogMetric(ctx context.Context, in *api.Metric, out *struct{}) error {
	return rpcapi.c.monitor.LogMetric(ctx, in)
}

// PeerMonitorLatestMetrics runs PeerMonitor.LatestMetrics().
func (rpcapi *RPCAPI) PeerMonitorLatestMetrics(ctx context.Context, in string, out *[]*api.Metric) error {
	*out = rpcapi.c.monitor.LatestMetrics(ctx, in)
	return nil
}
