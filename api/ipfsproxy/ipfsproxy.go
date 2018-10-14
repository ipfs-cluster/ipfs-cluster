package ipfsproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
	madns "github.com/multiformats/go-multiaddr-dns"
	manet "github.com/multiformats/go-multiaddr-net"

	"github.com/ipfs/ipfs-cluster/adder/adderutils"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/rpcutil"
)

// DNSTimeout is used when resolving DNS multiaddresses in this module
var DNSTimeout = 5 * time.Second

var logger = logging.Logger("ipfsproxy")

// IPFSProxy offers an IPFS API, hijacking some interesting requests
// and forwarding the rest to the ipfs daemon
// it proxies HTTP requests to the configured IPFS
// daemon. It is able to intercept these requests though, and
// perform extra operations on them.
type IPFSProxy struct {
	ctx    context.Context
	cancel func()

	config   *Config
	nodeAddr string

	rpcClient *rpc.Client
	rpcReady  chan struct{}

	listener net.Listener // proxy listener
	server   *http.Server // proxy server

	shutdownLock sync.Mutex
	shutdown     bool
	wg           sync.WaitGroup
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

// NewIPFSProxy returns and IPFSProxy component
func NewIPFSProxy(cfg *Config) (*IPFSProxy, error) {
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

	proxyNet, proxyAddr, err := manet.DialArgs(cfg.ProxyAddr)
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

	proxyHandler := httputil.NewSingleHostReverseProxy(proxyURL)

	smux := http.NewServeMux()
	s := &http.Server{
		ReadTimeout:       cfg.ProxyReadTimeout,
		WriteTimeout:      cfg.ProxyWriteTimeout,
		ReadHeaderTimeout: cfg.ProxyReadHeaderTimeout,
		IdleTimeout:       cfg.ProxyIdleTimeout,
		Handler:           smux,
	}

	// See: https://github.com/ipfs/go-ipfs/issues/5168
	// See: https://github.com/ipfs/ipfs-cluster/issues/548
	// on why this is re-enabled.
	s.SetKeepAlivesEnabled(false) // A reminder that this can be changed

	ctx, cancel := context.WithCancel(context.Background())

	ipfs := &IPFSProxy{
		ctx:      ctx,
		config:   cfg,
		cancel:   cancel,
		nodeAddr: nodeAddr,
		rpcReady: make(chan struct{}, 1),
		listener: l,
		server:   s,
	}
	smux.Handle("/", proxyHandler)
	smux.HandleFunc("/api/v0/pin/add", ipfs.pinHandler)
	smux.HandleFunc("/api/v0/pin/add/", ipfs.pinHandler)
	smux.HandleFunc("/api/v0/pin/rm", ipfs.unpinHandler)
	smux.HandleFunc("/api/v0/pin/rm/", ipfs.unpinHandler)
	smux.HandleFunc("/api/v0/pin/ls", ipfs.pinLsHandler) // required to handle /pin/ls for all pins
	smux.HandleFunc("/api/v0/pin/ls/", ipfs.pinLsHandler)
	smux.HandleFunc("/api/v0/add", ipfs.addHandler)
	smux.HandleFunc("/api/v0/add/", ipfs.addHandler)
	smux.HandleFunc("/api/v0/repo/stat", ipfs.repoStatHandler)
	smux.HandleFunc("/api/v0/repo/stat/", ipfs.repoStatHandler)

	go ipfs.run()
	return ipfs, nil
}

// SetClient makes the component ready to perform RPC
// requests.
func (ipfs *IPFSProxy) SetClient(c *rpc.Client) {
	ipfs.rpcClient = c
	ipfs.rpcReady <- struct{}{}
}

// Shutdown stops any listeners and stops the component from taking
// any requests.
func (ipfs *IPFSProxy) Shutdown() error {
	ipfs.shutdownLock.Lock()
	defer ipfs.shutdownLock.Unlock()

	if ipfs.shutdown {
		logger.Debug("already shutdown")
		return nil
	}

	logger.Info("stopping IPFS Proxy")

	ipfs.cancel()
	close(ipfs.rpcReady)
	ipfs.server.SetKeepAlivesEnabled(false)
	ipfs.listener.Close()

	ipfs.wg.Wait()
	ipfs.shutdown = true
	return nil
}

// launches proxy when we receive the rpcReady signal.
func (ipfs *IPFSProxy) run() {
	<-ipfs.rpcReady

	// Do not shutdown while launching threads
	// -- prevents race conditions with ipfs.wg.
	ipfs.shutdownLock.Lock()
	defer ipfs.shutdownLock.Unlock()

	// This launches the proxy
	ipfs.wg.Add(1)
	go func() {
		defer ipfs.wg.Done()
		logger.Infof(
			"IPFS Proxy: %s -> %s",
			ipfs.config.ProxyAddr,
			ipfs.config.NodeAddr,
		)
		err := ipfs.server.Serve(ipfs.listener) // hangs here
		if err != nil && !strings.Contains(err.Error(), "closed network connection") {
			logger.Error(err)
		}
	}()
}

// Handlers
func ipfsErrorResponder(w http.ResponseWriter, errMsg string) {
	res := ipfsError{errMsg}
	resBytes, _ := json.Marshal(res)
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	w.Write(resBytes)
	return
}

func (ipfs *IPFSProxy) pinOpHandler(op string, w http.ResponseWriter, r *http.Request) {
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

	err = ipfs.rpcClient.Call(
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
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(resBytes)
	return
}

func (ipfs *IPFSProxy) pinHandler(w http.ResponseWriter, r *http.Request) {
	ipfs.pinOpHandler("Pin", w, r)
}

func (ipfs *IPFSProxy) unpinHandler(w http.ResponseWriter, r *http.Request) {
	ipfs.pinOpHandler("Unpin", w, r)
}

func (ipfs *IPFSProxy) pinLsHandler(w http.ResponseWriter, r *http.Request) {
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
		err = ipfs.rpcClient.Call(
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
		var pins []api.PinSerial
		err := ipfs.rpcClient.Call(
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
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(resBytes)
}

func (ipfs *IPFSProxy) addHandler(w http.ResponseWriter, r *http.Request) {
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
		ipfs.ctx,
		ipfs.rpcClient,
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
	err = ipfs.rpcClient.CallContext(
		ipfs.ctx,
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

func (ipfs *IPFSProxy) repoStatHandler(w http.ResponseWriter, r *http.Request) {
	var peers []peer.ID
	err := ipfs.rpcClient.Call(
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

	ctxs, cancels := rpcutil.CtxsWithTimeout(ipfs.ctx, len(peers), ipfs.config.IPFSRequestTimeout)
	defer rpcutil.MultiCancel(cancels)

	repoStats := make([]api.IPFSRepoStat, len(peers), len(peers))
	repoStatsIfaces := make([]interface{}, len(repoStats), len(repoStats))
	for i := range repoStats {
		repoStatsIfaces[i] = &repoStats[i]
	}

	errs := ipfs.rpcClient.MultiCall(
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
	w.Header().Add("Content-Type", "application/json")
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
