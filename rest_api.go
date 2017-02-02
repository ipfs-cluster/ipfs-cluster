package ipfscluster

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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
	ctx        context.Context
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

type versionResp struct {
	Version string `json:"version"`
}

type pinResp struct {
	Pinned string `json:"pinned"`
}

type unpinResp struct {
	Unpinned string `json:"unpinned"`
}

type statusInfo struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type statusCidResp struct {
	Cid     string                `json:"cid"`
	PeerMap map[string]statusInfo `json:"peer_map"`
}

type restIPFSIDResp struct {
	ID        string   `json:"id"`
	Addresses []string `json:"addresses"`
	Error     string   `json:"error,omitempty"`
}

func newRestIPFSIDResp(id IPFSID) *restIPFSIDResp {
	addrs := make([]string, len(id.Addresses), len(id.Addresses))
	for i, a := range id.Addresses {
		addrs[i] = a.String()
	}

	return &restIPFSIDResp{
		ID:        id.ID.Pretty(),
		Addresses: addrs,
		Error:     id.Error,
	}
}

type restIDResp struct {
	ID                 string          `json:"id"`
	PublicKey          string          `json:"public_key"`
	Addresses          []string        `json:"addresses"`
	ClusterPeers       []string        `json:"cluster_peers"`
	Version            string          `json:"version"`
	Commit             string          `json:"commit"`
	RPCProtocolVersion string          `json:"rpc_protocol_version"`
	Error              string          `json:"error,omitempty"`
	IPFS               *restIPFSIDResp `json:"ipfs"`
}

func newRestIDResp(id ID) *restIDResp {
	pubKey := ""
	if id.PublicKey != nil {
		keyBytes, err := id.PublicKey.Bytes()
		if err == nil {
			pubKey = base64.StdEncoding.EncodeToString(keyBytes)
		}
	}
	addrs := make([]string, len(id.Addresses), len(id.Addresses))
	for i, a := range id.Addresses {
		addrs[i] = a.String()
	}
	peers := make([]string, len(id.ClusterPeers), len(id.ClusterPeers))
	for i, a := range id.ClusterPeers {
		peers[i] = a.String()
	}
	return &restIDResp{
		ID:                 id.ID.Pretty(),
		PublicKey:          pubKey,
		Addresses:          addrs,
		ClusterPeers:       peers,
		Version:            id.Version,
		Commit:             id.Commit,
		RPCProtocolVersion: string(id.RPCProtocolVersion),
		Error:              id.Error,
		IPFS:               newRestIPFSIDResp(id.IPFS),
	}
}

type statusResp []statusCidResp

// NewRESTAPI creates a new object which is ready to be
// started.
func NewRESTAPI(cfg *Config) (*RESTAPI, error) {
	ctx := context.Background()

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

	api := &RESTAPI{
		ctx:        ctx,
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

func (api *RESTAPI) routes() []route {
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
			"Pins",
			"GET",
			"/pinlist",
			api.pinListHandler,
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
	}
}

func (api *RESTAPI) run() {
	api.wg.Add(1)
	go func() {
		defer api.wg.Done()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		api.ctx = ctx

		<-api.rpcReady

		logger.Infof("REST API: %s", api.apiAddr)
		err := api.server.Serve(api.listener)
		if err != nil && !strings.Contains(err.Error(), "closed network connection") {
			logger.Error(err)
		}
	}()
}

// Shutdown stops any API listeners.
func (api *RESTAPI) Shutdown() error {
	api.shutdownLock.Lock()
	defer api.shutdownLock.Unlock()

	if api.shutdown {
		logger.Debug("already shutdown")
		return nil
	}

	logger.Info("stopping Cluster API")

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
func (api *RESTAPI) SetClient(c *rpc.Client) {
	api.rpcClient = c
	api.rpcReady <- struct{}{}
}

func (api *RESTAPI) idHandler(w http.ResponseWriter, r *http.Request) {
	idSerial := IDSerial{}
	err := api.rpcClient.Call("",
		"Cluster",
		"ID",
		struct{}{},
		&idSerial)
	if checkRPCErr(w, err) {
		resp := newRestIDResp(idSerial.ToID())
		sendJSONResponse(w, 200, resp)
	}
}

func (api *RESTAPI) versionHandler(w http.ResponseWriter, r *http.Request) {
	var v string
	err := api.rpcClient.Call("",
		"Cluster",
		"Version",
		struct{}{},
		&v)

	if checkRPCErr(w, err) {
		sendJSONResponse(w, 200, versionResp{v})
	}
}

func (api *RESTAPI) peerListHandler(w http.ResponseWriter, r *http.Request) {
	var peersSerial []IDSerial
	err := api.rpcClient.Call("",
		"Cluster",
		"Peers",
		struct{}{},
		&peersSerial)

	if checkRPCErr(w, err) {
		var resp []*restIDResp
		for _, pS := range peersSerial {
			p := pS.ToID()
			resp = append(resp, newRestIDResp(p))
		}
		sendJSONResponse(w, 200, resp)
	}
}

func (api *RESTAPI) peerAddHandler(w http.ResponseWriter, r *http.Request) {
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

	var ids IDSerial
	err = api.rpcClient.Call("",
		"Cluster",
		"PeerAdd",
		MultiaddrToSerial(mAddr),
		&ids)
	if checkRPCErr(w, err) {
		resp := newRestIDResp(ids.ToID())
		sendJSONResponse(w, 200, resp)
	}
}

func (api *RESTAPI) peerRemoveHandler(w http.ResponseWriter, r *http.Request) {
	if p := parsePidOrError(w, r); p != "" {
		err := api.rpcClient.Call("",
			"Cluster",
			"PeerRemove",
			p,
			&struct{}{})
		if checkRPCErr(w, err) {
			sendEmptyResponse(w)
		}
	}
}

func (api *RESTAPI) pinHandler(w http.ResponseWriter, r *http.Request) {
	if c := parseCidOrError(w, r); c != nil {
		err := api.rpcClient.Call("",
			"Cluster",
			"Pin",
			c,
			&struct{}{})
		if checkRPCErr(w, err) {
			sendAcceptedResponse(w)
		}
	}
}

func (api *RESTAPI) unpinHandler(w http.ResponseWriter, r *http.Request) {
	if c := parseCidOrError(w, r); c != nil {
		err := api.rpcClient.Call("",
			"Cluster",
			"Unpin",
			c,
			&struct{}{})
		if checkRPCErr(w, err) {
			sendAcceptedResponse(w)
		}
	}
}

func (api *RESTAPI) pinListHandler(w http.ResponseWriter, r *http.Request) {
	var pins []string
	err := api.rpcClient.Call("",
		"Cluster",
		"PinList",
		struct{}{},
		&pins)
	if checkRPCErr(w, err) {
		sendJSONResponse(w, 200, pins)
	}

}

func (api *RESTAPI) statusAllHandler(w http.ResponseWriter, r *http.Request) {
	var pinInfos []GlobalPinInfo
	err := api.rpcClient.Call("",
		"Cluster",
		"StatusAll",
		struct{}{},
		&pinInfos)
	if checkRPCErr(w, err) {
		sendStatusResponse(w, http.StatusOK, pinInfos)
	}
}

func (api *RESTAPI) statusHandler(w http.ResponseWriter, r *http.Request) {
	if c := parseCidOrError(w, r); c != nil {
		var pinInfo GlobalPinInfo
		err := api.rpcClient.Call("",
			"Cluster",
			"Status",
			c,
			&pinInfo)
		if checkRPCErr(w, err) {
			sendStatusCidResponse(w, http.StatusOK, pinInfo)
		}
	}
}

func (api *RESTAPI) syncAllHandler(w http.ResponseWriter, r *http.Request) {
	var pinInfos []GlobalPinInfo
	err := api.rpcClient.Call("",
		"Cluster",
		"SyncAll",
		struct{}{},
		&pinInfos)
	if checkRPCErr(w, err) {
		sendStatusResponse(w, http.StatusAccepted, pinInfos)
	}
}

func (api *RESTAPI) syncHandler(w http.ResponseWriter, r *http.Request) {
	if c := parseCidOrError(w, r); c != nil {
		var pinInfo GlobalPinInfo
		err := api.rpcClient.Call("",
			"Cluster",
			"Sync",
			c,
			&pinInfo)
		if checkRPCErr(w, err) {
			sendStatusCidResponse(w, http.StatusOK, pinInfo)
		}
	}
}

func (api *RESTAPI) recoverHandler(w http.ResponseWriter, r *http.Request) {
	if c := parseCidOrError(w, r); c != nil {
		var pinInfo GlobalPinInfo
		err := api.rpcClient.Call("",
			"Cluster",
			"Recover",
			c,
			&pinInfo)
		if checkRPCErr(w, err) {
			sendStatusCidResponse(w, http.StatusOK, pinInfo)
		}
	}
}

func parseCidOrError(w http.ResponseWriter, r *http.Request) *CidArg {
	vars := mux.Vars(r)
	hash := vars["hash"]
	_, err := cid.Decode(hash)
	if err != nil {
		sendErrorResponse(w, 400, "error decoding Cid: "+err.Error())
		return nil
	}
	return &CidArg{hash}
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

func sendEmptyResponse(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func sendAcceptedResponse(w http.ResponseWriter) {
	w.WriteHeader(http.StatusAccepted)
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

func transformPinToStatusCid(p GlobalPinInfo) statusCidResp {
	s := statusCidResp{}
	s.Cid = p.Cid.String()
	s.PeerMap = make(map[string]statusInfo)
	for k, v := range p.PeerMap {
		s.PeerMap[k.Pretty()] = statusInfo{
			Status: v.Status.String(),
			Error:  v.Error,
		}
	}
	return s
}

func sendStatusResponse(w http.ResponseWriter, code int, data []GlobalPinInfo) {
	pins := make(statusResp, 0, len(data))

	for _, d := range data {
		pins = append(pins, transformPinToStatusCid(d))
	}
	sendJSONResponse(w, code, pins)
}

func sendStatusCidResponse(w http.ResponseWriter, code int, data GlobalPinInfo) {
	st := transformPinToStatusCid(data)
	sendJSONResponse(w, code, st)
}
