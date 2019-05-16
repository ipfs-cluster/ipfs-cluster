package ipfscluster

import (
	"context"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/version"
	ocgorpc "github.com/lanzafame/go-libp2p-ocgorpc"

	cid "github.com/ipfs/go-cid"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p-peer"
	"go.opencensus.io/trace"
)

// newRPCServer returns a new RPC Server for Cluster.
func newRPCServer(c *Cluster) (*rpc.Server, error) {
	var s *rpc.Server
	if c.config.Tracing {
		s = rpc.NewServer(
			c.host,
			version.RPCProtocol,
			rpc.WithServerStatsHandler(&ocgorpc.ServerHandler{}),
		)
	} else {
		s = rpc.NewServer(c.host, version.RPCProtocol)
	}

	err := s.RegisterName("Cluster", &ClusterRPCAPI{c})
	if err != nil {
		return nil, err
	}
	err = s.RegisterName("PinTracker", &PinTrackerRPCAPI{c.tracker})
	if err != nil {
		return nil, err
	}
	err = s.RegisterName("IPFSConnector", &IPFSConnectorRPCAPI{c.ipfs})
	if err != nil {
		return nil, err
	}
	err = s.RegisterName("Consensus", &ConsensusRPCAPI{c.consensus})
	if err != nil {
		return nil, err
	}
	err = s.RegisterName("PeerMonitor", &PeerMonitorRPCAPI{c.monitor})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// ClusterRPCAPI is a go-libp2p-gorpc service which provides the internal peer
// API for the main cluster component.
type ClusterRPCAPI struct {
	c *Cluster
}

// PinTrackerRPCAPI is a go-libp2p-gorpc service which provides the internal
// peer API for the PinTracker component.
type PinTrackerRPCAPI struct {
	tracker PinTracker
}

// IPFSConnectorRPCAPI is a go-libp2p-gorpc service which provides the
// internal peer API for the IPFSConnector component.
type IPFSConnectorRPCAPI struct {
	ipfs IPFSConnector
}

// ConsensusRPCAPI is a go-libp2p-gorpc service which provides the
// internal peer API for the Consensus component.
type ConsensusRPCAPI struct {
	cons Consensus
}

// PeerMonitorRPCAPI is a go-libp2p-gorpc service which provides the
// internal peer API for the PeerMonitor component.
type PeerMonitorRPCAPI struct {
	mon PeerMonitor
}

/*
   Cluster component methods
*/

// ID runs Cluster.ID()
func (rpcapi *ClusterRPCAPI) ID(ctx context.Context, in struct{}, out *api.ID) error {
	id := rpcapi.c.ID(ctx)
	*out = *id
	return nil
}

// Pin runs Cluster.Pin().
func (rpcapi *ClusterRPCAPI) Pin(ctx context.Context, in *api.Pin, out *struct{}) error {
	return rpcapi.c.Pin(ctx, in)
}

// Unpin runs Cluster.Unpin().
func (rpcapi *ClusterRPCAPI) Unpin(ctx context.Context, in *api.Pin, out *struct{}) error {
	return rpcapi.c.Unpin(ctx, in.Cid)
}

// PinPath resolves path into a cid and runs Cluster.Pin().
func (rpcapi *ClusterRPCAPI) PinPath(ctx context.Context, in *api.PinPath, out *api.Pin) error {
	pin, err := rpcapi.c.PinPath(ctx, in)
	if err != nil {
		return err
	}
	*out = *pin
	return nil
}

// UnpinPath resolves path into a cid and runs Cluster.Unpin().
func (rpcapi *ClusterRPCAPI) UnpinPath(ctx context.Context, in *api.PinPath, out *api.Pin) error {
	pin, err := rpcapi.c.UnpinPath(ctx, in.Path)
	if err != nil {
		return err
	}
	*out = *pin
	return nil
}

// Pins runs Cluster.Pins().
func (rpcapi *ClusterRPCAPI) Pins(ctx context.Context, in struct{}, out *[]*api.Pin) error {
	cidList, err := rpcapi.c.Pins(ctx)
	if err != nil {
		return err
	}
	*out = cidList
	return nil
}

// PinGet runs Cluster.PinGet().
func (rpcapi *ClusterRPCAPI) PinGet(ctx context.Context, in cid.Cid, out *api.Pin) error {
	pin, err := rpcapi.c.PinGet(ctx, in)
	if err != nil {
		return err
	}
	*out = *pin
	return nil
}

// Version runs Cluster.Version().
func (rpcapi *ClusterRPCAPI) Version(ctx context.Context, in struct{}, out *api.Version) error {
	*out = api.Version{
		Version: rpcapi.c.Version(),
	}
	return nil
}

// Peers runs Cluster.Peers().
func (rpcapi *ClusterRPCAPI) Peers(ctx context.Context, in struct{}, out *[]*api.ID) error {
	*out = rpcapi.c.Peers(ctx)
	return nil
}

// PeerAdd runs Cluster.PeerAdd().
func (rpcapi *ClusterRPCAPI) PeerAdd(ctx context.Context, in peer.ID, out *api.ID) error {
	id, err := rpcapi.c.PeerAdd(ctx, in)
	if err != nil {
		return err
	}
	*out = *id
	return nil
}

// ConnectGraph runs Cluster.GetConnectGraph().
func (rpcapi *ClusterRPCAPI) ConnectGraph(ctx context.Context, in struct{}, out *api.ConnectGraph) error {
	graph, err := rpcapi.c.ConnectGraph()
	if err != nil {
		return err
	}
	*out = graph
	return nil
}

// PeerRemove runs Cluster.PeerRm().
func (rpcapi *ClusterRPCAPI) PeerRemove(ctx context.Context, in peer.ID, out *struct{}) error {
	return rpcapi.c.PeerRemove(ctx, in)
}

// Join runs Cluster.Join().
func (rpcapi *ClusterRPCAPI) Join(ctx context.Context, in api.Multiaddr, out *struct{}) error {
	return rpcapi.c.Join(ctx, in.Value())
}

// StatusAll runs Cluster.StatusAll().
func (rpcapi *ClusterRPCAPI) StatusAll(ctx context.Context, in struct{}, out *[]*api.GlobalPinInfo) error {
	pinfos, err := rpcapi.c.StatusAll(ctx)
	if err != nil {
		return err
	}
	*out = pinfos
	return nil
}

// StatusAllLocal runs Cluster.StatusAllLocal().
func (rpcapi *ClusterRPCAPI) StatusAllLocal(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	pinfos := rpcapi.c.StatusAllLocal(ctx)
	*out = pinfos
	return nil
}

// Status runs Cluster.Status().
func (rpcapi *ClusterRPCAPI) Status(ctx context.Context, in cid.Cid, out *api.GlobalPinInfo) error {
	pinfo, err := rpcapi.c.Status(ctx, in)
	if err != nil {
		return err
	}
	*out = *pinfo
	return nil
}

// StatusLocal runs Cluster.StatusLocal().
func (rpcapi *ClusterRPCAPI) StatusLocal(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	pinfo := rpcapi.c.StatusLocal(ctx, in)
	*out = *pinfo
	return nil
}

// SyncAll runs Cluster.SyncAll().
func (rpcapi *ClusterRPCAPI) SyncAll(ctx context.Context, in struct{}, out *[]*api.GlobalPinInfo) error {
	pinfos, err := rpcapi.c.SyncAll(ctx)
	if err != nil {
		return err
	}
	*out = pinfos
	return nil
}

// SyncAllLocal runs Cluster.SyncAllLocal().
func (rpcapi *ClusterRPCAPI) SyncAllLocal(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	pinfos, err := rpcapi.c.SyncAllLocal(ctx)
	if err != nil {
		return err
	}
	*out = pinfos
	return nil
}

// Sync runs Cluster.Sync().
func (rpcapi *ClusterRPCAPI) Sync(ctx context.Context, in cid.Cid, out *api.GlobalPinInfo) error {
	pinfo, err := rpcapi.c.Sync(ctx, in)
	if err != nil {
		return err
	}
	*out = *pinfo
	return nil
}

// SyncLocal runs Cluster.SyncLocal().
func (rpcapi *ClusterRPCAPI) SyncLocal(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	pinfo, err := rpcapi.c.SyncLocal(ctx, in)
	if err != nil {
		return err
	}
	*out = *pinfo
	return nil
}

// RecoverAllLocal runs Cluster.RecoverAllLocal().
func (rpcapi *ClusterRPCAPI) RecoverAllLocal(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	pinfos, err := rpcapi.c.RecoverAllLocal(ctx)
	if err != nil {
		return err
	}
	*out = pinfos
	return nil
}

// Recover runs Cluster.Recover().
func (rpcapi *ClusterRPCAPI) Recover(ctx context.Context, in cid.Cid, out *api.GlobalPinInfo) error {
	pinfo, err := rpcapi.c.Recover(ctx, in)
	if err != nil {
		return err
	}
	*out = *pinfo
	return nil
}

// RecoverLocal runs Cluster.RecoverLocal().
func (rpcapi *ClusterRPCAPI) RecoverLocal(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	pinfo, err := rpcapi.c.RecoverLocal(ctx, in)
	if err != nil {
		return err
	}
	*out = *pinfo
	return nil
}

// BlockAllocate returns allocations for blocks. This is used in the adders.
// It's different from pin allocations when ReplicationFactor < 0.
func (rpcapi *ClusterRPCAPI) BlockAllocate(ctx context.Context, in *api.Pin, out *[]peer.ID) error {
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

// RepoGC performs garbage collection sweep on all peers' repo.
func (rpcapi *ClusterRPCAPI) RepoGC(ctx context.Context, in struct{}, out *[]*api.IPFSRepoGC) error {
	res, err := rpcapi.c.RepoGC(ctx)
	out = &res
	return err
}

// SendInformerMetric runs Cluster.sendInformerMetric().
func (rpcapi *ClusterRPCAPI) SendInformerMetric(ctx context.Context, in struct{}, out *api.Metric) error {
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
func (rpcapi *PinTrackerRPCAPI) Track(ctx context.Context, in *api.Pin, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/Track")
	defer span.End()
	return rpcapi.tracker.Track(ctx, in)
}

// Untrack runs PinTracker.Untrack().
func (rpcapi *PinTrackerRPCAPI) Untrack(ctx context.Context, in *api.Pin, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/Untrack")
	defer span.End()
	return rpcapi.tracker.Untrack(ctx, in.Cid)
}

// StatusAll runs PinTracker.StatusAll().
func (rpcapi *PinTrackerRPCAPI) StatusAll(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/StatusAll")
	defer span.End()
	*out = rpcapi.tracker.StatusAll(ctx)
	return nil
}

// Status runs PinTracker.Status().
func (rpcapi *PinTrackerRPCAPI) Status(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/Status")
	defer span.End()
	pinfo := rpcapi.tracker.Status(ctx, in)
	*out = *pinfo
	return nil
}

// RecoverAll runs PinTracker.RecoverAll().f
func (rpcapi *PinTrackerRPCAPI) RecoverAll(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/RecoverAll")
	defer span.End()
	pinfos, err := rpcapi.tracker.RecoverAll(ctx)
	if err != nil {
		return err
	}
	*out = pinfos
	return nil
}

// Recover runs PinTracker.Recover().
func (rpcapi *PinTrackerRPCAPI) Recover(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/Recover")
	defer span.End()
	pinfo, err := rpcapi.tracker.Recover(ctx, in)
	*out = *pinfo
	return err
}

/*
   IPFS Connector component methods
*/

// Pin runs IPFSConnector.Pin().
func (rpcapi *IPFSConnectorRPCAPI) Pin(ctx context.Context, in *api.Pin, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/ipfsconn/IPFSPin")
	defer span.End()
	return rpcapi.ipfs.Pin(ctx, in.Cid, in.MaxDepth)
}

// Unpin runs IPFSConnector.Unpin().
func (rpcapi *IPFSConnectorRPCAPI) Unpin(ctx context.Context, in *api.Pin, out *struct{}) error {
	return rpcapi.ipfs.Unpin(ctx, in.Cid)
}

// PinLsCid runs IPFSConnector.PinLsCid().
func (rpcapi *IPFSConnectorRPCAPI) PinLsCid(ctx context.Context, in cid.Cid, out *api.IPFSPinStatus) error {
	b, err := rpcapi.ipfs.PinLsCid(ctx, in)
	if err != nil {
		return err
	}
	*out = b
	return nil
}

// PinLs runs IPFSConnector.PinLs().
func (rpcapi *IPFSConnectorRPCAPI) PinLs(ctx context.Context, in string, out *map[string]api.IPFSPinStatus) error {
	m, err := rpcapi.ipfs.PinLs(ctx, in)
	if err != nil {
		return err
	}
	*out = m
	return nil
}

// ConnectSwarms runs IPFSConnector.ConnectSwarms().
func (rpcapi *IPFSConnectorRPCAPI) ConnectSwarms(ctx context.Context, in struct{}, out *struct{}) error {
	err := rpcapi.ipfs.ConnectSwarms(ctx)
	return err
}

// ConfigKey runs IPFSConnector.ConfigKey().
func (rpcapi *IPFSConnectorRPCAPI) ConfigKey(ctx context.Context, in string, out *interface{}) error {
	res, err := rpcapi.ipfs.ConfigKey(in)
	if err != nil {
		return err
	}
	*out = res
	return nil
}

// RepoStat runs IPFSConnector.RepoStat().
func (rpcapi *IPFSConnectorRPCAPI) RepoStat(ctx context.Context, in struct{}, out *api.IPFSRepoStat) error {
	res, err := rpcapi.ipfs.RepoStat(ctx)
	if err != nil {
		return err
	}
	*out = *res
	return err
}

// RepoGC performs garbage a collection sweep on the repo.
func (rpcapi *IPFSConnectorRPCAPI) RepoGC(ctx context.Context, in struct{}, out *[]*api.IPFSRepoGC) error {
	res, err := rpcapi.ipfs.RepoGC(ctx)
	out = &res
	return err
}

// SwarmPeers runs IPFSConnector.SwarmPeers().
func (rpcapi *IPFSConnectorRPCAPI) SwarmPeers(ctx context.Context, in struct{}, out *[]peer.ID) error {
	res, err := rpcapi.ipfs.SwarmPeers(ctx)
	if err != nil {
		return err
	}
	*out = res
	return nil
}

// BlockPut runs IPFSConnector.BlockPut().
func (rpcapi *IPFSConnectorRPCAPI) BlockPut(ctx context.Context, in *api.NodeWithMeta, out *struct{}) error {
	return rpcapi.ipfs.BlockPut(ctx, in)
}

// BlockGet runs IPFSConnector.BlockGet().
func (rpcapi *IPFSConnectorRPCAPI) BlockGet(ctx context.Context, in cid.Cid, out *[]byte) error {
	res, err := rpcapi.ipfs.BlockGet(ctx, in)
	if err != nil {
		return err
	}
	*out = res
	return nil
}

// Resolve runs IPFSConnector.Resolve().
func (rpcapi *IPFSConnectorRPCAPI) Resolve(ctx context.Context, in string, out *cid.Cid) error {
	c, err := rpcapi.ipfs.Resolve(ctx, in)
	if err != nil {
		return err
	}
	*out = c
	return nil
}

/*
   Consensus component methods
*/

// LogPin runs Consensus.LogPin().
func (rpcapi *ConsensusRPCAPI) LogPin(ctx context.Context, in *api.Pin, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/consensus/LogPin")
	defer span.End()
	return rpcapi.cons.LogPin(ctx, in)
}

// LogUnpin runs Consensus.LogUnpin().
func (rpcapi *ConsensusRPCAPI) LogUnpin(ctx context.Context, in *api.Pin, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/consensus/LogUnpin")
	defer span.End()
	return rpcapi.cons.LogUnpin(ctx, in)
}

// AddPeer runs Consensus.AddPeer().
func (rpcapi *ConsensusRPCAPI) AddPeer(ctx context.Context, in peer.ID, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/consensus/AddPeer")
	defer span.End()
	return rpcapi.cons.AddPeer(ctx, in)
}

// RmPeer runs Consensus.RmPeer().
func (rpcapi *ConsensusRPCAPI) RmPeer(ctx context.Context, in peer.ID, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/consensus/RmPeer")
	defer span.End()
	return rpcapi.cons.RmPeer(ctx, in)
}

// Peers runs Consensus.Peers().
func (rpcapi *ConsensusRPCAPI) Peers(ctx context.Context, in struct{}, out *[]peer.ID) error {
	peers, err := rpcapi.cons.Peers(ctx)
	if err != nil {
		return err
	}
	*out = peers
	return nil
}

/*
   PeerMonitor
*/

// LogMetric runs PeerMonitor.LogMetric().
func (rpcapi *PeerMonitorRPCAPI) LogMetric(ctx context.Context, in *api.Metric, out *struct{}) error {
	return rpcapi.mon.LogMetric(ctx, in)
}

// LatestMetrics runs PeerMonitor.LatestMetrics().
func (rpcapi *PeerMonitorRPCAPI) LatestMetrics(ctx context.Context, in string, out *[]*api.Metric) error {
	*out = rpcapi.mon.LatestMetrics(ctx, in)
	return nil
}
