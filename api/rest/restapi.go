// Package rest implements an IPFS Cluster API component. It provides
// a REST-ish API to interact with Cluster.
//
// rest exposes the HTTP API in two ways. The first is through a regular
// HTTP(s) listener. The second is by tunneling HTTP through a libp2p
// stream (thus getting an encrypted channel without the need to setup
// TLS). Both ways can be used at the same time, or disabled.
package rest

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/cors"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/plugin/ochttp/propagation/tracecontext"
	"go.opencensus.io/trace"

	"github.com/ipfs/ipfs-cluster/adder/adderutils"
	types "github.com/ipfs/ipfs-cluster/api"

	mux "github.com/gorilla/mux"
	gostream "github.com/hsanjuan/go-libp2p-gostream"
	p2phttp "github.com/hsanjuan/go-libp2p-http"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	libp2p "github.com/libp2p/go-libp2p"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var logger = logging.Logger("restapi")

// Common errors
var (
	// ErrNoEndpointEnabled is returned when the API is created but
	// no HTTPListenAddr, nor libp2p configuration fields, nor a libp2p
	// Host are provided.
	ErrNoEndpointsEnabled = errors.New("neither the libp2p nor the HTTP endpoints are enabled")

	// ErrHTTPEndpointNotEnabled is returned when trying to perform
	// operations that rely on the HTTPEndpoint but it is disabled.
	ErrHTTPEndpointNotEnabled = errors.New("the HTTP endpoint is not enabled")
)

// Used by sendResponse to set the right status
const autoStatus = -1

// For making a random sharding ID
var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// API implements an API and aims to provides
// a RESTful HTTP API for Cluster.
type API struct {
	ctx    context.Context
	cancel func()

	config *Config

	rpcClient *rpc.Client
	rpcReady  chan struct{}
	router    *mux.Router

	server *http.Server
	host   host.Host

	httpListener   net.Listener
	libp2pListener net.Listener

	shutdownLock sync.Mutex
	shutdown     bool
	wg           sync.WaitGroup
}

type route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

type peerAddBody struct {
	PeerID string `json:"peer_id"`
}

// NewAPI creates a new REST API component with the given configuration.
func NewAPI(ctx context.Context, cfg *Config) (*API, error) {
	return NewAPIWithHost(ctx, cfg, nil)
}

// NewAPIWithHost creates a new REST API component and enables
// the libp2p-http endpoint using the given Host, if not nil.
func NewAPIWithHost(ctx context.Context, cfg *Config, h host.Host) (*API, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	// Our handler is a gorilla router,
	// wrapped with the cors handler,
	// wrapped with the basic auth handler.
	router := mux.NewRouter().StrictSlash(true)
	handler := basicAuthHandler(
		cfg.BasicAuthCreds,
		cors.New(*cfg.corsOptions()).Handler(router),
	)
	if cfg.Tracing {
		handler = &ochttp.Handler{
			IsPublicEndpoint: true,
			Propagation:      &tracecontext.HTTPFormat{},
			Handler:          handler,
			StartOptions:     trace.StartOptions{SpanKind: trace.SpanKindServer},
			FormatSpanName:   func(req *http.Request) string { return req.Host + ":" + req.URL.Path + ":" + req.Method },
		}
	}
	s := &http.Server{
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		Handler:           handler,
	}

	// See: https://github.com/ipfs/go-ipfs/issues/5168
	// See: https://github.com/ipfs/ipfs-cluster/issues/548
	// on why this is re-enabled.
	s.SetKeepAlivesEnabled(true)

	ctx, cancel := context.WithCancel(ctx)

	api := &API{
		ctx:      ctx,
		cancel:   cancel,
		config:   cfg,
		server:   s,
		host:     h,
		rpcReady: make(chan struct{}, 2),
	}
	api.addRoutes(router)

	// Set up api.httpListener if enabled
	err = api.setupHTTP(ctx)
	if err != nil {
		return nil, err
	}

	// Set up api.libp2pListener if enabled
	err = api.setupLibp2p(ctx)
	if err != nil {
		return nil, err
	}

	if api.httpListener == nil && api.libp2pListener == nil {
		return nil, ErrNoEndpointsEnabled
	}

	api.run(ctx)
	return api, nil
}

func (api *API) setupHTTP(ctx context.Context) error {
	if api.config.HTTPListenAddr == nil {
		return nil
	}

	n, addr, err := manet.DialArgs(api.config.HTTPListenAddr)
	if err != nil {
		return err
	}

	var l net.Listener
	if api.config.TLS != nil {
		l, err = tls.Listen(n, addr, api.config.TLS)
	} else {
		l, err = net.Listen(n, addr)
	}
	if err != nil {
		return err
	}
	api.httpListener = l
	return nil
}

func (api *API) setupLibp2p(ctx context.Context) error {
	// Make new host. Override any provided existing one
	// if we have config for a custom one.
	if api.config.Libp2pListenAddr != nil {
		h, err := libp2p.New(
			ctx,
			libp2p.Identity(api.config.PrivateKey),
			libp2p.ListenAddrs([]ma.Multiaddr{api.config.Libp2pListenAddr}...),
		)
		if err != nil {
			return err
		}
		api.host = h
	}

	if api.host == nil {
		return nil
	}

	l, err := gostream.Listen(api.host, p2phttp.DefaultP2PProtocol)
	if err != nil {
		return err
	}
	api.libp2pListener = l
	return nil
}

// HTTPAddress returns the HTTP(s) listening address
// in host:port format. Useful when configured to start
// on a random port (0). Returns error when the HTTP endpoint
// is not enabled.
func (api *API) HTTPAddress() (string, error) {
	if api.httpListener == nil {
		return "", ErrHTTPEndpointNotEnabled
	}
	return api.httpListener.Addr().String(), nil
}

// Host returns the libp2p Host used by the API, if any.
// The result is either the host provided during initialization,
// a default Host created with options from the configuration object,
// or nil.
func (api *API) Host() host.Host {
	return api.host
}

func (api *API) addRoutes(router *mux.Router) {
	for _, route := range api.routes() {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(
				ochttp.WithRouteTag(
					http.HandlerFunc(route.HandlerFunc),
					"/"+route.Name,
				),
			)
	}
	api.router = router
}

// basicAuth wraps a given handler with basic authentication
func basicAuthHandler(credentials map[string]string, h http.Handler) http.Handler {
	if credentials == nil {
		return h
	}

	wrap := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		username, password, ok := r.BasicAuth()
		if !ok {
			resp, err := unauthorizedResp()
			if err != nil {
				logger.Error(err)
				return
			}
			http.Error(w, resp, 401)
			return
		}

		authorized := false
		for u, p := range credentials {
			if u == username && p == password {
				authorized = true
			}
		}
		if !authorized {
			resp, err := unauthorizedResp()
			if err != nil {
				logger.Error(err)
				return
			}
			http.Error(w, resp, 401)
			return
		}
		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(wrap)
}

func unauthorizedResp() (string, error) {
	apiError := types.Error{
		Code:    401,
		Message: "Unauthorized",
	}
	resp, err := json.Marshal(apiError)
	return string(resp), err
}

func (api *API) routes() []route {
	return []route{
		{
			"ID",
			"GET",
			"/id",
			api.idHandler,
		},

		{
			"Version",
			"GET",
			"/version",
			api.versionHandler,
		},

		{
			"Peers",
			"GET",
			"/peers",
			api.peerListHandler,
		},
		{
			"PeerAdd",
			"POST",
			"/peers",
			api.peerAddHandler,
		},
		{
			"PeerRemove",
			"DELETE",
			"/peers/{peer}",
			api.peerRemoveHandler,
		},
		{
			"Add",
			"POST",
			"/add",
			api.addHandler,
		},
		{
			"Allocations",
			"GET",
			"/allocations",
			api.allocationsHandler,
		},
		{
			"Allocation",
			"GET",
			"/allocations/{hash}",
			api.allocationHandler,
		},
		{
			"StatusAll",
			"GET",
			"/pins",
			api.statusAllHandler,
		},
		{
			"SyncAll",
			"POST",
			"/pins/sync",
			api.syncAllHandler,
		},
		{
			"RecoverAll",
			"POST",
			"/pins/recover",
			api.recoverAllHandler,
		},
		{
			"Status",
			"GET",
			"/pins/{hash}",
			api.statusHandler,
		},
		{
			"Pin",
			"POST",
			"/pins/{hash}",
			api.pinHandler,
		},
		{
			"Unpin",
			"DELETE",
			"/pins/{hash}",
			api.unpinHandler,
		},
		{
			"Sync",
			"POST",
			"/pins/{hash}/sync",
			api.syncHandler,
		},
		{
			"Recover",
			"POST",
			"/pins/{hash}/recover",
			api.recoverHandler,
		},
		{
			"ConnectionGraph",
			"GET",
			"/health/graph",
			api.graphHandler,
		},
		{
			"Metrics",
			"GET",
			"/monitor/metrics/{name}",
			api.metricsHandler,
		},
	}
}

func (api *API) run(ctx context.Context) {
	if api.httpListener != nil {
		api.wg.Add(1)
		go api.runHTTPServer(ctx)
	}

	if api.libp2pListener != nil {
		api.wg.Add(1)
		go api.runLibp2pServer(ctx)
	}
}

// runs in goroutine from run()
func (api *API) runHTTPServer(ctx context.Context) {
	defer api.wg.Done()
	<-api.rpcReady

	logger.Infof("REST API (HTTP): %s", api.config.HTTPListenAddr)
	err := api.server.Serve(api.httpListener)
	if err != nil && !strings.Contains(err.Error(), "closed network connection") {
		logger.Error(err)
	}
}

// runs in goroutine from run()
func (api *API) runLibp2pServer(ctx context.Context) {
	defer api.wg.Done()
	<-api.rpcReady

	listenMsg := ""
	for _, a := range api.host.Addrs() {
		listenMsg += fmt.Sprintf("        %s/ipfs/%s\n", a, api.host.ID().Pretty())
	}

	logger.Infof("REST API (libp2p-http): ENABLED. Listening on:\n%s\n", listenMsg)

	err := api.server.Serve(api.libp2pListener)
	if err != nil && !strings.Contains(err.Error(), "context canceled") {
		logger.Error(err)
	}
}

// Shutdown stops any API listeners.
func (api *API) Shutdown(ctx context.Context) error {
	_, span := trace.StartSpan(ctx, "restapi/Shutdown")
	defer span.End()

	api.shutdownLock.Lock()
	defer api.shutdownLock.Unlock()

	if api.shutdown {
		logger.Debug("already shutdown")
		return nil
	}

	logger.Info("stopping Cluster API")

	api.cancel()
	close(api.rpcReady)
	// Cancel any outstanding ops
	api.server.SetKeepAlivesEnabled(false)

	if api.httpListener != nil {
		api.httpListener.Close()
	}
	if api.libp2pListener != nil {
		api.libp2pListener.Close()
	}

	// This means we created the host
	if api.config.Libp2pListenAddr != nil {
		api.host.Close()
	}

	api.wg.Wait()
	api.shutdown = true
	return nil
}

// SetClient makes the component ready to perform RPC
// requests.
func (api *API) SetClient(c *rpc.Client) {
	api.rpcClient = c

	// One notification for http server and one for libp2p server.
	api.rpcReady <- struct{}{}
	api.rpcReady <- struct{}{}
}

func (api *API) idHandler(w http.ResponseWriter, r *http.Request) {
	idSerial := types.IDSerial{}
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"ID",
		struct{}{},
		&idSerial,
	)

	api.sendResponse(w, autoStatus, err, idSerial)
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

	api.sendResponse(w, autoStatus, err, v)
}

func (api *API) graphHandler(w http.ResponseWriter, r *http.Request) {
	var graph types.ConnectGraphSerial
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"ConnectGraph",
		struct{}{},
		&graph,
	)
	api.sendResponse(w, autoStatus, err, graph)
}

func (api *API) metricsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	var metrics []types.Metric
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"PeerMonitorLatestMetrics",
		name,
		&metrics,
	)
	api.sendResponse(w, autoStatus, err, metrics)
}

func (api *API) addHandler(w http.ResponseWriter, r *http.Request) {
	reader, err := r.MultipartReader()
	if err != nil {
		api.sendResponse(w, http.StatusBadRequest, err, nil)
		return
	}

	params, err := types.AddParamsFromQuery(r.URL.Query())
	if err != nil {
		api.sendResponse(w, http.StatusBadRequest, err, nil)
		return
	}

	api.setHeaders(w)

	// any errors sent as trailer
	adderutils.AddMultipartHTTPHandler(
		r.Context(),
		api.rpcClient,
		params,
		reader,
		w,
		nil,
	)

	return
}

func (api *API) peerListHandler(w http.ResponseWriter, r *http.Request) {
	var peersSerial []types.IDSerial
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"Peers",
		struct{}{},
		&peersSerial,
	)

	api.sendResponse(w, autoStatus, err, peersSerial)
}

func (api *API) peerAddHandler(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var addInfo peerAddBody
	err := dec.Decode(&addInfo)
	if err != nil {
		api.sendResponse(w, http.StatusBadRequest, errors.New("error decoding request body"), nil)
		return
	}

	_, err = peer.IDB58Decode(addInfo.PeerID)
	if err != nil {
		api.sendResponse(w, http.StatusBadRequest, errors.New("error decoding peer_id"), nil)
		return
	}

	var ids types.IDSerial
	err = api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"PeerAdd",
		addInfo.PeerID,
		&ids,
	)
	api.sendResponse(w, autoStatus, err, ids)
}

func (api *API) peerRemoveHandler(w http.ResponseWriter, r *http.Request) {
	if p := api.parsePidOrError(w, r); p != "" {
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"PeerRemove",
			p,
			&struct{}{},
		)
		api.sendResponse(w, autoStatus, err, nil)
	}
}

func (api *API) pinHandler(w http.ResponseWriter, r *http.Request) {
	if ps := api.parseCidOrError(w, r); ps.Cid != "" {
		logger.Debugf("rest api pinHandler: %s", ps.Cid)
		// span.AddAttributes(trace.StringAttribute("cid", ps.Cid))
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"Pin",
			ps,
			&struct{}{},
		)
		api.sendResponse(w, http.StatusAccepted, err, nil)
		logger.Debug("rest api pinHandler done")
	}
}

func (api *API) unpinHandler(w http.ResponseWriter, r *http.Request) {
	if ps := api.parseCidOrError(w, r); ps.Cid != "" {
		logger.Debugf("rest api unpinHandler: %s", ps.Cid)
		// span.AddAttributes(trace.StringAttribute("cid", ps.Cid))
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"Unpin",
			ps,
			&struct{}{},
		)
		api.sendResponse(w, http.StatusAccepted, err, nil)
		logger.Debug("rest api unpinHandler done")
	}
}

func (api *API) allocationsHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	filterStr := queryValues.Get("filter")
	var filter types.PinType
	for _, f := range strings.Split(filterStr, ",") {
		filter |= types.PinTypeFromString(f)
	}
	var pins []types.PinSerial
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"Pins",
		struct{}{},
		&pins,
	)
	outPins := make([]types.PinSerial, 0)
	for _, pinS := range pins {
		if uint64(filter)&pinS.Type > 0 {
			// add this pin to output
			outPins = append(outPins, pinS)
		}
	}
	api.sendResponse(w, autoStatus, err, outPins)
}

func (api *API) allocationHandler(w http.ResponseWriter, r *http.Request) {
	if ps := api.parseCidOrError(w, r); ps.Cid != "" {
		var pin types.PinSerial
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"PinGet",
			ps,
			&pin,
		)
		if err != nil { // errors here are 404s
			api.sendResponse(w, http.StatusNotFound, err, nil)
			return
		}
		api.sendResponse(w, autoStatus, nil, pin)
	}
}

// filterGlobalPinInfos takes a GlobalPinInfo slice and discards
// any item in it which does not carry a PinInfo matching the
// filter (OR-wise).
func filterGlobalPinInfos(globalPinInfos []types.GlobalPinInfoSerial, filter types.TrackerStatus) []types.GlobalPinInfoSerial {
	if filter == types.TrackerStatusUndefined {
		return globalPinInfos
	}

	var filteredGlobalPinInfos []types.GlobalPinInfoSerial

	for _, globalPinInfo := range globalPinInfos {
		for _, pinInfo := range globalPinInfo.PeerMap {
			st := types.TrackerStatusFromString(pinInfo.Status)
			// silenced the error because we should have detected earlier if filters were invalid
			if st.Match(filter) {
				filteredGlobalPinInfos = append(filteredGlobalPinInfos, globalPinInfo)
				break
			}
		}
	}

	return filteredGlobalPinInfos
}

func (api *API) statusAllHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	var globalPinInfos []types.GlobalPinInfoSerial

	filterStr := queryValues.Get("filter")
	filter := types.TrackerStatusFromString(filterStr)
	if filter == types.TrackerStatusUndefined && filterStr != "" {
		api.sendResponse(w, autoStatus, errors.New("invalid filter value"), nil)
		return
	}

	if local == "true" {
		var pinInfos []types.PinInfoSerial

		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"StatusAllLocal",
			struct{}{},
			&pinInfos,
		)
		if err != nil {
			api.sendResponse(w, autoStatus, err, nil)
			return
		}
		globalPinInfos = pinInfosToGlobal(pinInfos)
	} else {
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"StatusAll",
			struct{}{},
			&globalPinInfos,
		)
		if err != nil {
			api.sendResponse(w, autoStatus, err, nil)
			return
		}
	}

	globalPinInfos = filterGlobalPinInfos(globalPinInfos, filter)

	api.sendResponse(w, autoStatus, nil, globalPinInfos)
}

func (api *API) statusHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	if ps := api.parseCidOrError(w, r); ps.Cid != "" {
		if local == "true" {
			var pinInfo types.PinInfoSerial
			err := api.rpcClient.CallContext(
				r.Context(),
				"",
				"Cluster",
				"StatusLocal",
				ps,
				&pinInfo,
			)
			api.sendResponse(w, autoStatus, err, pinInfoToGlobal(pinInfo))
		} else {
			var pinInfo types.GlobalPinInfoSerial
			err := api.rpcClient.CallContext(
				r.Context(),
				"",
				"Cluster",
				"Status",
				ps,
				&pinInfo,
			)
			api.sendResponse(w, autoStatus, err, pinInfo)
		}
	}
}

func (api *API) syncAllHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	if local == "true" {
		var pinInfos []types.PinInfoSerial
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"SyncAllLocal",
			struct{}{},
			&pinInfos,
		)
		api.sendResponse(w, autoStatus, err, pinInfosToGlobal(pinInfos))
	} else {
		var pinInfos []types.GlobalPinInfoSerial
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"SyncAll",
			struct{}{},
			&pinInfos,
		)
		api.sendResponse(w, autoStatus, err, pinInfos)
	}
}

func (api *API) syncHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	if ps := api.parseCidOrError(w, r); ps.Cid != "" {
		if local == "true" {
			var pinInfo types.PinInfoSerial
			err := api.rpcClient.CallContext(
				r.Context(),
				"",
				"Cluster",
				"SyncLocal",
				ps,
				&pinInfo,
			)
			api.sendResponse(w, autoStatus, err, pinInfoToGlobal(pinInfo))
		} else {
			var pinInfo types.GlobalPinInfoSerial
			err := api.rpcClient.CallContext(
				r.Context(),
				"",
				"Cluster",
				"Sync",
				ps,
				&pinInfo,
			)
			api.sendResponse(w, autoStatus, err, pinInfo)
		}
	}
}

func (api *API) recoverAllHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")
	if local == "true" {
		var pinInfos []types.PinInfoSerial
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"RecoverAllLocal",
			struct{}{},
			&pinInfos,
		)
		api.sendResponse(w, autoStatus, err, pinInfosToGlobal(pinInfos))
	} else {
		api.sendResponse(w, http.StatusBadRequest, errors.New("only requests with parameter local=true are supported"), nil)
	}
}

func (api *API) recoverHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	if ps := api.parseCidOrError(w, r); ps.Cid != "" {
		if local == "true" {
			var pinInfo types.PinInfoSerial
			err := api.rpcClient.CallContext(
				r.Context(),
				"",
				"Cluster",
				"RecoverLocal",
				ps,
				&pinInfo,
			)
			api.sendResponse(w, autoStatus, err, pinInfoToGlobal(pinInfo))
		} else {
			var pinInfo types.GlobalPinInfoSerial
			err := api.rpcClient.CallContext(
				r.Context(),
				"",
				"Cluster",
				"Recover",
				ps,
				&pinInfo,
			)
			api.sendResponse(w, autoStatus, err, pinInfo)
		}
	}
}

func (api *API) parseCidOrError(w http.ResponseWriter, r *http.Request) types.PinSerial {
	vars := mux.Vars(r)
	hash := vars["hash"]

	_, err := cid.Decode(hash)
	if err != nil {
		api.sendResponse(w, http.StatusBadRequest, errors.New("error decoding Cid: "+err.Error()), nil)
		return types.PinSerial{Cid: ""}
	}

	pin := types.PinSerial{
		Cid:  hash,
		Type: uint64(types.DataType),
	}

	queryValues := r.URL.Query()
	name := queryValues.Get("name")
	pin.Name = name
	pin.MaxDepth = -1 // For now, all pins are recursive
	rplStr := queryValues.Get("replication")
	if rplStr == "" { // compat <= 0.4.0
		rplStr = queryValues.Get("replication_factor")
	}
	rplStrMin := queryValues.Get("replication-min")
	if rplStrMin == "" { // compat <= 0.4.0
		rplStrMin = queryValues.Get("replication_factor_min")
	}
	rplStrMax := queryValues.Get("replication-max")
	if rplStrMax == "" { // compat <= 0.4.0
		rplStrMax = queryValues.Get("replication_factor_max")
	}
	if rplStr != "" { // override
		rplStrMin = rplStr
		rplStrMax = rplStr
	}
	if rpl, err := strconv.Atoi(rplStrMin); err == nil {
		pin.ReplicationFactorMin = rpl
	}
	if rpl, err := strconv.Atoi(rplStrMax); err == nil {
		pin.ReplicationFactorMax = rpl
	}

	return pin
}

func (api *API) parsePidOrError(w http.ResponseWriter, r *http.Request) peer.ID {
	vars := mux.Vars(r)
	idStr := vars["peer"]
	pid, err := peer.IDB58Decode(idStr)
	if err != nil {
		api.sendResponse(w, http.StatusBadRequest, errors.New("error decoding Peer ID: "+err.Error()), nil)
		return ""
	}
	return pid
}

func pinInfoToGlobal(pInfo types.PinInfoSerial) types.GlobalPinInfoSerial {
	return types.GlobalPinInfoSerial{
		Cid: pInfo.Cid,
		PeerMap: map[string]types.PinInfoSerial{
			pInfo.Peer: pInfo,
		},
	}
}

func pinInfosToGlobal(pInfos []types.PinInfoSerial) []types.GlobalPinInfoSerial {
	gPInfos := make([]types.GlobalPinInfoSerial, len(pInfos), len(pInfos))
	for i, p := range pInfos {
		gPInfos[i] = pinInfoToGlobal(p)
	}
	return gPInfos
}

// sendResponse wraps all the logic for writing the response to a request:
// * Write configured headers
// * Write application/json content type
// * Write status: determined automatically if given "autoStatus"
// * Write an error if there is or write the response if there is
func (api *API) sendResponse(
	w http.ResponseWriter,
	status int,
	err error,
	resp interface{},
) {

	api.setHeaders(w)
	enc := json.NewEncoder(w)

	// Send an error
	if err != nil {
		if status == autoStatus || status < 400 { // set a default error status
			status = http.StatusInternalServerError
		}
		w.WriteHeader(status)

		errorResp := types.Error{
			Code:    status,
			Message: err.Error(),
		}
		logger.Errorf("sending error response: %d: %s", status, err.Error())

		if err := enc.Encode(errorResp); err != nil {
			logger.Error(err)
		}
		return
	}

	// Send a body
	if resp != nil {
		if status == autoStatus {
			status = http.StatusOK
		}

		w.WriteHeader(status)

		if err = enc.Encode(resp); err != nil {
			logger.Error(err)
		}
		return
	}

	// Empty response
	if status == autoStatus {
		status = http.StatusNoContent
	}

	w.WriteHeader(status)
}

// this sets all the headers that are common to all responses
// from this API. Called from sendResponse() and /add.
func (api *API) setHeaders(w http.ResponseWriter) {
	for header, values := range api.config.Headers {
		for _, val := range values {
			w.Header().Add(header, val)
		}
	}

	w.Header().Add("Content-Type", "application/json")
}
