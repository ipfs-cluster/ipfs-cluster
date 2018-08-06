package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipfs-cmdkit/files"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/ipfs/ipfs-cluster/api"
)

// ID returns information about the cluster Peer.
func (c *Client) ID() (api.ID, error) {
	var id api.IDSerial
	err := c.do("GET", "/id", nil, nil, &id)
	return id.ToID(), err
}

// Peers requests ID information for all cluster peers.
func (c *Client) Peers() ([]api.ID, error) {
	var ids []api.IDSerial
	err := c.do("GET", "/peers", nil, nil, &ids)
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
func (c *Client) PeerAdd(addr ma.Multiaddr) (api.ID, error) {
	addrStr := addr.String()
	body := peerAddBody{addrStr}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.Encode(body)

	var id api.IDSerial
	err := c.do("POST", "/peers", nil, &buf, &id)
	return id.ToID(), err
}

// PeerRm removes a current peer from the cluster
func (c *Client) PeerRm(id peer.ID) error {
	return c.do("DELETE", fmt.Sprintf("/peers/%s", id.Pretty()), nil, nil, nil)
}

// Pin tracks a Cid with the given replication factor and a name for
// human-friendliness.
func (c *Client) Pin(ci *cid.Cid, replicationFactorMin, replicationFactorMax int, name string) error {
	escName := url.QueryEscape(name)
	err := c.do(
		"POST",
		fmt.Sprintf(
			"/pins/%s?replication_factor_min=%d&replication_factor_max=%d&name=%s",
			ci.String(),
			replicationFactorMin,
			replicationFactorMax,
			escName,
		),
		nil,
		nil,
		nil,
	)
	return err
}

// Unpin untracks a Cid from cluster.
func (c *Client) Unpin(ci *cid.Cid) error {
	return c.do("DELETE", fmt.Sprintf("/pins/%s", ci.String()), nil, nil, nil)
}

// Allocations returns the consensus state listing all tracked items and
// the peers that should be pinning them.
func (c *Client) Allocations(pinType api.PinType) ([]api.Pin, error) {
	var pins []api.PinSerial
	err := c.do("GET", fmt.Sprintf("/allocations?pintype=%s", pinType.String()), nil, nil, &pins)
	result := make([]api.Pin, len(pins))
	for i, p := range pins {
		result[i] = p.ToPin()
	}
	return result, err
}

// Allocation returns the current allocations for a given Cid.
func (c *Client) Allocation(ci *cid.Cid) (api.Pin, error) {
	var pin api.PinSerial
	err := c.do("GET", fmt.Sprintf("/allocations/%s", ci.String()), nil, nil, &pin)
	return pin.ToPin(), err
}

// Status returns the current ipfs state for a given Cid. If local is true,
// the information affects only the current peer, otherwise the information
// is fetched from all cluster peers.
func (c *Client) Status(ci *cid.Cid, local bool) (api.GlobalPinInfo, error) {
	var gpi api.GlobalPinInfoSerial
	err := c.do("GET", fmt.Sprintf("/pins/%s?local=%t", ci.String(), local), nil, nil, &gpi)
	return gpi.ToGlobalPinInfo(), err
}

// StatusAll gathers Status() for all tracked items.
func (c *Client) StatusAll(local bool) ([]api.GlobalPinInfo, error) {
	var gpis []api.GlobalPinInfoSerial
	err := c.do("GET", fmt.Sprintf("/pins?local=%t", local), nil, nil, &gpis)
	result := make([]api.GlobalPinInfo, len(gpis))
	for i, p := range gpis {
		result[i] = p.ToGlobalPinInfo()
	}
	return result, err
}

// Sync makes sure the state of a Cid corresponds to the state reported by
// the ipfs daemon, and returns it. If local is true, this operation only
// happens on the current peer, otherwise it happens on every cluster peer.
func (c *Client) Sync(ci *cid.Cid, local bool) (api.GlobalPinInfo, error) {
	var gpi api.GlobalPinInfoSerial
	err := c.do("POST", fmt.Sprintf("/pins/%s/sync?local=%t", ci.String(), local), nil, nil, &gpi)
	return gpi.ToGlobalPinInfo(), err
}

// SyncAll triggers Sync() operations for all tracked items. It only returns
// informations for items that were de-synced or have an error state. If
// local is true, the operation is limited to the current peer. Otherwise
// it happens on every cluster peer.
func (c *Client) SyncAll(local bool) ([]api.GlobalPinInfo, error) {
	var gpis []api.GlobalPinInfoSerial
	err := c.do("POST", fmt.Sprintf("/pins/sync?local=%t", local), nil, nil, &gpis)
	result := make([]api.GlobalPinInfo, len(gpis))
	for i, p := range gpis {
		result[i] = p.ToGlobalPinInfo()
	}
	return result, err
}

// Recover retriggers pin or unpin ipfs operations for a Cid in error state.
// If local is true, the operation is limited to the current peer, otherwise
// it happens on every cluster peer.
func (c *Client) Recover(ci *cid.Cid, local bool) (api.GlobalPinInfo, error) {
	var gpi api.GlobalPinInfoSerial
	err := c.do("POST", fmt.Sprintf("/pins/%s/recover?local=%t", ci.String(), local), nil, nil, &gpi)
	return gpi.ToGlobalPinInfo(), err
}

// RecoverAll triggers Recover() operations on all tracked items. If local is
// true, the operation is limited to the current peer. Otherwise, it happens
// everywhere.
func (c *Client) RecoverAll(local bool) ([]api.GlobalPinInfo, error) {
	var gpis []api.GlobalPinInfoSerial
	err := c.do("POST", fmt.Sprintf("/pins/recover?local=%t", local), nil, nil, &gpis)
	result := make([]api.GlobalPinInfo, len(gpis))
	for i, p := range gpis {
		result[i] = p.ToGlobalPinInfo()
	}
	return result, err
}

// Version returns the ipfs-cluster peer's version.
func (c *Client) Version() (api.Version, error) {
	var ver api.Version
	err := c.do("GET", "/version", nil, nil, &ver)
	return ver, err
}

// GetConnectGraph returns an ipfs-cluster connection graph.
// The serialized version, strings instead of pids, is returned
func (c *Client) GetConnectGraph() (api.ConnectGraphSerial, error) {
	var graphS api.ConnectGraphSerial
	err := c.do("GET", "/health/graph", nil, nil, &graphS)
	return graphS, err
}

// WaitFor is a utility function that allows for a caller to
// wait for a paticular status for a CID. It returns a channel
// upon which the caller can wait for the targetStatus.
func (c *Client) WaitFor(ctx context.Context, fp StatusFilterParams) (api.GlobalPinInfo, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sf := newStatusFilter()

	go sf.pollStatus(ctx, c, fp)
	go sf.filter(ctx, fp)

	var status api.GlobalPinInfo

	for {
		select {
		case <-ctx.Done():
			return api.GlobalPinInfo{}, ctx.Err()
		case err := <-sf.Err:
			return api.GlobalPinInfo{}, err
		case st, ok := <-sf.Out:
			if !ok { // channel closed
				return status, nil
			}
			status = st
		}
	}
}

// StatusFilterParams contains the parameters required
// to filter a stream of status results.
type StatusFilterParams struct {
	Cid       *cid.Cid
	Local     bool
	Target    api.TrackerStatus
	CheckFreq time.Duration
}

type statusFilter struct {
	In, Out chan api.GlobalPinInfo
	Done    chan struct{}
	Err     chan error
}

func newStatusFilter() *statusFilter {
	return &statusFilter{
		In:   make(chan api.GlobalPinInfo),
		Out:  make(chan api.GlobalPinInfo),
		Done: make(chan struct{}),
		Err:  make(chan error),
	}
}

func (sf *statusFilter) filter(ctx context.Context, fp StatusFilterParams) {
	defer close(sf.Done)
	defer close(sf.Out)

	for {
		select {
		case <-ctx.Done():
			sf.Err <- ctx.Err()
			return
		case gblPinInfo, more := <-sf.In:
			if !more {
				return
			}
			ok, err := statusReached(fp.Target, gblPinInfo)
			if err != nil {
				sf.Err <- err
				return
			}

			sf.Out <- gblPinInfo
			if !ok {
				continue
			}
			return
		}
	}
}

func (sf *statusFilter) pollStatus(ctx context.Context, c *Client, fp StatusFilterParams) {
	ticker := time.NewTicker(fp.CheckFreq)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			sf.Err <- ctx.Err()
			return
		case <-ticker.C:
			gblPinInfo, err := c.Status(fp.Cid, fp.Local)
			if err != nil {
				sf.Err <- err
				return
			}
			logger.Debugf("pollStatus: status: %#v", gblPinInfo)
			sf.In <- gblPinInfo
		case <-sf.Done:
			close(sf.In)
			return
		}
	}
}

func statusReached(target api.TrackerStatus, gblPinInfo api.GlobalPinInfo) (bool, error) {
	for _, pinInfo := range gblPinInfo.PeerMap {
		switch pinInfo.Status {
		case target:
			continue
		case api.TrackerStatusBug, api.TrackerStatusClusterError, api.TrackerStatusPinError, api.TrackerStatusUnpinError:
			return false, fmt.Errorf("error has occurred while attempting to reach status: %s", target.String())
		case api.TrackerStatusRemote:
			if target == api.TrackerStatusPinned {
				continue // to next pinInfo
			}
			return false, nil
		default:
			return false, nil
		}
	}
	return true, nil
}

// AddMultiFile adds new files to the cluster, importing and potentially
// sharding underlying dags across the ipfs daemons of multiple cluster peers.
// Progress can be tracked by passing a channel onto which deliver AddedOutput
// updates.
func (c *Client) AddMultiFile(
	multiFileR *files.MultiFileReader,
	params *api.AddParams,
	out chan<- *api.AddedOutput,
) error {
	defer close(out)

	headers := make(map[string]string)
	headers["Content-Type"] = "multipart/form-data; boundary=" + multiFileR.Boundary()
	queryStr := params.ToQueryString()

	handler := func(dec *json.Decoder) error {
		if out == nil {
			return nil
		}
		var obj api.AddedOutput
		err := dec.Decode(&obj)
		if err != nil {
			return err
		}
		select {
		case out <- &obj:
			//default:
		}
		return nil
	}

	err := c.doStream(
		"POST",
		"/allocations?"+queryStr,
		headers,
		multiFileR,
		handler,
	)
	return err
}

// TODO: Eventually an Add(io.Reader) method for adding raw readers as a multifile should be here.
