package ipfscluster

import (
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
)

type RPCAPI struct {
	cluster *Cluster
}

type CidArg struct {
	Cid string
}

func NewCidArg(c *cid.Cid) *CidArg {
	return &CidArg{c.String()}
}

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

func (api *RPCAPI) Pin(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.Pin(c)
}

func (api *RPCAPI) Unpin(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.Unpin(c)
}

func (api *RPCAPI) PinList(in struct{}, out *[]string) error {
	cidList := api.cluster.Pins()
	cidStrList := make([]string, 0, len(cidList))
	for _, c := range cidList {
		cidStrList = append(cidStrList, c.String())
	}
	*out = cidStrList
	return nil
}

func (api *RPCAPI) Version(in struct{}, out *string) error {
	*out = api.cluster.Version()
	return nil
}

func (api *RPCAPI) MemberList(in struct{}, out *[]peer.ID) error {
	*out = api.cluster.Members()
	return nil
}

func (api *RPCAPI) Status(in struct{}, out *[]GlobalPinInfo) error {
	pinfo, err := api.cluster.Status()
	*out = pinfo
	return err
}

func (api *RPCAPI) StatusCid(in *CidArg, out *GlobalPinInfo) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	pinfo, err := api.cluster.StatusCid(c)
	*out = pinfo
	return err
}

func (api *RPCAPI) LocalSync(in struct{}, out *[]PinInfo) error {
	pinfo, err := api.cluster.LocalSync()
	*out = pinfo
	return err
}

func (api *RPCAPI) LocalSyncCid(in *CidArg, out *PinInfo) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	pinfo, err := api.cluster.LocalSyncCid(c)
	*out = pinfo
	return err
}

func (api *RPCAPI) GlobalSync(in struct{}, out *[]GlobalPinInfo) error {
	pinfo, err := api.cluster.GlobalSync()
	*out = pinfo
	return err
}

func (api *RPCAPI) GlobalSyncCid(in *CidArg, out *GlobalPinInfo) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	pinfo, err := api.cluster.GlobalSyncCid(c)
	*out = pinfo
	return err
}

func (api *RPCAPI) StateSync(in struct{}, out *[]PinInfo) error {
	pinfo, err := api.cluster.StateSync()
	*out = pinfo
	return err
}

/*
   Tracker component methods
*/

func (api *RPCAPI) Track(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.tracker.Track(c)
}

func (api *RPCAPI) Untrack(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.tracker.Untrack(c)
}

func (api *RPCAPI) TrackerStatus(in struct{}, out *[]PinInfo) error {
	*out = api.cluster.tracker.Status()
	return nil
}

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

func (api *RPCAPI) IPFSPin(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.ipfs.Pin(c)
}

func (api *RPCAPI) IPFSUnpin(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.ipfs.Unpin(c)
}

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

func (api *RPCAPI) ConsensusLogPin(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.consensus.LogPin(c)
}

func (api *RPCAPI) ConsensusLogUnpin(in *CidArg, out *struct{}) error {
	c, err := in.CID()
	if err != nil {
		return err
	}
	return api.cluster.consensus.LogUnpin(c)
}
