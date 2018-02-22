// Package rest implements an IPFS Cluster API component. It provides
// a REST-ish API to interact with Cluster over HTTP.
package rest

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"math/rand"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	types "github.com/ipfs/ipfs-cluster/api"
	importer "github.com/ipfs/ipfs-cluster/ipld-importer"

	mux "github.com/gorilla/mux"
	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipfs-cmdkit/files"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var logger = logging.Logger("restapi")

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

// HTTPAddress returns the HTTP(s) listening address
// in host:port format. Useful when configured to start
// on a random port (0).
func (api *API) HTTPAddress() string {
	return api.listener.Addr().String()
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
		{
			"FilesAdd",
			"POST",
			"/allocations",
			api.addFileHandler,
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

func (api *API) graphHandler(w http.ResponseWriter, r *http.Request) {
	var graph types.ConnectGraphSerial
	err := api.rpcClient.Call("",
		"Cluster",
		"ConnectGraph",
		struct{}{},
		&graph)
	sendResponse(w, err, graph)
}

func (api *API) addFile(ctx context.Context, outChan <-chan *ipld.Node, w http.ResponseWriter) error {
	for nodePtr := range outChan {
		select {
		case <-ctx.Done():
			sendErrorResponse(w, 504, "context timeout terminated add")
			return errors.New("context timeout terminated add")
		default:
		}
		node := *nodePtr
		/* Send block data to ipfs */
		var hash string
		/*		c := node.Cid()
				format, ok := cid.CodecToStr[c.Type()]
				if !ok {
					format = ""
				}*/
		b := types.BlockWithFormat{
			Data:   node.RawData(),
			Format: "",
		}
		err := api.rpcClient.Call("",
			"Cluster",
			"IPFSBlockPut",
			b,
			&hash)
		/* Verify that block put cid matches */
		if node.String() != hash { // node string is just cid string
			logger.Warningf("mismatch. node cid: %s\nrpc cid: %s", node.String(), hash)
		}
		if err != nil {
			// TODO: think about how to handle this better.
			// We may not want to stop all work after one failure
			logger.Error(err)
			sendErrorResponse(w, 500, "error forwarding block")
			return err
		}
	}

	// TODO: when complete this call should answer with a cid
	// or better yet a pin-info describing the allocations
	// of the resulting allocation
	//
	// Before returning this we will need to trigger a pin
	// probably doing the same thing as Pin if its not a sharding call
	//
	// This is actually a bit tricky because right now we are not
	// recording the root of the imported file, we could have ToChannel
	// provide a root channel that can be read once the other channel is
	// closed that reports the root at the end.  Probably would need to
	// rewrite add for this to work
	return nil
}

// Get a random string of length n.  Used to generate sharding id
func randStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func (api *API) addShardedFile(ctx context.Context, outChan <-chan *ipld.Node, w http.ResponseWriter) error {
	// chosen so pow(2, 256) ~= pow(52, X) => X = 45.  Probably over thinking
	shardID := randStringRunes(45)
	for nodePtr := range outChan {
		select {
		case <-ctx.Done():
			sendErrorResponse(w, 504, "internal timeout")
			return errors.New("context timeout terminated add")
		default:
		}
		node := *nodePtr
		size, err := node.Size()
		if err != nil {
			logger.Error(err)
			sendErrorResponse(w, 500, "error getting size of node")
			return err
		}
		nodeS := types.ShardNodeSerial{
			Cid:  node.Cid().String(),
			Data: node.RawData(),
			Size: size,
			ID:   shardID,
		}
		err = api.rpcClient.Call("",
			"Cluster",
			"ShardAddNode",
			nodeS,
			&struct{}{})
		if err != nil {
			// TODO: even more important than in local add,
			// we should think about the best way to handle this
			// as we may not want to halt sharding with one error.
			// Retry? Carry on with missing information? Get user
			// feedback?
			logger.Error(err)
			sendErrorResponse(w, 500, "error adding block to shard")
			return err
		}
	}
	// Last node of final shard may not have pushed over the threshold,
	// force clusterDAG serialization and cluster pin tracking
	err := api.rpcClient.Call("",
		"Cluster",
		"ShardFinalize",
		shardID,
		&struct{}{})
	if err != nil {
		logger.Error(err)
		sendErrorResponse(w, 500, "error flushing final shard")
		return err
	}
	return nil
}

func (api *API) addFileHandler(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")
	mediatype, _, _ := mime.ParseMediaType(contentType)
	var f files.File
	if mediatype == "multipart/form-data" {
		reader, err := r.MultipartReader()
		if err != nil {
			sendErrorResponse(w, 400, err.Error())
			return
		}

		f = &files.MultipartFile{
			Mediatype: mediatype,
			Reader:    reader,
		}
	} else {
		sendErrorResponse(w, 415, "unsupported media type")
		return
	}

	ctx, cancel := context.WithCancel(api.ctx)
	defer cancel()

	outChan, err := importer.ToChannel(ctx, f)

	queryValues := r.URL.Query()
	shard := queryValues.Get("shard")

	if shard == "true" {
		if err := api.addShardedFile(ctx, outChan, w); err != nil {
			return
		}

	} else {
		if err := api.addFile(ctx, outChan, w); err != nil {
			return
		}
	}
	sendAcceptedResponse(w, err)
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
