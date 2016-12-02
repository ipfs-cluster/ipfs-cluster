package ipfscluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
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
	cancel     context.CancelFunc
	destHost   string
	destPort   int
	listenAddr string
	listenPort int
	handlers   map[string]func(http.ResponseWriter, *http.Request)
	rpcCh      chan ClusterRPC
}

type ipfsError struct {
	Message string
}

// NewIPFSHTTPConnector creates the component and leaves it ready to be started
func NewIPFSHTTPConnector(cfg *ClusterConfig) (*IPFSHTTPConnector, error) {
	ctx, cancel := context.WithCancel(context.Background())
	ipfs := &IPFSHTTPConnector{
		ctx:        ctx,
		cancel:     cancel,
		destHost:   cfg.IPFSHost,
		destPort:   cfg.IPFSPort,
		listenAddr: cfg.IPFSAPIListenAddr,
		listenPort: cfg.IPFSAPIListenPort,
		handlers:   make(map[string]func(http.ResponseWriter, *http.Request)),
		rpcCh:      make(chan ClusterRPC, RPCMaxQueue),
	}

	logger.Infof("Starting IPFS Proxy on %s:%d", ipfs.listenAddr, ipfs.listenPort)
	go ipfs.run()
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
	go func() {
		smux := http.NewServeMux()
		smux.HandleFunc("/", ipfs.handle)
		// Fixme: make this with closable net listener
		err := http.ListenAndServe(
			fmt.Sprintf("%s:%d", ipfs.listenAddr, ipfs.listenPort),
			smux)
		if err != nil {
			logger.Error(err)
			return
		}
	}()

	go func() {
		select {
		case <-ipfs.ctx.Done():
			return
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
	logger.Info("Stopping IPFS Proxy")
	return nil
}

// Pin performs a pin request against the configured IPFS
// daemon.
func (ipfs *IPFSHTTPConnector) Pin(hash *cid.Cid) error {
	logger.Infof("IPFS Pin request for: %s", hash)
	pinType, err := ipfs.pinType(hash)

	if err != nil || strings.Contains(pinType, "indirect") {
		// Not pinned or indirectly pinned
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
	pinType, err := ipfs.pinType(hash)

	if err == nil && !strings.Contains(pinType, "indirect") {
		path := fmt.Sprintf("pin/rm?arg=%s", hash)
		_, err := ipfs.get(path)
		return err
	}

	// It is not pinned we do nothing
	logger.Debug("object not directly pinned. Doing nothing")
	return nil
}

// Returns how a hash is pinned
func (ipfs *IPFSHTTPConnector) pinType(hash *cid.Cid) (string, error) {
	lsPath := fmt.Sprintf("pin/ls?arg=%s", hash)
	body, err := ipfs.get(lsPath)
	if err != nil { // Not pinned
		return "", err
	}

	// What type of pin it is
	var resp struct {
		Keys map[string]struct {
			Type string
		}
	}

	err = json.Unmarshal(body, &resp)
	if err != nil {
		logger.Error("parsing pin/ls response")
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
	url := fmt.Sprintf("%s/%s",
		ipfs.apiURL(),
		path)

	resp, err := http.Get(url)
	if err != nil {
		logger.Error("Error unpinning:", err)
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
			msg = fmt.Sprintf("IPFS error: %d: %s",
				resp.StatusCode, ipfsErr.Message)
		} else {
			msg = fmt.Sprintf("IPFS error: %d: %s",
				resp.StatusCode, body)
		}
		logger.Error(msg)
		return nil, errors.New(msg)
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
