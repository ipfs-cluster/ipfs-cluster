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
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	types "github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/api/common"
	"github.com/ipfs-cluster/ipfs-cluster/api/pinsvcapi/pinsvc"
	"github.com/ipfs-cluster/ipfs-cluster/state"
	"go.uber.org/multierr"

	logging "github.com/ipfs/go-log/v2"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	"github.com/libp2p/go-libp2p/core/host"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

var (
	logger    = logging.Logger("pinsvcapi")
	apiLogger = logging.Logger("pinsvcapilog")
)

var apiInfo map[string]string = map[string]string{
	"source":   "IPFS cluster API",
	"warning1": "CID used for requestID. Conflicts possible",
	"warning2": "experimental",
}

func trackerStatusToSvcStatus(st types.TrackerStatus) pinsvc.Status {
	switch {
	case st.Match(types.TrackerStatusError):
		return pinsvc.StatusFailed
	case st.Match(types.TrackerStatusPinQueued):
		return pinsvc.StatusQueued
	case st.Match(types.TrackerStatusPinning):
		return pinsvc.StatusPinning
	case st.Match(types.TrackerStatusPinned):
		return pinsvc.StatusPinned
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

func svcPinToClusterPin(p pinsvc.Pin) (types.Pin, error) {
	opts := types.PinOptions{
		Name:     string(p.Name),
		Origins:  p.Origins,
		Metadata: p.Meta,
		Mode:     types.PinModeRecursive,
	}
	return types.PinWithOpts(p.Cid, opts), nil
}

func globalPinInfoToSvcPinStatus(
	rID string,
	gpi types.GlobalPinInfo,
) pinsvc.PinStatus {

	status := pinsvc.PinStatus{
		RequestID: rID,
	}

	var statusMask types.TrackerStatus
	for _, pinfo := range gpi.PeerMap {
		statusMask |= pinfo.Status
	}

	status.Status = trackerStatusToSvcStatus(statusMask)
	status.Created = gpi.Created
	status.Pin = pinsvc.Pin{
		Cid:     gpi.Cid,
		Name:    pinsvc.PinName(gpi.Name),
		Origins: gpi.Origins,
		Meta:    gpi.Metadata,
	}

	status.Info = apiInfo

	status.Delegates = []types.Multiaddr{}
	for _, pi := range gpi.PeerMap {
		status.Delegates = append(status.Delegates, pi.IPFSAddresses...)
	}

	return status
}

// API implements the REST API Component.
// It embeds a common.API.
type API struct {
	*common.API

	rpcClient *rpc.Client
	config    *Config
}

// NewAPI creates a new REST API component.
func NewAPI(ctx context.Context, cfg *Config) (*API, error) {
	return NewAPIWithHost(ctx, cfg, nil)
}

// NewAPIWithHost creates a new REST API component using the given libp2p Host.
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
	return []common.Route{
		{
			Name:        "ListPins",
			Method:      "GET",
			Pattern:     "/pins",
			HandlerFunc: api.listPins,
		},
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
			Name:        "GetToken",
			Method:      "POST",
			Pattern:     "/token",
			HandlerFunc: api.GenerateTokenHandler,
		},
		{
			Name:        "Health",
			Method:      "GET",
			Pattern:     "/health",
			HandlerFunc: api.HealthHandler,
		},
	}
}

func (api *API) parseBodyOrFail(w http.ResponseWriter, r *http.Request) pinsvc.Pin {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var pin pinsvc.Pin
	err := dec.Decode(&pin)
	if err != nil {
		api.SendResponse(w, http.StatusBadRequest, fmt.Errorf("error decoding request body: %w", err), nil)
		return pinsvc.Pin{}
	}
	return pin
}

func (api *API) parseRequestIDOrFail(w http.ResponseWriter, r *http.Request) (types.Cid, bool) {
	vars := mux.Vars(r)
	cStr, ok := vars["requestID"]
	if !ok {
		return types.CidUndef, true
	}
	c, err := types.DecodeCid(cStr)
	if err != nil {
		api.SendResponse(w, http.StatusBadRequest, errors.New("error decoding requestID: "+err.Error()), nil)
		return c, false
	}
	return c, true
}

func (api *API) addPin(w http.ResponseWriter, r *http.Request) {
	if pin := api.parseBodyOrFail(w, r); pin.Defined() {
		api.config.Logger.Debugf("addPin: %s", pin.Cid)
		clusterPin, err := svcPinToClusterPin(pin)
		if err != nil {
			api.SendResponse(w, common.SetStatusAutomatically, err, nil)
			return
		}

		if updateCid, ok := api.parseRequestIDOrFail(w, r); updateCid.Defined() && ok {
			clusterPin.PinUpdate = updateCid
		}

		// Pin item
		var pinObj types.Pin
		err = api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"Pin",
			clusterPin,
			&pinObj,
		)
		if err != nil {
			api.SendResponse(w, common.SetStatusAutomatically, err, nil)
			return
		}

		// Unpin old item
		if clusterPin.PinUpdate.Defined() {
			var oldPin types.Pin
			err = api.rpcClient.CallContext(
				r.Context(),
				"",
				"Cluster",
				"Unpin",
				types.PinCid(clusterPin.PinUpdate),
				&oldPin,
			)
			if err != nil {
				api.SendResponse(w, common.SetStatusAutomatically, err, nil)
				return
			}
		}

		status := api.pinToSvcPinStatus(r.Context(), pin.Cid.String(), pinObj)
		api.SendResponse(w, common.SetStatusAutomatically, nil, status)
	}
}

func (api *API) getPinSvcStatus(ctx context.Context, c types.Cid) (pinsvc.PinStatus, error) {
	var pinInfo types.GlobalPinInfo

	err := api.rpcClient.CallContext(
		ctx,
		"",
		"Cluster",
		"Status",
		c,
		&pinInfo,
	)
	if err != nil {
		return pinsvc.PinStatus{}, err
	}
	return globalPinInfoToSvcPinStatus(c.String(), pinInfo), nil

}

func (api *API) getPin(w http.ResponseWriter, r *http.Request) {
	c, ok := api.parseRequestIDOrFail(w, r)
	if !ok {
		return
	}
	api.config.Logger.Debugf("getPin: %s", c)
	status, err := api.getPinSvcStatus(r.Context(), c)
	if status.Status == pinsvc.StatusUndefined {
		api.SendResponse(w, http.StatusNotFound, errors.New("pin not found"), nil)
		return
	}
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
		return
	}
	tst := svcStatusToTrackerStatus(opts.Status)

	var pinList pinsvc.PinList
	pinList.Results = []pinsvc.PinStatus{}
	count := uint64(0)

	if len(opts.Cids) > 0 {
		// copy approach from restapi
		type statusResult struct {
			st  pinsvc.PinStatus
			err error
		}
		stCh := make(chan statusResult, len(opts.Cids))
		var wg sync.WaitGroup
		wg.Add(len(opts.Cids))

		go func() {
			wg.Wait()
			close(stCh)
		}()

		for _, ci := range opts.Cids {
			go func(c types.Cid) {
				defer wg.Done()
				st, err := api.getPinSvcStatus(r.Context(), c)
				stCh <- statusResult{st: st, err: err}
			}(ci)
		}

		var err error

		for stResult := range stCh {
			if stResult.st.Status == pinsvc.StatusUndefined && stResult.err == nil {
				// ignore things unpinning
				continue
			}

			if count < opts.Limit {
				pinList.Results = append(pinList.Results, stResult.st)
				err = multierr.Append(err, stResult.err)
			}
			count++
		}

		if err != nil {
			api.SendResponse(w, common.SetStatusAutomatically, err, nil)
			return
		}
	} else {
		in := make(chan types.TrackerStatus, 1)
		in <- tst
		close(in)
		out := make(chan types.GlobalPinInfo, common.StreamChannelSize)
		errCh := make(chan error, 1)

		go func() {
			defer close(errCh)

			errCh <- api.rpcClient.Stream(
				r.Context(),
				"",
				"Cluster",
				"StatusAll",
				in,
				out,
			)
		}()

		for gpi := range out {
			st := globalPinInfoToSvcPinStatus(gpi.Cid.String(), gpi)
			if st.Status == pinsvc.StatusUndefined {
				// i.e things unpinning
				continue
			}
			if !opts.After.IsZero() && st.Created.Before(opts.After) {
				continue
			}

			if !opts.Before.IsZero() && st.Created.After(opts.Before) {
				continue
			}

			if !st.Pin.MatchesName(opts.Name, opts.MatchingStrategy) {
				continue
			}
			if !st.Pin.MatchesMeta(opts.Meta) {
				continue
			}
			if count < opts.Limit {
				pinList.Results = append(pinList.Results, st)
			}
			count++
		}

		err := <-errCh
		if err != nil {
			api.SendResponse(w, common.SetStatusAutomatically, err, nil)
			return
		}
	}

	pinList.Count = count
	api.SendResponse(w, common.SetStatusAutomatically, err, pinList)
}

func (api *API) pinToSvcPinStatus(ctx context.Context, rID string, pin types.Pin) pinsvc.PinStatus {
	status := pinsvc.PinStatus{
		RequestID: rID,
		Status:    pinsvc.StatusQueued,
		Created:   pin.Timestamp,
		Pin: pinsvc.Pin{
			Cid:     pin.Cid,
			Name:    pinsvc.PinName(pin.Name),
			Origins: pin.Origins,
			Meta:    pin.Metadata,
		},
		Info: apiInfo,
	}

	var peers []peer.ID

	if pin.IsPinEverywhere() { // all cluster peers
		err := api.rpcClient.CallContext(
			ctx,
			"",
			"Consensus",
			"Peers",
			struct{}{},
			&peers,
		)
		if err != nil {
			logger.Error(err)
		}
	} else { // Delegates should come from allocations
		peers = pin.Allocations
	}

	status.Delegates = []types.Multiaddr{}
	for _, peer := range peers {
		var ipfsid types.IPFSID
		err := api.rpcClient.CallContext(
			ctx,
			"", // call the local peer
			"Cluster",
			"IPFSID",
			peer, // retrieve ipfs info for this peer
			&ipfsid,
		)
		if err != nil {
			logger.Error(err)
		}
		status.Delegates = append(status.Delegates, ipfsid.Addresses...)
	}

	return status
}
