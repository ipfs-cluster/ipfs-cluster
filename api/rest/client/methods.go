package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"

	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/ipfs-cluster/api"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
)

// ID returns information about the cluster Peer.
func (c *Client) ID() (api.ID, *api.Error) {
	var id api.IDSerial
	err := c.do("GET", "/id", nil, &id)
	return id.ToID(), err
}

// Peers requests ID information for all cluster peers.
func (c *Client) Peers() ([]api.ID, *api.Error) {
	var ids []api.IDSerial
	err := c.do("GET", "/peers", nil, &ids)
	result := make([]api.ID, len(ids))
	for i, id := range ids {
		result[i] = id.ToID()
	}
	return result, err
}

type peerAddBody struct {
	Addr string `json:"peer_multiaddress"`
}

// PeerAdd adds a new peer to the cluster.
func (c *Client) PeerAdd(addr ma.Multiaddr) (api.ID, *api.Error) {
	addrStr := addr.String()
	body := peerAddBody{addrStr}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.Encode(body)

	var id api.IDSerial
	err := c.do("POST", "/peers", &buf, &id)
	return id.ToID(), err
}

// PeerRm removes a current peer from the cluster
func (c *Client) PeerRm(id peer.ID) *api.Error {
	return c.do("DELETE", fmt.Sprintf("/peers/%s", id.Pretty()), nil, nil)
}

// Pin tracks a Cid with the given replication factor and a name for
// human-friendliness.
func (c *Client) Pin(ci *cid.Cid, replicationFactor int, name string) *api.Error {
	escName := url.QueryEscape(name)
	err := c.do(
		"POST",
		fmt.Sprintf("/pins/%s?replication_factor=%d&name=%s",
			ci.String(),
			replicationFactor,
			escName),
		nil, nil)
	return err
}

// Unpin untracks a Cid from cluster.
func (c *Client) Unpin(ci *cid.Cid) *api.Error {
	return c.do("DELETE", fmt.Sprintf("/pins/%s", ci.String()), nil, nil)
}

// Allocations returns the consensus state listing all tracked items and
// the peers that should be pinning them.
func (c *Client) Allocations() ([]api.Pin, *api.Error) {
	var pins []api.PinSerial
	err := c.do("GET", "/allocations", nil, &pins)
	result := make([]api.Pin, len(pins))
	for i, p := range pins {
		result[i] = p.ToPin()
	}
	return result, err
}

// Allocation returns the current allocations for a given Cid.
func (c *Client) Allocation(ci *cid.Cid) (api.Pin, *api.Error) {
	var pin api.PinSerial
	err := c.do("GET", fmt.Sprintf("/allocations/%s", ci.String()), nil, &pin)
	return pin.ToPin(), err
}

// Status returns the current ipfs state for a given Cid. If local is true,
// the information affects only the current peer, otherwise the information
// is fetched from all cluster peers.
func (c *Client) Status(ci *cid.Cid, local bool) (api.GlobalPinInfo, *api.Error) {
	var gpi api.GlobalPinInfoSerial
	err := c.do("GET", fmt.Sprintf("/pins/%s?local=%t", ci.String(), local), nil, &gpi)
	return gpi.ToGlobalPinInfo(), err
}

// StatusAll gathers Status() for all tracked items.
func (c *Client) StatusAll(local bool) ([]api.GlobalPinInfo, *api.Error) {
	var gpis []api.GlobalPinInfoSerial
	err := c.do("GET", fmt.Sprintf("/pins?local=%t", local), nil, &gpis)
	result := make([]api.GlobalPinInfo, len(gpis))
	for i, p := range gpis {
		result[i] = p.ToGlobalPinInfo()
	}
	return result, err
}

// Sync makes sure the state of a Cid corresponds to the state reported by
// the ipfs daemon, and returns it. If local is true, this operation only
// happens on the current peer, otherwise it happens on every cluster peer.
func (c *Client) Sync(ci *cid.Cid, local bool) (api.GlobalPinInfo, *api.Error) {
	var gpi api.GlobalPinInfoSerial
	err := c.do("POST", fmt.Sprintf("/pins/%s/sync?local=%t", ci.String(), local), nil, &gpi)
	return gpi.ToGlobalPinInfo(), err
}

// SyncAll triggers Sync() operations for all tracked items. It only returns
// informations for items that were de-synced or have an error state. If
// local is true, the operation is limited to the current peer. Otherwise
// it happens on every cluster peer.
func (c *Client) SyncAll(local bool) ([]api.GlobalPinInfo, *api.Error) {
	var gpis []api.GlobalPinInfoSerial
	err := c.do("POST", fmt.Sprintf("/pins/sync?local=%t", local), nil, &gpis)
	result := make([]api.GlobalPinInfo, len(gpis))
	for i, p := range gpis {
		result[i] = p.ToGlobalPinInfo()
	}
	return result, err
}

// Recover retriggers pin or unpin ipfs operations for a Cid in error state.
// If local is true, the operation is limited to the current peer, otherwise
// it happens on every cluster peer.
func (c *Client) Recover(ci *cid.Cid, local bool) (api.GlobalPinInfo, *api.Error) {
	var gpi api.GlobalPinInfoSerial
	err := c.do("POST", fmt.Sprintf("/pins/%s/recover?local=%t", ci.String(), local), nil, &gpi)
	return gpi.ToGlobalPinInfo(), err
}

// RecoverAll triggers Recover() operations on all tracked items. If local is
// true, the operation is limited to the current peer. Otherwise, it happens
// everywhere.
func (c *Client) RecoverAll(local bool) ([]api.GlobalPinInfo, *api.Error) {
	var gpis []api.GlobalPinInfoSerial
	err := c.do("POST", fmt.Sprintf("/pins/recover?local=%t", local), nil, &gpis)
	result := make([]api.GlobalPinInfo, len(gpis))
	for i, p := range gpis {
		result[i] = p.ToGlobalPinInfo()
	}
	return result, err
}

// Version returns the ipfs-cluster peer's version.
func (c *Client) Version() (api.Version, *api.Error) {
	var ver api.Version
	err := c.do("GET", "/version", nil, &ver)
	return ver, err
}
