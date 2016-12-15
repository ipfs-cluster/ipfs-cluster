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
	"strings"
	"sync"

	cid "github.com/ipfs/go-cid"
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
	destHost   string
	destPort   int
	listenAddr string
	listenPort int
	handlers   map[string]func(http.ResponseWriter, *http.Request)
	rpcCh      chan ClusterRPC

	listener net.Listener
	server   *http.Server

	shutdownLock sync.Mutex
	shutdown     bool
	wg           sync.WaitGroup
}

type ipfsError struct {
	Message string
}

// NewIPFSHTTPConnector creates the component and leaves it ready to be started
func NewIPFSHTTPConnector(cfg *ClusterConfig) (*IPFSHTTPConnector, error) {
	ctx := context.Background()
	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d",
		cfg.IPFSAPIListenAddr, cfg.IPFSAPIListenPort))
	if err != nil {
		return nil, err
	}

	smux := http.NewServeMux()
	s := &http.Server{
		Handler: smux,
	}
	s.SetKeepAlivesEnabled(true) // A reminder that this can be changed

	ipfs := &IPFSHTTPConnector{
		ctx:        ctx,
		destHost:   cfg.IPFSHost,
		destPort:   cfg.IPFSPort,
		listenAddr: cfg.IPFSAPIListenAddr,
		listenPort: cfg.IPFSAPIListenPort,
		handlers:   make(map[string]func(http.ResponseWriter, *http.Request)),
		rpcCh:      make(chan ClusterRPC, RPCMaxQueue),
		listener:   l,
		server:     s,
	}

	smux.HandleFunc("/", ipfs.handle)

	logger.Infof("Starting IPFS Proxy on %s:%d", ipfs.listenAddr, ipfs.listenPort)
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
		err := ipfs.server.Serve(ipfs.listener)
		if err != nil && !strings.Contains(err.Error(), "closed network connection") {
			logger.Error(err)
		}
	}()
}

// RpcChan can be used by Cluster to read any
// requests from this component.
func (ipfs *IPFSHTTPConnector) RpcChan() <-chan ClusterRPC {
	return ipfs.rpcCh
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

	logger.Info("Stopping IPFS Proxy")

	ipfs.server.SetKeepAlivesEnabled(false)
	ipfs.listener.Close()

	ipfs.wg.Wait()
	ipfs.shutdown = true
	return nil
}

// Pin performs a pin request against the configured IPFS
// daemon.
func (ipfs *IPFSHTTPConnector) Pin(hash *cid.Cid) error {
	logger.Infof("IPFS Pin request for: %s", hash)
	pinned, err := ipfs.IsPinned(hash)
	if err != nil {
		return err
	}
	if !pinned {
		path := fmt.Sprintf("pin/add?arg=%s", hash)
		_, err = ipfs.get(path)
		return err
	}
	logger.Debug("object is already pinned. Doing nothing")
	return nil
}

// UnPin performs an unpin request against the configured IPFS
// daemon.
func (ipfs *IPFSHTTPConnector) Unpin(hash *cid.Cid) error {
	logger.Info("IPFS Unpin request for:", hash)
	pinned, err := ipfs.IsPinned(hash)
	if err != nil {
		return err
	}
	if pinned {
		path := fmt.Sprintf("pin/rm?arg=%s", hash)
		_, err := ipfs.get(path)
		return err
	}

	logger.Debug("object not [directly] pinned. Doing nothing")
	return nil
}

func (ipfs *IPFSHTTPConnector) IsPinned(hash *cid.Cid) (bool, error) {
	pinType, err := ipfs.pinType(hash)
	if err != nil {
		return false, err
	}

	if pinType == "unpinned" || strings.Contains(pinType, "indirect") {
		return false, nil
	}
	return true, nil
}

// Returns how a hash is pinned
func (ipfs *IPFSHTTPConnector) pinType(hash *cid.Cid) (string, error) {
	lsPath := fmt.Sprintf("pin/ls?arg=%s", hash)
	body, err := ipfs.get(lsPath)

	// Network error, daemon down
	if body == nil && err != nil {
		return "", err
	}

	// Pin not found likely here
	if err != nil { // Not pinned
		return "unpinned", nil
	}

	// What type of pin it is
	var resp struct {
		Keys map[string]struct {
			Type string
		}
	}

	err = json.Unmarshal(body, &resp)
	if err != nil {
		logger.Error("parsing pin/ls response:")
		logger.Error(string(body))
		return "", err
	}
	pinObj, ok := resp.Keys[hash.String()]
	if !ok {
		return "", errors.New("expected to find the pin in the response")
	}
	pinType := pinObj.Type
	logger.Debug("pinType check: ", pinType)
	return pinType, nil
}

// get performs the heavy lifting of a get request against
// the IPFS daemon.
func (ipfs *IPFSHTTPConnector) get(path string) ([]byte, error) {
	logger.Debugf("Getting %s", path)
	url := fmt.Sprintf("%s/%s",
		ipfs.apiURL(),
		path)

	resp, err := http.Get(url)
	if err != nil {
		logger.Error("Error getting:", err)
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Errorf("Error reading response body: %s", err)
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
