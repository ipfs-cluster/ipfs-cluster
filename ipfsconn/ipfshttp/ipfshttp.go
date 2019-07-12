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
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/observations"

	cid "github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	logging "github.com/ipfs/go-log"
	gopath "github.com/ipfs/go-path"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	madns "github.com/multiformats/go-multiaddr-dns"
	manet "github.com/multiformats/go-multiaddr-net"

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/plugin/ochttp/propagation/tracecontext"
	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
)

// DNSTimeout is used when resolving DNS multiaddresses in this module
var DNSTimeout = 5 * time.Second

var logger = logging.Logger("ipfshttp")

// updateMetricsMod only makes updates to informer metrics
// on the nth occasion. So, for example, for every BlockPut,
// only the 10th will trigger a SendInformerMetrics call.
var updateMetricMod = 10

// progressTick sets how often we check progress when doing refs and pins
// requests.
var progressTick = 5 * time.Second

// Connector implements the IPFSConnector interface
// and provides a component which  is used to perform
// on-demand requests against the configured IPFS daemom
// (such as a pin request).
type Connector struct {
	ctx    context.Context
	cancel func()

	config   *Config
	nodeAddr string

	rpcClient *rpc.Client
	rpcReady  chan struct{}

	client *http.Client // client to ipfs daemon

	updateMetricMutex sync.Mutex
	updateMetricCount int

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

type ipfsIDResp struct {
	ID        string
	Addresses []string
}

type ipfsResolveResp struct {
	Path string
}

type ipfsRepoGCResp struct {
	Key   cid.Cid
	Error string `json:",omitempty"`
}

type ipfsRefsResp struct {
	Ref string
	Err string
}

type ipfsPinsResp struct {
	Pins     []string
	Progress int
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

	c := &http.Client{} // timeouts are handled by context timeouts
	if cfg.Tracing {
		c.Transport = &ochttp.Transport{
			Base:           http.DefaultTransport,
			Propagation:    &tracecontext.HTTPFormat{},
			StartOptions:   trace.StartOptions{SpanKind: trace.SpanKindClient},
			FormatSpanName: func(req *http.Request) string { return req.Host + ":" + req.URL.Path + ":" + req.Method },
			NewClientTrace: ochttp.NewSpanAnnotatingClientTrace,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	ipfs := &Connector{
		ctx:      ctx,
		config:   cfg,
		cancel:   cancel,
		nodeAddr: nodeAddr,
		rpcReady: make(chan struct{}, 1),
		client:   c,
	}

	go ipfs.run()
	return ipfs, nil
}

// connects all ipfs daemons when
// we receive the rpcReady signal.
func (ipfs *Connector) run() {
	<-ipfs.rpcReady

	// Do not shutdown while launching threads
	// -- prevents race conditions with ipfs.wg.
	ipfs.shutdownLock.Lock()
	defer ipfs.shutdownLock.Unlock()

	if ipfs.config.ConnectSwarmsDelay == 0 {
		return
	}

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
			go ipfs.ConnectSwarms(ipfs.ctx)
		case <-ipfs.ctx.Done():
			return
		}
	}()
}

// SetClient makes the component ready to perform RPC
// requests.
func (ipfs *Connector) SetClient(c *rpc.Client) {
	ipfs.rpcClient = c
	ipfs.rpcReady <- struct{}{}
}

// Shutdown stops any listeners and stops the component from taking
// any requests.
func (ipfs *Connector) Shutdown(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/Shutdown")
	defer span.End()

	ipfs.shutdownLock.Lock()
	defer ipfs.shutdownLock.Unlock()

	if ipfs.shutdown {
		logger.Debug("already shutdown")
		return nil
	}

	logger.Info("stopping IPFS Connector")

	ipfs.cancel()
	close(ipfs.rpcReady)

	ipfs.wg.Wait()
	ipfs.shutdown = true

	return nil
}

// ID performs an ID request against the configured
// IPFS daemon. It returns the fetched information.
// If the request fails, or the parsing fails, it
// returns an error.
func (ipfs *Connector) ID(ctx context.Context) (*api.IPFSID, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/ID")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()

	body, err := ipfs.postCtx(ctx, "id", "", nil)
	if err != nil {
		return nil, err
	}

	var res ipfsIDResp
	err = json.Unmarshal(body, &res)
	if err != nil {
		return nil, err
	}

	pID, err := peer.IDB58Decode(res.ID)
	if err != nil {
		return nil, err
	}

	id := &api.IPFSID{
		ID: pID,
	}

	mAddrs := make([]api.Multiaddr, len(res.Addresses), len(res.Addresses))
	for i, strAddr := range res.Addresses {
		mAddr, err := api.NewMultiaddr(strAddr)
		if err != nil {
			id.Error = err.Error()
			return id, err
		}
		mAddrs[i] = mAddr
	}
	id.Addresses = mAddrs
	return id, nil
}

func pinArgs(maxDepth int) string {
	q := url.Values{}
	switch {
	case maxDepth < 0:
		q.Set("recursive", "true")
	case maxDepth == 0:
		q.Set("recursive", "false")
	default:
		q.Set("recursive", "true")
		q.Set("max-depth", strconv.Itoa(maxDepth))
	}
	return q.Encode()
}

// Pin performs a pin request against the configured IPFS
// daemon.
func (ipfs *Connector) Pin(ctx context.Context, hash cid.Cid, maxDepth int) error {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/Pin")
	defer span.End()

	pinStatus, err := ipfs.PinLsCid(ctx, hash)
	if err != nil {
		return err
	}

	if pinStatus.IsPinned(maxDepth) {
		logger.Debug("IPFS object is already pinned: ", hash)
		return nil
	}

	defer ipfs.updateInformerMetric(ctx)

	ctx, cancelRequest := context.WithCancel(ctx)
	defer cancelRequest()

	switch ipfs.config.PinMethod {
	case "refs":
		// do refs -r first and timeout if we don't get at least
		// one ref per pin timeout
		outRefs := make(chan string)
		go func() {
			lastRefTime := time.Now()
			ticker := time.NewTicker(progressTick)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if time.Since(lastRefTime) >= ipfs.config.PinTimeout {
						cancelRequest() // timeout
						return
					}
				case <-outRefs:
					lastRefTime = time.Now()
				case <-ctx.Done():
					return
				}
			}
		}()

		err := ipfs.refsProgress(ctx, hash, maxDepth, outRefs)
		if err != nil {
			return err
		}

		logger.Debugf("Refs for %s sucessfully fetched", hash)

	}

	// Pin request and timeout if there is no progress
	outPins := make(chan int)
	go func() {
		var lastProgress int
		lastProgressTime := time.Now()

		ticker := time.NewTicker(ipfs.config.PinTimeout)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if time.Since(lastProgressTime) > ipfs.config.PinTimeout {
					// timeout request
					cancelRequest()
					return
				}
			case p := <-outPins:
				// ipfs will send status messages every second
				// or so but we need make sure there was
				// progress by looking at number of nodes
				// fetched.
				if p > lastProgress {
					lastProgress = p
					lastProgressTime = time.Now()
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	err = ipfs.pinProgress(ctx, hash, maxDepth, outPins)
	if err != nil {
		return err
	}

	logger.Info("IPFS Pin request succeeded: ", hash)
	stats.Record(ctx, observations.Pins.M(1))
	return nil
}

// refsProgress fetches refs and puts them on a channel. Blocks until done or
// error. refsProgress will always close the out channel. refsProgres will
// not block on sending to the channel if it is full.
func (ipfs *Connector) refsProgress(ctx context.Context, hash cid.Cid, maxDepth int, out chan<- string) error {
	defer close(out)

	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/refsProgress")
	defer span.End()

	path := fmt.Sprintf("refs?arg=%s&%s", hash, pinArgs(maxDepth))
	res, err := ipfs.doPostCtx(ctx, ipfs.client, ipfs.apiURL(), path, "", nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	_, err = checkResponse(path, res)
	if err != nil {
		return err
	}

	dec := json.NewDecoder(res.Body)
	for {
		var ref ipfsRefsResp
		if err := dec.Decode(&ref); err != nil {
			// If we cancelled the request we should tell the user
			// (in case dec.Decode() exited cleanly with an EOF).
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				if err == io.EOF {
					return nil // clean exit
				}
				return err // error decoding
			}
		}

		// We have a Ref!
		if errStr := ref.Err; errStr != "" {
			logger.Error(errStr)
		}

		select { // do not lock
		case out <- ref.Ref:
		default:
		}
	}
}

// pinProgress pins an item and sends fetched node's progress on a
// channel. Blocks until done or error. pinProgress will always close the out
// channel.  pinProgress will not block on sending to the channel if it is full.
func (ipfs *Connector) pinProgress(ctx context.Context, hash cid.Cid, maxDepth int, out chan<- int) error {
	defer close(out)

	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/pinsProgress")
	defer span.End()

	pinArgs := pinArgs(maxDepth)
	path := fmt.Sprintf("pin/add?arg=%s&%s&progress=true", hash, pinArgs)
	res, err := ipfs.doPostCtx(ctx, ipfs.client, ipfs.apiURL(), path, "", nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	_, err = checkResponse(path, res)
	if err != nil {
		return err
	}

	dec := json.NewDecoder(res.Body)
	for {
		var pins ipfsPinsResp
		if err := dec.Decode(&pins); err != nil {
			// If we cancelled the request we should tell the user
			// (in case dec.Decode() exited cleanly with an EOF).
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				if err == io.EOF {
					return nil // clean exit. Pinned!
				}
				return err // error decoding
			}
		}

		select {
		case out <- pins.Progress:
		default:
		}
	}
}

// Unpin performs an unpin request against the configured IPFS
// daemon.
func (ipfs *Connector) Unpin(ctx context.Context, hash cid.Cid) error {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/Unpin")
	defer span.End()

	pinStatus, err := ipfs.PinLsCid(ctx, hash)
	if err != nil {
		return err
	}

	if pinStatus.IsPinned(-1) {
		defer ipfs.updateInformerMetric(ctx)
		path := fmt.Sprintf("pin/rm?arg=%s", hash)

		ctx, cancel := context.WithTimeout(ctx, ipfs.config.UnpinTimeout)
		defer cancel()

		_, err := ipfs.postCtx(ctx, path, "", nil)
		if err != nil {
			return err
		}
		logger.Info("IPFS Unpin request succeeded:", hash)
		stats.Record(ctx, observations.Pins.M(-1))
	}

	logger.Debug("IPFS object is already unpinned: ", hash)
	return nil
}

// PinLs performs a "pin ls --type typeFilter" request against the configured
// IPFS daemon and returns a map of cid strings and their status.
func (ipfs *Connector) PinLs(ctx context.Context, typeFilter string) (map[string]api.IPFSPinStatus, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/PinLs")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	body, err := ipfs.postCtx(ctx, "pin/ls?type="+typeFilter, "", nil)

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

// PinLsCid performs a "pin ls <hash>" request. It first tries with
// "type=recursive" and then, if not found, with "type=direct". It returns an
// api.IPFSPinStatus for that hash.
func (ipfs *Connector) PinLsCid(ctx context.Context, hash cid.Cid) (api.IPFSPinStatus, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/PinLsCid")
	defer span.End()

	pinLsType := func(pinType string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
		defer cancel()
		lsPath := fmt.Sprintf("pin/ls?arg=%s&type=%s", hash, pinType)
		return ipfs.postCtx(ctx, lsPath, "", nil)
	}

	var body []byte
	var err error
	// FIXME: Sharding may need to check more pin types here.
	for _, pinType := range []string{"recursive", "direct"} {
		body, err = pinLsType(pinType)
		// Network error, daemon down
		if body == nil && err != nil {
			return api.IPFSPinStatusError, err
		}

		// Pin found. Do not keep looking.
		if err == nil {
			break
		}
	}

	if err != nil { // we could not find the pin
		return api.IPFSPinStatusUnpinned, nil
	}

	var res ipfsPinLsResp
	err = json.Unmarshal(body, &res)
	if err != nil {
		logger.Error("error parsing pin/ls?arg=cid response:")
		logger.Error(string(body))
		return api.IPFSPinStatusError, err
	}

	// We do not know what string format the returned key has so
	// we parse as CID. There should only be one returned key.
	for k, pinObj := range res.Keys {
		c, err := cid.Decode(k)
		if err != nil || !c.Equals(hash) {
			continue
		}
		return api.IPFSPinStatusFromString(pinObj.Type), nil
	}
	return api.IPFSPinStatusError, errors.New("expected to find the pin in the response")
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
func checkResponse(path string, res *http.Response) ([]byte, error) {
	if res.StatusCode == http.StatusOK {
		return nil, nil
	}

	body, err := ioutil.ReadAll(res.Body)
	if err == nil {
		var ipfsErr ipfsError
		if err := json.Unmarshal(body, &ipfsErr); err == nil {
			return body, fmt.Errorf(
				"IPFS request unsuccessful (%s). Code: %d. Message: %s",
				path,
				res.StatusCode,
				ipfsErr.Message,
			)
		}
	}

	// No error response with useful message from ipfs
	return nil, fmt.Errorf(
		"IPFS request unsuccessful (%s). Code %d. Body: %s",
		path,
		res.StatusCode,
		string(body))
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

	errBody, err := checkResponse(path, res)
	if err != nil {
		return errBody, err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		logger.Errorf("error reading response body: %s", err)
		return nil, err
	}
	return body, nil
}

// postDiscardBodyCtx makes a POST requests but discards the body
// of the response directly after reading it.
func (ipfs *Connector) postDiscardBodyCtx(ctx context.Context, path string) error {
	res, err := ipfs.doPostCtx(ctx, ipfs.client, ipfs.apiURL(), path, "", nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	_, err = checkResponse(path, res)
	if err != nil {
		return err
	}

	_, err = io.Copy(ioutil.Discard, res.Body)
	return err
}

// apiURL is a short-hand for building the url of the IPFS
// daemon API.
func (ipfs *Connector) apiURL() string {
	return fmt.Sprintf("http://%s/api/v0", ipfs.nodeAddr)
}

// ConnectSwarms requests the ipfs addresses of other peers and
// triggers ipfs swarm connect requests
func (ipfs *Connector) ConnectSwarms(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/ConnectSwarms")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	var ids []*api.ID
	err := ipfs.rpcClient.CallContext(
		ctx,
		"",
		"Cluster",
		"Peers",
		struct{}{},
		&ids,
	)
	if err != nil {
		logger.Error(err)
		return err
	}

	for _, id := range ids {
		ipfsID := id.IPFS
		if ipfsID == nil || id.Error != "" || ipfsID.Error != "" {
			continue
		}
		for _, addr := range ipfsID.Addresses {
			// This is a best effort attempt
			// We ignore errors which happens
			// when passing in a bunch of addresses
			_, err := ipfs.postCtx(
				ctx,
				fmt.Sprintf("swarm/connect?arg=%s", addr.String()),
				"",
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
	res, err := ipfs.postCtx(ctx, "config/show", "", nil)
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

// RepoStat returns the DiskUsage and StorageMax repo/stat values from the
// ipfs daemon, in bytes, wrapped as an IPFSRepoStat object.
func (ipfs *Connector) RepoStat(ctx context.Context) (*api.IPFSRepoStat, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/RepoStat")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	res, err := ipfs.postCtx(ctx, "repo/stat?size-only=true", "", nil)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	var stats api.IPFSRepoStat
	err = json.Unmarshal(res, &stats)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	return &stats, nil
}

// RepoGC performs a garbage collection sweep on the cluster peer's IPFS repo.
func (ipfs *Connector) RepoGC(ctx context.Context) (*api.RepoGC, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/RepoGC")
	defer span.End()

	ctx1, cancel1 := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel1()

	id := &api.ID{}
	err := ipfs.rpcClient.CallContext(
		ctx1,
		"",
		"Cluster",
		"ID",
		struct{}{},
		id,
	)

	repoGC := api.RepoGC{
		Peer:     id.ID,
		Peername: id.Peername,
		Keys:     make([]api.IPFSRepoGC, 0),
	}

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()

	res, err := ipfs.doPostCtx(ctx, ipfs.client, ipfs.apiURL(), "repo/gc", "", nil)
	if err != nil {
		logger.Error(err)
		return &repoGC, err
	}
	defer res.Body.Close()

	dec := json.NewDecoder(res.Body)
	for {
		resp := ipfsRepoGCResp{}

		if err := dec.Decode(&resp); err != nil {
			// If we cancelled the request we should tell the user
			// (in case dec.Decode() exited cleanly with an EOF).
			select {
			case <-ctx.Done():
				return &repoGC, ctx.Err()
			default:
				if err == io.EOF {
					return &repoGC, nil // clean exit
				}
				return &repoGC, err // error decoding
			}
		}

		repoGC.Keys = append(repoGC.Keys, api.IPFSRepoGC{Key: resp.Key, Error: resp.Error})
	}
}

// Resolve accepts ipfs or ipns path and resolves it into a cid
func (ipfs *Connector) Resolve(ctx context.Context, path string) (cid.Cid, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/Resolve")
	defer span.End()

	validPath, err := gopath.ParsePath(path)
	if err != nil {
		logger.Error("could not parse path: " + err.Error())
		return cid.Undef, err
	}
	if !strings.HasPrefix(path, "/ipns") && validPath.IsJustAKey() {
		ci, _, err := gopath.SplitAbsPath(validPath)
		return ci, err
	}

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	res, err := ipfs.postCtx(ctx, "resolve?arg="+url.QueryEscape(path), "", nil)
	if err != nil {
		logger.Error(err)
		return cid.Undef, err
	}

	var resp ipfsResolveResp
	err = json.Unmarshal(res, &resp)
	if err != nil {
		logger.Error("could not unmarshal response: " + err.Error())
		return cid.Undef, err
	}

	ci, _, err := gopath.SplitAbsPath(gopath.FromString(resp.Path))
	return ci, err
}

// SwarmPeers returns the peers currently connected to this ipfs daemon.
func (ipfs *Connector) SwarmPeers(ctx context.Context) ([]peer.ID, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/SwarmPeers")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()

	res, err := ipfs.postCtx(ctx, "swarm/peers", "", nil)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	var peersRaw ipfsSwarmPeersResp
	err = json.Unmarshal(res, &peersRaw)
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	swarm := make([]peer.ID, len(peersRaw.Peers))
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
func (ipfs *Connector) BlockPut(ctx context.Context, b *api.NodeWithMeta) error {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/BlockPut")
	defer span.End()

	logger.Debugf("putting block to IPFS: %s", b.Cid)
	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	defer ipfs.updateInformerMetric(ctx)

	mapDir := files.NewMapDirectory(
		map[string]files.Node{ // IPFS reqs require a wrapping directory
			"": files.NewBytesFile(b.Data),
		},
	)

	multiFileR := files.NewMultiFileReader(mapDir, true)
	if b.Format == "" {
		b.Format = "v0"
	}
	url := "block/put?f=" + b.Format
	contentType := "multipart/form-data; boundary=" + multiFileR.Boundary()

	_, err := ipfs.postCtx(ctx, url, contentType, multiFileR)
	return err
}

// BlockGet retrieves an ipfs block with the given cid
func (ipfs *Connector) BlockGet(ctx context.Context, c cid.Cid) ([]byte, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/BlockGet")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	url := "block/get?arg=" + c.String()
	return ipfs.postCtx(ctx, url, "", nil)
}

// // FetchRefs asks IPFS to download blocks recursively to the given depth.
// // It discards the response, but waits until it completes.
// func (ipfs *Connector) FetchRefs(ctx context.Context, c cid.Cid, maxDepth int) error {
// 	ctx, cancel := context.WithTimeout(ipfs.ctx, ipfs.config.PinTimeout)
// 	defer cancel()

// 	q := url.Values{}
// 	q.Set("recursive", "true")
// 	q.Set("unique", "false") // same memory on IPFS side
// 	q.Set("max-depth", fmt.Sprintf("%d", maxDepth))
// 	q.Set("arg", c.String())

// 	url := fmt.Sprintf("refs?%s", q.Encode())
// 	err := ipfs.postDiscardBodyCtx(ctx, url)
// 	if err != nil {
// 		return err
// 	}
// 	logger.Debugf("refs for %s sucessfully fetched", c)
// 	return nil
// }

// Returns true every updateMetricsMod-th time that we
// call this function.
func (ipfs *Connector) shouldUpdateMetric() bool {
	ipfs.updateMetricMutex.Lock()
	defer ipfs.updateMetricMutex.Unlock()
	ipfs.updateMetricCount++
	if ipfs.updateMetricCount%updateMetricMod == 0 {
		ipfs.updateMetricCount = 0
		return true
	}
	return false
}

// Trigger a broadcast of the local informer metrics.
func (ipfs *Connector) updateInformerMetric(ctx context.Context) error {
	_, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/updateInformerMetric")
	defer span.End()
	ctx = trace.NewContext(ipfs.ctx, span)

	if !ipfs.shouldUpdateMetric() {
		return nil
	}

	var metric api.Metric

	err := ipfs.rpcClient.GoContext(
		ctx,
		"",
		"Cluster",
		"SendInformerMetric",
		struct{}{},
		&metric,
		nil,
	)
	if err != nil {
		logger.Error(err)
	}
	return err
}
