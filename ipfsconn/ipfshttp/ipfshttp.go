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
	manet "github.com/multiformats/go-multiaddr-net"
)

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

	listener net.Listener
	server   *http.Server

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

	_, nodeAddr, err := manet.DialArgs(cfg.NodeAddr)
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
	}

	smux.HandleFunc("/", ipfs.handle)
	ipfs.handlers["/api/v0/pin/add"] = ipfs.pinHandler
	ipfs.handlers["/api/v0/pin/rm"] = ipfs.unpinHandler
	ipfs.handlers["/api/v0/pin/ls"] = ipfs.pinLsHandler
	ipfs.handlers["/api/v0/add"] = ipfs.addHandler

	go ipfs.run()
	return ipfs, nil
}

// launches proxy and connects all ipfs daemons when
// we receive the rpcReady signal.
func (ipfs *Connector) run() {
	<-ipfs.rpcReady

	// This launches the proxy
	ipfs.wg.Add(1)
	go func() {
		defer ipfs.wg.Done()
		logger.Infof("IPFS Proxy: %s -> %s",
			ipfs.config.ProxyAddr,
			ipfs.config.NodeAddr)
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

// This will run a custom handler if we have one for a URL.Path, or
// otherwise just proxy the requests.
func (ipfs *Connector) handle(w http.ResponseWriter, r *http.Request) {
	if customHandler, ok := ipfs.handlers[r.URL.Path]; ok {
		customHandler(w, r)
	} else {
		ipfs.defaultHandler(w, r)
	}

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
	argA := r.URL.Query()["arg"]
	if len(argA) == 0 {
		ipfsErrorResponder(w, "Error: bad argument")
		return
	}
	arg := argA[0]
	_, err := cid.Decode(arg)
	if err != nil {
		ipfsErrorResponder(w, "Error parsing CID: "+err.Error())
		return
	}

	err = ipfs.rpcClient.Call("",
		"Cluster",
		op,
		api.PinSerial{
			Cid: arg,
		},
		&struct{}{})

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

	q := r.URL.Query()
	arg := q.Get("arg")
	if arg != "" {
		c, err := cid.Decode(arg)
		if err != nil {
			ipfsErrorResponder(w, err.Error())
			return
		}
		var pin api.PinSerial
		err = ipfs.rpcClient.Call("",
			"Cluster",
			"PinGet",
			api.PinCid(c).ToSerial(),
			&pin)
		if err != nil {
			ipfsErrorResponder(w, fmt.Sprintf(
				"Error: path '%s' is not pinned",
				arg))
			return
		}
		pinLs.Keys[pin.Cid] = ipfsPinType{
			Type: "recursive",
		}
	} else {
		var pins []api.PinSerial
		err := ipfs.rpcClient.Call("",
			"Cluster",
			"Pins",
			struct{}{},
			&pins)

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
		err := ipfs.rpcClient.Call("",
			"Cluster",
			"Pin",
			api.PinSerial{
				Cid: pin,
			},
			&struct{}{})
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
	id := api.IPFSID{}
	body, err := ipfs.post("id", "", nil)
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
func (ipfs *Connector) Pin(hash *cid.Cid) error {
	pinStatus, err := ipfs.PinLsCid(hash)
	if err != nil {
		return err
	}
	if !pinStatus.IsPinned() {
		path := fmt.Sprintf("pin/add?arg=%s", hash)
		_, err = ipfs.post(path, "", nil)
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
func (ipfs *Connector) Unpin(hash *cid.Cid) error {
	pinStatus, err := ipfs.PinLsCid(hash)
	if err != nil {
		return err
	}
	if pinStatus.IsPinned() {
		path := fmt.Sprintf("pin/rm?arg=%s", hash)
		_, err := ipfs.post(path, "", nil)
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
func (ipfs *Connector) PinLs(typeFilter string) (map[string]api.IPFSPinStatus, error) {
	body, err := ipfs.post("pin/ls?type="+typeFilter, "", nil)

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
func (ipfs *Connector) PinLsCid(hash *cid.Cid) (api.IPFSPinStatus, error) {
	lsPath := fmt.Sprintf("pin/ls?arg=%s&type=recursive", hash)
	body, err := ipfs.post(lsPath, "", nil)

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

// post performs the heavy lifting of a post request against
// the IPFS daemon.
func (ipfs *Connector) post(path string, contentType string, postBody io.Reader) ([]byte, error) {
	logger.Debugf("posting %s", path)
	url := fmt.Sprintf("%s/%s",
		ipfs.apiURL(),
		path)

	res, err := http.Post(url, contentType, postBody)
	if err != nil {
		logger.Error("error posting:", err)
		return nil, err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		logger.Errorf("error reading response body: %s", err)
		return nil, err
	}

	var ipfsErr ipfsError
	decodeErr := json.Unmarshal(body, &ipfsErr)

	if res.StatusCode != http.StatusOK {
		var msg string
		if decodeErr == nil {
			msg = fmt.Sprintf("IPFS unsuccessful: %d: %s",
				res.StatusCode, ipfsErr.Message)
		} else {
			msg = fmt.Sprintf("IPFS-get '%s' unsuccessful: %d: %s",
				path, res.StatusCode, body)
		}

		return body, errors.New(msg)
	}
	return body, nil
}

// apiURL is a short-hand for building the url of the IPFS
// daemon API.
func (ipfs *Connector) apiURL() string {
	return fmt.Sprintf("http://%s/api/v0", ipfs.nodeAddr)
}

// ConnectSwarms requests the ipfs addresses of other peers and
// triggers ipfs swarm connect requests
func (ipfs *Connector) ConnectSwarms() error {
	var idsSerial []api.IDSerial
	err := ipfs.rpcClient.Call("",
		"Cluster",
		"Peers",
		struct{}{},
		&idsSerial)
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
			_, err := ipfs.post(
				fmt.Sprintf("swarm/connect?arg=%s", addr), "", nil)
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
	res, err := ipfs.post("config/show", "", nil)
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
	res, err := ipfs.post("repo/stat", "", nil)
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
	res, err := ipfs.post("repo/stat", "", nil)
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
	swarm := api.SwarmPeers{}
	res, err := ipfs.post("swarm/peers", "", nil)
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

// BlockPut triggers an ipfs block put on the given data, inserting the block
// into the ipfs daemon's repo.
func (ipfs *Connector) BlockPut(b api.NodeWithMeta) (string, error) {
	r := ioutil.NopCloser(bytes.NewReader(b.Data))
	rFile := files.NewReaderFile("", "", r, nil)
	sliceFile := files.NewSliceFile("", "", []files.File{rFile}) // IPFS reqs require a wrapping directory
	multiFileR := files.NewMultiFileReader(sliceFile, true)
	if b.Format == "" {
		b.Format = "v0"
	}
	url := "block/put?f=" + b.Format
	contentType := "multipart/form-data; boundary=" + multiFileR.Boundary()
	res, err := ipfs.post(url, contentType, multiFileR)
	if err != nil {
		return "", err
	}
	var keyRaw ipfsBlockPutResp
	err = json.Unmarshal(res, &keyRaw)
	if err != nil {
		return "", err
	}
	return keyRaw.Key, nil
}
