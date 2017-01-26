package ipfscluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
)

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

// IPFSHTTPConnector implements the IPFSConnector interface
// and provides a component which does two tasks:
//
// On one side, it proxies HTTP requests to the configured IPFS
// daemon. It is able to intercept these requests though, and
// perform extra operations on them.
//
// On the other side, it is used to perform on-demand requests
// against the configured IPFS daemom (such as a pin request).
type IPFSHTTPConnector struct {
	ctx        context.Context
	nodeAddr   ma.Multiaddr
	proxyAddr  ma.Multiaddr
	destHost   string
	destPort   int
	listenAddr string
	listenPort int

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

type pinLsResp struct {
	Keys map[string]struct {
		Type string
	}
}

type ipfsIDResp struct {
	ID        string
	Addresses []string
}

// NewIPFSHTTPConnector creates the component and leaves it ready to be started
func NewIPFSHTTPConnector(cfg *Config) (*IPFSHTTPConnector, error) {
	ctx := context.Background()
	destHost, err := cfg.IPFSNodeAddr.ValueForProtocol(ma.P_IP4)
	if err != nil {
		return nil, err
	}
	destPortStr, err := cfg.IPFSNodeAddr.ValueForProtocol(ma.P_TCP)
	if err != nil {
		return nil, err
	}
	destPort, err := strconv.Atoi(destPortStr)
	if err != nil {
		return nil, err
	}

	listenAddr, err := cfg.IPFSProxyAddr.ValueForProtocol(ma.P_IP4)
	if err != nil {
		return nil, err
	}
	listenPortStr, err := cfg.IPFSProxyAddr.ValueForProtocol(ma.P_TCP)
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

	smux := http.NewServeMux()
	s := &http.Server{
		ReadTimeout:  IPFSProxyServerReadTimeout,
		WriteTimeout: IPFSProxyServerWriteTimeout,
		// IdleTimeout:  IPFSProxyServerIdleTimeout, // TODO Go 1.8
		Handler: smux,
	}
	s.SetKeepAlivesEnabled(true) // A reminder that this can be changed

	ipfs := &IPFSHTTPConnector{
		ctx:       ctx,
		nodeAddr:  cfg.IPFSProxyAddr,
		proxyAddr: cfg.IPFSNodeAddr,

		destHost:   destHost,
		destPort:   destPort,
		listenAddr: listenAddr,
		listenPort: listenPort,
		handlers:   make(map[string]func(http.ResponseWriter, *http.Request)),
		rpcReady:   make(chan struct{}, 1),
		listener:   l,
		server:     s,
	}

	smux.HandleFunc("/", ipfs.handle)

	ipfs.run()
	return ipfs, nil
}

// This will run a custom handler if we have one for a URL.Path, or
// otherwise just proxy the requests.
func (ipfs *IPFSHTTPConnector) handle(w http.ResponseWriter, r *http.Request) {
	if customHandler, ok := ipfs.handlers[r.URL.Path]; ok {
		customHandler(w, r)
	} else {
		ipfs.defaultHandler(w, r)
	}

}

// defaultHandler just proxies the requests
func (ipfs *IPFSHTTPConnector) defaultHandler(w http.ResponseWriter, r *http.Request) {
	newURL := *r.URL
	newURL.Host = fmt.Sprintf("%s:%d", ipfs.destHost, ipfs.destPort)
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

	// Set response headers
	for k, v := range resp.Header {
		for _, s := range v {
			w.Header().Add(k, s)
		}
	}

	// And body
	io.Copy(w, resp.Body)
}

func (ipfs *IPFSHTTPConnector) run() {
	// This launches the proxy
	ipfs.wg.Add(1)
	go func() {
		defer ipfs.wg.Done()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ipfs.ctx = ctx

		<-ipfs.rpcReady

		logger.Infof("IPFS Proxy: %s -> %s",
			ipfs.proxyAddr,
			ipfs.nodeAddr)
		err := ipfs.server.Serve(ipfs.listener)
		if err != nil && !strings.Contains(err.Error(), "closed network connection") {
			logger.Error(err)
		}
	}()
}

// SetClient makes the component ready to perform RPC
// requests.
func (ipfs *IPFSHTTPConnector) SetClient(c *rpc.Client) {
	ipfs.rpcClient = c
	ipfs.rpcReady <- struct{}{}
}

// Shutdown stops any listeners and stops the component from taking
// any requests.
func (ipfs *IPFSHTTPConnector) Shutdown() error {
	ipfs.shutdownLock.Lock()
	defer ipfs.shutdownLock.Unlock()

	if ipfs.shutdown {
		logger.Debug("already shutdown")
		return nil
	}

	logger.Info("stopping IPFS Proxy")

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
func (ipfs *IPFSHTTPConnector) ID() (IPFSID, error) {
	id := IPFSID{}
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
func (ipfs *IPFSHTTPConnector) Pin(hash *cid.Cid) error {
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
func (ipfs *IPFSHTTPConnector) Unpin(hash *cid.Cid) error {
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

func parseIPFSPinType(t string) IPFSPinStatus {
	switch {
	case t == "indirect":
		return IPFSPinStatusIndirect
	case t == "direct":
		return IPFSPinStatusDirect
	case t == "recursive":
		return IPFSPinStatusRecursive
	default:
		return IPFSPinStatusBug
	}
}

// PinLs performs a "pin ls" request against the configured IPFS daemon and
// returns a map of cid strings and their status.
func (ipfs *IPFSHTTPConnector) PinLs() (map[string]IPFSPinStatus, error) {
	body, err := ipfs.get("pin/ls")

	// Some error talking to the daemon
	if err != nil {
		return nil, err
	}

	var resp pinLsResp
	err = json.Unmarshal(body, &resp)
	if err != nil {
		logger.Error("parsing pin/ls response")
		logger.Error(string(body))
		return nil, err
	}

	statusMap := make(map[string]IPFSPinStatus)
	for k, v := range resp.Keys {
		statusMap[k] = parseIPFSPinType(v.Type)
	}
	return statusMap, nil
}

// PinLsCid performs a "pin ls <hash> "request and returns IPFSPinStatus for
// that hash.
func (ipfs *IPFSHTTPConnector) PinLsCid(hash *cid.Cid) (IPFSPinStatus, error) {
	lsPath := fmt.Sprintf("pin/ls?arg=%s", hash)
	body, err := ipfs.get(lsPath)

	// Network error, daemon down
	if body == nil && err != nil {
		return IPFSPinStatusError, err
	}

	// Pin not found likely here
	if err != nil { // Not pinned
		return IPFSPinStatusUnpinned, nil
	}

	var resp pinLsResp
	err = json.Unmarshal(body, &resp)
	if err != nil {
		logger.Error("parsing pin/ls?arg=cid response:")
		logger.Error(string(body))
		return IPFSPinStatusError, err
	}
	pinObj, ok := resp.Keys[hash.String()]
	if !ok {
		return IPFSPinStatusError, errors.New("expected to find the pin in the response")
	}

	return parseIPFSPinType(pinObj.Type), nil
}

// get performs the heavy lifting of a get request against
// the IPFS daemon.
func (ipfs *IPFSHTTPConnector) get(path string) ([]byte, error) {
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
			msg = fmt.Sprintf("IPFS-get unsuccessful: %d: %s",
				resp.StatusCode, body)
		}
		logger.Warning(msg)
		return body, errors.New(msg)
	}
	return body, nil
}

// apiURL is a short-hand for building the url of the IPFS
// daemon API.
func (ipfs *IPFSHTTPConnector) apiURL() string {
	return fmt.Sprintf("http://%s:%d/api/v0",
		ipfs.destHost,
		ipfs.destPort)
}
