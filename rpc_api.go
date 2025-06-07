package ipfscluster

import (
	"context"
	"errors"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/state"
	"github.com/ipfs-cluster/ipfs-cluster/version"

	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p/core/peer"

	ocgorpc "github.com/lanzafame/go-libp2p-ocgorpc"
	"go.opencensus.io/trace"
)

// RPC endpoint types w.r.t. trust level
const (
	// RPCClosed endpoints can only be called by the local cluster peer
	// on itself.
	RPCClosed RPCEndpointType = iota
	// RPCTrusted endpoints can be called by "trusted" peers.
	// It depends which peers are considered trusted. For example,
	// in "raft" mode, Cluster will all peers as "trusted". In "crdt" mode,
	// trusted peers are those specified in the configuration.
	RPCTrusted
	// RPCOpen endpoints can be called by any peer in the Cluster swarm.
	RPCOpen
)

// RPCEndpointType controls how access is granted to an RPC endpoint
type RPCEndpointType int

const rpcStreamBufferSize = 1024

// A trick to find where something is used (i.e. Cluster.Pin):
// grep -R -B 3 '"Pin"' | grep -C 1 '"Cluster"'.
// This does not cover globalPinInfo*(...) broadcasts nor redirects to leader
// in Raft.

// newRPCServer returns a new RPC Server for Cluster.
func newRPCServer(c *Cluster) (*rpc.Server, error) {
	var s *rpc.Server

	authF := func(pid peer.ID, svc, method string) bool {
		endpointType, ok := c.config.RPCPolicy[svc+"."+method]
		if !ok {
			return false
		}

		switch endpointType {
		case RPCTrusted:
			return c.consensus.IsTrustedPeer(c.ctx, pid)
		case RPCOpen:
			return true
		default:
			return false
		}
	}

	if c.config.Tracing {
		s = rpc.NewServer(
			c.host,
			version.RPCProtocol,
			rpc.WithServerStatsHandler(&ocgorpc.ServerHandler{}),
			rpc.WithAuthorizeFunc(authF),
			rpc.WithStreamBufferSize(rpcStreamBufferSize),
		)
	} else {
		s = rpc.NewServer(c.host, version.RPCProtocol, rpc.WithAuthorizeFunc(authF))
	}

	cl := &ClusterRPCAPI{c}
	err := s.RegisterName(RPCServiceID(cl), cl)
	if err != nil {
		return nil, err
	}
	pt := &PinTrackerRPCAPI{c.tracker}
	err = s.RegisterName(RPCServiceID(pt), pt)
	if err != nil {
		return nil, err
	}
	ic := &IPFSConnectorRPCAPI{c.ipfs}
	err = s.RegisterName(RPCServiceID(ic), ic)
	if err != nil {
		return nil, err
	}
	cons := &ConsensusRPCAPI{c.consensus}
	err = s.RegisterName(RPCServiceID(cons), cons)
	if err != nil {
		return nil, err
	}
	pm := &PeerMonitorRPCAPI{mon: c.monitor, pid: c.id}
	err = s.RegisterName(RPCServiceID(pm), pm)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// RPCServiceID returns the Service ID for the given RPCAPI object.
func RPCServiceID(rpcSvc interface{}) string {
	switch rpcSvc.(type) {
	case *ClusterRPCAPI:
		return "Cluster"
	case *PinTrackerRPCAPI:
		return "PinTracker"
	case *IPFSConnectorRPCAPI:
		return "IPFSConnector"
	case *ConsensusRPCAPI:
		return "Consensus"
	case *PeerMonitorRPCAPI:
		return "PeerMonitor"
	default:
		return ""
	}
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
	pid peer.ID
}

/*
   Cluster component methods
*/

// ID runs Cluster.ID()
func (rpcapi *ClusterRPCAPI) ID(ctx context.Context, in struct{}, out *api.ID) error {
	id := rpcapi.c.ID(ctx)
	*out = id
	return nil
}

// IDStream runs Cluster.ID() but in streaming form.
func (rpcapi *ClusterRPCAPI) IDStream(ctx context.Context, in <-chan struct{}, out chan<- api.ID) error {
	defer close(out)
	id := rpcapi.c.ID(ctx)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- id:
	}
	return nil
}

// Pin runs Cluster.pin().
func (rpcapi *ClusterRPCAPI) Pin(ctx context.Context, in api.Pin, out *api.Pin) error {
	// we do not call the Pin method directly since that method does not
	// allow to pin other than regular DataType pins. The adder will
	// however send Meta, Shard and ClusterDAG pins.
	pin, _, err := rpcapi.c.pin(ctx, in, []peer.ID{})
	if err != nil {
		return err
	}
	*out = pin
	return nil
}

// Unpin runs Cluster.Unpin().
func (rpcapi *ClusterRPCAPI) Unpin(ctx context.Context, in api.Pin, out *api.Pin) error {
	pin, err := rpcapi.c.Unpin(ctx, in.Cid)
	if err != nil {
		return err
	}
	*out = pin
	return nil
}

// PinPath resolves path into a cid and runs Cluster.Pin().
func (rpcapi *ClusterRPCAPI) PinPath(ctx context.Context, in api.PinPath, out *api.Pin) error {
	pin, err := rpcapi.c.PinPath(ctx, in.Path, in.PinOptions)
	if err != nil {
		return err
	}
	*out = pin
	return nil
}

// UnpinPath resolves path into a cid and runs Cluster.Unpin().
func (rpcapi *ClusterRPCAPI) UnpinPath(ctx context.Context, in api.PinPath, out *api.Pin) error {
	pin, err := rpcapi.c.UnpinPath(ctx, in.Path)
	if err != nil {
		return err
	}
	*out = pin
	return nil
}

// Pins runs Cluster.Pins().
func (rpcapi *ClusterRPCAPI) Pins(ctx context.Context, in <-chan struct{}, out chan<- api.Pin) error {
	return rpcapi.c.Pins(ctx, out)
}

// PinGet runs Cluster.PinGet().
func (rpcapi *ClusterRPCAPI) PinGet(ctx context.Context, in api.Cid, out *api.Pin) error {
	pin, err := rpcapi.c.PinGet(ctx, in)
	if err != nil {
		return err
	}
	*out = pin
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
func (rpcapi *ClusterRPCAPI) Peers(ctx context.Context, in <-chan struct{}, out chan<- api.ID) error {
	rpcapi.c.Peers(ctx, out)
	return nil
}

// PeersWithFilter runs Cluster.peersWithFilter().
func (rpcapi *ClusterRPCAPI) PeersWithFilter(ctx context.Context, in <-chan []peer.ID, out chan<- api.ID) error {
	peers := <-in
	rpcapi.c.peersWithFilter(ctx, peers, out)
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
func (rpcapi *ClusterRPCAPI) StatusAll(ctx context.Context, in <-chan api.TrackerStatus, out chan<- api.GlobalPinInfo) error {
	filter := <-in
	return rpcapi.c.StatusAll(ctx, filter, out)
}

// StatusAllLocal runs Cluster.StatusAllLocal().
func (rpcapi *ClusterRPCAPI) StatusAllLocal(ctx context.Context, in <-chan api.TrackerStatus, out chan<- api.PinInfo) error {
	filter := <-in
	return rpcapi.c.StatusAllLocal(ctx, filter, out)
}

// Status runs Cluster.Status().
func (rpcapi *ClusterRPCAPI) Status(ctx context.Context, in api.Cid, out *api.GlobalPinInfo) error {
	pinfo, err := rpcapi.c.Status(ctx, in)
	if err != nil {
		return err
	}
	*out = pinfo
	return nil
}

// StatusLocal runs Cluster.StatusLocal().
func (rpcapi *ClusterRPCAPI) StatusLocal(ctx context.Context, in api.Cid, out *api.PinInfo) error {
	pinfo := rpcapi.c.StatusLocal(ctx, in)
	*out = pinfo
	return nil
}

// RecoverAll runs Cluster.RecoverAll().
func (rpcapi *ClusterRPCAPI) RecoverAll(ctx context.Context, in <-chan struct{}, out chan<- api.GlobalPinInfo) error {
	return rpcapi.c.RecoverAll(ctx, out)
}

// RecoverAllLocal runs Cluster.RecoverAllLocal().
func (rpcapi *ClusterRPCAPI) RecoverAllLocal(ctx context.Context, in <-chan struct{}, out chan<- api.PinInfo) error {
	return rpcapi.c.RecoverAllLocal(ctx, out)
}

// Recover runs Cluster.Recover().
func (rpcapi *ClusterRPCAPI) Recover(ctx context.Context, in api.Cid, out *api.GlobalPinInfo) error {
	pinfo, err := rpcapi.c.Recover(ctx, in)
	if err != nil {
		return err
	}
	*out = pinfo
	return nil
}

// RecoverLocal runs Cluster.RecoverLocal().
func (rpcapi *ClusterRPCAPI) RecoverLocal(ctx context.Context, in api.Cid, out *api.PinInfo) error {
	pinfo, err := rpcapi.c.RecoverLocal(ctx, in)
	if err != nil {
		return err
	}
	*out = pinfo
	return nil
}

// BlockAllocate returns allocations for blocks. This is used in the adders.
// It's different from pin allocations when ReplicationFactor < 0.
func (rpcapi *ClusterRPCAPI) BlockAllocate(ctx context.Context, in api.Pin, out *[]peer.ID) error {
	if rpcapi.c.config.FollowerMode {
		return errFollowerMode
	}

	// Allocating for a existing pin. Usually the adder calls this with
	// cid.Undef.
	existing, err := rpcapi.c.PinGet(ctx, in.Cid)
	if err != nil && err != state.ErrNotFound {
		return err
	}

	in, err = rpcapi.c.setupPin(ctx, in, existing)
	if err != nil {
		return err
	}

	// Return the current peer list.
	if in.ReplicationFactorMin < 0 {
		// Returned metrics are Valid and belong to current
		// Cluster peers.
		metrics := rpcapi.c.monitor.LatestMetrics(ctx, pingMetricName)
		peers := make([]peer.ID, len(metrics))
		for i, m := range metrics {
			peers[i] = m.Peer
		}

		*out = peers
		return nil
	}

	allocs, err := rpcapi.c.allocate(
		ctx,
		in.Cid,
		existing,
		in.ReplicationFactorMin,
		in.ReplicationFactorMax,
		[]peer.ID{},        // blacklist
		in.UserAllocations, // prio list
	)

	if err != nil {
		return err
	}

	*out = allocs
	return nil
}

// RepoGC performs garbage collection sweep on all peers' repos.
func (rpcapi *ClusterRPCAPI) RepoGC(ctx context.Context, in struct{}, out *api.GlobalRepoGC) error {
	res, err := rpcapi.c.RepoGC(ctx)
	if err != nil {
		return err
	}
	*out = res
	return nil
}

// RepoGCLocal performs garbage collection sweep only on the local peer's IPFS daemon.
func (rpcapi *ClusterRPCAPI) RepoGCLocal(ctx context.Context, in struct{}, out *api.RepoGC) error {
	res, err := rpcapi.c.RepoGCLocal(ctx)
	if err != nil {
		return err
	}
	*out = res
	return nil
}

// SendInformerMetrics runs Cluster.sendInformerMetric().
func (rpcapi *ClusterRPCAPI) SendInformerMetrics(ctx context.Context, in struct{}, out *struct{}) error {
	return rpcapi.c.sendInformersMetrics(ctx)
}

// SendInformersMetrics runs Cluster.sendInformerMetric() on all informers.
func (rpcapi *ClusterRPCAPI) SendInformersMetrics(ctx context.Context, in struct{}, out *struct{}) error {
	return rpcapi.c.sendInformersMetrics(ctx)
}

// Alerts runs Cluster.Alerts().
func (rpcapi *ClusterRPCAPI) Alerts(ctx context.Context, in struct{}, out *[]api.Alert) error {
	alerts := rpcapi.c.Alerts()
	*out = alerts
	return nil
}

// BandwidthByProtocol runs Cluster.BandwidthByProtocol().
func (rpcapi *ClusterRPCAPI) BandwidthByProtocol(ctx context.Context, in struct{}, out *api.BandwidthByProtocol) error {
	bw := rpcapi.c.BandwidthByProtocol()
	*out = bw
	return nil
}

// IPFSID returns the current cached IPFS ID for a peer.
func (rpcapi *ClusterRPCAPI) IPFSID(ctx context.Context, in peer.ID, out *api.IPFSID) error {
	if in == "" {
		in = rpcapi.c.host.ID()
	}
	pingVal := pingValueFromMetric(rpcapi.c.monitor.LatestForPeer(ctx, pingMetricName, in))
	i := api.IPFSID{
		ID:        pingVal.IPFSID,
		Addresses: pingVal.IPFSAddresses,
	}
	*out = i
	return nil
}

/*
   Tracker component methods
*/

// Track runs PinTracker.Track().
func (rpcapi *PinTrackerRPCAPI) Track(ctx context.Context, in api.Pin, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/Track")
	defer span.End()
	return rpcapi.tracker.Track(ctx, in)
}

// Untrack runs PinTracker.Untrack().
func (rpcapi *PinTrackerRPCAPI) Untrack(ctx context.Context, in api.Pin, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/Untrack")
	defer span.End()
	return rpcapi.tracker.Untrack(ctx, in.Cid)
}

// StatusAll runs PinTracker.StatusAll().
func (rpcapi *PinTrackerRPCAPI) StatusAll(ctx context.Context, in <-chan api.TrackerStatus, out chan<- api.PinInfo) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/StatusAll")
	defer span.End()

	select {
	case <-ctx.Done():
		close(out)
		return ctx.Err()
	case filter := <-in:
		return rpcapi.tracker.StatusAll(ctx, filter, out)
	}
}

// Status runs PinTracker.Status().
func (rpcapi *PinTrackerRPCAPI) Status(ctx context.Context, in api.Cid, out *api.PinInfo) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/Status")
	defer span.End()
	pinfo := rpcapi.tracker.Status(ctx, in)
	*out = pinfo
	return nil
}

// RecoverAll runs PinTracker.RecoverAll().f
func (rpcapi *PinTrackerRPCAPI) RecoverAll(ctx context.Context, in <-chan struct{}, out chan<- api.PinInfo) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/RecoverAll")
	defer span.End()
	return rpcapi.tracker.RecoverAll(ctx, out)
}

// Recover runs PinTracker.Recover().
func (rpcapi *PinTrackerRPCAPI) Recover(ctx context.Context, in api.Cid, out *api.PinInfo) error {
	ctx, span := trace.StartSpan(ctx, "rpc/tracker/Recover")
	defer span.End()
	pinfo, err := rpcapi.tracker.Recover(ctx, in)
	*out = pinfo
	return err
}

// PinQueueSize runs PinTracker.PinQueueSize().
func (rpcapi *PinTrackerRPCAPI) PinQueueSize(ctx context.Context, in struct{}, out *int64) error {
	size, err := rpcapi.tracker.PinQueueSize(ctx)
	*out = size
	return err
}

/*
   IPFS Connector component methods
*/

// Pin runs IPFSConnector.Pin().
func (rpcapi *IPFSConnectorRPCAPI) Pin(ctx context.Context, in api.Pin, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/ipfsconn/IPFSPin")
	defer span.End()
	return rpcapi.ipfs.Pin(ctx, in)
}

// Unpin runs IPFSConnector.Unpin().
func (rpcapi *IPFSConnectorRPCAPI) Unpin(ctx context.Context, in api.Pin, out *struct{}) error {
	return rpcapi.ipfs.Unpin(ctx, in.Cid)
}

// PinLsCid runs IPFSConnector.PinLsCid().
func (rpcapi *IPFSConnectorRPCAPI) PinLsCid(ctx context.Context, in api.Pin, out *api.IPFSPinStatus) error {
	b, err := rpcapi.ipfs.PinLsCid(ctx, in)
	if err != nil {
		return err
	}
	*out = b
	return nil
}

// PinLs runs IPFSConnector.PinLs().
func (rpcapi *IPFSConnectorRPCAPI) PinLs(ctx context.Context, in <-chan []string, out chan<- api.IPFSPinInfo) error {
	select {
	case <-ctx.Done():
		close(out)
		return ctx.Err()
	case pinTypes, ok := <-in:
		if !ok {
			close(out)
			return errors.New("no pinType provided for pin/ls")
		}
		return rpcapi.ipfs.PinLs(ctx, pinTypes, out)
	}
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
	*out = res
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

// BlockStream runs IPFSConnector.BlockStream().
func (rpcapi *IPFSConnectorRPCAPI) BlockStream(ctx context.Context, in <-chan api.NodeWithMeta, out chan<- struct{}) error {
	defer close(out) // very important to do at the end
	return rpcapi.ipfs.BlockStream(ctx, in)
}

// BlockGet runs IPFSConnector.BlockGet().
func (rpcapi *IPFSConnectorRPCAPI) BlockGet(ctx context.Context, in api.Cid, out *[]byte) error {
	res, err := rpcapi.ipfs.BlockGet(ctx, in)
	if err != nil {
		return err
	}
	*out = res
	return nil
}

// Resolve runs IPFSConnector.Resolve().
func (rpcapi *IPFSConnectorRPCAPI) Resolve(ctx context.Context, in string, out *api.Cid) error {
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
func (rpcapi *ConsensusRPCAPI) LogPin(ctx context.Context, in api.Pin, out *struct{}) error {
	ctx, span := trace.StartSpan(ctx, "rpc/consensus/LogPin")
	defer span.End()
	return rpcapi.cons.LogPin(ctx, in)
}

// LogUnpin runs Consensus.LogUnpin().
func (rpcapi *ConsensusRPCAPI) LogUnpin(ctx context.Context, in api.Pin, out *struct{}) error {
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

// LatestMetrics runs PeerMonitor.LatestMetrics().
func (rpcapi *PeerMonitorRPCAPI) LatestMetrics(ctx context.Context, in string, out *[]api.Metric) error {
	*out = rpcapi.mon.LatestMetrics(ctx, in)
	return nil
}

// MetricNames runs PeerMonitor.MetricNames().
func (rpcapi *PeerMonitorRPCAPI) MetricNames(ctx context.Context, in struct{}, out *[]string) error {
	*out = rpcapi.mon.MetricNames(ctx)
	return nil
}
