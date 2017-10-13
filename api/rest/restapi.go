// Package rest implements an IPFS Cluster API component. It provides
// a REST-ish API to interact with Cluster over HTTP.
package rest

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	types "github.com/ipfs/ipfs-cluster/api"

	mux "github.com/gorilla/mux"
	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

var logger = logging.Logger("restapi")

// API implements an API and aims to provides
// a RESTful HTTP API for Cluster.
type API struct {
	ctx    context.Context
	cancel func()

	config *Config

	rpcClient *rpc.Client
	rpcReady  chan struct{}
	router    *mux.Router

	listener net.Listener
	server   *http.Server

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

// NewAPI creates a new REST API component. It receives
// the multiaddress on which the API listens.
func NewAPI(cfg *Config) (*API, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	n, addr, err := manet.DialArgs(cfg.ListenAddr)
	if err != nil {
		return nil, err
	}

	var l net.Listener
	if cfg.TLS != nil {
		l, err = tls.Listen(n, addr, cfg.TLS)
	} else {
		l, err = net.Listen(n, addr)
	}
	if err != nil {
		return nil, err
	}

	return newAPI(cfg, l)
}

func newAPI(cfg *Config, l net.Listener) (*API, error) {
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
		listener: l,
		server:   s,
		rpcReady: make(chan struct{}, 1),
	}
	api.addRoutes(router)
	api.run()

	return api, nil
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
			"/graph",
			api.graphHandler,
		},
	}
}

func (api *API) run() {
	api.wg.Add(1)
	go func() {
		defer api.wg.Done()
		<-api.rpcReady

		logger.Infof("REST API: %s", api.config.ListenAddr)
		err := api.server.Serve(api.listener)
		if err != nil && !strings.Contains(err.Error(), "closed network connection") {
			logger.Error(err)
		}
	}()
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
	api.listener.Close()

	api.wg.Wait()
	api.shutdown = true
	return nil
}

// SetClient makes the component ready to perform RPC
// requests.
func (api *API) SetClient(c *rpc.Client) {
	api.rpcClient = c
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

func (rest *API) graphHandler(w http.ResponseWriter, r *http.Request) {
	var graph types.ConnectGraphSerial
	err := rest.rpcClient.Call("",
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
		err := api.rpcClient.Call("",
			"Cluster",
			"Pin",
			ps,
			&struct{}{})
		sendAcceptedResponse(w, err)
	}
}

func (api *API) unpinHandler(w http.ResponseWriter, r *http.Request) {
	if ps := parseCidOrError(w, r); ps.Cid != "" {
		err := api.rpcClient.Call("",
			"Cluster",
			"Unpin",
			ps,
			&struct{}{})
		sendAcceptedResponse(w, err)
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
	rplStr := queryValues.Get("replication_factor")
	if rpl, err := strconv.Atoi(rplStr); err == nil {
		pin.ReplicationFactor = rpl
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
