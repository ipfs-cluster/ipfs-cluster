// Package ipfsproxy implements the Cluster API interface by providing an
// IPFS HTTP interface as exposed by the go-ipfs daemon.
//
// In this API, select endpoints like pin*, add*, and repo* endpoints are used
// to instead perform cluster operations. Requests for any other endpoints are
// passed to the underlying IPFS daemon.
package ipfsproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/adder/adderutils"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/rpcutil"

	handlers "github.com/gorilla/handlers"
	mux "github.com/gorilla/mux"
	path "github.com/ipfs/boxo/path"
	cid "github.com/ipfs/go-cid"
	cmd "github.com/ipfs/go-ipfs-cmds"
	logging "github.com/ipfs/go-log/v2"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p/core/peer"
	madns "github.com/multiformats/go-multiaddr-dns"
	manet "github.com/multiformats/go-multiaddr/net"

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/plugin/ochttp/propagation/tracecontext"
	"go.opencensus.io/trace"
)

// DNSTimeout is used when resolving DNS multiaddresses in this module
var DNSTimeout = 5 * time.Second

var (
	logger      = logging.Logger("ipfsproxy")
	proxyLogger = logging.Logger("ipfsproxylog")
)

// Server offers an IPFS API, hijacking some interesting requests
// and forwarding the rest to the ipfs daemon
// it proxies HTTP requests to the configured IPFS
// daemon. It is able to intercept these requests though, and
// perform extra operations on them.
type Server struct {
	ctx    context.Context
	cancel func()

	config     *Config
	nodeScheme string
	nodeAddr   string

	rpcClient *rpc.Client
	rpcReady  chan struct{}

	listeners    []net.Listener         // proxy listener
	server       *http.Server           // proxy server
	reverseProxy *httputil.ReverseProxy // allows to talk to IPFS

	ipfsHeadersStore sync.Map

	shutdownLock sync.Mutex
	shutdown     bool
	wg           sync.WaitGroup
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

type logWriter struct {
}

func (lw logWriter) Write(b []byte) (int, error) {
	proxyLogger.Infof(string(b))
	return len(b), nil
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

	var listeners []net.Listener
	for _, addr := range cfg.ListenAddr {
		proxyNet, proxyAddr, err := manet.DialArgs(addr)
		if err != nil {
			return nil, err
		}

		l, err := net.Listen(proxyNet, proxyAddr)
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, l)
	}

	nodeScheme := "http"
	if cfg.NodeHTTPS {
		nodeScheme = "https"
	}
	nodeHTTPAddr := fmt.Sprintf("%s://%s", nodeScheme, nodeAddr)
	proxyURL, err := url.Parse(nodeHTTPAddr)
	if err != nil {
		return nil, err
	}

	var handler http.Handler
	router := mux.NewRouter()
	handler = router

	if cfg.Tracing {
		handler = &ochttp.Handler{
			IsPublicEndpoint: true,
			Propagation:      &tracecontext.HTTPFormat{},
			Handler:          router,
			StartOptions:     trace.StartOptions{SpanKind: trace.SpanKindServer},
			FormatSpanName: func(req *http.Request) string {
				return "proxy:" + req.Host + ":" + req.URL.Path + ":" + req.Method
			},
		}
	}

	var writer io.Writer
	if cfg.LogFile != "" {
		f, err := os.OpenFile(cfg.getLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		writer = f
	} else {
		writer = logWriter{}
	}

	s := &http.Server{
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		Handler:           handlers.LoggingHandler(writer, handler),
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
	}

	// See: https://github.com/ipfs/go-ipfs/issues/5168
	// See: https://github.com/ipfs-cluster/ipfs-cluster/issues/548
	// on why this is re-enabled.
	s.SetKeepAlivesEnabled(true) // A reminder that this can be changed

	reverseProxy := httputil.NewSingleHostReverseProxy(proxyURL)
	reverseProxy.Transport = http.DefaultTransport
	ctx, cancel := context.WithCancel(context.Background())
	proxy := &Server{
		ctx:          ctx,
		config:       cfg,
		cancel:       cancel,
		nodeAddr:     nodeHTTPAddr,
		nodeScheme:   nodeScheme,
		rpcReady:     make(chan struct{}, 1),
		listeners:    listeners,
		server:       s,
		reverseProxy: reverseProxy,
	}

	// Ideally, we should only intercept POST requests, but
	// people may be calling the API with GET or worse, PUT
	// because IPFS has been allowing this traditionally.
	// The main idea here is that we do not intercept
	// OPTIONS requests (or HEAD).
	hijackSubrouter := router.
		Methods(http.MethodPost, http.MethodGet, http.MethodPut).
		PathPrefix("/api/v0").
		Subrouter()

	// Add hijacked routes
	hijackSubrouter.
		Path("/pin/add/{arg}").
		HandlerFunc(slashHandler(proxy.pinHandler)).
		Name("PinAddSlash") // supports people using the API wrong.
	hijackSubrouter.
		Path("/pin/add").
		HandlerFunc(proxy.pinHandler).
		Name("PinAdd")
	hijackSubrouter.
		Path("/pin/rm/{arg}").
		HandlerFunc(slashHandler(proxy.unpinHandler)).
		Name("PinRmSlash") // supports people using the API wrong.
	hijackSubrouter.
		Path("/pin/rm").
		HandlerFunc(proxy.unpinHandler).
		Name("PinRm")
	hijackSubrouter.
		Path("/pin/ls/{arg}").
		HandlerFunc(slashHandler(proxy.pinLsHandler)).
		Name("PinLsSlash") // supports people using the API wrong.
	hijackSubrouter.
		Path("/pin/ls").
		HandlerFunc(proxy.pinLsHandler).
		Name("PinLs")
	hijackSubrouter.
		Path("/pin/update").
		HandlerFunc(proxy.pinUpdateHandler).
		Name("PinUpdate")
	hijackSubrouter.
		Path("/add").
		HandlerFunc(proxy.addHandler).
		Name("Add")
	hijackSubrouter.
		Path("/repo/stat").
		HandlerFunc(proxy.repoStatHandler).
		Name("RepoStat")
	hijackSubrouter.
		Path("/repo/gc").
		HandlerFunc(proxy.repoGCHandler).
		Name("RepoGC")
	hijackSubrouter.
		Path("/block/put").
		HandlerFunc(proxy.blockPutHandler).
		Name("BlockPut")
	hijackSubrouter.
		Path("/dag/put").
		HandlerFunc(proxy.dagPutHandler).
		Name("DagPut")

	// Everything else goes to the IPFS daemon.
	router.PathPrefix("/").Handler(reverseProxy)

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
func (proxy *Server) Shutdown(ctx context.Context) error {
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
	for _, l := range proxy.listeners {
		l.Close()
	}

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
	proxy.wg.Add(len(proxy.listeners))
	for _, l := range proxy.listeners {
		go func(l net.Listener) {
			defer proxy.wg.Done()

			maddr, err := manet.FromNetAddr(l.Addr())
			if err != nil {
				logger.Error(err)
			}

			logger.Infof(
				"IPFS Proxy: %s -> %s",
				maddr,
				proxy.config.NodeAddr,
			)
			err = proxy.server.Serve(l) // hangs here
			if err != nil && !strings.Contains(err.Error(), "closed network connection") {
				logger.Error(err)
			}
		}(l)
	}
}

// ipfsErrorResponder writes an http error response just like IPFS would.
func ipfsErrorResponder(w http.ResponseWriter, errMsg string, code int) {
	res := cmd.Errorf(cmd.ErrNormal, errMsg)

	resBytes, _ := json.Marshal(res)
	if code > 0 {
		w.WriteHeader(code)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.Write(resBytes)
}

func (proxy *Server) pinOpHandler(op string, w http.ResponseWriter, r *http.Request) {
	proxy.setHeaders(w.Header(), r)

	q := r.URL.Query()
	arg := q.Get("arg")
	p, err := path.ParsePath(arg)
	if err != nil {
		ipfsErrorResponder(w, "Error parsing IPFS Path: "+err.Error(), -1)
		return
	}

	pinPath := api.PinPath{Path: p.String()}
	pinPath.Mode = api.PinModeFromString(q.Get("type"))

	var pin api.Pin
	err = proxy.rpcClient.Call(
		"",
		"Cluster",
		op,
		pinPath,
		&pin,
	)
	if err != nil {
		ipfsErrorResponder(w, err.Error(), -1)
		return
	}

	res := ipfsPinOpResp{
		Pins: []string{pin.Cid.String()},
	}
	resBytes, _ := json.Marshal(res)
	w.WriteHeader(http.StatusOK)
	w.Write(resBytes)
}

func (proxy *Server) pinHandler(w http.ResponseWriter, r *http.Request) {
	proxy.pinOpHandler("PinPath", w, r)
}

func (proxy *Server) unpinHandler(w http.ResponseWriter, r *http.Request) {
	proxy.pinOpHandler("UnpinPath", w, r)
}

func (proxy *Server) pinLsHandler(w http.ResponseWriter, r *http.Request) {
	proxy.setHeaders(w.Header(), r)

	arg := r.URL.Query().Get("arg")

	stream := false
	streamArg := r.URL.Query().Get("stream")
	streamArg2 := r.URL.Query().Get("s")
	if streamArg == "true" || streamArg2 == "true" {
		stream = true
	}

	if arg != "" {
		c, err := api.DecodeCid(arg)
		if err != nil {
			ipfsErrorResponder(w, err.Error(), -1)
			return
		}
		var pin api.Pin
		err = proxy.rpcClient.CallContext(
			r.Context(),
			"",
			"Cluster",
			"PinGet",
			c,
			&pin,
		)
		if err != nil {
			ipfsErrorResponder(w, fmt.Sprintf("Error: path '%s' is not pinned", arg), -1)
			return
		}
		if stream {
			ipinfo := api.IPFSPinInfo{
				Cid:  api.Cid(pin.Cid),
				Type: pin.Mode.ToIPFSPinStatus(),
			}
			resBytes, _ := json.Marshal(ipinfo)
			w.WriteHeader(http.StatusOK)
			w.Write(resBytes)
		} else {
			pinLs := ipfsPinLsResp{}
			pinLs.Keys = make(map[string]ipfsPinType)
			pinLs.Keys[pin.Cid.String()] = ipfsPinType{
				Type: "recursive",
			}
			resBytes, _ := json.Marshal(pinLs)
			w.WriteHeader(http.StatusOK)
			w.Write(resBytes)
		}
	} else {
		in := make(chan struct{})
		close(in)

		pins := make(chan api.Pin)
		var err error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			err = proxy.rpcClient.Stream(
				r.Context(),
				"",
				"Cluster",
				"Pins",
				in,
				pins,
			)
		}()

		if stream {
			w.Header().Set("Trailer", "X-Stream-Error")
			w.WriteHeader(http.StatusOK)
			for pin := range pins {
				ipinfo := api.IPFSPinInfo{
					Cid:  api.Cid(pin.Cid),
					Type: pin.Mode.ToIPFSPinStatus(),
				}
				resBytes, _ := json.Marshal(ipinfo)
				w.Write(resBytes)
				w.Write([]byte("\n"))
			}
			wg.Wait()
			if err != nil {
				w.Header().Add("X-Stream-Error", err.Error())
				return
			}
		} else {
			pinLs := ipfsPinLsResp{}
			pinLs.Keys = make(map[string]ipfsPinType)

			for pin := range pins {
				pinLs.Keys[pin.Cid.String()] = ipfsPinType{
					Type: "recursive",
				}
			}

			wg.Wait()
			if err != nil {
				ipfsErrorResponder(w, err.Error(), -1)
				return
			}
			resBytes, _ := json.Marshal(pinLs)
			w.WriteHeader(http.StatusOK)
			w.Write(resBytes)
		}
	}
}

func (proxy *Server) pinUpdateHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "ipfsproxy/pinUpdateHandler")
	defer span.End()

	proxy.setHeaders(w.Header(), r)

	// Check that we have enough arguments and mimic ipfs response when not
	q := r.URL.Query()
	args := q["arg"]
	if len(args) == 0 {
		ipfsErrorResponder(w, "argument \"from-path\" is required", http.StatusBadRequest)
		return
	}
	if len(args) == 1 {
		ipfsErrorResponder(w, "argument \"to-path\" is required", http.StatusBadRequest)
		return
	}

	unpin := !(q.Get("unpin") == "false")
	from := args[0]
	to := args[1]

	// Parse paths (we will need to resolve them)
	pFrom, err := path.ParsePath(from)
	if err != nil {
		ipfsErrorResponder(w, "error parsing \"from-path\" argument: "+err.Error(), -1)
		return
	}

	pTo, err := path.ParsePath(to)
	if err != nil {
		ipfsErrorResponder(w, "error parsing \"to-path\" argument: "+err.Error(), -1)
		return
	}

	// Resolve the FROM argument
	var fromCid api.Cid
	err = proxy.rpcClient.CallContext(
		ctx,
		"",
		"IPFSConnector",
		"Resolve",
		pFrom.String(),
		&fromCid,
	)
	if err != nil {
		ipfsErrorResponder(w, err.Error(), -1)
		return
	}

	// Do a PinPath setting PinUpdate
	pinPath := api.PinPath{Path: pTo.String()}
	pinPath.PinUpdate = fromCid

	var pin api.Pin
	err = proxy.rpcClient.Call(
		"",
		"Cluster",
		"PinPath",
		pinPath,
		&pin,
	)
	if err != nil {
		ipfsErrorResponder(w, err.Error(), -1)
		return
	}

	// If unpin != "false", unpin the FROM argument
	// (it was already resolved).
	var pinObj api.Pin
	if unpin {
		err = proxy.rpcClient.CallContext(
			ctx,
			"",
			"Cluster",
			"Unpin",
			api.PinCid(fromCid),
			&pinObj,
		)
		if err != nil {
			ipfsErrorResponder(w, err.Error(), -1)
			return
		}
	}

	res := ipfsPinOpResp{
		Pins: []string{fromCid.String(), pin.Cid.String()},
	}
	resBytes, _ := json.Marshal(res)
	w.WriteHeader(http.StatusOK)
	w.Write(resBytes)
}

func (proxy *Server) addHandler(w http.ResponseWriter, r *http.Request) {
	proxy.setHeaders(w.Header(), r)

	reader, err := r.MultipartReader()
	if err != nil {
		ipfsErrorResponder(w, "error reading request: "+err.Error(), -1)
		return
	}

	q := r.URL.Query()
	if q.Get("only-hash") == "true" {
		ipfsErrorResponder(w, "only-hash is not supported when adding to cluster", -1)
	}

	// Luckily, most IPFS add query params are compatible with cluster's
	// /add params. We can parse most of them directly from the query.
	params, err := api.AddParamsFromQuery(q)
	if err != nil {
		ipfsErrorResponder(w, "error parsing options:"+err.Error(), -1)
		return
	}
	trickle := q.Get("trickle")
	if trickle == "true" {
		params.Layout = "trickle"
	}
	nopin := q.Get("pin") == "false"
	if nopin {
		params.NoPin = true
	}

	logger.Warnf("Proxy/add does not support all IPFS params. Current options: %+v", params)

	outputTransform := func(in api.AddedOutput) interface{} {
		cidStr := ""
		if in.Cid.Defined() {
			cidStr = in.Cid.String()
		}
		r := &ipfsAddResp{
			Name:  in.Name,
			Hash:  cidStr,
			Bytes: int64(in.Bytes),
		}
		if in.Size != 0 {
			r.Size = strconv.FormatUint(in.Size, 10)
		}
		return r
	}

	_, err = adderutils.AddMultipartHTTPHandler(
		proxy.ctx,
		proxy.rpcClient,
		params,
		reader,
		w,
		outputTransform,
	)
	if err != nil {
		logger.Error(err)
	}
}

func (proxy *Server) repoStatHandler(w http.ResponseWriter, r *http.Request) {
	proxy.setHeaders(w.Header(), r)

	peers := make([]peer.ID, 0)
	err := proxy.rpcClient.Call(
		"",
		"Consensus",
		"Peers",
		struct{}{},
		&peers,
	)
	if err != nil {
		ipfsErrorResponder(w, err.Error(), -1)
		return
	}

	ctxs, cancels := rpcutil.CtxsWithCancel(proxy.ctx, len(peers))
	defer rpcutil.MultiCancel(cancels)

	repoStats := make([]*api.IPFSRepoStat, len(peers))
	repoStatsIfaces := make([]interface{}, len(repoStats))
	for i := range repoStats {
		repoStats[i] = &api.IPFSRepoStat{}
		repoStatsIfaces[i] = repoStats[i]
	}

	errs := proxy.rpcClient.MultiCall(
		ctxs,
		peers,
		"IPFSConnector",
		"RepoStat",
		struct{}{},
		repoStatsIfaces,
	)

	totalStats := api.IPFSRepoStat{}

	for i, err := range errs {
		if err != nil {
			if rpc.IsAuthorizationError(err) {
				logger.Debug(err)
				continue
			}
			logger.Errorf("%s repo/stat errored: %s", peers[i], err)
			continue
		}
		totalStats.RepoSize += repoStats[i].RepoSize
		totalStats.StorageMax += repoStats[i].StorageMax
	}

	resBytes, _ := json.Marshal(totalStats)
	w.WriteHeader(http.StatusOK)
	w.Write(resBytes)
}

type ipfsRepoGCResp struct {
	Key   cid.Cid `json:",omitempty"`
	Error string  `json:",omitempty"`
}

func (proxy *Server) repoGCHandler(w http.ResponseWriter, r *http.Request) {
	queryValues := r.URL.Query()
	streamErrors := queryValues.Get("stream-errors") == "true"
	// ignoring `quiet` since it only affects text output

	proxy.setHeaders(w.Header(), r)

	w.Header().Set("Trailer", "X-Stream-Error")
	var repoGC api.GlobalRepoGC
	err := proxy.rpcClient.CallContext(
		r.Context(),
		"",
		"Cluster",
		"RepoGC",
		struct{}{},
		&repoGC,
	)
	if err != nil {
		ipfsErrorResponder(w, err.Error(), -1)
		return
	}

	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	var ipfsRepoGC ipfsRepoGCResp
	mError := multiError{}
	for _, gc := range repoGC.PeerMap {
		for _, key := range gc.Keys {
			if streamErrors {
				ipfsRepoGC = ipfsRepoGCResp{Key: key.Key.Cid, Error: key.Error}
			} else {
				ipfsRepoGC = ipfsRepoGCResp{Key: key.Key.Cid}
				if key.Error != "" {
					mError.add(key.Error)
				}
			}

			// Cluster tags start with small letter, but IPFS tags with capital letter.
			if err := enc.Encode(ipfsRepoGC); err != nil {
				logger.Error(err)
			}
		}
	}

	mErrStr := mError.Error()
	if !streamErrors && mErrStr != "" {
		w.Header().Set("X-Stream-Error", mErrStr)
	}
}

type ipfsBlockPutResp struct {
	Key  api.Cid
	Size int
}

func (proxy *Server) blockPutHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("pin") != "true" {
		proxy.reverseProxy.ServeHTTP(w, r)
		return
	}

	proxy.setHeaders(w.Header(), r)
	u2, err := url.Parse(proxy.nodeAddr)
	if err != nil {
		logger.Error(err)
		ipfsErrorResponder(w, err.Error(), -1)
		return
	}

	r.URL.Host = u2.Host
	r.URL.Scheme = u2.Scheme
	r.Host = u2.Host
	r.RequestURI = ""

	res, err := proxy.reverseProxy.Transport.RoundTrip(r)
	if err != nil {
		ipfsErrorResponder(w, err.Error(), -1)
		return
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		w.WriteHeader(res.StatusCode)
		_, err = io.Copy(w, res.Body)
		if err != nil {
			logger.Error(err)
		}
		return
	}

	// Returned 200. Parse responses.
	w.Header().Set("Trailer", "X-Stream-Error")
	w.WriteHeader(http.StatusOK) // any errors from here go into trailers

	dec := json.NewDecoder(res.Body)
	enc := json.NewEncoder(w)
	for {
		var blockInfo ipfsBlockPutResp
		err = dec.Decode(&blockInfo)
		if err == io.EOF {
			return
		}
		if err != nil {
			logger.Error(err)
			w.Header().Add("X-Stream-Error", err.Error())
			return
		}
		p := api.PinCid(blockInfo.Key)
		var pinObj api.Pin
		if err := proxy.rpcClient.Call(
			"",
			"Cluster",
			"Pin",
			p,
			&pinObj,
		); err != nil {
			logger.Error(err)
			w.Header().Add("X-Stream-Error", err.Error())
			// keep going though blocks
		}
		if err := enc.Encode(blockInfo); err != nil {
			logger.Error(err)
			w.Header().Add("X-Stream-Error", err.Error())
			return
		}
	}
}

type ipfsDagPutResp struct {
	Cid cid.Cid
}

func (proxy *Server) dagPutHandler(w http.ResponseWriter, r *http.Request) {
	// Note this mostly duplicates blockPutHandler
	if r.URL.Query().Get("pin") != "true" {
		proxy.reverseProxy.ServeHTTP(w, r)
		return
	}

	proxy.setHeaders(w.Header(), r)
	u2, err := url.Parse(proxy.nodeAddr)
	if err != nil {
		logger.Error(err)
		ipfsErrorResponder(w, err.Error(), -1)
		return
	}

	r.URL.Host = u2.Host
	r.URL.Scheme = u2.Scheme
	r.Host = u2.Host
	newQuery := r.URL.Query()
	newQuery.Set("pin", "false")
	r.URL.RawQuery = newQuery.Encode()
	r.RequestURI = ""

	res, err := proxy.reverseProxy.Transport.RoundTrip(r)
	if err != nil {
		ipfsErrorResponder(w, err.Error(), -1)
		return
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		w.WriteHeader(res.StatusCode)
		_, err = io.Copy(w, res.Body)
		if err != nil {
			logger.Error(err)
		}
		return
	}

	// Returned 200. Parse responses.
	w.Header().Set("Trailer", "X-Stream-Error")
	w.WriteHeader(http.StatusOK) // any errors from here go into trailers

	dec := json.NewDecoder(res.Body)
	enc := json.NewEncoder(w)
	for {
		var dagInfo ipfsDagPutResp
		err = dec.Decode(&dagInfo)
		if err == io.EOF {
			return
		}
		if err != nil {
			logger.Error(err)
			w.Header().Add("X-Stream-Error", err.Error())
			return
		}
		p := api.PinCid(api.NewCid(dagInfo.Cid))
		var pinObj api.Pin
		if err := proxy.rpcClient.Call(
			"",
			"Cluster",
			"Pin",
			p,
			&pinObj,
		); err != nil {
			logger.Error(err)
			w.Header().Add("X-Stream-Error", err.Error())
			// keep going though blocks
		}
		if err := enc.Encode(dagInfo); err != nil {
			logger.Error(err)
			w.Header().Add("X-Stream-Error", err.Error())
			return
		}
	}
}

// slashHandler returns a handler which converts a /a/b/c/<argument> request
// into an /a/b/c/<argument>?arg=<argument> one. And uses the given origHandler
// for it. Our handlers expect that arguments are passed in the ?arg query
// value.
func slashHandler(origHandler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		warnMsg := "You are using an undocumented form of the IPFS API. "
		warnMsg += "Consider passing your command arguments"
		warnMsg += "with the '?arg=' query parameter"
		logger.Error(warnMsg)

		vars := mux.Vars(r)
		arg := vars["arg"]

		// IF we needed to modify the request path, we could do
		// something along these lines. This is not the case
		// at the moment. We just need to set the query argument.
		//
		// route := mux.CurrentRoute(r)
		// path, err := route.GetPathTemplate()
		// if err != nil {
		// 	// I'd like to panic, but I don' want to kill a full
		// 	// peer just because of a buggy use.
		// 	logger.Critical("BUG: wrong use of slashHandler")
		// 	origHandler(w, r) // proceed as nothing
		// 	return
		// }
		// fixedPath := strings.TrimSuffix(path, "/{arg}")
		// r.URL.Path = url.PathEscape(fixedPath)
		// r.URL.RawPath = fixedPath

		q := r.URL.Query()
		q.Set("arg", arg)
		r.URL.RawQuery = q.Encode()
		origHandler(w, r)
	}
}
