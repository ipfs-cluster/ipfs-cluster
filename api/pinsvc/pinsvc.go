// Package pinsvc implements an IPFS Cluster API component which provides
// an IPFS Pinning Services API to the cluster.
//
// The implented API is based on the common.API component (refer to module
// description there). The only thing this module does is to provide route
// handling for the otherwise common API component.
package pinsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ipfs/go-cid"
	types "github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/api/common"
	"github.com/ipfs/ipfs-cluster/state"
	"github.com/multiformats/go-multiaddr"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	madns "github.com/multiformats/go-multiaddr-dns"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var (
	logger    = logging.Logger("pinsvcapi")
	apiLogger = logging.Logger("pinsvcapilog")
)

// APIError is returned by the API as a body when an error
// occurs.
type APIError struct {
	Reason  string `json:"reason"`
	Details string `json:"string"`
}

func (apiErr APIError) Error() string {
	return apiErr.Reason
}

// Pin contains information about a Pin.
type Pin struct {
	Cid     cid.Cid               `json:"cid"`
	Name    string                `json:"name"`
	Origins []multiaddr.Multiaddr `json:"origins"`
	Meta    map[string]string     `json:"meta"`
}

// ToClusterPin converts a Pin to a cluster Pin.
func (p Pin) ToClusterPin() *types.Pin {
	opts := types.PinOptions{
		Name:     p.Name,
		Origins:  p.Origins,
		Metadata: p.Meta,
		Mode:     types.PinModeRecursive,
	}
	return types.PinWithOpts(p.Cid, opts)
}

// PinStatus wraps pins with additional information about
// the pinning status.
type PinStatus struct {
	clusterPinInfo types.GlobalPinInfo

	RequestID string                `json:"request_id"`
	Status    string                `json:"status"`
	Created   time.Time             `json:"created"`
	Pin       Pin                   `json:"pin"`
	Delegates []multiaddr.Multiaddr `json:"delegates"`
	Info      map[string]string     `json:"info"`
}

// PinList is the result of a call to List pins
type PinList struct {
	Count   int         `json:"count"`
	Results []PinStatus `json:"results"`
}

// Assemble a PinStatus
func toPinStatus(
	rID string,
	gpi types.GlobalPinInfo,
	clusterIDs []*types.ID,
) PinStatus {

	status := PinStatus{
		clusterPinInfo: gpi,
		RequestID:      rID,
	}

	var statusMask types.TrackerStatus
	for _, pinfo := range gpi.PeerMap {
		statusMask |= pinfo.Status
	}

	switch {
	case statusMask.Match(types.TrackerStatusError):
		status.Status = "failed"
	case statusMask.Match(types.TrackerStatusPinQueued):
		status.Status = "queued"
	case statusMask.Match(types.TrackerStatusPinning):
		status.Status = "pinning"
	case statusMask.Match(types.TrackerStatusPinned):
		status.Status = "pinning"
	default:
		status.Status = statusMask.String()
	}

	status.Created = time.Now()
	status.Pin = Pin{
		Cid:     gpi.Cid,
		Name:    gpi.Name,
		Origins: gpi.Origins,
		Meta:    gpi.Metadata,
	}

	delegates := []multiaddr.Multiaddr{}
	idMap := make(map[peer.ID]*types.ID)
	for _, clusterID := range clusterIDs {
		idMap[clusterID.ID] = clusterID
	}
	filteredClusterIDs := clusterIDs
	if len(gpi.Allocations) > 0 {
		filteredClusterIDs := []*types.ID{}
		for _, alloc := range gpi.Allocations {
			clid, ok := idMap[alloc]
			if ok && clid.Error == "" {
				filteredClusterIDs = append(filteredClusterIDs, clid)
			}
		}
	}

	// Get the multiaddresses of the IPFS peers storing this content.
	for _, clid := range filteredClusterIDs {
		if clid.IPFS == nil {
			continue // should not be
		}
		for _, ma := range clid.IPFS.Addresses {
			if madns.Matches(ma.Value()) { // a dns multiaddress: take it
				delegates = append(delegates, ma.Value())
				continue
			}

			ip, err := ma.ValueForProtocol(multiaddr.P_IP4)
			if err != nil {
				ip, err = ma.ValueForProtocol(multiaddr.P_IP6)
				if err != nil {
					continue
				}
			}
			// We have an IP in the multiaddress. Only include
			// global unicast.
			netip := net.ParseIP(ip)
			if netip == nil {
				continue
			}

			if !netip.IsGlobalUnicast() {
				continue
			}
			delegates = append(delegates, ma.Value())
		}
		status.Delegates = delegates
	}

	status.Info = map[string]string{
		"source":   "IPFS cluster API",
		"warning1": "disregard created time",
		"warning2": "CID used for requestID. Conflicts possible",
		"warning3": "experimenal",
	}
	return status
}

// API implements the REST API Component.
// It embeds a common.API.
type API struct {
	*common.API

	rpcClient *rpc.Client
	config    *Config

	peersMux         sync.RWMutex
	peers            []*types.ID
	peersHaveBeenSet chan struct{}
}

// NewAPI creates a new REST API component.
func NewAPI(ctx context.Context, cfg *Config) (*API, error) {
	return NewAPIWithHost(ctx, cfg, nil)
}

// NewAPI creates a new REST API component using the given libp2p Host.
func NewAPIWithHost(ctx context.Context, cfg *Config, h host.Host) (*API, error) {
	api := API{
		config: cfg,
	}
	capi, err := common.NewAPIWithHost(ctx, &cfg.Config, h, api.routes)
	api.API = capi
	return &api, err
}

// Routes returns endpoints supported by this API.
func (api *API) routes(c *rpc.Client) []common.Route {
	api.rpcClient = c
	go api.refreshPeerset()
	return []common.Route{
		{
			Name:        "AddPin",
			Method:      "POST",
			Pattern:     "/pins",
			HandlerFunc: api.addPin,
		},
		{
			Name:        "GetPin",
			Method:      "GET",
			Pattern:     "/pins/{requestID}",
			HandlerFunc: api.getPin,
		},
		{
			Name:        "ReplacePin",
			Method:      "POST",
			Pattern:     "/pins/{requestID}",
			HandlerFunc: api.addPin,
		},
		{
			Name:        "RemovePin",
			Method:      "DELETE",
			Pattern:     "/pins/{requestID}",
			HandlerFunc: api.removePin,
		},
		{
			Name:        "ListPins",
			Method:      "GET",
			Pattern:     "/pins",
			HandlerFunc: api.listPins,
		},
	}
}

func (api *API) refreshPeerset() {
	t := time.NewTimer(0) // fire asap
	api.peersHaveBeenSet = make(chan struct{})

	for range t.C {
		select {
		case <-api.Context().Done():
			return
		default:
		}

		logger.Debug("Fetching peers for caching")
		ctx, cancel := context.WithTimeout(api.Context(), time.Minute)
		var peers []*types.ID
		err := api.rpcClient.CallContext(
			ctx,
			"",
			"Cluster",
			"Peers",
			struct{}{},
			&peers,
		)
		cancel()

		if err != nil {
			logger.Errorf("error fetching peers for caching: %s", err)
			t.Reset(10 * time.Second)
			continue
		}

		api.peersMux.Lock()
		if api.peers == nil && peers != nil {
			close(api.peersHaveBeenSet)
		}
		if peers != nil {
			api.peers = peers
		}
		api.peersMux.Unlock()
		t.Reset(time.Minute)
	}
}

func (api *API) getPeers() (peers []*types.ID) {
	<-api.peersHaveBeenSet

	api.peersMux.RLock()
	defer api.peersMux.RUnlock()
	return api.peers
}

func (api *API) parseBodyOrFail(w http.ResponseWriter, r *http.Request) *Pin {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var pin Pin
	err := dec.Decode(&pin)
	if err != nil {
		api.SendResponse(w, http.StatusBadRequest, errors.New("error decoding request body"), nil)
		return nil
	}
	return &pin
}

func (api *API) parseRequestIDOrFail(w http.ResponseWriter, r *http.Request) (cid.Cid, bool) {
	vars := mux.Vars(r)
	cStr, ok := vars["requestID"]
	if !ok {
		return cid.Undef, true
	}
	c, err := cid.Decode(cStr)
	if err != nil {
		api.SendResponse(w, http.StatusBadRequest, errors.New("error decoding requestID: "+err.Error()), nil)
		return c, false
	}
	return c, true
}

func (api *API) getPinStatus(ctx context.Context, c cid.Cid) (types.GlobalPinInfo, error) {
	var pinInfo types.GlobalPinInfo

	err := api.rpcClient.CallContext(
		ctx,
		"",
		"Cluster",
		"Status",
		c,
		&pinInfo,
	)
	return pinInfo, err

}

func (api *API) addPin(w http.ResponseWriter, r *http.Request) {
	if pin := api.parseBodyOrFail(w, r); pin != nil {
		api.config.Logger.Debugf("addPin: %s", pin.Cid)
		clusterPin := pin.ToClusterPin()

		if updateCid, ok := api.parseRequestIDOrFail(w, r); updateCid.Defined() && ok {
			clusterPin.PinUpdate = updateCid
		}

		// Pin item
		var pinObj types.Pin
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"Pin",
			clusterPin,
			&pinObj,
		)
		if err != nil {
			api.SendResponse(w, common.SetStatusAutomatically, err, nil)
		}

		// Status is intelligent enough to not request
		// status from remote peers.
		var pinInfo types.GlobalPinInfo

		// Request status until someone is not "unpinned"
		for {
			pinInfo, err = api.getPinStatus(r.Context(), pinObj.Cid)
			if err != nil {
				api.SendResponse(w, common.SetStatusAutomatically, err, nil)
			}

			pinArrived := false
			for _, pi := range pinInfo.PeerMap {
				if pi.Status != types.TrackerStatusUnpinned {
					pinArrived = true
					break
				}
			}

			if pinArrived {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		status := toPinStatus(pinObj.Cid.String(), pinInfo, api.getPeers())
		api.SendResponse(w, common.SetStatusAutomatically, nil, status)
	}
}

func (api *API) getPinObject(ctx context.Context, c cid.Cid) (PinStatus, error) {
	clusterPinStatus, err := api.getPinStatus(ctx, c)
	if err != nil {
		return PinStatus{}, err
	}
	return toPinStatus(c.String(), clusterPinStatus, api.getPeers()), nil

}

func (api *API) getPin(w http.ResponseWriter, r *http.Request) {
	c, ok := api.parseRequestIDOrFail(w, r)
	if !ok {
		return
	}
	api.config.Logger.Debugf("getPin: %s", c)
	status, err := api.getPinObject(r.Context(), c)
	api.SendResponse(w, common.SetStatusAutomatically, err, status)
}

func (api *API) removePin(w http.ResponseWriter, r *http.Request) {
	c, ok := api.parseRequestIDOrFail(w, r)
	if !ok {
		return
	}
	api.config.Logger.Debugf("removePin: %s", c)
	var pinObj types.Pin
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"Unpin",
		types.PinCid(c),
		&pinObj,
	)
	if err != nil && err.Error() == state.ErrNotFound.Error() {
		api.SendResponse(w, http.StatusNotFound, err, nil)
		return
	}
	api.SendResponse(w, http.StatusAccepted, err, nil)
}

const (
	matchUndefined match = iota
	matchExact
	matchIexact
	matchPartial
	matchIpartial
)

type match int

func matchFromString(str string) match {
	switch str {
	case "exact":
		return matchExact
	case "iexact":
		return matchIexact
	case "partial":
		return matchPartial
	case "ipartial":
		return matchIpartial
	default:
		return matchUndefined
	}
}

type listOptions struct {
	Cids   []cid.Cid
	Name   string
	Match  match
	Status types.TrackerStatus
	Before time.Time
	After  time.Time
	Limit  int
	Meta   map[string]string
}

func listOptionsFromQuery(q url.Values) (listOptions, error) {
	lo := listOptions{}

	for _, cstr := range strings.Split(q.Get("cid"), ",") {
		c, err := cid.Decode(cstr)
		if err != nil {
			return lo, fmt.Errorf("error decoding cid %s: %w", cstr, err)
		}
		lo.Cids = append(lo.Cids, c)
	}

	lo.Name = q.Get("name")
	lo.Match = matchFromString(q.Get("match"))
	lo.Status = types.TrackerStatusFromString(q.Get("status"))

	if bef := q.Get("before"); bef != "" {
		err := lo.Before.UnmarshalText([]byte(bef))
		if err != nil {
			return lo, fmt.Errorf("error decoding 'before' query param: %s: %w", bef, err)
		}
	}

	if after := q.Get("after"); after != "" {
		err := lo.After.UnmarshalText([]byte(after))
		if err != nil {
			return lo, fmt.Errorf("error decoding 'after' query param: %s: %w", after, err)
		}
	}

	if v := q.Get("limit"); v != "" {
		lim, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return lo, fmt.Errorf("error parsing 'limit' query param: %s: %w", v, err)
		}
		lo.Limit = int(lim)
	}

	if meta := q.Get("meta"); meta != "" {
		err := json.Unmarshal([]byte(meta), &lo.Meta)
		if err != nil {
			return lo, fmt.Errorf("error unmarshalling 'meta' query param: %s: %w", meta, err)
		}
	}

	return lo, nil
}

func (api *API) listPins(w http.ResponseWriter, r *http.Request) {
	opts, err := listOptionsFromQuery(r.URL.Query())
	if err != nil {
		api.SendResponse(w, common.SetStatusAutomatically, err, nil)
	}

	var pinList PinList
	if len(opts.Cids) > 0 {
		for i, c := range opts.Cids {
			st, err := api.getPinObject(r.Context(), c)
			if err != nil {
				api.SendResponse(w, common.SetStatusAutomatically, err, nil)
				return
			}
			if !st.clusterPinInfo.Match(opts.Status) {
				continue
			}
			pinList.Results = append(pinList.Results, st)
			if i+1 == opts.Limit {
				break
			}
		}
	} else {
		var globalPinInfos []*types.GlobalPinInfo
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"StatusAll",
			opts.Status,
			&globalPinInfos,
		)
		if err != nil {
			api.SendResponse(w, common.SetStatusAutomatically, err, nil)
			return
		}
		for i, gpi := range globalPinInfos {
			st := toPinStatus(gpi.Cid.String(), *gpi, api.getPeers())
			pinList.Results = append(pinList.Results, st)
			if i+1 == opts.Limit {
				break
			}
		}
	}

	pinList.Count = len(pinList.Results)
	api.SendResponse(w, common.SetStatusAutomatically, err, pinList)
}
