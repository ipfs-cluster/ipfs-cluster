package ipfsproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/adder/adderutils"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/rpcutil"
	"github.com/ipfs/ipfs-cluster/version"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p-peer"
	madns "github.com/multiformats/go-multiaddr-dns"
	manet "github.com/multiformats/go-multiaddr-net"
)

// DNSTimeout is used when resolving DNS multiaddresses in this module
var DNSTimeout = 5 * time.Second

var logger = logging.Logger("ipfsproxy")

var ipfsHeaderList = []string{
	"Server",
	"Access-Control-Allow-Headers",
	"Access-Control-Expose-Headers",
	"Trailer",
	"Vary",
}

// Server offers an IPFS API, hijacking some interesting requests
// and forwarding the rest to the ipfs daemon
// it proxies HTTP requests to the configured IPFS
// daemon. It is able to intercept these requests though, and
// perform extra operations on them.
type Server struct {
	ctx    context.Context
	cancel func()

	config   *Config
	nodeAddr string

	rpcClient *rpc.Client
	rpcReady  chan struct{}

	listener net.Listener // proxy listener
	server   *http.Server // proxy server

	onceHeaders sync.Once
	ipfsHeaders sync.Map

	shutdownLock sync.Mutex
	shutdown     bool
	wg           sync.WaitGroup
}

// An http.Handler through which all proxied calls
// must pass (wraps the actual handler).
type proxyHandler struct {
	server  *Server
	handler http.Handler
}

// ServeHTTP extracts interesting headers returned by IPFS responses
// and stores them in our cache.
func (ph *proxyHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	ph.handler.ServeHTTP(rw, req)

	hdrs := make(http.Header)
	ok := ph.server.setIPFSHeaders(hdrs)
	if !ok {
		// we are missing some headers we want, try
		// to copy the ones coming on this proxied request.
		srcHeaders := rw.Header()
		for _, k := range ipfsHeaderList {
			ph.server.ipfsHeaders.Store(k, srcHeaders[k])
		}
	}
}

type ipfsError struct {
	Message string
}

type ipfsPinType struct {
	Type string
}

type ipfsPinLsResp struct {
	Keys map[string]ipfsPinType
}

type ipfsPinOpResp struct {
	Pins []string
}

// From https://github.com/ipfs/go-ipfs/blob/master/core/coreunix/add.go#L49
type ipfsAddResp struct {
	Name  string
	Hash  string `json:",omitempty"`
	Bytes int64  `json:",omitempty"`
	Size  string `json:",omitempty"`
}

// New returns and ipfs Proxy component
func New(cfg *Config) (*Server, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	nodeMAddr := cfg.NodeAddr
	// dns multiaddresses need to be resolved first
	if madns.Matches(nodeMAddr) {
		ctx, cancel := context.WithTimeout(context.Background(), DNSTimeout)
		defer cancel()
		resolvedAddrs, err := madns.Resolve(ctx, cfg.NodeAddr)
		if err != nil {
			logger.Error(err)
			return nil, err
		}
		nodeMAddr = resolvedAddrs[0]
	}

	_, nodeAddr, err := manet.DialArgs(nodeMAddr)
	if err != nil {
		return nil, err
	}

	proxyNet, proxyAddr, err := manet.DialArgs(cfg.ListenAddr)
	if err != nil {
		return nil, err
	}

	l, err := net.Listen(proxyNet, proxyAddr)
	if err != nil {
		return nil, err
	}

	nodeHTTPAddr := "http://" + nodeAddr
	proxyURL, err := url.Parse(nodeHTTPAddr)
	if err != nil {
		return nil, err
	}

	smux := http.NewServeMux()
	s := &http.Server{
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		Handler:           smux,
	}

	// See: https://github.com/ipfs/go-ipfs/issues/5168
	// See: https://github.com/ipfs/ipfs-cluster/issues/548
	// on why this is re-enabled.
	s.SetKeepAlivesEnabled(true) // A reminder that this can be changed

	ctx, cancel := context.WithCancel(context.Background())

	proxy := &Server{
		ctx:      ctx,
		config:   cfg,
		cancel:   cancel,
		nodeAddr: nodeAddr,
		rpcReady: make(chan struct{}, 1),
		listener: l,
		server:   s,
	}

	proxyHandler := &proxyHandler{
		server:  proxy,
		handler: httputil.NewSingleHostReverseProxy(proxyURL),
	}

	smux.Handle("/", proxyHandler)
	smux.HandleFunc("/api/v0/pin/add", proxy.pinHandler)   // add?arg=xxx
	smux.HandleFunc("/api/v0/pin/add/", proxy.pinHandler)  // add/xxx
	smux.HandleFunc("/api/v0/pin/rm", proxy.unpinHandler)  // rm?arg=xxx
	smux.HandleFunc("/api/v0/pin/rm/", proxy.unpinHandler) // rm/xxx
	smux.HandleFunc("/api/v0/pin/ls", proxy.pinLsHandler)  // required to handle /pin/ls for all pins
	smux.HandleFunc("/api/v0/pin/ls/", proxy.pinLsHandler) // ls/xxx
	smux.HandleFunc("/api/v0/add", proxy.addHandler)
	smux.HandleFunc("/api/v0/repo/stat", proxy.repoStatHandler)

	go proxy.run()
	return proxy, nil
}

// SetClient makes the component ready to perform RPC
// requests.
func (proxy *Server) SetClient(c *rpc.Client) {
	proxy.rpcClient = c
	proxy.rpcReady <- struct{}{}
}

// Shutdown stops any listeners and stops the component from taking
// any requests.
func (proxy *Server) Shutdown() error {
	proxy.shutdownLock.Lock()
	defer proxy.shutdownLock.Unlock()

	if proxy.shutdown {
		logger.Debug("already shutdown")
		return nil
	}

	logger.Info("stopping IPFS Proxy")

	proxy.cancel()
	close(proxy.rpcReady)
	proxy.server.SetKeepAlivesEnabled(false)
	proxy.listener.Close()

	proxy.wg.Wait()
	proxy.shutdown = true
	return nil
}

// launches proxy when we receive the rpcReady signal.
func (proxy *Server) run() {
	<-proxy.rpcReady

	// Do not shutdown while launching threads
	// -- prevents race conditions with proxy.wg.
	proxy.shutdownLock.Lock()
	defer proxy.shutdownLock.Unlock()

	// This launches the proxy
	proxy.wg.Add(1)
	go func() {
		defer proxy.wg.Done()
		logger.Infof(
			"IPFS Proxy: %s -> %s",
			proxy.config.ListenAddr,
			proxy.config.NodeAddr,
		)
		err := proxy.server.Serve(proxy.listener) // hangs here
		if err != nil && !strings.Contains(err.Error(), "closed network connection") {
			logger.Error(err)
		}
	}()
}

// Handlers
func ipfsErrorResponder(w http.ResponseWriter, errMsg string) {
	res := ipfsError{errMsg}
	resBytes, _ := json.Marshal(res)
	w.WriteHeader(http.StatusInternalServerError)
	w.Write(resBytes)
	return
}

// setIPFSHeaders adds the known IPFS Headers to the destination
// and returns true if we could set all the headers in the list.
func (proxy *Server) setIPFSHeaders(dest http.Header) bool {
	r := true
	for _, h := range ipfsHeaderList {
		v, ok := proxy.ipfsHeaders.Load(h)
		if !ok {
			r = false
			continue
		}
		dest[h] = v.([]string)
	}
	return r
}

// Set headers that all hijacked endpoints share.
func (proxy *Server) setHeaders(dest http.Header) {
	if ok := proxy.setIPFSHeaders(dest); !ok {
		req, err := http.NewRequest("POST", "/api/v0/version", nil)
		if err != nil {
			logger.Error(err)
		} else {
			// We use the Recorder() ResponseWriter to simply
			// save implementing one ourselves.
			// This uses our proxy handler to trigger a proxied
			// request which will record the headers once completed.
			proxy.server.Handler.ServeHTTP(httptest.NewRecorder(), req)
			proxy.setIPFSHeaders(dest)
		}
	}

	// Set Cluster global headers for all hijacked requests
	dest.Set("Content-Type", "application/json")
	dest.Set("Server", fmt.Sprintf("ipfs-cluster/ipfsproxy/%s", version.Version))
}

func (proxy *Server) pinOpHandler(op string, w http.ResponseWriter, r *http.Request) {
	proxy.setHeaders(w.Header())

	arg, ok := extractArgument(r.URL)
	if !ok {
		ipfsErrorResponder(w, "Error: bad argument")
		return
	}
	c, err := cid.Decode(arg)
	if err != nil {
		ipfsErrorResponder(w, "Error parsing CID: "+err.Error())
		return
	}

	err = proxy.rpcClient.Call(
		"",
		"Cluster",
		op,
		api.PinCid(c).ToSerial(),
		&struct{}{},
	)
	if err != nil {
		ipfsErrorResponder(w, err.Error())
		return
	}

	res := ipfsPinOpResp{
		Pins: []string{arg},
	}
	resBytes, _ := json.Marshal(res)
	w.WriteHeader(http.StatusOK)
	w.Write(resBytes)
	return
}

func (proxy *Server) pinHandler(w http.ResponseWriter, r *http.Request) {
	proxy.pinOpHandler("Pin", w, r)
}

func (proxy *Server) unpinHandler(w http.ResponseWriter, r *http.Request) {
	proxy.pinOpHandler("Unpin", w, r)
}

func (proxy *Server) pinLsHandler(w http.ResponseWriter, r *http.Request) {
	proxy.setHeaders(w.Header())

	pinLs := ipfsPinLsResp{}
	pinLs.Keys = make(map[string]ipfsPinType)

	arg, ok := extractArgument(r.URL)
	if ok {
		c, err := cid.Decode(arg)
		if err != nil {
			ipfsErrorResponder(w, err.Error())
			return
		}
		var pin api.PinSerial
		err = proxy.rpcClient.Call(
			"",
			"Cluster",
			"PinGet",
			api.PinCid(c).ToSerial(),
			&pin,
		)
		if err != nil {
			ipfsErrorResponder(w, fmt.Sprintf("Error: path '%s' is not pinned", arg))
			return
		}
		pinLs.Keys[pin.Cid] = ipfsPinType{
			Type: "recursive",
		}
	} else {
		pins := make([]api.PinSerial, 0)
		err := proxy.rpcClient.Call(
			"",
			"Cluster",
			"Pins",
			struct{}{},
			&pins,
		)
		if err != nil {
			ipfsErrorResponder(w, err.Error())
			return
		}

		for _, pin := range pins {
			pinLs.Keys[pin.Cid] = ipfsPinType{
				Type: "recursive",
			}
		}
	}

	resBytes, _ := json.Marshal(pinLs)
	w.WriteHeader(http.StatusOK)
	w.Write(resBytes)
}

func (proxy *Server) addHandler(w http.ResponseWriter, r *http.Request) {
	proxy.setHeaders(w.Header())

	reader, err := r.MultipartReader()
	if err != nil {
		ipfsErrorResponder(w, "error reading request: "+err.Error())
		return
	}

	q := r.URL.Query()
	if q.Get("only-hash") == "true" {
		ipfsErrorResponder(w, "only-hash is not supported when adding to cluster")
	}

	unpin := q.Get("pin") == "false"

	// Luckily, most IPFS add query params are compatible with cluster's
	// /add params. We can parse most of them directly from the query.
	params, err := api.AddParamsFromQuery(q)
	if err != nil {
		ipfsErrorResponder(w, "error parsing options:"+err.Error())
		return
	}
	trickle := q.Get("trickle")
	if trickle == "true" {
		params.Layout = "trickle"
	}

	logger.Warningf("Proxy/add does not support all IPFS params. Current options: %+v", params)

	outputTransform := func(in *api.AddedOutput) interface{} {
		r := &ipfsAddResp{
			Name:  in.Name,
			Hash:  in.Cid,
			Bytes: int64(in.Bytes),
		}
		if in.Size != 0 {
			r.Size = strconv.FormatUint(in.Size, 10)
		}
		return r
	}

	root, err := adderutils.AddMultipartHTTPHandler(
		proxy.ctx,
		proxy.rpcClient,
		params,
		reader,
		w,
		outputTransform,
	)

	// any errors have been sent as Trailer
	if err != nil {
		return
	}

	if !unpin {
		return
	}

	// Unpin because the user doesn't want to pin
	time.Sleep(100 * time.Millisecond)
	err = proxy.rpcClient.CallContext(
		proxy.ctx,
		"",
		"Cluster",
		"Unpin",
		api.PinCid(root).ToSerial(),
		&struct{}{},
	)
	if err != nil {
		w.Header().Set("X-Stream-Error", err.Error())
		return
	}
}

func (proxy *Server) repoStatHandler(w http.ResponseWriter, r *http.Request) {
	proxy.setHeaders(w.Header())

	peers := make([]peer.ID, 0)
	err := proxy.rpcClient.Call(
		"",
		"Cluster",
		"ConsensusPeers",
		struct{}{},
		&peers,
	)
	if err != nil {
		ipfsErrorResponder(w, err.Error())
		return
	}

	ctxs, cancels := rpcutil.CtxsWithCancel(proxy.ctx, len(peers))
	defer rpcutil.MultiCancel(cancels)

	repoStats := make([]api.IPFSRepoStat, len(peers), len(peers))
	repoStatsIfaces := make([]interface{}, len(repoStats), len(repoStats))
	for i := range repoStats {
		repoStatsIfaces[i] = &repoStats[i]
	}

	errs := proxy.rpcClient.MultiCall(
		ctxs,
		peers,
		"Cluster",
		"IPFSRepoStat",
		struct{}{},
		repoStatsIfaces,
	)

	totalStats := api.IPFSRepoStat{}

	for i, err := range errs {
		if err != nil {
			logger.Errorf("%s repo/stat errored: %s", peers[i], err)
			continue
		}
		totalStats.RepoSize += repoStats[i].RepoSize
		totalStats.StorageMax += repoStats[i].StorageMax
	}

	resBytes, _ := json.Marshal(totalStats)
	w.WriteHeader(http.StatusOK)
	w.Write(resBytes)
	return
}

// extractArgument extracts the cid argument from a url.URL, either via
// the query string parameters or from the url path itself.
func extractArgument(u *url.URL) (string, bool) {
	arg := u.Query().Get("arg")
	if arg != "" {
		return arg, true
	}

	p := strings.TrimPrefix(u.Path, "/api/v0/")
	segs := strings.Split(p, "/")

	if len(segs) > 2 {
		warnMsg := "You are using an undocumented form of the IPFS API."
		warnMsg += "Consider passing your command arguments"
		warnMsg += "with the '?arg=' query parameter"
		logger.Warning(warnMsg)
		return segs[len(segs)-1], true
	}
	return "", false
}
