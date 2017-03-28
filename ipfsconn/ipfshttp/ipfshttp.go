// Package ipfshttp implements an IPFS Cluster IPFSConnector component. It
// uses the IPFS HTTP API to communicate to IPFS.
package ipfshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

var logger = logging.Logger("ipfshttp")

// ConnectSwarmsDelay specifies how long to wait after startup before attempting
// to open connections from this peer's IPFS daemon to the IPFS daemons
// of other peers.
var ConnectSwarmsDelay = 7 * time.Second

// IPFS Proxy settings
var (
	// maximum duration before timing out read of the request
	IPFSProxyServerReadTimeout = 5 * time.Second
	// maximum duration before timing out write of the response
	IPFSProxyServerWriteTimeout = 10 * time.Second
	// server-side the amount of time a Keep-Alive connection will be
	// kept idle before being reused
	IPFSProxyServerIdleTimeout = 60 * time.Second
)

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

	nodeMAddr  ma.Multiaddr
	nodeAddr   string
	proxyMAddr ma.Multiaddr

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
	RepoSize   int
	NumObjects int
}

// NewConnector creates the component and leaves it ready to be started
func NewConnector(ipfsNodeMAddr ma.Multiaddr, ipfsProxyMAddr ma.Multiaddr) (*Connector, error) {
	_, nodeAddr, err := manet.DialArgs(ipfsNodeMAddr)
	if err != nil {
		return nil, err
	}

	proxyNet, proxyAddr, err := manet.DialArgs(ipfsProxyMAddr)
	if err != nil {
		return nil, err
	}

	l, err := net.Listen(proxyNet, proxyAddr)
	if err != nil {
		return nil, err
	}

	smux := http.NewServeMux()
	s := &http.Server{
		ReadTimeout:  IPFSProxyServerReadTimeout,
		WriteTimeout: IPFSProxyServerWriteTimeout,
		// IdleTimeout:  IPFSProxyServerIdleTimeout, // TODO Go 1.8
		Handler: smux,
	}
	s.SetKeepAlivesEnabled(true) // A reminder that this can be changed

	ctx, cancel := context.WithCancel(context.Background())

	ipfs := &Connector{
		ctx:        ctx,
		cancel:     cancel,
		nodeMAddr:  ipfsNodeMAddr,
		nodeAddr:   nodeAddr,
		proxyMAddr: ipfsProxyMAddr,
		handlers:   make(map[string]func(http.ResponseWriter, *http.Request)),
		rpcReady:   make(chan struct{}, 1),
		listener:   l,
		server:     s,
	}

	smux.HandleFunc("/", ipfs.handle)
	ipfs.handlers["/api/v0/pin/add"] = ipfs.pinHandler
	ipfs.handlers["/api/v0/pin/rm"] = ipfs.unpinHandler
	ipfs.handlers["/api/v0/pin/ls"] = ipfs.pinLsHandler

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
			ipfs.proxyMAddr,
			ipfs.nodeMAddr)
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
		tmr := time.NewTimer(ConnectSwarmsDelay)
		defer tmr.Stop()
		select {
		case <-tmr.C:
			ipfs.ConnectSwarms()
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

// defaultHandler just proxies the requests
func (ipfs *Connector) defaultHandler(w http.ResponseWriter, r *http.Request) {
	newURL := *r.URL
	newURL.Host = ipfs.nodeAddr
	newURL.Scheme = "http"

	proxyReq, err := http.NewRequest(r.Method, newURL.String(), r.Body)
	if err != nil {
		logger.Error("error creating proxy request: ", err)
		http.Error(w, "error forwarding request", 500)
		return
	}

	resp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		logger.Error("error forwarding request: ", err)
		http.Error(w, "error forwaring request", 500)
		return
	}

	w.WriteHeader(resp.StatusCode)

	// Set response headers
	for k, v := range resp.Header {
		for _, s := range v {
			w.Header().Add(k, s)
		}
	}

	// And body
	io.Copy(w, resp.Body)
	resp.Body.Close()
}

func ipfsErrorResponder(w http.ResponseWriter, errMsg string) {
	resp := ipfsError{errMsg}
	respBytes, _ := json.Marshal(resp)
	w.WriteHeader(http.StatusInternalServerError)
	w.Write(respBytes)
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

	resp := ipfsPinOpResp{
		Pins: []string{arg},
	}
	respBytes, _ := json.Marshal(resp)
	w.WriteHeader(http.StatusOK)
	w.Write(respBytes)
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

	var pins []api.PinSerial
	err := ipfs.rpcClient.Call("",
		"Cluster",
		"PinList",
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

	argA, ok := r.URL.Query()["arg"]
	if ok {
		if len(argA) == 0 {
			ipfsErrorResponder(w, "Error: bad argument")
			return
		}
		arg := argA[0]
		singlePin, ok := pinLs.Keys[arg]
		if ok {
			pinLs.Keys = map[string]ipfsPinType{
				arg: singlePin,
			}
		} else {
			ipfsErrorResponder(w, fmt.Sprintf(
				"Error: path '%s' is not pinned",
				arg))
			return
		}
	}

	respBytes, _ := json.Marshal(pinLs)
	w.WriteHeader(http.StatusOK)
	w.Write(respBytes)
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
	body, err := ipfs.get("id")
	if err != nil {
		id.Error = err.Error()
		return id, err
	}

	var resp ipfsIDResp
	err = json.Unmarshal(body, &resp)
	if err != nil {
		id.Error = err.Error()
		return id, err
	}

	pID, err := peer.IDB58Decode(resp.ID)
	if err != nil {
		id.Error = err.Error()
		return id, err
	}
	id.ID = pID

	mAddrs := make([]ma.Multiaddr, len(resp.Addresses), len(resp.Addresses))
	for i, strAddr := range resp.Addresses {
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
		_, err = ipfs.get(path)
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
		_, err := ipfs.get(path)
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
	body, err := ipfs.get("pin/ls?type=" + typeFilter)

	// Some error talking to the daemon
	if err != nil {
		return nil, err
	}

	var resp ipfsPinLsResp
	err = json.Unmarshal(body, &resp)
	if err != nil {
		logger.Error("parsing pin/ls response")
		logger.Error(string(body))
		return nil, err
	}

	statusMap := make(map[string]api.IPFSPinStatus)
	for k, v := range resp.Keys {
		statusMap[k] = api.IPFSPinStatusFromString(v.Type)
	}
	return statusMap, nil
}

// PinLsCid performs a "pin ls --type=recursive <hash> "request and returns
// an api.IPFSPinStatus for that hash.
func (ipfs *Connector) PinLsCid(hash *cid.Cid) (api.IPFSPinStatus, error) {
	lsPath := fmt.Sprintf("pin/ls?arg=%s&type=recursive", hash)
	body, err := ipfs.get(lsPath)

	// Network error, daemon down
	if body == nil && err != nil {
		return api.IPFSPinStatusError, err
	}

	// Pin not found likely here
	if err != nil { // Not pinned
		return api.IPFSPinStatusUnpinned, nil
	}

	var resp ipfsPinLsResp
	err = json.Unmarshal(body, &resp)
	if err != nil {
		logger.Error("parsing pin/ls?arg=cid response:")
		logger.Error(string(body))
		return api.IPFSPinStatusError, err
	}
	pinObj, ok := resp.Keys[hash.String()]
	if !ok {
		return api.IPFSPinStatusError, errors.New("expected to find the pin in the response")
	}

	return api.IPFSPinStatusFromString(pinObj.Type), nil
}

// get performs the heavy lifting of a get request against
// the IPFS daemon.
func (ipfs *Connector) get(path string) ([]byte, error) {
	logger.Debugf("getting %s", path)
	url := fmt.Sprintf("%s/%s",
		ipfs.apiURL(),
		path)

	resp, err := http.Get(url)
	if err != nil {
		logger.Error("error getting:", err)
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Errorf("error reading response body: %s", err)
		return nil, err
	}

	var ipfsErr ipfsError
	decodeErr := json.Unmarshal(body, &ipfsErr)

	if resp.StatusCode != http.StatusOK {
		var msg string
		if decodeErr == nil {
			msg = fmt.Sprintf("IPFS unsuccessful: %d: %s",
				resp.StatusCode, ipfsErr.Message)
		} else {
			msg = fmt.Sprintf("IPFS-get '%s' unsuccessful: %d: %s",
				path, resp.StatusCode, body)
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
			_, err := ipfs.get(
				fmt.Sprintf("swarm/connect?arg=%s", addr))
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
	resp, err := ipfs.get("config/show")
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	var cfg map[string]interface{}
	err = json.Unmarshal(resp, &cfg)
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

// RepoSize returns the current repository size of the ipfs daemon as
// provided by "repo stats". The value is in bytes.
func (ipfs *Connector) RepoSize() (int, error) {
	resp, err := ipfs.get("repo/stat")
	if err != nil {
		logger.Error(err)
		return 0, err
	}

	var stats ipfsRepoStatResp
	err = json.Unmarshal(resp, &stats)
	if err != nil {
		logger.Error(err)
		return 0, err
	}
	return stats.RepoSize, nil
}
