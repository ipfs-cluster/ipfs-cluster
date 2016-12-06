package ipfscluster

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	peer "gx/ipfs/QmfMmLGoKzCHDN7cGgk64PJr4iipzidDRME8HABSJqvmhC/go-libp2p-peer"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"

	mux "github.com/gorilla/mux"
)

// ClusterHTTPAPI implements a ClusterAPI and aims to provides
// a RESTful HTTP API for Cluster.
type ClusterHTTPAPI struct {
	ctx        context.Context
	cancel     context.CancelFunc
	listenAddr string
	listenPort int
	rpcCh      chan ClusterRPC
	router     *mux.Router

	listener net.Listener

	doneCh     chan bool
	shutdownCh chan bool
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

type pinElemResp struct {
	Cid    string `json:"cid"`
	Status string `json:"status"`
}

type pinListResp []pinElemResp

// NewHTTPClusterAPI creates a new object which is ready to be
// started.
func NewHTTPClusterAPI(cfg *ClusterConfig) (*ClusterHTTPAPI, error) {
	ctx, cancel := context.WithCancel(context.Background())
	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d",
		cfg.ClusterAPIListenAddr,
		cfg.ClusterAPIListenPort))
	if err != nil {
		return nil, err
	}

	api := &ClusterHTTPAPI{
		ctx:        ctx,
		cancel:     cancel,
		listenAddr: cfg.ClusterAPIListenAddr,
		listenPort: cfg.ClusterAPIListenPort,
		listener:   l,
		rpcCh:      make(chan ClusterRPC, RPCMaxQueue),
		doneCh:     make(chan bool),
		shutdownCh: make(chan bool),
	}

	router := mux.NewRouter().StrictSlash(true)
	for _, route := range api.routes() {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(route.HandlerFunc)
	}

	api.router = router
	logger.Infof("Starting Cluster API on %s:%d", api.listenAddr, api.listenPort)
	go api.run()
	return api, nil
}

func (api *ClusterHTTPAPI) routes() []route {
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
	}
}

func (api *ClusterHTTPAPI) run() {
	go func() {
		err := http.Serve(api.listener, api.router)
		select {
		case <-api.shutdownCh:
			close(api.doneCh)
		default:
			if err != nil {
				logger.Error(err)
			}
		}
	}()
}

// Shutdown stops any API listeners.
func (api *ClusterHTTPAPI) Shutdown() error {
	logger.Info("Stopping Cluster API")
	api.cancel()
	close(api.shutdownCh)
	api.listener.Close()
	<-api.doneCh
	return nil
}

// RpcChan can be used by Cluster to read any
// requests from this component
func (api *ClusterHTTPAPI) RpcChan() <-chan ClusterRPC {
	return api.rpcCh
}

func (api *ClusterHTTPAPI) versionHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()
	rRpc := RPC(VersionRPC, nil)
	resp := MakeRPC(ctx, api.rpcCh, rRpc, true)
	if checkResponse(w, rRpc.Op(), resp) {
		v := resp.Data.(string)
		sendJSONResponse(w, 200, versionResp{v})
	}
}

func (api *ClusterHTTPAPI) memberListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()
	rRpc := RPC(MemberListRPC, nil)
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

func (api *ClusterHTTPAPI) pinHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()
	vars := mux.Vars(r)
	hash := vars["hash"]
	c, err := cid.Decode(hash)
	if err != nil {
		sendErrorResponse(w, 400, err.Error())
	}

	rRpc := RPC(PinRPC, c)
	resp := MakeRPC(ctx, api.rpcCh, rRpc, true)

	if checkResponse(w, rRpc.Op(), resp) {
		c := resp.Data.(cid.Cid)
		sendJSONResponse(w, 200, pinResp{c.String()})
	}
}

func (api *ClusterHTTPAPI) unpinHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()
	vars := mux.Vars(r)
	hash := vars["hash"]
	c, err := cid.Decode(hash)
	if err != nil {
		sendErrorResponse(w, 400, err.Error())
	}

	rRpc := RPC(UnpinRPC, c)
	resp := MakeRPC(ctx, api.rpcCh, rRpc, true)
	if checkResponse(w, rRpc.Op(), resp) {
		c := resp.Data.(cid.Cid)
		sendJSONResponse(w, 200, unpinResp{c.String()})
	}
}

func (api *ClusterHTTPAPI) pinListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()
	rRpc := RPC(PinListRPC, nil)
	resp := MakeRPC(ctx, api.rpcCh, rRpc, true)
	if checkResponse(w, rRpc.Op(), resp) {
		data := resp.Data.([]Pin)
		pins := make(pinListResp, 0, len(data))
		for _, d := range data {
			var st string
			switch d.Status {
			case Error:
				st = "error"
			case Pinned:
				st = "pinned"
			case Pinning:
				st = "pinning"
			case Unpinning:
				st = "unpinning"
			}
			pins = append(pins, pinElemResp{
				Cid:    d.Cid,
				Status: st,
			})
		}
		sendJSONResponse(w, 200, pins)
	}

}

// checkResponse does basic checking on an RPCResponse. It takes care of
// using the http.ResponseWriter to send an empty response, or to send
// an error if the RPCResponse contains one. It also checks that the RPC
// response data can be casted back into the expected value. It returns false
// if the checks fail or an empty response is sent, and true otherwise.
func checkResponse(w http.ResponseWriter, op RPCOp, resp RPCResponse) bool {
	if resp.Error == nil && resp.Data == nil {
		sendEmptyResponse(w)
		return false
	} else if err := resp.Error; err != nil {
		sendErrorResponse(w, 500, err.Error())
		return false
	}

	// Check thatwe can cast to the expected response format
	ok := true
	switch op {
	case PinRPC:
		_, ok = resp.Data.(cid.Cid)
	case UnpinRPC:
		_, ok = resp.Data.(cid.Cid)
	case PinListRPC:
		_, ok = resp.Data.([]Pin)
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
		logger.Error("unexpected RPC Response format")
		sendErrorResponse(w, 500, "Unexpected RPC Response format")
		return false
	}

	return true
}

func sendEmptyResponse(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func sendJSONResponse(w http.ResponseWriter, code int, resp interface{}) {
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		panic(err)
	}
}

func sendErrorResponse(w http.ResponseWriter, code int, msg string) {
	errorResp := errorResp{code, msg}
	logger.Errorf("Sending error response: %d: %s", code, msg)
	sendJSONResponse(w, code, errorResp)
}
