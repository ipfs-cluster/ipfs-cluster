package ipfscluster

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	peer "github.com/libp2p/go-libp2p-peer"

	cid "github.com/ipfs/go-cid"

	mux "github.com/gorilla/mux"
)

// RESTAPI implements an API and aims to provides
// a RESTful HTTP API for Cluster.
type RESTAPI struct {
	ctx        context.Context
	listenAddr string
	listenPort int
	rpcCh      chan RPC
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

type statusCidResp struct {
	Cid    string `json:"cid"`
	Status string `json:"status"`
}

type statusResp []statusCidResp

// NewHTTPAPI creates a new object which is ready to be
// started.
func NewHTTPAPI(cfg *Config) (*RESTAPI, error) {
	ctx := context.Background()
	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d",
		cfg.APIAddr,
		cfg.APIPort))
	if err != nil {
		return nil, err
	}

	router := mux.NewRouter().StrictSlash(true)
	s := &http.Server{Handler: router}
	s.SetKeepAlivesEnabled(true) // A reminder that this can be changed

	api := &RESTAPI{
		ctx:        ctx,
		listenAddr: cfg.APIAddr,
		listenPort: cfg.APIPort,
		listener:   l,
		server:     s,
		rpcCh:      make(chan RPC, RPCMaxQueue),
	}

	for _, route := range api.routes() {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(route.HandlerFunc)
	}

	api.router = router
	logger.Infof("starting Cluster API on %s:%d", api.listenAddr, api.listenPort)
	api.run()
	return api, nil
}

func (api *RESTAPI) routes() []route {
	return []route{
		route{
			"Members",
			"GET",
			"/members",
			api.memberListHandler,
		},
		route{
			"Pins",
			"GET",
			"/pins",
			api.pinListHandler,
		},
		route{
			"Version",
			"GET",
			"/version",
			api.versionHandler,
		},
		route{
			"Pin",
			"POST",
			"/pins/{hash}",
			api.pinHandler,
		},
		route{
			"Unpin",
			"DELETE",
			"/pins/{hash}",
			api.unpinHandler,
		},
		route{
			"Status",
			"GET",
			"/status",
			api.statusHandler,
		},
		route{
			"StatusCid",
			"GET",
			"/status/{hash}",
			api.statusCidHandler,
		},
		route{
			"Sync",
			"POST",
			"/status",
			api.syncHandler,
		},
		route{
			"SyncCid",
			"POST",
			"/status/{hash}",
			api.syncCidHandler,
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

	// Cancel any outstanding ops
	api.server.SetKeepAlivesEnabled(false)
	api.listener.Close()

	api.wg.Wait()
	api.shutdown = true
	return nil
}

// RpcChan can be used by Cluster to read any
// requests from this component
func (api *RESTAPI) RpcChan() <-chan RPC {
	return api.rpcCh
}

func (api *RESTAPI) versionHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()
	rRpc := NewRPC(VersionRPC, nil)
	resp := MakeRPC(ctx, api.rpcCh, rRpc, true)
	if checkResponse(w, rRpc.Op(), resp) {
		v := resp.Data.(string)
		sendJSONResponse(w, 200, versionResp{v})
	}
}

func (api *RESTAPI) memberListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()
	rRpc := NewRPC(MemberListRPC, nil)
	resp := MakeRPC(ctx, api.rpcCh, rRpc, true)
	if checkResponse(w, rRpc.Op(), resp) {
		data := resp.Data.([]peer.ID)
		var strPeers []string
		for _, p := range data {
			strPeers = append(strPeers, p.Pretty())
		}
		sendJSONResponse(w, 200, strPeers)
	}
}

func (api *RESTAPI) pinHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()

	if c := parseCidOrError(w, r); c != nil {
		rRpc := NewRPC(PinRPC, c)
		resp := MakeRPC(ctx, api.rpcCh, rRpc, true)
		if checkResponse(w, rRpc.Op(), resp) {
			sendAcceptedResponse(w)
		}
	}
}

func (api *RESTAPI) unpinHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()

	if c := parseCidOrError(w, r); c != nil {
		rRpc := NewRPC(UnpinRPC, c)
		resp := MakeRPC(ctx, api.rpcCh, rRpc, true)
		if checkResponse(w, rRpc.Op(), resp) {
			sendAcceptedResponse(w)
		}
	}
}

func (api *RESTAPI) pinListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()
	rRpc := NewRPC(PinListRPC, nil)
	resp := MakeRPC(ctx, api.rpcCh, rRpc, true)
	if checkResponse(w, rRpc.Op(), resp) {
		data := resp.Data.([]*cid.Cid)
		sendJSONResponse(w, 200, data)
	}

}

func (api *RESTAPI) statusHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()
	rRpc := NewRPC(StatusRPC, nil)
	resp := MakeRPC(ctx, api.rpcCh, rRpc, true)
	if checkResponse(w, rRpc.Op(), resp) {
		sendStatusResponse(w, resp)
	}
}

func (api *RESTAPI) statusCidHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()

	if c := parseCidOrError(w, r); c != nil {
		op := NewRPC(StatusCidRPC, c)
		resp := MakeRPC(ctx, api.rpcCh, op, true)
		if checkResponse(w, op.Op(), resp) {
			sendStatusCidResponse(w, resp)
		}
	}
}

func (api *RESTAPI) syncHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()
	rRpc := NewRPC(LocalSyncRPC, nil)
	resp := MakeRPC(ctx, api.rpcCh, rRpc, true)
	if checkResponse(w, rRpc.Op(), resp) {
		sendStatusResponse(w, resp)
	}
}

func (api *RESTAPI) syncCidHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()

	if c := parseCidOrError(w, r); c != nil {
		op := NewRPC(LocalSyncCidRPC, c)
		resp := MakeRPC(ctx, api.rpcCh, op, true)
		if checkResponse(w, op.Op(), resp) {
			sendStatusCidResponse(w, resp)
		}
	}
}

func parseCidOrError(w http.ResponseWriter, r *http.Request) *cid.Cid {
	vars := mux.Vars(r)
	hash := vars["hash"]
	c, err := cid.Decode(hash)
	if err != nil {
		sendErrorResponse(w, 400, "error decoding Cid: "+err.Error())
		return nil
	}
	return c
}

// checkResponse does basic checking on an RPCResponse. It takes care of
// using the http.ResponseWriter to send
// an error if the RPCResponse contains one. It also checks that the RPC
// response data can be casted back into the expected value. It returns false
// if the checks fail or an empty response is sent, and true otherwise.
func checkResponse(w http.ResponseWriter, op RPCOp, resp RPCResponse) bool {
	if err := resp.Error; err != nil {
		sendErrorResponse(w, 500, err.Error())
		return false
	}

	// Check thatwe can cast to the expected response format
	ok := true
	switch op {
	case PinRPC: // Pin/Unpin only return errors
	case UnpinRPC:
	case StatusRPC, LocalSyncRPC, GlobalSyncRPC:
		_, ok = resp.Data.([]Pin)
	case StatusCidRPC, LocalSyncCidRPC, GlobalSyncCidRPC:
		_, ok = resp.Data.(Pin)
	case PinListRPC:
		_, ok = resp.Data.([]*cid.Cid)
	case IPFSPinRPC:
	case IPFSUnpinRPC:
	case VersionRPC:
		_, ok = resp.Data.(string)
	case MemberListRPC:
		_, ok = resp.Data.([]peer.ID)
	default:
		ok = false
	}
	if !ok {
		logger.Errorf("unexpected RPC Response format for %d:", op)
		logger.Errorf("%+v", resp.Data)
		sendErrorResponse(w, 500, "unexpected RPC Response format")
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

func sendStatusResponse(w http.ResponseWriter, resp RPCResponse) {
	data := resp.Data.([]Pin)
	pins := make(statusResp, 0, len(data))
	for _, d := range data {
		pins = append(pins, statusCidResp{
			Cid:    d.Cid.String(),
			Status: d.Status.String(),
		})
	}
	sendJSONResponse(w, 200, pins)
}

func sendStatusCidResponse(w http.ResponseWriter, resp RPCResponse) {
	data := resp.Data.(Pin)
	pin := statusCidResp{
		Cid:    data.Cid.String(),
		Status: data.Status.String(),
	}
	sendJSONResponse(w, 200, pin)
}
