package ipfscluster

import (
	"context"
	"errors"

	peer "github.com/libp2p/go-libp2p-peer"

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
func (rpcapi *RPCAPI) ID(ctx context.Context, in struct{}, out *api.IDSerial) error {
	id := rpcapi.c.ID().ToSerial()
	*out = id
	return nil
}

// Pin runs Cluster.Pin().
func (rpcapi *RPCAPI) Pin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	return rpcapi.c.Pin(in.ToPin())
}

// Unpin runs Cluster.Unpin().
func (rpcapi *RPCAPI) Unpin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	c := in.ToPin().Cid
	return rpcapi.c.Unpin(c)
}

// Pins runs Cluster.Pins().
func (rpcapi *RPCAPI) Pins(ctx context.Context, in struct{}, out *[]api.PinSerial) error {
	cidList := rpcapi.c.Pins()
	cidSerialList := make([]api.PinSerial, 0, len(cidList))
	for _, c := range cidList {
		cidSerialList = append(cidSerialList, c.ToSerial())
	}
	*out = cidSerialList
	return nil
}

// PinGet runs Cluster.PinGet().
func (rpcapi *RPCAPI) PinGet(ctx context.Context, in api.PinSerial, out *api.PinSerial) error {
	cidarg := in.ToPin()
	pin, err := rpcapi.c.PinGet(cidarg.Cid)
	if err == nil {
		*out = pin.ToSerial()
	}
	return err
}

// Version runs Cluster.Version().
func (rpcapi *RPCAPI) Version(ctx context.Context, in struct{}, out *api.Version) error {
	*out = api.Version{
		Version: rpcapi.c.Version(),
	}
	return nil
}

// Peers runs Cluster.Peers().
func (rpcapi *RPCAPI) Peers(ctx context.Context, in struct{}, out *[]api.IDSerial) error {
	peers := rpcapi.c.Peers()
	var sPeers []api.IDSerial
	for _, p := range peers {
		sPeers = append(sPeers, p.ToSerial())
	}
	*out = sPeers
	return nil
}

// PeerAdd runs Cluster.PeerAdd().
func (rpcapi *RPCAPI) PeerAdd(ctx context.Context, in api.MultiaddrSerial, out *api.IDSerial) error {
	addr := in.ToMultiaddr()
	id, err := rpcapi.c.PeerAdd(addr)
	*out = id.ToSerial()
	return err
}

// ConnectGraph runs Cluster.GetConnectGraph().
func (rpcapi *RPCAPI) ConnectGraph(ctx context.Context, in struct{}, out *api.ConnectGraphSerial) error {
	graph, err := rpcapi.c.ConnectGraph()
	*out = graph.ToSerial()
	return err
}

// PeerRemove runs Cluster.PeerRm().
func (rpcapi *RPCAPI) PeerRemove(ctx context.Context, in peer.ID, out *struct{}) error {
	return rpcapi.c.PeerRemove(in)
}

// Join runs Cluster.Join().
func (rpcapi *RPCAPI) Join(ctx context.Context, in api.MultiaddrSerial, out *struct{}) error {
	addr := in.ToMultiaddr()
	err := rpcapi.c.Join(addr)
	return err
}

// StatusAll runs Cluster.StatusAll().
func (rpcapi *RPCAPI) StatusAll(ctx context.Context, in struct{}, out *[]api.GlobalPinInfoSerial) error {
	pinfos, err := rpcapi.c.StatusAll()
	*out = globalPinInfoSliceToSerial(pinfos)
	return err
}

// StatusAllLocal runs Cluster.StatusAllLocal().
func (rpcapi *RPCAPI) StatusAllLocal(ctx context.Context, in struct{}, out *[]api.PinInfoSerial) error {
	pinfos := rpcapi.c.StatusAllLocal()
	*out = pinInfoSliceToSerial(pinfos)
	return nil
}

// Status runs Cluster.Status().
func (rpcapi *RPCAPI) Status(ctx context.Context, in api.PinSerial, out *api.GlobalPinInfoSerial) error {
	c := in.ToPin().Cid
	pinfo, err := rpcapi.c.Status(c)
	*out = pinfo.ToSerial()
	return err
}

// StatusLocal runs Cluster.StatusLocal().
func (rpcapi *RPCAPI) StatusLocal(ctx context.Context, in api.PinSerial, out *api.PinInfoSerial) error {
	c := in.ToPin().Cid
	pinfo := rpcapi.c.StatusLocal(c)
	*out = pinfo.ToSerial()
	return nil
}

// SyncAll runs Cluster.SyncAll().
func (rpcapi *RPCAPI) SyncAll(ctx context.Context, in struct{}, out *[]api.GlobalPinInfoSerial) error {
	pinfos, err := rpcapi.c.SyncAll()
	*out = globalPinInfoSliceToSerial(pinfos)
	return err
}

// SyncAllLocal runs Cluster.SyncAllLocal().
func (rpcapi *RPCAPI) SyncAllLocal(ctx context.Context, in struct{}, out *[]api.PinInfoSerial) error {
	pinfos, err := rpcapi.c.SyncAllLocal()
	*out = pinInfoSliceToSerial(pinfos)
	return err
}

// Sync runs Cluster.Sync().
func (rpcapi *RPCAPI) Sync(ctx context.Context, in api.PinSerial, out *api.GlobalPinInfoSerial) error {
	c := in.ToPin().Cid
	pinfo, err := rpcapi.c.Sync(c)
	*out = pinfo.ToSerial()
	return err
}

// SyncLocal runs Cluster.SyncLocal().
func (rpcapi *RPCAPI) SyncLocal(ctx context.Context, in api.PinSerial, out *api.PinInfoSerial) error {
	c := in.ToPin().Cid
	pinfo, err := rpcapi.c.SyncLocal(c)
	*out = pinfo.ToSerial()
	return err
}

// RecoverAllLocal runs Cluster.RecoverAllLocal().
func (rpcapi *RPCAPI) RecoverAllLocal(ctx context.Context, in struct{}, out *[]api.PinInfoSerial) error {
	pinfos, err := rpcapi.c.RecoverAllLocal()
	*out = pinInfoSliceToSerial(pinfos)
	return err
}

// Recover runs Cluster.Recover().
func (rpcapi *RPCAPI) Recover(ctx context.Context, in api.PinSerial, out *api.GlobalPinInfoSerial) error {
	c := in.ToPin().Cid
	pinfo, err := rpcapi.c.Recover(c)
	*out = pinfo.ToSerial()
	return err
}

// RecoverLocal runs Cluster.RecoverLocal().
func (rpcapi *RPCAPI) RecoverLocal(ctx context.Context, in api.PinSerial, out *api.PinInfoSerial) error {
	c := in.ToPin().Cid
	pinfo, err := rpcapi.c.RecoverLocal(c)
	*out = pinfo.ToSerial()
	return err
}

// StateSync runs Cluster.StateSync().
func (rpcapi *RPCAPI) StateSync(ctx context.Context, in struct{}, out *[]api.PinInfoSerial) error {
	pinfos, err := rpcapi.c.StateSync()
	*out = pinInfoSliceToSerial(pinfos)
	return err
}

/*
   Tracker component methods
*/

// Track runs PinTracker.Track().
func (rpcapi *RPCAPI) Track(ctx context.Context, in api.PinSerial, out *struct{}) error {
	return rpcapi.c.tracker.Track(in.ToPin())
}

// Untrack runs PinTracker.Untrack().
func (rpcapi *RPCAPI) Untrack(ctx context.Context, in api.PinSerial, out *struct{}) error {
	c := in.ToPin().Cid
	return rpcapi.c.tracker.Untrack(c)
}

// TrackerStatusAll runs PinTracker.StatusAll().
func (rpcapi *RPCAPI) TrackerStatusAll(ctx context.Context, in struct{}, out *[]api.PinInfoSerial) error {
	*out = pinInfoSliceToSerial(rpcapi.c.tracker.StatusAll())
	return nil
}

// TrackerStatus runs PinTracker.Status().
func (rpcapi *RPCAPI) TrackerStatus(ctx context.Context, in api.PinSerial, out *api.PinInfoSerial) error {
	c := in.ToPin().Cid
	pinfo := rpcapi.c.tracker.Status(c)
	*out = pinfo.ToSerial()
	return nil
}

// TrackerRecoverAll runs PinTracker.RecoverAll().
func (rpcapi *RPCAPI) TrackerRecoverAll(ctx context.Context, in struct{}, out *[]api.PinInfoSerial) error {
	pinfos, err := rpcapi.c.tracker.RecoverAll()
	*out = pinInfoSliceToSerial(pinfos)
	return err
}

// TrackerRecover runs PinTracker.Recover().
func (rpcapi *RPCAPI) TrackerRecover(ctx context.Context, in api.PinSerial, out *api.PinInfoSerial) error {
	c := in.ToPin().Cid
	pinfo, err := rpcapi.c.tracker.Recover(c)
	*out = pinfo.ToSerial()
	return err
}

/*
   IPFS Connector component methods
*/

// IPFSPin runs IPFSConnector.Pin().
func (rpcapi *RPCAPI) IPFSPin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	c := in.ToPin().Cid
	r := in.ToPin().Recursive
	return rpcapi.c.ipfs.Pin(ctx, c, r)
}

// IPFSUnpin runs IPFSConnector.Unpin().
func (rpcapi *RPCAPI) IPFSUnpin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	c := in.ToPin().Cid
	return rpcapi.c.ipfs.Unpin(ctx, c)
}

// IPFSPinLsCid runs IPFSConnector.PinLsCid().
func (rpcapi *RPCAPI) IPFSPinLsCid(ctx context.Context, in api.PinSerial, out *api.IPFSPinStatus) error {
	c := in.ToPin().Cid
	b, err := rpcapi.c.ipfs.PinLsCid(ctx, c)
	*out = b
	return err
}

// IPFSPinLs runs IPFSConnector.PinLs().
func (rpcapi *RPCAPI) IPFSPinLs(ctx context.Context, in string, out *map[string]api.IPFSPinStatus) error {
	m, err := rpcapi.c.ipfs.PinLs(ctx, in)
	*out = m
	return err
}

// IPFSConnectSwarms runs IPFSConnector.ConnectSwarms().
func (rpcapi *RPCAPI) IPFSConnectSwarms(ctx context.Context, in struct{}, out *struct{}) error {
	err := rpcapi.c.ipfs.ConnectSwarms()
	return err
}

// IPFSConfigKey runs IPFSConnector.ConfigKey().
func (rpcapi *RPCAPI) IPFSConfigKey(ctx context.Context, in string, out *interface{}) error {
	res, err := rpcapi.c.ipfs.ConfigKey(in)
	*out = res
	return err
}

// IPFSFreeSpace runs IPFSConnector.FreeSpace().
func (rpcapi *RPCAPI) IPFSFreeSpace(ctx context.Context, in struct{}, out *uint64) error {
	res, err := rpcapi.c.ipfs.FreeSpace()
	*out = res
	return err
}

// IPFSRepoSize runs IPFSConnector.RepoSize().
func (rpcapi *RPCAPI) IPFSRepoSize(ctx context.Context, in struct{}, out *uint64) error {
	res, err := rpcapi.c.ipfs.RepoSize()
	*out = res
	return err
}

// IPFSSwarmPeers runs IPFSConnector.SwarmPeers().
func (rpcapi *RPCAPI) IPFSSwarmPeers(ctx context.Context, in struct{}, out *api.SwarmPeersSerial) error {
	res, err := rpcapi.c.ipfs.SwarmPeers()
	*out = res.ToSerial()
	return err
}

/*
   Consensus component methods
*/

// ConsensusLogPin runs Consensus.LogPin().
func (rpcapi *RPCAPI) ConsensusLogPin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	c := in.ToPin()
	return rpcapi.c.consensus.LogPin(c)
}

// ConsensusLogUnpin runs Consensus.LogUnpin().
func (rpcapi *RPCAPI) ConsensusLogUnpin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	c := in.ToPin()
	return rpcapi.c.consensus.LogUnpin(c)
}

// ConsensusAddPeer runs Consensus.AddPeer().
func (rpcapi *RPCAPI) ConsensusAddPeer(ctx context.Context, in peer.ID, out *struct{}) error {
	return rpcapi.c.consensus.AddPeer(in)
}

// ConsensusRmPeer runs Consensus.RmPeer().
func (rpcapi *RPCAPI) ConsensusRmPeer(ctx context.Context, in peer.ID, out *struct{}) error {
	return rpcapi.c.consensus.RmPeer(in)
}

// ConsensusPeers runs Consensus.Peers().
func (rpcapi *RPCAPI) ConsensusPeers(ctx context.Context, in struct{}, out *[]peer.ID) error {
	peers, err := rpcapi.c.consensus.Peers()
	*out = peers
	return err
}

/*
   Peer Manager methods
*/

// PeerManagerAddPeer runs peerManager.addPeer().
func (rpcapi *RPCAPI) PeerManagerAddPeer(ctx context.Context, in api.MultiaddrSerial, out *struct{}) error {
	addr := in.ToMultiaddr()
	err := rpcapi.c.peerManager.addPeer(addr, false)
	return err
}

// PeerManagerImportAddresses runs peerManager.importAddresses().
func (rpcapi *RPCAPI) PeerManagerImportAddresses(ctx context.Context, in api.MultiaddrsSerial, out *struct{}) error {
	addrs := in.ToMultiaddrs()
	err := rpcapi.c.peerManager.importAddresses(addrs, false)
	return err
}

/*
   PeerMonitor
*/

// PeerMonitorLogMetric runs PeerMonitor.LogMetric().
func (rpcapi *RPCAPI) PeerMonitorLogMetric(ctx context.Context, in api.Metric, out *struct{}) error {
	rpcapi.c.monitor.LogMetric(in)
	return nil
}

// PeerMonitorLastMetrics runs PeerMonitor.LastMetrics().
func (rpcapi *RPCAPI) PeerMonitorLastMetrics(ctx context.Context, in string, out *[]api.Metric) error {
	*out = rpcapi.c.monitor.LastMetrics(in)
	return nil
}

/*
   Other
*/

// RemoteMultiaddrForPeer returns the multiaddr of a peer as seen by this peer.
// This is necessary for a peer to figure out which of its multiaddresses the
// peers are seeing (also when crossing NATs). It should be called from
// the peer the IN parameter indicates.
func (rpcapi *RPCAPI) RemoteMultiaddrForPeer(ctx context.Context, in peer.ID, out *api.MultiaddrSerial) error {
	conns := rpcapi.c.host.Network().ConnsToPeer(in)
	if len(conns) == 0 {
		return errors.New("no connections to: " + in.Pretty())
	}
	*out = api.MultiaddrToSerial(api.MustLibp2pMultiaddrJoin(conns[0].RemoteMultiaddr(), in))
	return nil
}
