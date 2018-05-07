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
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	types "github.com/ipfs/ipfs-cluster/api"

	mux "github.com/gorilla/mux"
	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	gostream "github.com/hsanjuan/go-libp2p-gostream"
	p2phttp "github.com/hsanjuan/go-libp2p-http"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

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
	PeerMultiaddr string `json:"peer_multiaddress"`
}

// NewAPI creates a new REST API component with the given configuration.
func NewAPI(cfg *Config) (*API, error) {
	return NewAPIWithHost(cfg, nil)
}

// NewAPIWithHost creates a new REST API component and enables
// the libp2p-http endpoint using the given Host, if not nil.
func NewAPIWithHost(cfg *Config, h host.Host) (*API, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	router := mux.NewRouter().StrictSlash(true)
	s := &http.Server{
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		Handler:           router,
	}
	s.SetKeepAlivesEnabled(true) // A reminder that this can be changed

	ctx, cancel := context.WithCancel(context.Background())

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
	err = api.setupHTTP()
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

	api.run()
	return api, nil
}

func (api *API) setupHTTP() error {
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

	l, err := gostream.Listen(api.host, p2phttp.P2PProtocol)
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
		if api.config.BasicAuthCreds != nil {
			route.HandlerFunc = basicAuth(route.HandlerFunc, api.config.BasicAuthCreds)
		}
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(route.HandlerFunc)
	}
	api.router = router
}

func basicAuth(h http.HandlerFunc, credentials map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
	}
}

func (api *API) run() {
	if api.httpListener != nil {
		api.wg.Add(1)
		go api.runHTTPServer()
	}

	if api.libp2pListener != nil {
		api.wg.Add(1)
		go api.runLibp2pServer()
	}
}

// runs in goroutine from run()
func (api *API) runHTTPServer() {
	defer api.wg.Done()
	<-api.rpcReady

	logger.Infof("REST API (HTTP): %s", api.config.HTTPListenAddr)
	err := api.server.Serve(api.httpListener)
	if err != nil && !strings.Contains(err.Error(), "closed network connection") {
		logger.Error(err)
	}
}

// runs in goroutine from run()
func (api *API) runLibp2pServer() {
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
func (api *API) Shutdown() error {
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
	err := api.rpcClient.Call("",
		"Cluster",
		"ID",
		struct{}{},
		&idSerial)

	sendResponse(w, err, idSerial)
}

func (api *API) versionHandler(w http.ResponseWriter, r *http.Request) {
	var v types.Version
	err := api.rpcClient.Call("",
		"Cluster",
		"Version",
		struct{}{},
		&v)

	sendResponse(w, err, v)
}

func (api *API) graphHandler(w http.ResponseWriter, r *http.Request) {
	var graph types.ConnectGraphSerial
	err := api.rpcClient.Call("",
		"Cluster",
		"ConnectGraph",
		struct{}{},
		&graph)
	sendResponse(w, err, graph)
}

func (api *API) peerListHandler(w http.ResponseWriter, r *http.Request) {
	var peersSerial []types.IDSerial
	err := api.rpcClient.Call("",
		"Cluster",
		"Peers",
		struct{}{},
		&peersSerial)

	sendResponse(w, err, peersSerial)
}

func (api *API) peerAddHandler(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var addInfo peerAddBody
	err := dec.Decode(&addInfo)
	if err != nil {
		sendErrorResponse(w, 400, "error decoding request body")
		return
	}

	mAddr, err := ma.NewMultiaddr(addInfo.PeerMultiaddr)
	if err != nil {
		sendErrorResponse(w, 400, "error decoding peer_multiaddress")
		return
	}

	var ids types.IDSerial
	err = api.rpcClient.Call("",
		"Cluster",
		"PeerAdd",
		types.MultiaddrToSerial(mAddr),
		&ids)
	sendResponse(w, err, ids)
}

func (api *API) peerRemoveHandler(w http.ResponseWriter, r *http.Request) {
	if p := parsePidOrError(w, r); p != "" {
		err := api.rpcClient.Call("",
			"Cluster",
			"PeerRemove",
			p,
			&struct{}{})
		sendEmptyResponse(w, err)
	}
}

func (api *API) pinHandler(w http.ResponseWriter, r *http.Request) {
	if ps := parseCidOrError(w, r); ps.Cid != "" {
		logger.Debugf("rest api pinHandler: %s", ps.Cid)

		err := api.rpcClient.Call("",
			"Cluster",
			"Pin",
			ps,
			&struct{}{})
		sendAcceptedResponse(w, err)
		logger.Debug("rest api pinHandler done")
	}
}

func (api *API) unpinHandler(w http.ResponseWriter, r *http.Request) {
	if ps := parseCidOrError(w, r); ps.Cid != "" {
		logger.Debugf("rest api unpinHandler: %s", ps.Cid)
		err := api.rpcClient.Call("",
			"Cluster",
			"Unpin",
			ps,
			&struct{}{})
		sendAcceptedResponse(w, err)
		logger.Debug("rest api unpinHandler done")
	}
}

func (api *API) allocationsHandler(w http.ResponseWriter, r *http.Request) {
	var pins []types.PinSerial
	err := api.rpcClient.Call("",
		"Cluster",
		"Pins",
		struct{}{},
		&pins)
	sendResponse(w, err, pins)
}

func (api *API) allocationHandler(w http.ResponseWriter, r *http.Request) {
	if ps := parseCidOrError(w, r); ps.Cid != "" {
		var pin types.PinSerial
		err := api.rpcClient.Call("",
			"Cluster",
			"PinGet",
			ps,
			&pin)
		if err != nil { // errors here are 404s
			sendErrorResponse(w, 404, err.Error())
			return
		}
		sendJSONResponse(w, 200, pin)
	}
}

func (api *API) statusAllHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	if local == "true" {
		var pinInfos []types.PinInfoSerial
		err := api.rpcClient.Call("",
			"Cluster",
			"StatusAllLocal",
			struct{}{},
			&pinInfos)
		sendResponse(w, err, pinInfosToGlobal(pinInfos))
	} else {
		var pinInfos []types.GlobalPinInfoSerial
		err := api.rpcClient.Call("",
			"Cluster",
			"StatusAll",
			struct{}{},
			&pinInfos)
		sendResponse(w, err, pinInfos)
	}
}

func (api *API) statusHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	if ps := parseCidOrError(w, r); ps.Cid != "" {
		if local == "true" {
			var pinInfo types.PinInfoSerial
			err := api.rpcClient.Call("",
				"Cluster",
				"StatusLocal",
				ps,
				&pinInfo)
			sendResponse(w, err, pinInfoToGlobal(pinInfo))
		} else {
			var pinInfo types.GlobalPinInfoSerial
			err := api.rpcClient.Call("",
				"Cluster",
				"Status",
				ps,
				&pinInfo)
			sendResponse(w, err, pinInfo)
		}
	}
}

func (api *API) syncAllHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	if local == "true" {
		var pinInfos []types.PinInfoSerial
		err := api.rpcClient.Call("",
			"Cluster",
			"SyncAllLocal",
			struct{}{},
			&pinInfos)
		sendResponse(w, err, pinInfosToGlobal(pinInfos))
	} else {
		var pinInfos []types.GlobalPinInfoSerial
		err := api.rpcClient.Call("",
			"Cluster",
			"SyncAll",
			struct{}{},
			&pinInfos)
		sendResponse(w, err, pinInfos)
	}
}

func (api *API) syncHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	if ps := parseCidOrError(w, r); ps.Cid != "" {
		if local == "true" {
			var pinInfo types.PinInfoSerial
			err := api.rpcClient.Call("",
				"Cluster",
				"SyncLocal",
				ps,
				&pinInfo)
			sendResponse(w, err, pinInfoToGlobal(pinInfo))
		} else {
			var pinInfo types.GlobalPinInfoSerial
			err := api.rpcClient.Call("",
				"Cluster",
				"Sync",
				ps,
				&pinInfo)
			sendResponse(w, err, pinInfo)
		}
	}
}

func (api *API) recoverAllHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")
	if local == "true" {
		var pinInfos []types.PinInfoSerial
		err := api.rpcClient.Call("",
			"Cluster",
			"RecoverAllLocal",
			struct{}{},
			&pinInfos)
		sendResponse(w, err, pinInfosToGlobal(pinInfos))
	} else {
		sendErrorResponse(w, 400, "only requests with parameter local=true are supported")
	}
}

func (api *API) recoverHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	local := queryValues.Get("local")

	if ps := parseCidOrError(w, r); ps.Cid != "" {
		if local == "true" {
			var pinInfo types.PinInfoSerial
			err := api.rpcClient.Call("",
				"Cluster",
				"RecoverLocal",
				ps,
				&pinInfo)
			sendResponse(w, err, pinInfoToGlobal(pinInfo))
		} else {
			var pinInfo types.GlobalPinInfoSerial
			err := api.rpcClient.Call("",
				"Cluster",
				"Recover",
				ps,
				&pinInfo)
			sendResponse(w, err, pinInfo)
		}
	}
}

func parseCidOrError(w http.ResponseWriter, r *http.Request) types.PinSerial {
	vars := mux.Vars(r)
	hash := vars["hash"]

	_, err := cid.Decode(hash)
	if err != nil {
		sendErrorResponse(w, 400, "error decoding Cid: "+err.Error())
		return types.PinSerial{Cid: ""}
	}

	pin := types.PinSerial{
		Cid: hash,
	}

	queryValues := r.URL.Query()
	name := queryValues.Get("name")
	pin.Name = name
	pin.Recursive = true // For now all CLI pins are recursive
	rplStr := queryValues.Get("replication_factor")
	rplStrMin := queryValues.Get("replication_factor_min")
	rplStrMax := queryValues.Get("replication_factor_max")
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

func parsePidOrError(w http.ResponseWriter, r *http.Request) peer.ID {
	vars := mux.Vars(r)
	idStr := vars["peer"]
	pid, err := peer.IDB58Decode(idStr)
	if err != nil {
		sendErrorResponse(w, 400, "error decoding Peer ID: "+err.Error())
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

func sendResponse(w http.ResponseWriter, rpcErr error, resp interface{}) {
	if checkRPCErr(w, rpcErr) {
		sendJSONResponse(w, 200, resp)
	}
}

// checkRPCErr takes care of returning standard error responses if we
// pass an error to it. It returns true when everythings OK (no error
// was handled), or false otherwise.
func checkRPCErr(w http.ResponseWriter, err error) bool {
	if err != nil {
		sendErrorResponse(w, 500, err.Error())
		return false
	}
	return true
}

func sendEmptyResponse(w http.ResponseWriter, rpcErr error) {
	if checkRPCErr(w, rpcErr) {
		w.WriteHeader(http.StatusNoContent)
	}
}

func sendAcceptedResponse(w http.ResponseWriter, rpcErr error) {
	if checkRPCErr(w, rpcErr) {
		w.WriteHeader(http.StatusAccepted)
	}
}

func sendJSONResponse(w http.ResponseWriter, code int, resp interface{}) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		panic(err)
	}
}

func sendErrorResponse(w http.ResponseWriter, code int, msg string) {
	errorResp := types.Error{
		Code:    code,
		Message: msg,
	}
	logger.Errorf("sending error response: %d: %s", code, msg)
	sendJSONResponse(w, code, errorResp)
}
