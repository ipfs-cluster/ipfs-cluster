// Package ipfshttp implements an IPFS Cluster IPFSConnector component. It
// uses the IPFS HTTP API to communicate to IPFS.
package ipfshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipfs-cmdkit/files"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
	manet "github.com/multiformats/go-multiaddr-net"
)

// DNSTimeout is used when resolving DNS multiaddresses in this module
var DNSTimeout = 5 * time.Second

var logger = logging.Logger("ipfshttp")

// Connector implements the IPFSConnector interface
// and provides a component which does two tasks:
//
// On one side, it proxies HTTP requests to the configured IPFS
// daemon. It is able to intercept these requests though, and
// perform extra operations on them.
//
// On the other side, it is used to perform on-demand requests
// against the configured IPFS daemom (such as a pin request).
type Connector struct {
	ctx    context.Context
	cancel func()

	config   *Config
	nodeAddr string

	handlers map[string]func(http.ResponseWriter, *http.Request)

	rpcClient *rpc.Client
	rpcReady  chan struct{}

	listener net.Listener // proxy listener
	server   *http.Server // proxy server
	client   *http.Client // client to ipfs daemon

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

type ipfsIDResp struct {
	ID        string
	Addresses []string
}

type ipfsRepoStatResp struct {
	RepoSize   uint64
	StorageMax uint64
	NumObjects uint64
}

type ipfsAddResp struct {
	Name  string
	Hash  string
	Bytes uint64
}

type ipfsSwarmPeersResp struct {
	Peers []ipfsPeer
}

type ipfsPeer struct {
	Peer string
}

type ipfsStream struct {
	Protocol string
}

type ipfsBlockPutResp struct {
	Key string
}

// NewConnector creates the component and leaves it ready to be started
func NewConnector(cfg *Config) (*Connector, error) {
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

	smux := http.NewServeMux()
	s := &http.Server{
		ReadTimeout:       cfg.ProxyReadTimeout,
		WriteTimeout:      cfg.ProxyWriteTimeout,
		ReadHeaderTimeout: cfg.ProxyReadHeaderTimeout,
		IdleTimeout:       cfg.ProxyIdleTimeout,
		Handler:           smux,
	}
	s.SetKeepAlivesEnabled(true) // A reminder that this can be changed

	c := &http.Client{} // timeouts are handled by context timeouts

	ctx, cancel := context.WithCancel(context.Background())

	ipfs := &Connector{
		ctx:      ctx,
		config:   cfg,
		cancel:   cancel,
		nodeAddr: nodeAddr,
		handlers: make(map[string]func(http.ResponseWriter, *http.Request)),
		rpcReady: make(chan struct{}, 1),
		listener: l,
		server:   s,
		client:   c,
	}

	smux.HandleFunc("/", ipfs.defaultHandler)
	smux.HandleFunc("/api/v0/pin/add", ipfs.pinHandler) // required for go1.9 as it doesn't redirect query args correctly
	smux.HandleFunc("/api/v0/pin/add/", ipfs.pinHandler)
	smux.HandleFunc("/api/v0/pin/rm", ipfs.unpinHandler) // required for go1.9 as it doesn't redirect query args correctly
	smux.HandleFunc("/api/v0/pin/rm/", ipfs.unpinHandler)
	smux.HandleFunc("/api/v0/pin/ls", ipfs.pinLsHandler) // required to handle /pin/ls for all pins
	smux.HandleFunc("/api/v0/pin/ls/", ipfs.pinLsHandler)
	smux.HandleFunc("/api/v0/add", ipfs.addHandler)
	smux.HandleFunc("/api/v0/add/", ipfs.addHandler)

	go ipfs.run()
	return ipfs, nil
}

// launches proxy and connects all ipfs daemons when
// we receive the rpcReady signal.
func (ipfs *Connector) run() {
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

	// This runs ipfs swarm connect to the daemons of other cluster members
	ipfs.wg.Add(1)
	go func() {
		defer ipfs.wg.Done()

		// It does not hurt to wait a little bit. i.e. think cluster
		// peers which are started at the same time as the ipfs
		// daemon...
		tmr := time.NewTimer(ipfs.config.ConnectSwarmsDelay)
		defer tmr.Stop()
		select {
		case <-tmr.C:
			// do not hang this goroutine if this call hangs
			// otherwise we hang during shutdown
			go ipfs.ConnectSwarms()
		case <-ipfs.ctx.Done():
			return
		}
	}()
}

func (ipfs *Connector) proxyRequest(r *http.Request) (*http.Response, error) {
	newURL := *r.URL
	newURL.Host = ipfs.nodeAddr
	newURL.Scheme = "http"

	proxyReq, err := http.NewRequest(r.Method, newURL.String(), r.Body)
	if err != nil {
		logger.Error("error creating proxy request: ", err)
		return nil, err
	}

	for k, v := range r.Header {
		for _, s := range v {
			proxyReq.Header.Add(k, s)
		}
	}

	res, err := http.DefaultTransport.RoundTrip(proxyReq)
	if err != nil {
		logger.Error("error forwarding request: ", err)
		return nil, err
	}
	return res, nil
}

// Writes a response to a ResponseWriter using the given body
// (which maybe resp.Body or a copy if it was already used).
func (ipfs *Connector) proxyResponse(w http.ResponseWriter, res *http.Response, body io.Reader) {
	// Set response headers
	for k, v := range res.Header {
		for _, s := range v {
			w.Header().Add(k, s)
		}
	}

	w.WriteHeader(res.StatusCode)

	// And copy body
	io.Copy(w, body)
}

// defaultHandler just proxies the requests.
func (ipfs *Connector) defaultHandler(w http.ResponseWriter, r *http.Request) {
	res, err := ipfs.proxyRequest(r)
	if err != nil {
		http.Error(w, "error forwarding request: "+err.Error(), 500)
		return
	}
	ipfs.proxyResponse(w, res, res.Body)
	res.Body.Close()
}

func ipfsErrorResponder(w http.ResponseWriter, errMsg string) {
	res := ipfsError{errMsg}
	resBytes, _ := json.Marshal(res)
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	w.Write(resBytes)
	return
}

func (ipfs *Connector) pinOpHandler(op string, w http.ResponseWriter, r *http.Request) {
	arg, ok := extractArgument(r.URL)
	if !ok {
		ipfsErrorResponder(w, "Error: bad argument")
		return
	}
	_, err := cid.Decode(arg)
	if err != nil {
		ipfsErrorResponder(w, "Error parsing CID: "+err.Error())
		return
	}

	err = ipfs.rpcClient.Call(
		"",
		"Cluster",
		op,
		api.PinSerial{
			Cid: arg,
		},
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

func (ipfs *Connector) pinHandler(w http.ResponseWriter, r *http.Request) {
	ipfs.pinOpHandler("Pin", w, r)
}

func (ipfs *Connector) unpinHandler(w http.ResponseWriter, r *http.Request) {
	ipfs.pinOpHandler("Unpin", w, r)
}

func (ipfs *Connector) pinLsHandler(w http.ResponseWriter, r *http.Request) {
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

func (ipfs *Connector) addHandler(w http.ResponseWriter, r *http.Request) {
	// Handle some request options
	q := r.URL.Query()
	// Remember if the user does not want cluster/ipfs to pin
	doNotPin := q.Get("pin") == "false"
	// make sure the local peer does not pin.
	// Cluster will decide where to pin based on metrics and current
	// allocations.
	q.Set("pin", "false")
	r.URL.RawQuery = q.Encode()

	res, err := ipfs.proxyRequest(r)
	if err != nil {
		http.Error(w, "error forwarding request: "+err.Error(), 500)
		return
	}
	defer res.Body.Close()

	// Shortcut some cases where there is nothing else to do
	if scode := res.StatusCode; scode != http.StatusOK {
		logger.Warningf("proxy /add request returned %d", scode)
		ipfs.proxyResponse(w, res, res.Body)
		return
	}

	if doNotPin {
		logger.Debug("proxy /add requests has pin==false")
		ipfs.proxyResponse(w, res, res.Body)
		return
	}

	// The ipfs-add response is a streaming-like body where
	// { "Name" : "filename", "Hash": "cid" } objects are provided
	// for every added object.

	// We will need to re-read the response in order to re-play it to
	// the client at the end, therefore we make a copy in bodyCopy
	// while decoding.
	bodyCopy := new(bytes.Buffer)
	bodyReader := io.TeeReader(res.Body, bodyCopy)

	ipfsAddResps := []ipfsAddResp{}
	dec := json.NewDecoder(bodyReader)
	for dec.More() {
		var addResp ipfsAddResp
		err := dec.Decode(&addResp)
		if err != nil {
			http.Error(w, "error decoding response: "+err.Error(), 502)
			return
		}
		if addResp.Bytes != 0 {
			// This is a progress notification, so we ignore it
			continue
		}
		ipfsAddResps = append(ipfsAddResps, addResp)
	}

	if len(ipfsAddResps) == 0 {
		logger.Warning("proxy /add request response was OK but empty")
		ipfs.proxyResponse(w, res, bodyCopy)
		return
	}

	// An ipfs-add call can add multiple files and pin multiple items.
	// The go-ipfs api is not perfectly behaved here (i.e. when passing in
	// two directories to pin). There is no easy way to know for sure what
	// has been pinned recursively and what not.
	// Usually when pinning a directory, the recursive pin comes last.
	// But we may just be pinning different files and no directories.
	// In that case, we need to recursively pin them separately.
	// decideRecursivePins() takes a conservative approach. It
	// works on the regular use-cases. Otherwise, it might pin
	// more things than it should.
	pinHashes := decideRecursivePins(ipfsAddResps, r.URL.Query())

	logger.Debugf("proxy /add request and will pin %s", pinHashes)
	for _, pin := range pinHashes {
		err := ipfs.rpcClient.Call(
			"",
			"Cluster",
			"Pin",
			api.PinSerial{
				Cid: pin,
			},
			&struct{}{},
		)
		if err != nil {
			// we need to fail the operation and make sure the
			// user knows about it.
			msg := "add operation was successful but "
			msg += "an error occurred performing the cluster "
			msg += "pin operation: " + err.Error()
			logger.Error(msg)
			http.Error(w, msg, 500)
			return
		}
	}
	// Finally, send the original response back
	ipfs.proxyResponse(w, res, bodyCopy)
}

// decideRecursivePins takes the answers from ipfsAddResp and
// figures out which of the pinned items need to be pinned
// recursively in cluster. That is, it guesses which items
// ipfs would have pinned recursively.
// When adding multiple files+directories, it may end up
// pinning more than it should because ipfs API does not
// behave well in these cases.
// It should work well for regular usecases: pin 1 file,
// pin 1 directory, pin several files.
func decideRecursivePins(added []ipfsAddResp, q url.Values) []string {
	// When wrap-in-directory, return last element only.
	_, ok := q["wrap-in-directory"]
	if ok && q.Get("wrap-in-directory") == "true" {
		return []string{
			added[len(added)-1].Hash,
		}
	}

	toPin := []string{}
	baseFolders := make(map[string]struct{})
	// Guess base folder names
	baseFolder := func(path string) string {
		slashed := filepath.ToSlash(path)
		parts := strings.Split(slashed, "/")
		if len(parts) == 0 {
			return ""
		}
		if parts[0] == "" && len(parts) > 1 {
			return parts[1]
		}
		return parts[0]
	}

	for _, add := range added {
		if add.Hash == "" {
			continue
		}
		b := baseFolder(add.Name)
		if b != "" {
			baseFolders[b] = struct{}{}
		}
	}
	for _, add := range added {
		if add.Hash == "" {
			continue
		}
		_, ok := baseFolders[add.Name]
		if ok { // it's a base folder, pin it
			toPin = append(toPin, add.Hash)
		} else { // otherwise, pin if there is no
			// basefolder to it.
			b := baseFolder(add.Name)
			_, ok := baseFolders[b]
			if !ok {
				toPin = append(toPin, add.Hash)
			}
		}
	}
	return toPin
}

// SetClient makes the component ready to perform RPC
// requests.
func (ipfs *Connector) SetClient(c *rpc.Client) {
	ipfs.rpcClient = c
	ipfs.rpcReady <- struct{}{}
}

// Shutdown stops any listeners and stops the component from taking
// any requests.
func (ipfs *Connector) Shutdown() error {
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

// ID performs an ID request against the configured
// IPFS daemon. It returns the fetched information.
// If the request fails, or the parsing fails, it
// returns an error and an empty IPFSID which also
// contains the error message.
func (ipfs *Connector) ID() (api.IPFSID, error) {
	ctx, cancel := context.WithTimeout(ipfs.ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	id := api.IPFSID{}
	body, err := ipfs.postCtx(ctx, "id", nil)
	if err != nil {
		id.Error = err.Error()
		return id, err
	}

	var res ipfsIDResp
	err = json.Unmarshal(body, &res)
	if err != nil {
		id.Error = err.Error()
		return id, err
	}

	pID, err := peer.IDB58Decode(res.ID)
	if err != nil {
		id.Error = err.Error()
		return id, err
	}
	id.ID = pID

	mAddrs := make([]ma.Multiaddr, len(res.Addresses), len(res.Addresses))
	for i, strAddr := range res.Addresses {
		mAddr, err := ma.NewMultiaddr(strAddr)
		if err != nil {
			id.Error = err.Error()
			return id, err
		}
		mAddrs[i] = mAddr
	}
	id.Addresses = mAddrs
	return id, nil
}

// Pin performs a pin request against the configured IPFS
// daemon.
func (ipfs *Connector) Pin(ctx context.Context, hash *cid.Cid, recursive bool) error {
	ctx, cancel := context.WithTimeout(ctx, ipfs.config.PinTimeout)
	defer cancel()
	pinStatus, err := ipfs.PinLsCid(ctx, hash)
	if err != nil {
		return err
	}
	if !pinStatus.IsPinned() {
		switch ipfs.config.PinMethod {
		case "refs":
			path := fmt.Sprintf("refs?arg=%s&recursive=%t", hash, recursive)
			err := ipfs.postDiscardBodyCtx(ctx, path)
			if err != nil {
				return err
			}
			logger.Debugf("Refs for %s sucessfully fetched", hash)
		}

		path := fmt.Sprintf("pin/add?arg=%s&recursive=%t", hash, recursive)
		_, err = ipfs.postCtx(ctx, path, nil)
		if err == nil {
			logger.Info("IPFS Pin request succeeded: ", hash)
		}
		return err
	}
	logger.Debug("IPFS object is already pinned: ", hash)
	return nil
}

// Unpin performs an unpin request against the configured IPFS
// daemon.
func (ipfs *Connector) Unpin(ctx context.Context, hash *cid.Cid) error {
	ctx, cancel := context.WithTimeout(ctx, ipfs.config.UnpinTimeout)
	defer cancel()
	pinStatus, err := ipfs.PinLsCid(ctx, hash)
	if err != nil {
		return err
	}
	if pinStatus.IsPinned() {
		path := fmt.Sprintf("pin/rm?arg=%s", hash)
		_, err := ipfs.postCtx(ctx, path, nil)
		if err == nil {
			logger.Info("IPFS Unpin request succeeded:", hash)
		}
		return err
	}

	logger.Debug("IPFS object is already unpinned: ", hash)
	return nil
}

// PinLs performs a "pin ls --type typeFilter" request against the configured
// IPFS daemon and returns a map of cid strings and their status.
func (ipfs *Connector) PinLs(ctx context.Context, typeFilter string) (map[string]api.IPFSPinStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	body, err := ipfs.postCtx(ctx, "pin/ls?type="+typeFilter, nil)

	// Some error talking to the daemon
	if err != nil {
		return nil, err
	}

	var res ipfsPinLsResp
	err = json.Unmarshal(body, &res)
	if err != nil {
		logger.Error("parsing pin/ls response")
		logger.Error(string(body))
		return nil, err
	}

	statusMap := make(map[string]api.IPFSPinStatus)
	for k, v := range res.Keys {
		statusMap[k] = api.IPFSPinStatusFromString(v.Type)
	}
	return statusMap, nil
}

// PinLsCid performs a "pin ls --type=recursive <hash> "request and returns
// an api.IPFSPinStatus for that hash.
func (ipfs *Connector) PinLsCid(ctx context.Context, hash *cid.Cid) (api.IPFSPinStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	lsPath := fmt.Sprintf("pin/ls?arg=%s&type=recursive", hash)
	body, err := ipfs.postCtx(ctx, lsPath, nil)

	// Network error, daemon down
	if body == nil && err != nil {
		return api.IPFSPinStatusError, err
	}

	// Pin not found likely here
	if err != nil { // Not pinned
		return api.IPFSPinStatusUnpinned, nil
	}

	var res ipfsPinLsResp
	err = json.Unmarshal(body, &res)
	if err != nil {
		logger.Error("parsing pin/ls?arg=cid response:")
		logger.Error(string(body))
		return api.IPFSPinStatusError, err
	}
	pinObj, ok := res.Keys[hash.String()]
	if !ok {
		return api.IPFSPinStatusError, errors.New("expected to find the pin in the response")
	}

	return api.IPFSPinStatusFromString(pinObj.Type), nil
}

func (ipfs *Connector) doPostCtx(ctx context.Context, client *http.Client, apiURL, path string, contentType string, postBody io.Reader) (*http.Response, error) {
	logger.Debugf("posting %s", path)
	urlstr := fmt.Sprintf("%s/%s", apiURL, path)

	req, err := http.NewRequest("POST", urlstr, postBody)
	if err != nil {
		logger.Error("error creating POST request:", err)
	}

	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(ctx)
	res, err := ipfs.client.Do(req)
	if err != nil {
		logger.Error("error posting to IPFS:", err)
	}

	return res, err
}

// checkResponse tries to parse an error message on non StatusOK responses
// from ipfs.
func checkResponse(path string, code int, body []byte) error {
	if code == http.StatusOK {
		return nil
	}

	var ipfsErr ipfsError

	if body != nil && json.Unmarshal(body, &ipfsErr) == nil {
		return fmt.Errorf("IPFS unsuccessful: %d: %s", code, ipfsErr.Message)
	}
	// No error response with useful message from ipfs
	return fmt.Errorf("IPFS-post '%s' unsuccessful: %d: %s", path, code, body)
}

// postCtx makes a POST request against
// the ipfs daemon, reads the full body of the response and
// returns it after checking for errors.
func (ipfs *Connector) postCtx(ctx context.Context, path string, contentType string, postBody io.Reader) ([]byte, error) {
	res, err := ipfs.doPostCtx(ctx, ipfs.client, ipfs.apiURL(), path, contentType, postBody)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		logger.Errorf("error reading response body: %s", err)
		return nil, err
	}
	return body, checkResponse(path, res.StatusCode, body)
}

// postDiscardBodyCtx makes a POST requests but discards the body
// of the response directly after reading it.
func (ipfs *Connector) postDiscardBodyCtx(ctx context.Context, path string) error {
	res, err := ipfs.doPostCtx(ctx, ipfs.client, ipfs.apiURL(), path)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, err = io.Copy(ioutil.Discard, res.Body)
	if err != nil {
		return err
	}
	return checkResponse(path, res.StatusCode, nil)
}

// apiURL is a short-hand for building the url of the IPFS
// daemon API.
func (ipfs *Connector) apiURL() string {
	return fmt.Sprintf("http://%s/api/v0", ipfs.nodeAddr)
}

// ConnectSwarms requests the ipfs addresses of other peers and
// triggers ipfs swarm connect requests
func (ipfs *Connector) ConnectSwarms() error {
	ctx, cancel := context.WithTimeout(ipfs.ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	var idsSerial []api.IDSerial
	err := ipfs.rpcClient.Call(
		"",
		"Cluster",
		"Peers",
		struct{}{},
		&idsSerial,
	)
	if err != nil {
		logger.Error(err)
		return err
	}
	logger.Debugf("%+v", idsSerial)

	for _, idSerial := range idsSerial {
		ipfsID := idSerial.IPFS
		for _, addr := range ipfsID.Addresses {
			// This is a best effort attempt
			// We ignore errors which happens
			// when passing in a bunch of addresses
			_, err := ipfs.postCtx(
				ctx,
				fmt.Sprintf("swarm/connect?arg=%s", addr),
				nil,
			)
			if err != nil {
				logger.Debug(err)
				continue
			}
			logger.Debugf("ipfs successfully connected to %s", addr)
		}
	}
	return nil
}

// ConfigKey fetches the IPFS daemon configuration and retrieves the value for
// a given configuration key. For example, "Datastore/StorageMax" will return
// the value for StorageMax in the Datastore configuration object.
func (ipfs *Connector) ConfigKey(keypath string) (interface{}, error) {
	ctx, cancel := context.WithTimeout(ipfs.ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	res, err := ipfs.postCtx(ctx, "config/show", nil)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	var cfg map[string]interface{}
	err = json.Unmarshal(res, &cfg)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	path := strings.SplitN(keypath, "/", 2)
	if len(path) == 0 {
		return nil, errors.New("cannot lookup without a path")
	}

	return getConfigValue(path, cfg)
}

func getConfigValue(path []string, cfg map[string]interface{}) (interface{}, error) {
	value, ok := cfg[path[0]]
	if !ok {
		return nil, errors.New("key not found in configuration")
	}

	if len(path) == 1 {
		return value, nil
	}

	switch value.(type) {
	case map[string]interface{}:
		v := value.(map[string]interface{})
		return getConfigValue(path[1:], v)
	default:
		return nil, errors.New("invalid path")
	}
}

// FreeSpace returns the amount of unused space in the ipfs repository. This
// value is derived from the RepoSize and StorageMax values given by "repo
// stats". The value is in bytes.
func (ipfs *Connector) FreeSpace() (uint64, error) {
	ctx, cancel := context.WithTimeout(ipfs.ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	res, err := ipfs.postCtx(ctx, "repo/stat", nil)
	if err != nil {
		logger.Error(err)
		return 0, err
	}

	var stats ipfsRepoStatResp
	err = json.Unmarshal(res, &stats)
	if err != nil {
		logger.Error(err)
		return 0, err
	}
	return stats.StorageMax - stats.RepoSize, nil
}

// RepoSize returns the current repository size of the ipfs daemon as
// provided by "repo stats". The value is in bytes.
func (ipfs *Connector) RepoSize() (uint64, error) {
	ctx, cancel := context.WithTimeout(ipfs.ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	res, err := ipfs.postCtx(ctx, "repo/stat", nil)
	if err != nil {
		logger.Error(err)
		return 0, err
	}

	var stats ipfsRepoStatResp
	err = json.Unmarshal(res, &stats)
	if err != nil {
		logger.Error(err)
		return 0, err
	}
	return stats.RepoSize, nil
}

// SwarmPeers returns the peers currently connected to this ipfs daemon.
func (ipfs *Connector) SwarmPeers() (api.SwarmPeers, error) {
	ctx, cancel := context.WithTimeout(ipfs.ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	swarm := api.SwarmPeers{}
	res, err := ipfs.postCtx(ctx, "swarm/peers", nil)
	if err != nil {
		logger.Error(err)
		return swarm, err
	}
	var peersRaw ipfsSwarmPeersResp
	err = json.Unmarshal(res, &peersRaw)
	if err != nil {
		logger.Error(err)
		return swarm, err
	}

	swarm = make([]peer.ID, len(peersRaw.Peers))
	for i, p := range peersRaw.Peers {
		pID, err := peer.IDB58Decode(p.Peer)
		if err != nil {
			logger.Error(err)
			return swarm, err
		}
		swarm[i] = pID
	}
	return swarm, nil
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

// BlockPut triggers an ipfs block put on the given data, inserting the block
// into the ipfs daemon's repo.
func (ipfs *Connector) BlockPut(data []byte) (api.Pin, error) {
	pin := api.Pin{}

	r := ioutil.NopCloser(bytes.NewReader(data))
	rFile := files.NewReaderFile("", "", r, nil)
	sliceFile := files.NewSliceFile("", "", []files.File{rFile}) // IPFS reqs require a wrapping directory
	multiFileR := files.NewMultiFileReader(sliceFile, true)
	contentType := "multipart/form-data; boundary=" + multiFileR.Boundary()

	res, err := ipfs.post("block/put", contentType, multiFileR)
	if err != nil {
		return pin, err
	}
	var keyRaw ipfsBlockPutResp
	err = json.Unmarshal(res, &keyRaw)
	if err != nil {
		return pin, err
	}
	c, err := cid.Decode(keyRaw.Key)
	if err != nil {
		return pin, err
	}

	return api.PinCid(c), nil
}
