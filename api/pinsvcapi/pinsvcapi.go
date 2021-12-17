// Package pinsvcapi implements an IPFS Cluster API component which provides
// an IPFS Pinning Services API to the cluster.
//
// The implented API is based on the common.API component (refer to module
// description there). The only thing this module does is to provide route
// handling for the otherwise common API component.
package pinsvcapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ipfs/go-cid"
	types "github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/api/common"
	"github.com/ipfs/ipfs-cluster/api/pinsvcapi/pinsvc"
	"github.com/ipfs/ipfs-cluster/state"
	"github.com/multiformats/go-multiaddr"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	madns "github.com/multiformats/go-multiaddr-dns"
)

var (
	logger    = logging.Logger("pinsvcapi")
	apiLogger = logging.Logger("pinsvcapilog")
)

func trackerStatusToSvcStatus(st types.TrackerStatus) pinsvc.Status {
	switch {
	case st.Match(types.TrackerStatusError):
		return pinsvc.StatusFailed
	case st.Match(types.TrackerStatusPinQueued):
		return pinsvc.StatusQueued
	case st.Match(types.TrackerStatusPinning):
		return pinsvc.StatusPinned
	case st.Match(types.TrackerStatusPinned):
		return pinsvc.StatusPinning
	default:
		return pinsvc.StatusUndefined
	}
}

func svcStatusToTrackerStatus(st pinsvc.Status) types.TrackerStatus {
	var tst types.TrackerStatus

	if st.Match(pinsvc.StatusFailed) {
		tst |= types.TrackerStatusError
	}
	if st.Match(pinsvc.StatusQueued) {
		tst |= types.TrackerStatusPinQueued
	}
	if st.Match(pinsvc.StatusPinned) {
		tst |= types.TrackerStatusPinned
	}
	if st.Match(pinsvc.StatusPinning) {
		tst |= types.TrackerStatusPinning
	}
	return tst
}

func svcPinToClusterPin(p pinsvc.Pin) *types.Pin {
	opts := types.PinOptions{
		Name:     p.Name,
		Origins:  p.Origins,
		Metadata: p.Meta,
		Mode:     types.PinModeRecursive,
	}
	return types.PinWithOpts(p.Cid, opts)
}

func globalPinInfoToSvcPinStatus(
	rID string,
	gpi types.GlobalPinInfo,
	clusterIDs []*types.ID,
) pinsvc.PinStatus {

	status := pinsvc.PinStatus{
		RequestID: rID,
	}

	var statusMask types.TrackerStatus
	for _, pinfo := range gpi.PeerMap {
		statusMask |= pinfo.Status
	}

	status.Status = trackerStatusToSvcStatus(statusMask)
	status.Created = time.Now()
	status.Pin = pinsvc.Pin{
		Cid:     gpi.Cid,
		Name:    gpi.Name,
		Origins: gpi.Origins,
		Meta:    gpi.Metadata,
	}

	delegates := []types.Multiaddr{}
	idMap := make(map[peer.ID]*types.ID)
	for _, clusterID := range clusterIDs {
		idMap[clusterID.ID] = clusterID
	}
	filteredClusterIDs := clusterIDs
	if len(gpi.Allocations) > 0 {
		filteredClusterIDs = []*types.ID{}
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
				delegates = append(delegates, ma)
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
			delegates = append(delegates, ma)
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

func (api *API) parseBodyOrFail(w http.ResponseWriter, r *http.Request) *pinsvc.Pin {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var pin pinsvc.Pin
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
		clusterPin := svcPinToClusterPin(*pin)

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

		status := globalPinInfoToSvcPinStatus(pinObj.Cid.String(), pinInfo, api.getPeers())
		api.SendResponse(w, common.SetStatusAutomatically, nil, status)
	}
}

func (api *API) getPinObject(ctx context.Context, c cid.Cid) (pinsvc.PinStatus, types.GlobalPinInfo, error) {
	clusterPinStatus, err := api.getPinStatus(ctx, c)
	if err != nil {
		return pinsvc.PinStatus{}, types.GlobalPinInfo{}, err
	}
	return globalPinInfoToSvcPinStatus(c.String(), clusterPinStatus, api.getPeers()), clusterPinStatus, nil

}

func (api *API) getPin(w http.ResponseWriter, r *http.Request) {
	c, ok := api.parseRequestIDOrFail(w, r)
	if !ok {
		return
	}
	api.config.Logger.Debugf("getPin: %s", c)
	status, _, err := api.getPinObject(r.Context(), c)
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

func (api *API) listPins(w http.ResponseWriter, r *http.Request) {
	opts := &pinsvc.ListOptions{}
	err := opts.FromQuery(r.URL.Query())
	if err != nil {
		api.SendResponse(w, common.SetStatusAutomatically, err, nil)
	}
	tst := svcStatusToTrackerStatus(opts.Status)

	var pinList pinsvc.PinList
	if len(opts.Cids) > 0 {
		for i, c := range opts.Cids {
			st, gpi, err := api.getPinObject(r.Context(), c)
			if err != nil {
				api.SendResponse(w, common.SetStatusAutomatically, err, nil)
				return
			}
			if !gpi.Match(tst) {
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
			st := globalPinInfoToSvcPinStatus(gpi.Cid.String(), *gpi, api.getPeers())
			pinList.Results = append(pinList.Results, st)
			if i+1 == opts.Limit {
				break
			}
		}
	}

	pinList.Count = len(pinList.Results)
	api.SendResponse(w, common.SetStatusAutomatically, err, pinList)
}
