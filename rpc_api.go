package ipfscluster

import (
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
)

// RPCAPI is a go-libp2p-rpc service which provides the internal ipfs-cluster
// API, which enables components and members of the cluster to communicate and
// request actions from each other.
//
// The RPC API methods are usually redirects to the actual methods in
// the different components of ipfs-cluster, with very little added logic.
// Refer to documentation on those methods for details on their behaviour.
type RPCAPI struct {
	cluster *Cluster
}

// CidArg is an arguments that carry a Cid. It may carry more things in the
// future.
type CidArg struct {
	Cid string
}

// NewCidArg returns a CidArg which carries the given Cid. It panics if it is
// nil.
func NewCidArg(c *cid.Cid) *CidArg {
	if c == nil {
		panic("Cid cannot be nil")
	}
	return &CidArg{c.String()}
}

// CID decodes and returns a Cid from a CidArg.
func (arg *CidArg) CID() (*cid.Cid, error) {
	c, err := cid.Decode(arg.Cid)
	if err != nil {
		return nil, err
	}
	return c, nil
}

/*
   Cluster components methods
*/

// Pin runs Cluster.Pin().
func (api *RPCAPI) Pin(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.Pin(c)
}

// Unpin runs Cluster.Unpin().
func (api *RPCAPI) Unpin(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.Unpin(c)
}

// PinList runs Cluster.Pins().
func (api *RPCAPI) PinList(in struct{}, out *[]string) error {
	cidList := api.cluster.Pins()
	cidStrList := make([]string, 0, len(cidList))
	for _, c := range cidList {
		cidStrList = append(cidStrList, c.String())
	}
	*out = cidStrList
	return nil
}

// Version runs Cluster.Version().
func (api *RPCAPI) Version(in struct{}, out *string) error {
	*out = api.cluster.Version()
	return nil
}

// MemberList runs Cluster.Members().
func (api *RPCAPI) MemberList(in struct{}, out *[]peer.ID) error {
	*out = api.cluster.Members()
	return nil
}

// Status runs Cluster.Status().
func (api *RPCAPI) Status(in struct{}, out *[]GlobalPinInfo) error {
	pinfo, err := api.cluster.Status()
	*out = pinfo
	return err
}

// StatusCid runs Cluster.StatusCid().
func (api *RPCAPI) StatusCid(in *CidArg, out *GlobalPinInfo) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	pinfo, err := api.cluster.StatusCid(c)
	*out = pinfo
	return err
}

// LocalSync runs Cluster.LocalSync().
func (api *RPCAPI) LocalSync(in struct{}, out *[]PinInfo) error {
	pinfo, err := api.cluster.LocalSync()
	*out = pinfo
	return err
}

// LocalSyncCid runs Cluster.LocalSyncCid().
func (api *RPCAPI) LocalSyncCid(in *CidArg, out *PinInfo) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	pinfo, err := api.cluster.LocalSyncCid(c)
	*out = pinfo
	return err
}

// GlobalSync runs Cluster.GlobalSync().
func (api *RPCAPI) GlobalSync(in struct{}, out *[]GlobalPinInfo) error {
	pinfo, err := api.cluster.GlobalSync()
	*out = pinfo
	return err
}

// GlobalSyncCid runs Cluster.GlobalSyncCid().
func (api *RPCAPI) GlobalSyncCid(in *CidArg, out *GlobalPinInfo) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	pinfo, err := api.cluster.GlobalSyncCid(c)
	*out = pinfo
	return err
}

// StateSync runs Cluster.StateSync().
func (api *RPCAPI) StateSync(in struct{}, out *[]PinInfo) error {
	pinfo, err := api.cluster.StateSync()
	*out = pinfo
	return err
}

/*
   Tracker component methods
*/

// Track runs PinTracker.Track().
func (api *RPCAPI) Track(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.tracker.Track(c)
}

// Untrack runs PinTracker.Untrack().
func (api *RPCAPI) Untrack(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.tracker.Untrack(c)
}

// TrackerStatus runs PinTracker.Status().
func (api *RPCAPI) TrackerStatus(in struct{}, out *[]PinInfo) error {
	*out = api.cluster.tracker.Status()
	return nil
}

// TrackerStatusCid runs PinTracker.StatusCid().
func (api *RPCAPI) TrackerStatusCid(in *CidArg, out *PinInfo) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	pinfo := api.cluster.tracker.StatusCid(c)
	*out = pinfo
	return nil
}

// TrackerRecover not sure if needed

/*
   IPFS Connector component methods
*/

// IPFSPin runs IPFSConnector.Pin().
func (api *RPCAPI) IPFSPin(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.ipfs.Pin(c)
}

// IPFSUnpin runs IPFSConnector.Unpin().
func (api *RPCAPI) IPFSUnpin(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.ipfs.Unpin(c)
}

// IPFSIsPinned runs IPFSConnector.IsPinned().
func (api *RPCAPI) IPFSIsPinned(in *CidArg, out *bool) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	b, err := api.cluster.ipfs.IsPinned(c)
	*out = b
	return err
}

/*
   Consensus component methods
*/

// ConsensusLogPin runs Consensus.LogPin().
func (api *RPCAPI) ConsensusLogPin(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.consensus.LogPin(c)
}

// ConsensusLogUnpin runs Consensus.LogUnpin().
func (api *RPCAPI) ConsensusLogUnpin(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.consensus.LogUnpin(c)
}
