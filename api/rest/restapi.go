// Package rest implements an IPFS Cluster API component. It provides
// a REST-ish API to interact with Cluster.
//
// The implented API is based on the common.API component (refer to module
// description there). The only thing this module does is to provide route
// handling for the otherwise common API component.
package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/ipfs-cluster/ipfs-cluster/adder/adderutils"
	types "github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/api/common"

	logging "github.com/ipfs/go-log/v2"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	"github.com/libp2p/go-libp2p/core/host"
	peer "github.com/libp2p/go-libp2p/core/peer"

	mux "github.com/gorilla/mux"
)

var (
	logger    = logging.Logger("restapi")
	apiLogger = logging.Logger("restapilog")
)

type peerAddBody struct {
	PeerID string `json:"peer_id"`
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
			Name:        "ID",
			Method:      "GET",
			Pattern:     "/id",
			HandlerFunc: api.idHandler,
		},

		{
			Name:        "Version",
			Method:      "GET",
			Pattern:     "/version",
			HandlerFunc: api.versionHandler,
		},

		{
			Name:        "Peers",
			Method:      "GET",
			Pattern:     "/peers",
			HandlerFunc: api.peerListHandler,
		},
		{
			Name:        "PeerAdd",
			Method:      "POST",
			Pattern:     "/peers",
			HandlerFunc: api.peerAddHandler,
		},
		{
			Name:        "PeerRemove",
			Method:      "DELETE",
			Pattern:     "/peers/{peer}",
			HandlerFunc: api.peerRemoveHandler,
		},
		{
			Name:        "Add",
			Method:      "POST",
			Pattern:     "/add",
			HandlerFunc: api.addHandler,
		},
		{
			Name:        "Allocations",
			Method:      "GET",
			Pattern:     "/allocations",
			HandlerFunc: api.allocationsHandler,
		},
		{
			Name:        "Allocation",
			Method:      "GET",
			Pattern:     "/allocations/{hash}",
			HandlerFunc: api.allocationHandler,
		},
		{
			Name:        "StatusAll",
			Method:      "GET",
			Pattern:     "/pins",
			HandlerFunc: api.statusAllHandler,
		},
		{
			Name:        "Recover",
			Method:      "POST",
			Pattern:     "/pins/{hash}/recover",
			HandlerFunc: api.recoverHandler,
		},
		{
			Name:        "RecoverAll",
			Method:      "POST",
			Pattern:     "/pins/recover",
			HandlerFunc: api.recoverAllHandler,
		},
		{
			Name:        "Status",
			Method:      "GET",
			Pattern:     "/pins/{hash}",
			HandlerFunc: api.statusHandler,
		},
		{
			Name:        "Pin",
			Method:      "POST",
			Pattern:     "/pins/{hash}",
			HandlerFunc: api.pinHandler,
		},
		{
			Name:        "PinPath",
			Method:      "POST",
			Pattern:     "/pins/{keyType:ipfs|ipns|ipld}/{path:.*}",
			HandlerFunc: api.pinPathHandler,
		},
		{
			Name:        "Unpin",
			Method:      "DELETE",
			Pattern:     "/pins/{hash}",
			HandlerFunc: api.unpinHandler,
		},
		{
			Name:        "UnpinPath",
			Method:      "DELETE",
			Pattern:     "/pins/{keyType:ipfs|ipns|ipld}/{path:.*}",
			HandlerFunc: api.unpinPathHandler,
		},
		{
			Name:        "RepoGC",
			Method:      "POST",
			Pattern:     "/ipfs/gc",
			HandlerFunc: api.repoGCHandler,
		},
		{
			Name:        "ConnectionGraph",
			Method:      "GET",
			Pattern:     "/health/graph",
			HandlerFunc: api.graphHandler,
		},
		{
			Name:        "Alerts",
			Method:      "GET",
			Pattern:     "/health/alerts",
			HandlerFunc: api.alertsHandler,
		},
		{
			Name:        "Bandwidth by protocol stats",
			Method:      "GET",
			Pattern:     "/health/bandwidth",
			HandlerFunc: api.bandwidthByProtocolHandler,
		},
		{
			Name:        "Metrics",
			Method:      "GET",
			Pattern:     "/monitor/metrics/{name}",
			HandlerFunc: api.metricsHandler,
		},
		{
			Name:        "MetricNames",
			Method:      "GET",
			Pattern:     "/monitor/metrics",
			HandlerFunc: api.metricNamesHandler,
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

func (api *API) idHandler(w http.ResponseWriter, r *http.Request) {
	var id types.ID
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"ID",
		struct{}{},
		&id,
	)

	api.SendResponse(w, common.SetStatusAutomatically, err, &id)
}

func (api *API) versionHandler(w http.ResponseWriter, r *http.Request) {
	var v types.Version
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"Version",
		struct{}{},
		&v,
	)

	api.SendResponse(w, common.SetStatusAutomatically, err, v)
}

func (api *API) graphHandler(w http.ResponseWriter, r *http.Request) {
	var graph types.ConnectGraph
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"ConnectGraph",
		struct{}{},
		&graph,
	)
	api.SendResponse(w, common.SetStatusAutomatically, err, graph)
}

func (api *API) metricsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	var metrics []types.Metric
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"PeerMonitor",
		"LatestMetrics",
		name,
		&metrics,
	)
	api.SendResponse(w, common.SetStatusAutomatically, err, metrics)
}

func (api *API) metricNamesHandler(w http.ResponseWriter, r *http.Request) {
	var metricNames []string
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"PeerMonitor",
		"MetricNames",
		struct{}{},
		&metricNames,
	)
	api.SendResponse(w, common.SetStatusAutomatically, err, metricNames)
}

func (api *API) alertsHandler(w http.ResponseWriter, r *http.Request) {
	var alerts []types.Alert
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"Alerts",
		struct{}{},
		&alerts,
	)
	api.SendResponse(w, common.SetStatusAutomatically, err, alerts)
}

func (api *API) bandwidthByProtocolHandler(w http.ResponseWriter, r *http.Request) {
	var bw types.BandwidthByProtocol
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"BandwidthByProtocol",
		struct{}{},
		&bw,
	)
	api.SendResponse(w, common.SetStatusAutomatically, err, bw)
}

func (api *API) addHandler(w http.ResponseWriter, r *http.Request) {
	reader, err := r.MultipartReader()
	if err != nil {
		api.SendResponse(w, http.StatusBadRequest, err, nil)
		return
	}

	params, err := types.AddParamsFromQuery(r.URL.Query())
	if err != nil {
		api.SendResponse(w, http.StatusBadRequest, err, nil)
		return
	}

	api.SetHeaders(w)

	// any errors sent as trailer
	adderutils.AddMultipartHTTPHandler(
		r.Context(),
		api.rpcClient,
		params,
		reader,
		w,
		nil,
	)
}

func (api *API) peerListHandler(w http.ResponseWriter, r *http.Request) {
	in := make(chan struct{})
	close(in)
	out := make(chan types.ID, common.StreamChannelSize)
	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)

		errCh <- api.rpcClient.Stream(
			r.Context(),
			"",
			"Cluster",
			"Peers",
			in,
			out,
		)
	}()

	iter := func() (interface{}, bool, error) {
		p, ok := <-out
		return p, ok, nil
	}
	api.StreamResponse(w, iter, errCh)
}

func (api *API) peerAddHandler(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var addInfo peerAddBody
	err := dec.Decode(&addInfo)
	if err != nil {
		api.SendResponse(w, http.StatusBadRequest, errors.New("error decoding request body"), nil)
		return
	}

	pid, err := peer.Decode(addInfo.PeerID)
	if err != nil {
		api.SendResponse(w, http.StatusBadRequest, errors.New("error decoding peer_id"), nil)
		return
	}

	var id types.ID
	err = api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"PeerAdd",
		pid,
		&id,
	)
	api.SendResponse(w, common.SetStatusAutomatically, err, &id)
}

func (api *API) peerRemoveHandler(w http.ResponseWriter, r *http.Request) {
	if p := api.ParsePidOrFail(w, r); p != "" {
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"PeerRemove",
			p,
			&struct{}{},
		)
		api.SendResponse(w, common.SetStatusAutomatically, err, nil)
	}
}

func (api *API) pinHandler(w http.ResponseWriter, r *http.Request) {
	if pin := api.ParseCidOrFail(w, r); pin.Defined() {
		api.config.Logger.Debugf("rest api pinHandler: %s", pin.Cid)
		// span.AddAttributes(trace.StringAttribute("cid", pin.Cid))
		var pinObj types.Pin
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"Pin",
			pin,
			&pinObj,
		)
		api.SendResponse(w, common.SetStatusAutomatically, err, pinObj)
		api.config.Logger.Debug("rest api pinHandler done")
	}
}

func (api *API) unpinHandler(w http.ResponseWriter, r *http.Request) {
	if pin := api.ParseCidOrFail(w, r); pin.Defined() {
		api.config.Logger.Debugf("rest api unpinHandler: %s", pin.Cid)
		// span.AddAttributes(trace.StringAttribute("cid", pin.Cid))
		var pinObj types.Pin
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"Unpin",
			pin,
			&pinObj,
		)
		api.SendResponse(w, common.SetStatusAutomatically, err, pinObj)
		api.config.Logger.Debug("rest api unpinHandler done")
	}
}

func (api *API) pinPathHandler(w http.ResponseWriter, r *http.Request) {
	var pin types.Pin
	if pinpath := api.ParsePinPathOrFail(w, r); pinpath.Defined() {
		api.config.Logger.Debugf("rest api pinPathHandler: %s", pinpath.Path)
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"PinPath",
			pinpath,
			&pin,
		)

		api.SendResponse(w, common.SetStatusAutomatically, err, pin)
		api.config.Logger.Debug("rest api pinPathHandler done")
	}
}

func (api *API) unpinPathHandler(w http.ResponseWriter, r *http.Request) {
	var pin types.Pin
	if pinpath := api.ParsePinPathOrFail(w, r); pinpath.Defined() {
		api.config.Logger.Debugf("rest api unpinPathHandler: %s", pinpath.Path)
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"UnpinPath",
			pinpath,
			&pin,
		)
		api.SendResponse(w, common.SetStatusAutomatically, err, pin)
		api.config.Logger.Debug("rest api unpinPathHandler done")
	}
}

func (api *API) allocationsHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	filterStr := queryValues.Get("filter")
	var filter types.PinType
	for _, f := range strings.Split(filterStr, ",") {
		filter |= types.PinTypeFromString(f)
	}

	if filter == types.BadType {
		api.SendResponse(w, http.StatusBadRequest, errors.New("invalid filter value"), nil)
		return
	}

	in := make(chan struct{})
	close(in)

	out := make(chan types.Pin, common.StreamChannelSize)
	errCh := make(chan error, 1)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() {
		defer close(errCh)

		errCh <- api.rpcClient.Stream(
			r.Context(),
			"",
			"Cluster",
			"Pins",
			in,
			out,
		)
	}()

	iter := func() (interface{}, bool, error) {
		var p types.Pin
		var ok bool
	iterloop:
		for {

			select {
			case <-ctx.Done():
				break iterloop
			case p, ok = <-out:
				if !ok {
					break iterloop
				}
				// this means we keep iterating if no filter
				// matched
				if filter == types.AllType || filter&p.Type > 0 {
					break iterloop
				}
			}
		}
		return p, ok, ctx.Err()
	}

	api.StreamResponse(w, iter, errCh)
}

func (api *API) allocationHandler(w http.ResponseWriter, r *http.Request) {
	if pin := api.ParseCidOrFail(w, r); pin.Defined() {
		var pinResp types.Pin
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"PinGet",
			pin.Cid,
			&pinResp,
		)
		api.SendResponse(w, common.SetStatusAutomatically, err, pinResp)
	}
}

func (api *API) statusAllHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	queryValues := r.URL.Query()
	if queryValues.Get("cids") != "" {
		api.statusCidsHandler(w, r)
		return
	}

	local := queryValues.Get("local")

	filterStr := queryValues.Get("filter")
	filter := types.TrackerStatusFromString(filterStr)
	// FIXME: This is a bit lazy, as "invalidxx,pinned" would result in a
	// valid "pinned" filter.
	if filter == types.TrackerStatusUndefined && filterStr != "" {
		api.SendResponse(w, http.StatusBadRequest, errors.New("invalid filter value"), nil)
		return
	}

	var iter common.StreamIterator
	in := make(chan types.TrackerStatus, 1)
	in <- filter
	close(in)
	errCh := make(chan error, 1)

	if local == "true" {
		out := make(chan types.PinInfo, common.StreamChannelSize)
		iter = func() (interface{}, bool, error) {
			select {
			case <-ctx.Done():
				return nil, false, ctx.Err()
			case p, ok := <-out:
				return p.ToGlobal(), ok, nil
			}
		}

		go func() {
			defer close(errCh)

			errCh <- api.rpcClient.Stream(
				r.Context(),
				"",
				"Cluster",
				"StatusAllLocal",
				in,
				out,
			)
		}()

	} else {
		out := make(chan types.GlobalPinInfo, common.StreamChannelSize)
		iter = func() (interface{}, bool, error) {
			select {
			case <-ctx.Done():
				return nil, false, ctx.Err()
			case p, ok := <-out:
				return p, ok, nil
			}
		}
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
	}

	api.StreamResponse(w, iter, errCh)
}

// request statuses for multiple CIDs in parallel.
func (api *API) statusCidsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	queryValues := r.URL.Query()
	filterCidsStr := strings.Split(queryValues.Get("cids"), ",")
	var cids []types.Cid

	for _, cidStr := range filterCidsStr {
		c, err := types.DecodeCid(cidStr)
		if err != nil {
			api.SendResponse(w, http.StatusBadRequest, fmt.Errorf("error decoding Cid: %w", err), nil)
			return
		}
		cids = append(cids, c)
	}

	local := queryValues.Get("local")

	gpiCh := make(chan types.GlobalPinInfo, len(cids))
	errCh := make(chan error, len(cids))
	var wg sync.WaitGroup
	wg.Add(len(cids))

	// Close channel when done
	go func() {
		wg.Wait()
		close(errCh)
		close(gpiCh)
	}()

	if local == "true" {
		for _, ci := range cids {
			go func(c types.Cid) {
				defer wg.Done()
				var pinInfo types.PinInfo
				err := api.rpcClient.CallContext(
					ctx,
					"",
					"Cluster",
					"StatusLocal",
					c,
					&pinInfo,
				)
				if err != nil {
					errCh <- err
					return
				}
				gpiCh <- pinInfo.ToGlobal()
			}(ci)
		}
	} else {
		for _, ci := range cids {
			go func(c types.Cid) {
				defer wg.Done()
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
					errCh <- err
					return
				}
				gpiCh <- pinInfo
			}(ci)
		}
	}

	iter := func() (interface{}, bool, error) {
		gpi, ok := <-gpiCh
		return gpi, ok, nil
	}

	api.StreamResponse(w, iter, errCh)
}

func (api *API) statusHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	if pin := api.ParseCidOrFail(w, r); pin.Defined() {
		if local == "true" {
			var pinInfo types.PinInfo
			err := api.rpcClient.CallContext(
				r.Context(),
				"",
				"Cluster",
				"StatusLocal",
				pin.Cid,
				&pinInfo,
			)
			api.SendResponse(w, common.SetStatusAutomatically, err, pinInfo.ToGlobal())
		} else {
			var pinInfo types.GlobalPinInfo
			err := api.rpcClient.CallContext(
				r.Context(),
				"",
				"Cluster",
				"Status",
				pin.Cid,
				&pinInfo,
			)
			api.SendResponse(w, common.SetStatusAutomatically, err, pinInfo)
		}
	}
}

func (api *API) recoverAllHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	var iter common.StreamIterator
	in := make(chan struct{})
	close(in)
	errCh := make(chan error, 1)

	if local == "true" {
		out := make(chan types.PinInfo, common.StreamChannelSize)
		iter = func() (interface{}, bool, error) {
			select {
			case <-ctx.Done():
				return nil, false, ctx.Err()
			case p, ok := <-out:
				return p.ToGlobal(), ok, nil
			}
		}

		go func() {
			defer close(errCh)

			errCh <- api.rpcClient.Stream(
				r.Context(),
				"",
				"Cluster",
				"RecoverAllLocal",
				in,
				out,
			)
		}()

	} else {
		out := make(chan types.GlobalPinInfo, common.StreamChannelSize)
		iter = func() (interface{}, bool, error) {
			select {
			case <-ctx.Done():
				return nil, false, ctx.Err()
			case p, ok := <-out:
				return p, ok, nil
			}
		}
		go func() {
			defer close(errCh)

			errCh <- api.rpcClient.Stream(
				r.Context(),
				"",
				"Cluster",
				"RecoverAll",
				in,
				out,
			)
		}()
	}

	api.StreamResponse(w, iter, errCh)
}

func (api *API) recoverHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	if pin := api.ParseCidOrFail(w, r); pin.Defined() {
		if local == "true" {
			var pinInfo types.PinInfo
			err := api.rpcClient.CallContext(
				r.Context(),
				"",
				"Cluster",
				"RecoverLocal",
				pin.Cid,
				&pinInfo,
			)
			api.SendResponse(w, common.SetStatusAutomatically, err, pinInfo.ToGlobal())
		} else {
			var pinInfo types.GlobalPinInfo
			err := api.rpcClient.CallContext(
				r.Context(),
				"",
				"Cluster",
				"Recover",
				pin.Cid,
				&pinInfo,
			)
			api.SendResponse(w, common.SetStatusAutomatically, err, pinInfo)
		}
	}
}

func (api *API) repoGCHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	if local == "true" {
		var localRepoGC types.RepoGC
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"RepoGCLocal",
			struct{}{},
			&localRepoGC,
		)

		api.SendResponse(w, common.SetStatusAutomatically, err, repoGCToGlobal(localRepoGC))
		return
	}

	var repoGC types.GlobalRepoGC
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"RepoGC",
		struct{}{},
		&repoGC,
	)
	api.SendResponse(w, common.SetStatusAutomatically, err, repoGC)
}

func repoGCToGlobal(r types.RepoGC) types.GlobalRepoGC {
	return types.GlobalRepoGC{
		PeerMap: map[string]types.RepoGC{
			r.Peer.String(): r,
		},
	}
}
