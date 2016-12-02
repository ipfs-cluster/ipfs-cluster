package ipfscluster

import (
	peer "gx/ipfs/QmfMmLGoKzCHDN7cGgk64PJr4iipzidDRME8HABSJqvmhC/go-libp2p-peer"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

type pinMode int

const (
	pinEverywhere = -1
)

// MapState is a very simple pin map representation
// PinMap must be public as it will be serialized
type MapState struct {
	PinMap map[string]pinMode
	rpcCh  chan ClusterRPC
}

func NewMapState() MapState {
	return MapState{
		PinMap: make(map[string]pinMode),
		rpcCh:  make(chan ClusterRPC),
	}
}

func (st MapState) AddPin(c *cid.Cid) error {
	st.PinMap[c.String()] = pinEverywhere
	return nil
}

func (st MapState) RmPin(c *cid.Cid) error {
	delete(st.PinMap, c.String())
	return nil
}

func (st MapState) Exists(c *cid.Cid) bool {
	_, ok := st.PinMap[c.String()]
	return ok
}

func (st MapState) ListPins() []*cid.Cid {
	keys := make([]*cid.Cid, 0, len(st.PinMap))
	for k := range st.PinMap {
		c, _ := cid.Decode(k)
		keys = append(keys, c)
	}
	return keys
}

func (st MapState) ShouldPin(p peer.ID, c *cid.Cid) bool {
	return true
}

func (st MapState) Shutdown() error {
	return nil
}

func (st MapState) RpcChan() <-chan ClusterRPC {
	return st.rpcCh
}
