package ipfscluster

import (
	"errors"

	peer "gx/ipfs/QmZcUPvPhD1Xvk6mwijYF8AfR3mG31S1YsEfHG4khrFPRr/go-libp2p-peer"

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
func (rpcapi *RPCAPI) ID(in struct{}, out *api.IDSerial) error {
	id := rpcapi.c.ID().ToSerial()
	*out = id
	return nil
}

// Pin runs Cluster.Pin().
func (rpcapi *RPCAPI) Pin(in api.PinSerial, out *struct{}) error {
	return rpcapi.c.Pin(in.ToPin())
}

// Unpin runs Cluster.Unpin().
func (rpcapi *RPCAPI) Unpin(in api.PinSerial, out *struct{}) error {
	c := in.ToPin().Cid
	return rpcapi.c.Unpin(c)
}

// Pins runs Cluster.Pins().
func (rpcapi *RPCAPI) Pins(in struct{}, out *[]api.PinSerial) error {
	cidList := rpcapi.c.Pins()
	cidSerialList := make([]api.PinSerial, 0, len(cidList))
	for _, c := range cidList {
		cidSerialList = append(cidSerialList, c.ToSerial())
	}
	*out = cidSerialList
	return nil
}

// PinGet runs Cluster.PinGet().
func (rpcapi *RPCAPI) PinGet(in api.PinSerial, out *api.PinSerial) error {
	cidarg := in.ToPin()
	pin, err := rpcapi.c.PinGet(cidarg.Cid)
	if err == nil {
		*out = pin.ToSerial()
	}
	return err
}

// Version runs Cluster.Version().
func (rpcapi *RPCAPI) Version(in struct{}, out *api.Version) error {
	*out = api.Version{
		Version: rpcapi.c.Version(),
	}
	return nil
}

// Peers runs Cluster.Peers().
func (rpcapi *RPCAPI) Peers(in struct{}, out *[]api.IDSerial) error {
	peers := rpcapi.c.Peers()
	var sPeers []api.IDSerial
	for _, p := range peers {
		sPeers = append(sPeers, p.ToSerial())
	}
	*out = sPeers
	return nil
}

// PeerAdd runs Cluster.PeerAdd().
func (rpcapi *RPCAPI) PeerAdd(in api.MultiaddrSerial, out *api.IDSerial) error {
	addr := in.ToMultiaddr()
	id, err := rpcapi.c.PeerAdd(addr)
	*out = id.ToSerial()
	return err
}

// PeerRemove runs Cluster.PeerRm().
func (rpcapi *RPCAPI) PeerRemove(in peer.ID, out *struct{}) error {
	return rpcapi.c.PeerRemove(in)
}

// Join runs Cluster.Join().
func (rpcapi *RPCAPI) Join(in api.MultiaddrSerial, out *struct{}) error {
	addr := in.ToMultiaddr()
	err := rpcapi.c.Join(addr)
	return err
}

// StatusAll runs Cluster.StatusAll().
func (rpcapi *RPCAPI) StatusAll(in struct{}, out *[]api.GlobalPinInfoSerial) error {
	pinfos, err := rpcapi.c.StatusAll()
	*out = globalPinInfoSliceToSerial(pinfos)
	return err
}

// Status runs Cluster.Status().
func (rpcapi *RPCAPI) Status(in api.PinSerial, out *api.GlobalPinInfoSerial) error {
	c := in.ToPin().Cid
	pinfo, err := rpcapi.c.Status(c)
	*out = pinfo.ToSerial()
	return err
}

// SyncAllLocal runs Cluster.SyncAllLocal().
func (rpcapi *RPCAPI) SyncAllLocal(in struct{}, out *[]api.PinInfoSerial) error {
	pinfos, err := rpcapi.c.SyncAllLocal()
	*out = pinInfoSliceToSerial(pinfos)
	return err
}

// SyncLocal runs Cluster.SyncLocal().
func (rpcapi *RPCAPI) SyncLocal(in api.PinSerial, out *api.PinInfoSerial) error {
	c := in.ToPin().Cid
	pinfo, err := rpcapi.c.SyncLocal(c)
	*out = pinfo.ToSerial()
	return err
}

// SyncAll runs Cluster.SyncAll().
func (rpcapi *RPCAPI) SyncAll(in struct{}, out *[]api.GlobalPinInfoSerial) error {
	pinfos, err := rpcapi.c.SyncAll()
	*out = globalPinInfoSliceToSerial(pinfos)
	return err
}

// Sync runs Cluster.Sync().
func (rpcapi *RPCAPI) Sync(in api.PinSerial, out *api.GlobalPinInfoSerial) error {
	c := in.ToPin().Cid
	pinfo, err := rpcapi.c.Sync(c)
	*out = pinfo.ToSerial()
	return err
}

// StateSync runs Cluster.StateSync().
func (rpcapi *RPCAPI) StateSync(in struct{}, out *[]api.PinInfoSerial) error {
	pinfos, err := rpcapi.c.StateSync()
	*out = pinInfoSliceToSerial(pinfos)
	return err
}

// Recover runs Cluster.Recover().
func (rpcapi *RPCAPI) Recover(in api.PinSerial, out *api.GlobalPinInfoSerial) error {
	c := in.ToPin().Cid
	pinfo, err := rpcapi.c.Recover(c)
	*out = pinfo.ToSerial()
	return err
}

/*
   Tracker component methods
*/

// Track runs PinTracker.Track().
func (rpcapi *RPCAPI) Track(in api.PinSerial, out *struct{}) error {
	return rpcapi.c.tracker.Track(in.ToPin())
}

// Untrack runs PinTracker.Untrack().
func (rpcapi *RPCAPI) Untrack(in api.PinSerial, out *struct{}) error {
	c := in.ToPin().Cid
	return rpcapi.c.tracker.Untrack(c)
}

// TrackerStatusAll runs PinTracker.StatusAll().
func (rpcapi *RPCAPI) TrackerStatusAll(in struct{}, out *[]api.PinInfoSerial) error {
	*out = pinInfoSliceToSerial(rpcapi.c.tracker.StatusAll())
	return nil
}

// TrackerStatus runs PinTracker.Status().
func (rpcapi *RPCAPI) TrackerStatus(in api.PinSerial, out *api.PinInfoSerial) error {
	c := in.ToPin().Cid
	pinfo := rpcapi.c.tracker.Status(c)
	*out = pinfo.ToSerial()
	return nil
}

// TrackerRecover runs PinTracker.Recover().
func (rpcapi *RPCAPI) TrackerRecover(in api.PinSerial, out *api.PinInfoSerial) error {
	c := in.ToPin().Cid
	pinfo, err := rpcapi.c.tracker.Recover(c)
	*out = pinfo.ToSerial()
	return err
}

/*
   IPFS Connector component methods
*/

// IPFSPin runs IPFSConnector.Pin().
func (rpcapi *RPCAPI) IPFSPin(in api.PinSerial, out *struct{}) error {
	c := in.ToPin().Cid
	return rpcapi.c.ipfs.Pin(c)
}

// IPFSUnpin runs IPFSConnector.Unpin().
func (rpcapi *RPCAPI) IPFSUnpin(in api.PinSerial, out *struct{}) error {
	c := in.ToPin().Cid
	return rpcapi.c.ipfs.Unpin(c)
}

// IPFSPinLsCid runs IPFSConnector.PinLsCid().
func (rpcapi *RPCAPI) IPFSPinLsCid(in api.PinSerial, out *api.IPFSPinStatus) error {
	c := in.ToPin().Cid
	b, err := rpcapi.c.ipfs.PinLsCid(c)
	*out = b
	return err
}

// IPFSPinLs runs IPFSConnector.PinLs().
func (rpcapi *RPCAPI) IPFSPinLs(in string, out *map[string]api.IPFSPinStatus) error {
	m, err := rpcapi.c.ipfs.PinLs(in)
	*out = m
	return err
}

// IPFSConnectSwarms runs IPFSConnector.ConnectSwarms().
func (rpcapi *RPCAPI) IPFSConnectSwarms(in struct{}, out *struct{}) error {
	err := rpcapi.c.ipfs.ConnectSwarms()
	return err
}

// IPFSConfigKey runs IPFSConnector.ConfigKey().
func (rpcapi *RPCAPI) IPFSConfigKey(in string, out *interface{}) error {
	res, err := rpcapi.c.ipfs.ConfigKey(in)
	*out = res
	return err
}

// IPFSRepoSize runs IPFSConnector.RepoSize().
func (rpcapi *RPCAPI) IPFSRepoSize(in struct{}, out *int) error {
	res, err := rpcapi.c.ipfs.RepoSize()
	*out = res
	return err
}

/*
   Consensus component methods
*/

// ConsensusLogPin runs Consensus.LogPin().
func (rpcapi *RPCAPI) ConsensusLogPin(in api.PinSerial, out *struct{}) error {
	c := in.ToPin()
	return rpcapi.c.consensus.LogPin(c)
}

// ConsensusLogUnpin runs Consensus.LogUnpin().
func (rpcapi *RPCAPI) ConsensusLogUnpin(in api.PinSerial, out *struct{}) error {
	c := in.ToPin()
	return rpcapi.c.consensus.LogUnpin(c)
}

// ConsensusLogAddPeer runs Consensus.LogAddPeer().
func (rpcapi *RPCAPI) ConsensusLogAddPeer(in api.MultiaddrSerial, out *struct{}) error {
	addr := in.ToMultiaddr()
	return rpcapi.c.consensus.LogAddPeer(addr)
}

// ConsensusLogRmPeer runs Consensus.LogRmPeer().
func (rpcapi *RPCAPI) ConsensusLogRmPeer(in peer.ID, out *struct{}) error {
	return rpcapi.c.consensus.LogRmPeer(in)
}

/*
   Peer Manager methods
*/

// PeerManagerAddPeer runs peerManager.addPeer().
func (rpcapi *RPCAPI) PeerManagerAddPeer(in api.MultiaddrSerial, out *struct{}) error {
	addr := in.ToMultiaddr()
	err := rpcapi.c.peerManager.addPeer(addr)
	return err
}

// PeerManagerAddFromMultiaddrs runs peerManager.addFromMultiaddrs().
func (rpcapi *RPCAPI) PeerManagerAddFromMultiaddrs(in api.MultiaddrsSerial, out *struct{}) error {
	addrs := in.ToMultiaddrs()
	err := rpcapi.c.peerManager.addFromMultiaddrs(addrs)
	return err
}

// PeerManagerRmPeerShutdown runs peerManager.rmPeer().
func (rpcapi *RPCAPI) PeerManagerRmPeerShutdown(in peer.ID, out *struct{}) error {
	return rpcapi.c.peerManager.rmPeer(in, true)
}

// PeerManagerRmPeer runs peerManager.rmPeer().
func (rpcapi *RPCAPI) PeerManagerRmPeer(in peer.ID, out *struct{}) error {
	return rpcapi.c.peerManager.rmPeer(in, false)
}

// PeerManagerPeers runs peerManager.peers().
func (rpcapi *RPCAPI) PeerManagerPeers(in struct{}, out *[]peer.ID) error {
	*out = rpcapi.c.peerManager.peers()
	return nil
}

/*
   PeerMonitor
*/

// PeerMonitorLogMetric runs PeerMonitor.LogMetric().
func (rpcapi *RPCAPI) PeerMonitorLogMetric(in api.Metric, out *struct{}) error {
	rpcapi.c.monitor.LogMetric(in)
	return nil
}

// PeerMonitorLastMetrics runs PeerMonitor.LastMetrics().
func (rpcapi *RPCAPI) PeerMonitorLastMetrics(in string, out *[]api.Metric) error {
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
func (rpcapi *RPCAPI) RemoteMultiaddrForPeer(in peer.ID, out *api.MultiaddrSerial) error {
	conns := rpcapi.c.host.Network().ConnsToPeer(in)
	if len(conns) == 0 {
		return errors.New("no connections to: " + in.Pretty())
	}
	*out = api.MultiaddrToSerial(multiaddrJoin(conns[0].RemoteMultiaddr(), in))
	return nil
}
