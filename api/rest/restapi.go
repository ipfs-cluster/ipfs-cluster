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
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/adder/adderutils"
	types "github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/state"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	gopath "github.com/ipfs/go-path"
	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	gostream "github.com/libp2p/go-libp2p-gostream"
	p2phttp "github.com/libp2p/go-libp2p-http"
	libp2pquic "github.com/libp2p/go-libp2p-quic-transport"
	secio "github.com/libp2p/go-libp2p-secio"
	libp2ptls "github.com/libp2p/go-libp2p-tls"
	manet "github.com/multiformats/go-multiaddr-net"

	handlers "github.com/gorilla/handlers"
	mux "github.com/gorilla/mux"
	"github.com/rs/cors"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/plugin/ochttp/propagation/tracecontext"
	"go.opencensus.io/trace"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var (
	logger    = logging.Logger("restapi")
	apiLogger = logging.Logger("restapilog")
)

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

	httpListeners  []net.Listener
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

type logWriter struct {
}

func (lw logWriter) Write(b []byte) (int, error) {
	apiLogger.Info(string(b))
	return len(b), nil
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
		cfg.BasicAuthCredentials,
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

	var writer io.Writer
	if cfg.HTTPLogFile != "" {
		f, err := os.OpenFile(cfg.getHTTPLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		writer = f
	} else {
		writer = logWriter{}
	}

	s := &http.Server{
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		Handler:           handlers.LoggingHandler(writer, handler),
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
	}

	// See: https://github.com/ipfs/go-ipfs/issues/5168
	// See: https://github.com/ipfs/ipfs-cluster/issues/548
	// on why this is re-enabled.
	s.SetKeepAlivesEnabled(true)
	s.MaxHeaderBytes = cfg.MaxHeaderBytes

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

	// Set up api.httpListeners if enabled
	err = api.setupHTTP()
	if err != nil {
		return nil, err
	}

	// Set up api.libp2pListeners if enabled
	err = api.setupLibp2p()
	if err != nil {
		return nil, err
	}

	if len(api.httpListeners) == 0 && api.libp2pListener == nil {
		return nil, ErrNoEndpointsEnabled
	}

	api.run(ctx)
	return api, nil
}

func (api *API) setupHTTP() error {
	if len(api.config.HTTPListenAddr) == 0 {
		return nil
	}

	for _, listenMAddr := range api.config.HTTPListenAddr {
		n, addr, err := manet.DialArgs(listenMAddr)
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
		api.httpListeners = append(api.httpListeners, l)
	}
	return nil
}

func (api *API) setupLibp2p() error {
	// Make new host. Override any provided existing one
	// if we have config for a custom one.
	if len(api.config.Libp2pListenAddr) > 0 {
		// We use a new host context. We will call
		// Close() on shutdown(). Avoids things like:
		// https://github.com/ipfs/ipfs-cluster/issues/853
		h, err := libp2p.New(
			context.Background(),
			libp2p.Identity(api.config.PrivateKey),
			libp2p.ListenAddrs(api.config.Libp2pListenAddr...),
			libp2p.Security(libp2ptls.ID, libp2ptls.New),
			libp2p.Security(secio.ID, secio.New),
			libp2p.Transport(libp2pquic.NewTransport),
			libp2p.DefaultTransports,
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

// HTTPAddresses returns the HTTP(s) listening address
// in host:port format. Useful when configured to start
// on a random port (0). Returns error when the HTTP endpoint
// is not enabled.
func (api *API) HTTPAddresses() ([]string, error) {
	if len(api.httpListeners) == 0 {
		return nil, ErrHTTPEndpointNotEnabled
	}
	var addrs []string
	for _, l := range api.httpListeners {
		addrs = append(addrs, l.Addr().String())
	}

	return addrs, nil
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
	router.NotFoundHandler = ochttp.WithRouteTag(
		http.HandlerFunc(api.notFoundHandler),
		"/notfound",
	)
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
			http.Error(w, resp, http.StatusUnauthorized)
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
			http.Error(w, resp, http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(wrap)
}

func unauthorizedResp() (string, error) {
	apiError := &types.Error{
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
			"Recover",
			"POST",
			"/pins/{hash}/recover",
			api.recoverHandler,
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
			"PinPath",
			"POST",
			"/pins/{keyType:ipfs|ipns|ipld}/{path:.*}",
			api.pinPathHandler,
		},
		{
			"Unpin",
			"DELETE",
			"/pins/{hash}",
			api.unpinHandler,
		},
		{
			"UnpinPath",
			"DELETE",
			"/pins/{keyType:ipfs|ipns|ipld}/{path:.*}",
			api.unpinPathHandler,
		},
		{
			"RepoGC",
			"POST",
			"/ipfs/gc",
			api.repoGCHandler,
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
		{
			"MetricNames",
			"GET",
			"/monitor/metrics",
			api.metricNamesHandler,
		},
	}
}

func (api *API) run(ctx context.Context) {
	api.wg.Add(len(api.httpListeners))
	for _, l := range api.httpListeners {
		go func(l net.Listener) {
			defer api.wg.Done()
			api.runHTTPServer(ctx, l)
		}(l)
	}

	if api.libp2pListener != nil {
		api.wg.Add(1)
		go func() {
			defer api.wg.Done()
			api.runLibp2pServer(ctx)
		}()
	}
}

// runs in goroutine from run()
func (api *API) runHTTPServer(ctx context.Context, l net.Listener) {
	select {
	case <-api.rpcReady:
	case <-api.ctx.Done():
		return
	}

	maddr, err := manet.FromNetAddr(l.Addr())
	if err != nil {
		logger.Error(err)
	}

	logger.Infof("REST API (HTTP): %s", maddr)
	err = api.server.Serve(l)
	if err != nil && !strings.Contains(err.Error(), "closed network connection") {
		logger.Error(err)
	}
}

// runs in goroutine from run()
func (api *API) runLibp2pServer(ctx context.Context) {
	select {
	case <-api.rpcReady:
	case <-api.ctx.Done():
		return
	}

	listenMsg := ""
	for _, a := range api.host.Addrs() {
		listenMsg += fmt.Sprintf("        %s/p2p/%s\n", a, api.host.ID().Pretty())
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

	for _, l := range api.httpListeners {
		l.Close()
	}

	if api.libp2pListener != nil {
		api.libp2pListener.Close()
	}

	api.wg.Wait()

	// This means we created the host
	if api.config.Libp2pListenAddr != nil {
		api.host.Close()
	}
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
	var id types.ID
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"ID",
		struct{}{},
		&id,
	)

	api.sendResponse(w, autoStatus, err, &id)
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
	var graph types.ConnectGraph
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

	var metrics []*types.Metric
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"PeerMonitor",
		"LatestMetrics",
		name,
		&metrics,
	)
	api.sendResponse(w, autoStatus, err, metrics)
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
	api.sendResponse(w, autoStatus, err, metricNames)
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
}

func (api *API) peerListHandler(w http.ResponseWriter, r *http.Request) {
	var peers []*types.ID
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"Peers",
		struct{}{},
		&peers,
	)

	api.sendResponse(w, autoStatus, err, peers)
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

	pid, err := peer.Decode(addInfo.PeerID)
	if err != nil {
		api.sendResponse(w, http.StatusBadRequest, errors.New("error decoding peer_id"), nil)
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
	api.sendResponse(w, autoStatus, err, &id)
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
	if pin := api.parseCidOrError(w, r); pin != nil {
		logger.Debugf("rest api pinHandler: %s", pin.Cid)
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
		api.sendResponse(w, autoStatus, err, pinObj)
		logger.Debug("rest api pinHandler done")
	}
}

func (api *API) unpinHandler(w http.ResponseWriter, r *http.Request) {
	if pin := api.parseCidOrError(w, r); pin != nil {
		logger.Debugf("rest api unpinHandler: %s", pin.Cid)
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
		if err != nil && err.Error() == state.ErrNotFound.Error() {
			api.sendResponse(w, http.StatusNotFound, err, nil)
			return
		}
		api.sendResponse(w, autoStatus, err, pinObj)
		logger.Debug("rest api unpinHandler done")
	}
}

func (api *API) pinPathHandler(w http.ResponseWriter, r *http.Request) {
	var pin types.Pin
	if pinpath := api.parsePinPathOrError(w, r); pinpath != nil {
		logger.Debugf("rest api pinPathHandler: %s", pinpath.Path)
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"PinPath",
			pinpath,
			&pin,
		)

		api.sendResponse(w, autoStatus, err, pin)
		logger.Debug("rest api pinPathHandler done")
	}
}

func (api *API) unpinPathHandler(w http.ResponseWriter, r *http.Request) {
	var pin types.Pin
	if pinpath := api.parsePinPathOrError(w, r); pinpath != nil {
		logger.Debugf("rest api unpinPathHandler: %s", pinpath.Path)
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"UnpinPath",
			pinpath,
			&pin,
		)
		if err != nil && err.Error() == state.ErrNotFound.Error() {
			api.sendResponse(w, http.StatusNotFound, err, nil)
			return
		}
		api.sendResponse(w, autoStatus, err, pin)
		logger.Debug("rest api unpinPathHandler done")
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
		api.sendResponse(w, http.StatusBadRequest, errors.New("invalid filter value"), nil)
		return
	}

	var pins []*types.Pin
	err := api.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"Pins",
		struct{}{},
		&pins,
	)
	outPins := make([]*types.Pin, 0)
	for _, pin := range pins {
		if filter&pin.Type > 0 {
			// add this pin to output
			outPins = append(outPins, pin)
		}
	}
	api.sendResponse(w, autoStatus, err, outPins)
}

func (api *API) allocationHandler(w http.ResponseWriter, r *http.Request) {
	if pin := api.parseCidOrError(w, r); pin != nil {
		var pinResp types.Pin
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"PinGet",
			pin.Cid,
			&pinResp,
		)
		if err != nil { // errors here are 404s
			api.sendResponse(w, http.StatusNotFound, err, nil)
			return
		}
		api.sendResponse(w, autoStatus, nil, pinResp)
	}
}

// filterGlobalPinInfos takes a GlobalPinInfo slice and discards
// any item in it which does not carry a PinInfo matching the
// filter (OR-wise).
func filterGlobalPinInfos(globalPinInfos []*types.GlobalPinInfo, filter types.TrackerStatus) []*types.GlobalPinInfo {
	if filter == types.TrackerStatusUndefined {
		return globalPinInfos
	}

	var filteredGlobalPinInfos []*types.GlobalPinInfo

	for _, globalPinInfo := range globalPinInfos {
		for _, pinInfo := range globalPinInfo.PeerMap {
			// silenced the error because we should have detected
			// earlier if filters were invalid
			if pinInfo.Status.Match(filter) {
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

	var globalPinInfos []*types.GlobalPinInfo

	filterStr := queryValues.Get("filter")
	filter := types.TrackerStatusFromString(filterStr)
	if filter == types.TrackerStatusUndefined && filterStr != "" {
		api.sendResponse(w, http.StatusBadRequest, errors.New("invalid filter value"), nil)
		return
	}

	if local == "true" {
		var pinInfos []*types.PinInfo

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

	if pin := api.parseCidOrError(w, r); pin != nil {
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
			api.sendResponse(w, autoStatus, err, pinInfoToGlobal(&pinInfo))
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
			api.sendResponse(w, autoStatus, err, pinInfo)
		}
	}
}

func (api *API) recoverAllHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")
	if local == "true" {
		var pinInfos []*types.PinInfo
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
		var globalPinInfos []*types.GlobalPinInfo
		err := api.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"RecoverAll",
			struct{}{},
			&globalPinInfos,
		)
		api.sendResponse(w, autoStatus, err, globalPinInfos)
	}
}

func (api *API) recoverHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	if pin := api.parseCidOrError(w, r); pin != nil {
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
			api.sendResponse(w, autoStatus, err, pinInfoToGlobal(&pinInfo))
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
			api.sendResponse(w, autoStatus, err, pinInfo)
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

		api.sendResponse(w, autoStatus, err, repoGCToGlobal(&localRepoGC))
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
	api.sendResponse(w, autoStatus, err, repoGC)
}

func repoGCToGlobal(r *types.RepoGC) types.GlobalRepoGC {
	return types.GlobalRepoGC{
		PeerMap: map[string]*types.RepoGC{
			peer.Encode(r.Peer): r,
		},
	}
}

func (api *API) notFoundHandler(w http.ResponseWriter, r *http.Request) {
	api.sendResponse(w, http.StatusNotFound, errors.New("not found"), nil)
}

func (api *API) parsePinPathOrError(w http.ResponseWriter, r *http.Request) *types.PinPath {
	vars := mux.Vars(r)
	urlpath := "/" + vars["keyType"] + "/" + strings.TrimSuffix(vars["path"], "/")

	path, err := gopath.ParsePath(urlpath)
	if err != nil {
		api.sendResponse(w, http.StatusBadRequest, errors.New("error parsing path: "+err.Error()), nil)
		return nil
	}

	pinPath := &types.PinPath{Path: path.String()}
	err = pinPath.PinOptions.FromQuery(r.URL.Query())
	if err != nil {
		api.sendResponse(w, http.StatusBadRequest, err, nil)
	}
	return pinPath
}

func (api *API) parseCidOrError(w http.ResponseWriter, r *http.Request) *types.Pin {
	vars := mux.Vars(r)
	hash := vars["hash"]

	c, err := cid.Decode(hash)
	if err != nil {
		api.sendResponse(w, http.StatusBadRequest, errors.New("error decoding Cid: "+err.Error()), nil)
		return nil
	}

	opts := types.PinOptions{}
	err = opts.FromQuery(r.URL.Query())
	if err != nil {
		api.sendResponse(w, http.StatusBadRequest, err, nil)
	}
	pin := types.PinWithOpts(c, opts)
	pin.MaxDepth = -1 // For now, all pins are recursive
	return pin
}

func (api *API) parsePidOrError(w http.ResponseWriter, r *http.Request) peer.ID {
	vars := mux.Vars(r)
	idStr := vars["peer"]
	pid, err := peer.Decode(idStr)
	if err != nil {
		api.sendResponse(w, http.StatusBadRequest, errors.New("error decoding Peer ID: "+err.Error()), nil)
		return ""
	}
	return pid
}

func pinInfoToGlobal(pInfo *types.PinInfo) *types.GlobalPinInfo {
	return &types.GlobalPinInfo{
		Cid: pInfo.Cid,
		PeerMap: map[string]*types.PinInfo{
			peer.Encode(pInfo.Peer): pInfo,
		},
	}
}

func pinInfosToGlobal(pInfos []*types.PinInfo) []*types.GlobalPinInfo {
	gPInfos := make([]*types.GlobalPinInfo, len(pInfos))
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
