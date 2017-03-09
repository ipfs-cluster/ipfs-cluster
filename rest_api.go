package ipfscluster

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	mux "github.com/gorilla/mux"
	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
)

// Server settings
var (
	// maximum duration before timing out read of the request
	RESTAPIServerReadTimeout = 5 * time.Second
	// maximum duration before timing out write of the response
	RESTAPIServerWriteTimeout = 10 * time.Second
	// server-side the amount of time a Keep-Alive connection will be
	// kept idle before being reused
	RESTAPIServerIdleTimeout = 60 * time.Second
)

// RESTAPI implements an API and aims to provides
// a RESTful HTTP API for Cluster.
type RESTAPI struct {
	ctx    context.Context
	cancel func()

	apiAddr    ma.Multiaddr
	listenAddr string
	listenPort int
	rpcClient  *rpc.Client
	rpcReady   chan struct{}
	router     *mux.Router

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

type errorResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e errorResp) Error() string {
	return e.Message
}

// NewRESTAPI creates a new object which is ready to be
// started.
func NewRESTAPI(cfg *Config) (*RESTAPI, error) {
	listenAddr, err := cfg.APIAddr.ValueForProtocol(ma.P_IP4)
	if err != nil {
		return nil, err
	}
	listenPortStr, err := cfg.APIAddr.ValueForProtocol(ma.P_TCP)
	if err != nil {
		return nil, err
	}
	listenPort, err := strconv.Atoi(listenPortStr)
	if err != nil {
		return nil, err
	}

	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d",
		listenAddr, listenPort))
	if err != nil {
		return nil, err
	}

	router := mux.NewRouter().StrictSlash(true)
	s := &http.Server{
		ReadTimeout:  RESTAPIServerReadTimeout,
		WriteTimeout: RESTAPIServerWriteTimeout,
		//IdleTimeout:  RESTAPIServerIdleTimeout, // TODO: Go 1.8
		Handler: router,
	}
	s.SetKeepAlivesEnabled(true) // A reminder that this can be changed

	ctx, cancel := context.WithCancel(context.Background())

	api := &RESTAPI{
		ctx:        ctx,
		cancel:     cancel,
		apiAddr:    cfg.APIAddr,
		listenAddr: listenAddr,
		listenPort: listenPort,
		listener:   l,
		server:     s,
		rpcReady:   make(chan struct{}, 1),
	}

	for _, route := range api.routes() {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(route.HandlerFunc)
	}

	api.router = router
	api.run()
	return api, nil
}

func (rest *RESTAPI) routes() []route {
	return []route{
		{
			"ID",
			"GET",
			"/id",
			rest.idHandler,
		},

		{
			"Version",
			"GET",
			"/version",
			rest.versionHandler,
		},

		{
			"Peers",
			"GET",
			"/peers",
			rest.peerListHandler,
		},
		{
			"PeerAdd",
			"POST",
			"/peers",
			rest.peerAddHandler,
		},
		{
			"PeerRemove",
			"DELETE",
			"/peers/{peer}",
			rest.peerRemoveHandler,
		},

		{
			"Pins",
			"GET",
			"/pinlist",
			rest.pinListHandler,
		},

		{
			"StatusAll",
			"GET",
			"/pins",
			rest.statusAllHandler,
		},
		{
			"SyncAll",
			"POST",
			"/pins/sync",
			rest.syncAllHandler,
		},
		{
			"Status",
			"GET",
			"/pins/{hash}",
			rest.statusHandler,
		},
		{
			"Pin",
			"POST",
			"/pins/{hash}",
			rest.pinHandler,
		},
		{
			"Unpin",
			"DELETE",
			"/pins/{hash}",
			rest.unpinHandler,
		},
		{
			"Sync",
			"POST",
			"/pins/{hash}/sync",
			rest.syncHandler,
		},
		{
			"Recover",
			"POST",
			"/pins/{hash}/recover",
			rest.recoverHandler,
		},
	}
}

func (rest *RESTAPI) run() {
	rest.wg.Add(1)
	go func() {
		defer rest.wg.Done()
		<-rest.rpcReady

		logger.Infof("REST API: %s", rest.apiAddr)
		err := rest.server.Serve(rest.listener)
		if err != nil && !strings.Contains(err.Error(), "closed network connection") {
			logger.Error(err)
		}
	}()
}

// Shutdown stops any API listeners.
func (rest *RESTAPI) Shutdown() error {
	rest.shutdownLock.Lock()
	defer rest.shutdownLock.Unlock()

	if rest.shutdown {
		logger.Debug("already shutdown")
		return nil
	}

	logger.Info("stopping Cluster API")

	rest.cancel()
	close(rest.rpcReady)
	// Cancel any outstanding ops
	rest.server.SetKeepAlivesEnabled(false)
	rest.listener.Close()

	rest.wg.Wait()
	rest.shutdown = true
	return nil
}

// SetClient makes the component ready to perform RPC
// requests.
func (rest *RESTAPI) SetClient(c *rpc.Client) {
	rest.rpcClient = c
	rest.rpcReady <- struct{}{}
}

func (rest *RESTAPI) idHandler(w http.ResponseWriter, r *http.Request) {
	idSerial := api.IDSerial{}
	err := rest.rpcClient.Call("",
		"Cluster",
		"ID",
		struct{}{},
		&idSerial)

	sendResponse(w, err, idSerial)
}

func (rest *RESTAPI) versionHandler(w http.ResponseWriter, r *http.Request) {
	var v api.Version
	err := rest.rpcClient.Call("",
		"Cluster",
		"Version",
		struct{}{},
		&v)

	sendResponse(w, err, v)
}

func (rest *RESTAPI) peerListHandler(w http.ResponseWriter, r *http.Request) {
	var peersSerial []api.IDSerial
	err := rest.rpcClient.Call("",
		"Cluster",
		"Peers",
		struct{}{},
		&peersSerial)

	sendResponse(w, err, peersSerial)
}

func (rest *RESTAPI) peerAddHandler(w http.ResponseWriter, r *http.Request) {
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

	var ids api.IDSerial
	err = rest.rpcClient.Call("",
		"Cluster",
		"PeerAdd",
		api.MultiaddrToSerial(mAddr),
		&ids)
	sendResponse(w, err, ids)
}

func (rest *RESTAPI) peerRemoveHandler(w http.ResponseWriter, r *http.Request) {
	if p := parsePidOrError(w, r); p != "" {
		err := rest.rpcClient.Call("",
			"Cluster",
			"PeerRemove",
			p,
			&struct{}{})
		sendEmptyResponse(w, err)
	}
}

func (rest *RESTAPI) pinHandler(w http.ResponseWriter, r *http.Request) {
	if c := parseCidOrError(w, r); c.Cid != "" {
		err := rest.rpcClient.Call("",
			"Cluster",
			"Pin",
			c,
			&struct{}{})
		sendAcceptedResponse(w, err)
	}
}

func (rest *RESTAPI) unpinHandler(w http.ResponseWriter, r *http.Request) {
	if c := parseCidOrError(w, r); c.Cid != "" {
		err := rest.rpcClient.Call("",
			"Cluster",
			"Unpin",
			c,
			&struct{}{})
		sendAcceptedResponse(w, err)
	}
}

func (rest *RESTAPI) pinListHandler(w http.ResponseWriter, r *http.Request) {
	var pins []api.PinSerial
	err := rest.rpcClient.Call("",
		"Cluster",
		"PinList",
		struct{}{},
		&pins)
	sendResponse(w, err, pins)
}

func (rest *RESTAPI) statusAllHandler(w http.ResponseWriter, r *http.Request) {
	var pinInfos []api.GlobalPinInfoSerial
	err := rest.rpcClient.Call("",
		"Cluster",
		"StatusAll",
		struct{}{},
		&pinInfos)
	sendResponse(w, err, pinInfos)
}

func (rest *RESTAPI) statusHandler(w http.ResponseWriter, r *http.Request) {
	if c := parseCidOrError(w, r); c.Cid != "" {
		var pinInfo api.GlobalPinInfoSerial
		err := rest.rpcClient.Call("",
			"Cluster",
			"Status",
			c,
			&pinInfo)
		sendResponse(w, err, pinInfo)
	}
}

func (rest *RESTAPI) syncAllHandler(w http.ResponseWriter, r *http.Request) {
	var pinInfos []api.GlobalPinInfoSerial
	err := rest.rpcClient.Call("",
		"Cluster",
		"SyncAll",
		struct{}{},
		&pinInfos)
	sendResponse(w, err, pinInfos)
}

func (rest *RESTAPI) syncHandler(w http.ResponseWriter, r *http.Request) {
	if c := parseCidOrError(w, r); c.Cid != "" {
		var pinInfo api.GlobalPinInfoSerial
		err := rest.rpcClient.Call("",
			"Cluster",
			"Sync",
			c,
			&pinInfo)
		sendResponse(w, err, pinInfo)
	}
}

func (rest *RESTAPI) recoverHandler(w http.ResponseWriter, r *http.Request) {
	if c := parseCidOrError(w, r); c.Cid != "" {
		var pinInfo api.GlobalPinInfoSerial
		err := rest.rpcClient.Call("",
			"Cluster",
			"Recover",
			c,
			&pinInfo)
		sendResponse(w, err, pinInfo)
	}
}

func parseCidOrError(w http.ResponseWriter, r *http.Request) api.PinSerial {
	vars := mux.Vars(r)
	hash := vars["hash"]

	_, err := cid.Decode(hash)
	if err != nil {
		sendErrorResponse(w, 400, "error decoding Cid: "+err.Error())
		return api.PinSerial{Cid: ""}
	}

	pin := api.PinSerial{
		Cid: hash,
	}

	queryValues := r.URL.Query()
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
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		panic(err)
	}
}

func sendErrorResponse(w http.ResponseWriter, code int, msg string) {
	errorResp := errorResp{code, msg}
	logger.Errorf("sending error response: %d: %s", code, msg)
	sendJSONResponse(w, code, errorResp)
}
