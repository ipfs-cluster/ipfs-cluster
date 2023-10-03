// Package ipfshttp implements an IPFS Cluster IPFSConnector component. It
// uses the IPFS HTTP API to communicate to IPFS.
package ipfshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/observations"

	files "github.com/ipfs/boxo/files"
	gopath "github.com/ipfs/boxo/path"
	ipfspinner "github.com/ipfs/boxo/pinning/pinner"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p/core/peer"
	madns "github.com/multiformats/go-multiaddr-dns"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/multiformats/go-multicodec"
	multihash "github.com/multiformats/go-multihash"

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/plugin/ochttp/propagation/tracecontext"
	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
)

// DNSTimeout is used when resolving DNS multiaddresses in this module
var DNSTimeout = 5 * time.Second

var logger = logging.Logger("ipfshttp")

// Connector implements the IPFSConnector interface
// and provides a component which  is used to perform
// on-demand requests against the configured IPFS daemom
// (such as a pin request).
type Connector struct {
	// struct alignment! These fields must be up-front.
	updateMetricCount uint64
	ipfsPinCount      int64

	ctx    context.Context
	cancel func()
	ready  chan struct{}

	config   *Config
	nodeAddr string

	rpcClient *rpc.Client
	rpcReady  chan struct{}

	client *http.Client // client to ipfs daemon

	failedRequests atomic.Uint64 // count failed requests.
	reqRateLimitCh chan struct{}

	shutdownLock sync.Mutex
	shutdown     bool
	wg           sync.WaitGroup
}

type ipfsError struct {
	path    string
	code    int
	Message string
}

func (ie ipfsError) Error() string {
	return fmt.Sprintf(
		"IPFS error (%s). Code: %d. Message: %s",
		ie.path,
		ie.code,
		ie.Message,
	)
}

type ipfsUnpinnedError ipfsError

func (unpinned ipfsUnpinnedError) Is(target error) bool {
	ierr, ok := target.(ipfsError)
	if !ok {
		return false
	}
	return strings.HasSuffix(ierr.Message, "not pinned")
}

func (unpinned ipfsUnpinnedError) Error() string {
	return ipfsError(unpinned).Error()
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
	Error string
}

type ipfsPinsResp struct {
	Pins     []string
	Progress int
}

type ipfsSwarmPeersResp struct {
	Peers []ipfsPeer
}

type ipfsBlockPutResp struct {
	Key  api.Cid
	Size int
}

type ipfsPeer struct {
	Peer string
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
		ctx:            ctx,
		cancel:         cancel,
		ready:          make(chan struct{}),
		config:         cfg,
		nodeAddr:       nodeAddr,
		rpcReady:       make(chan struct{}, 1),
		reqRateLimitCh: make(chan struct{}),
		client:         c,
	}

	initializeMetrics(ctx)

	go ipfs.rateLimiter()

	go ipfs.run()
	return ipfs, nil
}

func initializeMetrics(ctx context.Context) {
	// initialize metrics
	stats.Record(ctx, observations.PinsIpfsPins.M(0))
	stats.Record(ctx, observations.PinsPinAdd.M(0))
	stats.Record(ctx, observations.PinsPinAddError.M(0))
	stats.Record(ctx, observations.BlocksPut.M(0))
	stats.Record(ctx, observations.BlocksAddedSize.M(0))
	stats.Record(ctx, observations.BlocksAdded.M(0))
	stats.Record(ctx, observations.BlocksAddedError.M(0))
}

// rateLimiter issues ticks in the reqRateLimitCh that allow requests to
// proceed. See doPostCtx.
func (ipfs *Connector) rateLimiter() {
	isRateLimiting := false

	// TODO: The rate-limiter is configured to start rate-limiting after
	// 10 failed requests at a rate of 1 req/s. This should probably be
	// configurable.
	for {
		failed := ipfs.failedRequests.Load()
		switch {
		case failed == 0:
			if isRateLimiting {
				// This does not print always,
				// only when there were several requests
				// waiting to read.
				logger.Warning("Lifting up rate limit")
			}
			isRateLimiting = false
		case failed > 0 && failed <= 10:
			isRateLimiting = false
		case failed > 10:
			if !isRateLimiting {
				logger.Warning("Rate-limiting requests to 1req/s")
			}
			isRateLimiting = true
			time.Sleep(time.Second)
		}

		// Send tick
		select {
		case <-ipfs.ctx.Done():
			close(ipfs.reqRateLimitCh)
			return
		case ipfs.reqRateLimitCh <- struct{}{}:
			// note that the channel is unbuffered,
			// therefore we will sit here until a method
			// wants to read from us, and they don't if
			// failed == 0.
		}
	}
}

// connects all ipfs daemons when
// we receive the rpcReady signal.
func (ipfs *Connector) run() {
	<-ipfs.rpcReady

	// wait for IPFS to be available
	i := 0
	for {
		select {
		case <-ipfs.ctx.Done():
			return
		default:
		}
		i++
		_, err := ipfs.ID(ipfs.ctx)
		if err == nil {
			close(ipfs.ready)
			break
		}
		if i%10 == 0 {
			logger.Warningf("ipfs does not seem to be available after %d retries", i)
		}

		// Requests will be rate-limited when going faster.
		time.Sleep(time.Second)
	}

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
	_, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/Shutdown")
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

// Ready returns a channel which gets notified when a testing request to the
// IPFS daemon first succeeds.
func (ipfs *Connector) Ready(ctx context.Context) <-chan struct{} {
	return ipfs.ready
}

// ID performs an ID request against the configured
// IPFS daemon. It returns the fetched information.
// If the request fails, or the parsing fails, it
// returns an error.
func (ipfs *Connector) ID(ctx context.Context) (api.IPFSID, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/ID")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()

	body, err := ipfs.postCtx(ctx, "id", "", nil)
	if err != nil {
		return api.IPFSID{}, err
	}

	var res ipfsIDResp
	err = json.Unmarshal(body, &res)
	if err != nil {
		return api.IPFSID{}, err
	}

	pID, err := peer.Decode(res.ID)
	if err != nil {
		return api.IPFSID{}, err
	}

	id := api.IPFSID{
		ID: pID,
	}

	mAddrs := make([]api.Multiaddr, len(res.Addresses))
	for i, strAddr := range res.Addresses {
		mAddr, err := api.NewMultiaddr(strAddr)
		if err != nil {
			logger.Warningf("cannot parse IPFS multiaddress: %s (%w)... ignoring", strAddr, err)
			continue
		}
		mAddrs[i] = mAddr
	}
	id.Addresses = mAddrs
	return id, nil
}

func pinArgs(maxDepth api.PinDepth) string {
	q := url.Values{}
	switch {
	case maxDepth < 0:
		q.Set("recursive", "true")
	case maxDepth == 0:
		q.Set("recursive", "false")
	default:
		q.Set("recursive", "true")
		q.Set("max-depth", strconv.Itoa(int(maxDepth)))
	}
	return q.Encode()
}

// Pin performs a pin request against the configured IPFS
// daemon.
func (ipfs *Connector) Pin(ctx context.Context, pin api.Pin) error {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/Pin")
	defer span.End()

	hash := pin.Cid
	maxDepth := pin.MaxDepth

	pinStatus, err := ipfs.PinLsCid(ctx, pin)
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

	// If the pin has origins, tell ipfs to connect to a maximum of 10.
	bound := len(pin.Origins)
	if bound > 10 {
		bound = 10
	}
	for _, orig := range pin.Origins[0:bound] {
		// do it in the background, ignoring errors.
		go func(o string) {
			logger.Debugf("swarm-connect to origin before pinning: %s", o)
			_, err := ipfs.postCtx(
				ctx,
				fmt.Sprintf("swarm/connect?arg=%s", o),
				"",
				nil,
			)
			if err != nil {
				logger.Debug(err)
				return
			}
			logger.Debugf("swarm-connect success to origin: %s", o)
		}(url.QueryEscape(orig.String()))
	}

	// If we have a pin-update, and the old object
	// is pinned recursively, then do pin/update.
	// Otherwise do a normal pin.
	if from := pin.PinUpdate; from.Defined() {
		fromPin := api.PinWithOpts(from, pin.PinOptions)
		pinStatus, _ := ipfs.PinLsCid(ctx, fromPin)
		if pinStatus.IsPinned(-1) { // pinned recursively.
			// As a side note, if PinUpdate == pin.Cid, we are
			// somehow pinning an already pinned thing and we'd
			// better use update for that
			return ipfs.pinUpdate(ctx, from, pin.Cid)
		}
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

	stats.Record(ipfs.ctx, observations.PinsPinAdd.M(1))
	err = ipfs.pinProgress(ctx, hash, maxDepth, outPins)
	if err != nil {
		stats.Record(ipfs.ctx, observations.PinsPinAddError.M(1))
		return err
	}
	totalPins := atomic.AddInt64(&ipfs.ipfsPinCount, 1)
	stats.Record(ipfs.ctx, observations.PinsIpfsPins.M(totalPins))

	logger.Info("IPFS Pin request succeeded: ", hash)
	return nil
}

// pinProgress pins an item and sends fetched node's progress on a
// channel. Blocks until done or error. pinProgress will always close the out
// channel.  pinProgress will not block on sending to the channel if it is full.
func (ipfs *Connector) pinProgress(ctx context.Context, hash api.Cid, maxDepth api.PinDepth, out chan<- int) error {
	defer close(out)

	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/pinsProgress")
	defer span.End()

	pinArgs := pinArgs(maxDepth)
	path := fmt.Sprintf("pin/add?arg=%s&%s&progress=true", hash, pinArgs)

	body, err := ipfs.postCtxStreamResponse(ctx, path, "", nil)
	if err != nil {
		return err
	}
	defer body.Close()

	dec := json.NewDecoder(body)
	for {
		var pins ipfsPinsResp
		if err := dec.Decode(&pins); err != nil {
			// If we canceled the request we should tell the user
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

func (ipfs *Connector) pinUpdate(ctx context.Context, from, to api.Cid) error {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/pinUpdate")
	defer span.End()

	path := fmt.Sprintf("pin/update?arg=%s&arg=%s&unpin=false", from, to)
	_, err := ipfs.postCtx(ctx, path, "", nil)
	if err != nil {
		return err
	}
	totalPins := atomic.AddInt64(&ipfs.ipfsPinCount, 1)
	stats.Record(ipfs.ctx, observations.PinsIpfsPins.M(totalPins))
	logger.Infof("IPFS Pin Update request succeeded. %s -> %s (unpin=false)", from, to)
	return nil
}

// Unpin performs an unpin request against the configured IPFS
// daemon.
func (ipfs *Connector) Unpin(ctx context.Context, hash api.Cid) error {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/Unpin")
	defer span.End()

	if ipfs.config.UnpinDisable {
		return errors.New("ipfs unpinning is disallowed by configuration on this peer")
	}

	defer ipfs.updateInformerMetric(ctx)

	path := fmt.Sprintf("pin/rm?arg=%s", hash)

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.UnpinTimeout)
	defer cancel()

	// We will call unpin in any case, if the CID is not pinned,
	// then we ignore the error (although this is a bit flaky).
	_, err := ipfs.postCtx(ctx, path, "", nil)
	if err != nil {
		ipfsErr, ok := err.(ipfsError)
		if !ok || ipfsErr.Message != ipfspinner.ErrNotPinned.Error() {
			return err
		}
		logger.Debug("IPFS object is already unpinned: ", hash)
		return nil
	}

	totalPins := atomic.AddInt64(&ipfs.ipfsPinCount, -1)
	stats.Record(ipfs.ctx, observations.PinsIpfsPins.M(totalPins))

	logger.Info("IPFS Unpin request succeeded:", hash)
	return nil
}

// PinLs performs a "pin ls --type typeFilter" request against the configured
// IPFS daemon and sends the results on the given channel. Returns when done.
func (ipfs *Connector) PinLs(ctx context.Context, typeFilters []string, out chan<- api.IPFSPinInfo) error {
	defer close(out)
	bodies := make([]io.ReadCloser, len(typeFilters))

	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/PinLs")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()

	var err error
	var totalPinCount int64
	defer func() {
		if err != nil {
			atomic.StoreInt64(&ipfs.ipfsPinCount, totalPinCount)
			stats.Record(ipfs.ctx, observations.PinsIpfsPins.M(totalPinCount))
		}
	}()

nextFilter:
	for i, typeFilter := range typeFilters {
		// Post and read streaming response
		path := "pin/ls?stream=true&type=" + typeFilter
		bodies[i], err = ipfs.postCtxStreamResponse(ctx, path, "", nil)
		if err != nil {
			return err
		}
		defer bodies[i].Close()

		dec := json.NewDecoder(bodies[i])

		for {
			select {
			case <-ctx.Done():
				err = fmt.Errorf("aborting pin/ls operation: %w", ctx.Err())
				logger.Error(err)
				return err
			default:
			}

			var ipfsPin api.IPFSPinInfo
			err = dec.Decode(&ipfsPin)
			if err == io.EOF {
				break nextFilter
			}
			if err != nil {
				err = fmt.Errorf("error decoding ipfs pin: %w", err)
				return err
			}

			select {
			case <-ctx.Done():
				err = fmt.Errorf("aborting pin/ls operation: %w", ctx.Err())
				logger.Error(err)
				return err
			case out <- ipfsPin:
				totalPinCount++
			}
		}
	}

	return nil
}

// PinLsCid performs a "pin ls <hash>" request. It will use "type=recursive" or
// "type=direct" (or other) depending on the given pin's MaxDepth setting.
// It returns an api.IPFSPinStatus for that hash.
func (ipfs *Connector) PinLsCid(ctx context.Context, pin api.Pin) (api.IPFSPinStatus, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/PinLsCid")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()

	if !pin.Defined() {
		return api.IPFSPinStatusBug, errors.New("calling PinLsCid without a defined CID")
	}

	pinType := pin.MaxDepth.ToPinMode().String()
	lsPath := fmt.Sprintf("pin/ls?stream=true&arg=%s&type=%s", pin.Cid, pinType)
	body, err := ipfs.postCtxStreamResponse(ctx, lsPath, "", nil)
	if err != nil {
		if errors.Is(ipfsUnpinnedError{}, err) {
			return api.IPFSPinStatusUnpinned, nil
		}
		return api.IPFSPinStatusError, err
	}
	defer body.Close()

	var res api.IPFSPinInfo
	dec := json.NewDecoder(body)

	err = dec.Decode(&res)
	if err != nil {
		logger.Error("error parsing pin/ls?arg=cid response")
		return api.IPFSPinStatusError, err
	}

	return res.Type, nil
}

// ConnectSwarms requests the ipfs addresses of other peers and
// triggers ipfs swarm connect requests
func (ipfs *Connector) ConnectSwarms(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/ConnectSwarms")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()

	in := make(chan struct{})
	close(in)
	out := make(chan api.ID)
	go func() {
		err := ipfs.rpcClient.Stream(
			ctx,
			"",
			"Cluster",
			"Peers",
			in,
			out,
		)
		if err != nil {
			logger.Error(err)
		}
	}()

	for id := range out {
		ipfsID := id.IPFS
		if id.Error != "" || ipfsID.Error != "" {
			continue
		}
		for _, addr := range ipfsID.Addresses {
			// This is a best effort attempt
			// We ignore errors which happens
			// when passing in a bunch of addresses
			_, err := ipfs.postCtx(
				ctx,
				fmt.Sprintf("swarm/connect?arg=%s", url.QueryEscape(addr.String())),
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

	switch v := value.(type) {
	case map[string]interface{}:
		return getConfigValue(path[1:], v)
	default:
		return nil, errors.New("invalid path")
	}
}

// RepoStat returns the DiskUsage and StorageMax repo/stat values from the
// ipfs daemon, in bytes, wrapped as an IPFSRepoStat object.
func (ipfs *Connector) RepoStat(ctx context.Context) (api.IPFSRepoStat, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/RepoStat")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	res, err := ipfs.postCtx(ctx, "repo/stat?size-only=true", "", nil)
	if err != nil {
		return api.IPFSRepoStat{}, err
	}

	var stats api.IPFSRepoStat
	err = json.Unmarshal(res, &stats)
	if err != nil {
		logger.Error(err)
		return api.IPFSRepoStat{}, err
	}
	return stats, nil
}

// RepoGC performs a garbage collection sweep on the cluster peer's IPFS repo.
func (ipfs *Connector) RepoGC(ctx context.Context) (api.RepoGC, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/RepoGC")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.RepoGCTimeout)
	defer cancel()

	body, err := ipfs.postCtxStreamResponse(ctx, "repo/gc?stream-errors=true", "", nil)
	if err != nil {
		return api.RepoGC{}, err
	}

	defer body.Close()

	dec := json.NewDecoder(body)
	repoGC := api.RepoGC{
		Keys: []api.IPFSRepoGC{},
	}
	for {
		resp := ipfsRepoGCResp{}

		if err := dec.Decode(&resp); err != nil {
			// If we canceled the request we should tell the user
			// (in case dec.Decode() exited cleanly with an EOF).
			select {
			case <-ctx.Done():
				return repoGC, ctx.Err()
			default:
				if err == io.EOF {
					return repoGC, nil // clean exit
				}
				logger.Error(err)
				return repoGC, err // error decoding
			}
		}

		repoGC.Keys = append(repoGC.Keys, api.IPFSRepoGC{Key: api.NewCid(resp.Key), Error: resp.Error})
	}
}

// Resolve accepts ipfs or ipns path and resolves it into a cid
func (ipfs *Connector) Resolve(ctx context.Context, path string) (api.Cid, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/Resolve")
	defer span.End()

	validPath, err := gopath.ParsePath(path)
	if err != nil {
		logger.Error("could not parse path: " + err.Error())
		return api.CidUndef, err
	}
	if !strings.HasPrefix(path, "/ipns") && validPath.IsJustAKey() {
		ci, _, err := gopath.SplitAbsPath(validPath)
		return api.NewCid(ci), err
	}

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	res, err := ipfs.postCtx(ctx, "resolve?arg="+url.QueryEscape(path), "", nil)
	if err != nil {
		return api.CidUndef, err
	}

	var resp ipfsResolveResp
	err = json.Unmarshal(res, &resp)
	if err != nil {
		logger.Error("could not unmarshal response: " + err.Error())
		return api.CidUndef, err
	}

	ci, _, err := gopath.SplitAbsPath(gopath.FromString(resp.Path))
	return api.NewCid(ci), err
}

// SwarmPeers returns the peers currently connected to this ipfs daemon.
func (ipfs *Connector) SwarmPeers(ctx context.Context) ([]peer.ID, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/SwarmPeers")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()

	res, err := ipfs.postCtx(ctx, "swarm/peers", "", nil)
	if err != nil {
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
		pID, err := peer.Decode(p.Peer)
		if err != nil {
			logger.Error(err)
			return swarm, err
		}
		swarm[i] = pID
	}
	return swarm, nil
}

// chanDirectory implements the files.Directory interface
type chanDirectory struct {
	iterator files.DirIterator
}

// Close is a no-op and it is not used.
func (cd *chanDirectory) Close() error {
	return nil
}

// not implemented, I think not needed for multipart.
func (cd *chanDirectory) Size() (int64, error) {
	return 0, nil
}

func (cd *chanDirectory) Entries() files.DirIterator {
	return cd.iterator
}

// chanIterator implements the files.DirIterator interface.
type chanIterator struct {
	ctx    context.Context
	blocks <-chan api.NodeWithMeta

	current api.NodeWithMeta
	peeked  api.NodeWithMeta
	done    bool
	err     error

	seenMu sync.Mutex
	seen   map[string]int
}

func (ci *chanIterator) Name() string {
	if !ci.current.Cid.Defined() {
		return ""
	}
	return ci.current.Cid.String()
}

// return NewBytesFile.
// This function might and is actually called multiple times for the same node
// by the multifile Reader to send the multipart.
func (ci *chanIterator) Node() files.Node {
	if !ci.current.Cid.Defined() {
		return nil
	}
	logger.Debugf("it.Node(): %s", ci.current.Cid)
	return files.NewBytesFile(ci.current.Data)
}

// Seen returns whether we have seen a multihash. It keeps count so it will
// return true as many times as we have seen it.
func (ci *chanIterator) Seen(c api.Cid) bool {
	ci.seenMu.Lock()
	n, ok := ci.seen[string(c.Cid.Hash())]
	logger.Debugf("Seen(): %s, %d, %t", c, n, ok)
	if ok {
		if n == 1 {
			delete(ci.seen, string(c.Cid.Hash()))
		} else {
			ci.seen[string(c.Cid.Hash())] = n - 1
		}
	}
	ci.seenMu.Unlock()
	return ok
}

func (ci *chanIterator) Done() bool {
	return ci.done
}

// Peek reads one block from the channel but saves it so that Next also
// returns it.
func (ci *chanIterator) Peek() (api.NodeWithMeta, bool) {
	if ci.done {
		return api.NodeWithMeta{}, false
	}

	select {
	case <-ci.ctx.Done():
		return api.NodeWithMeta{}, false
	case next, ok := <-ci.blocks:
		if !ok {
			return api.NodeWithMeta{}, false
		}
		ci.peeked = next
		return next, true
	}
}

func (ci *chanIterator) Next() bool {
	if ci.done {
		return false
	}

	seeBlock := func(b api.NodeWithMeta) {
		ci.seenMu.Lock()
		ci.seen[string(b.Cid.Hash())]++
		ci.seenMu.Unlock()
		stats.Record(ci.ctx, observations.BlocksAdded.M(1))
		stats.Record(ci.ctx, observations.BlocksAddedSize.M(int64(len(b.Data))))

	}

	if ci.peeked.Cid.Defined() {
		ci.current = ci.peeked
		ci.peeked = api.NodeWithMeta{}
		seeBlock(ci.current)
		return true
	}
	select {
	case <-ci.ctx.Done():
		ci.done = true
		ci.err = ci.ctx.Err()
		return false
	case next, ok := <-ci.blocks:
		if !ok {
			ci.done = true
			return false
		}
		// Record that we have seen this block. This has to be done
		// here, not in Node() as Node() is called multiple times per
		// block received.
		logger.Debugf("it.Next() %s", next.Cid)
		ci.current = next
		seeBlock(ci.current)
		return true
	}
}

func (ci *chanIterator) Err() error {
	return ci.err
}

func blockPutQuery(prefix cid.Prefix) (url.Values, error) {
	q := make(url.Values, 3)

	codec := multicodec.Code(prefix.Codec).String()
	if codec == "" {
		return q, fmt.Errorf("cannot find name for the blocks' CID codec: %x", prefix.Codec)
	}

	mhType, ok := multihash.Codes[prefix.MhType]
	if !ok {
		return q, fmt.Errorf("cannot find name for the blocks' Multihash type: %x", prefix.MhType)
	}

	// From go-ipfs 0.13.0 format is deprecated and we use cid-codec
	q.Set("cid-codec", codec)
	q.Set("mhtype", mhType)
	q.Set("mhlen", strconv.Itoa(prefix.MhLength))
	q.Set("pin", "false")
	q.Set("allow-big-block", "true")
	return q, nil
}

// BlockStream performs a multipart request to block/put with the blocks
// received on the channel.
func (ipfs *Connector) BlockStream(ctx context.Context, blocks <-chan api.NodeWithMeta) error {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/BlockStream")
	defer span.End()

	logger.Debug("streaming blocks to IPFS")
	defer ipfs.updateInformerMetric(ctx)

	it := &chanIterator{
		ctx:    ctx,
		blocks: blocks,
		seen:   make(map[string]int),
	}
	dir := &chanDirectory{
		iterator: it,
	}

	// We need to pick into the first block to know which Cid prefix we
	// are writing blocks with, so that ipfs calculates the expected
	// multihash (we select the function used). This means that all blocks
	// in a stream should use the same.
	peek, ok := it.Peek()
	if !ok {
		return errors.New("BlockStream: no blocks to peek in blocks channel")
	}

	q, err := blockPutQuery(peek.Cid.Prefix())
	if err != nil {
		return err
	}
	url := "block/put?" + q.Encode()

	// Now we stream the blocks to ipfs. In case of error, we return
	// directly, but leave a goroutine draining the channel until it is
	// closed, which should be soon after returning.
	stats.Record(ctx, observations.BlocksPut.M(1))
	multiFileR := files.NewMultiFileReader(dir, true, false)
	contentType := "multipart/form-data; boundary=" + multiFileR.Boundary()
	body, err := ipfs.postCtxStreamResponse(ctx, url, contentType, multiFileR)
	if err != nil {
		return err
	}
	defer body.Close()

	dec := json.NewDecoder(body)
	for {
		var res ipfsBlockPutResp
		err = dec.Decode(&res)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			logger.Error(err)
			break
		}
		logger.Debugf("response block: %s", res.Key)
		if !it.Seen(res.Key) {
			logger.Warningf("blockPut response CID (%s) does not match the multihash of any blocks sent", res.Key)
		}
	}

	// keep draining blocks channel until closed.
	go func() {
		for range blocks {
		}
	}()

	if err != nil {
		stats.Record(ipfs.ctx, observations.BlocksAddedError.M(1))
	}
	return err
}

// BlockGet retrieves an ipfs block with the given cid
func (ipfs *Connector) BlockGet(ctx context.Context, c api.Cid) ([]byte, error) {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/BlockGet")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ipfs.config.IPFSRequestTimeout)
	defer cancel()
	url := "block/get?arg=" + c.String()
	return ipfs.postCtx(ctx, url, "", nil)
}

// // FetchRefs asks IPFS to download blocks recursively to the given depth.
// // It discards the response, but waits until it completes.
// func (ipfs *Connector) FetchRefs(ctx context.Context, c api.Cid, maxDepth int) error {
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
// 	logger.Debugf("refs for %s successfully fetched", c)
// 	return nil
// }

// Returns true every updateMetricsMod-th time that we
// call this function.
func (ipfs *Connector) shouldUpdateMetric() bool {
	if ipfs.config.InformerTriggerInterval <= 0 {
		return false
	}
	curCount := atomic.AddUint64(&ipfs.updateMetricCount, 1)
	if curCount%uint64(ipfs.config.InformerTriggerInterval) == 0 {
		atomic.StoreUint64(&ipfs.updateMetricCount, 0)
		return true
	}
	return false
}

// Trigger a broadcast of the local informer metrics.
func (ipfs *Connector) updateInformerMetric(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "ipfsconn/ipfshttp/updateInformerMetric")
	defer span.End()

	if !ipfs.shouldUpdateMetric() {
		return nil
	}

	err := ipfs.rpcClient.GoContext(
		ctx,
		"",
		"Cluster",
		"SendInformersMetrics",
		struct{}{},
		&struct{}{},
		nil,
	)
	if err != nil {
		logger.Error(err)
	}
	return err
}

// daemon API.
func (ipfs *Connector) apiURL() string {
	return fmt.Sprintf("http://%s/api/v0", ipfs.nodeAddr)
}

func (ipfs *Connector) doPostCtx(ctx context.Context, client *http.Client, apiURL, path string, contentType string, postBody io.Reader) (*http.Response, error) {
	logger.Debugf("posting /%s", path)
	urlstr := fmt.Sprintf("%s/%s", apiURL, path)

	req, err := http.NewRequest("POST", urlstr, postBody)
	if err != nil {
		logger.Error("error creating POST request:", err)
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(ctx)

	// Rate limiter. If we have a number of failed requests,
	// then wait for a tick.
	if failed := ipfs.failedRequests.Load(); failed > 0 {
		select {
		case <-ipfs.reqRateLimitCh:
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ipfs.ctx.Done():
			return nil, ipfs.ctx.Err()
		}

	}

	res, err := ipfs.client.Do(req)
	if err != nil {
		// request error: ipfs was unreachable, record it.
		ipfs.failedRequests.Add(1)
		logger.Error("error posting to IPFS:", err)
	} else {
		ipfs.failedRequests.Store(0)
	}

	return res, err
}

// checkResponse tries to parse an error message on non StatusOK responses
// from ipfs.
func checkResponse(path string, res *http.Response) ([]byte, error) {
	if res.StatusCode == http.StatusOK {
		return nil, nil
	}

	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err == nil {
		var ipfsErr ipfsError
		if err := json.Unmarshal(body, &ipfsErr); err == nil {
			ipfsErr.code = res.StatusCode
			ipfsErr.path = path
			return body, ipfsErr
		}
	}

	// No error response with useful message from ipfs
	return nil, fmt.Errorf(
		"IPFS request failed (is it running?) (%s). Code %d: %s",
		path,
		res.StatusCode,
		string(body))
}

// postCtxStreamResponse makes a POST request against the ipfs daemon, and
// returns the body reader after checking the request for errros.
func (ipfs *Connector) postCtxStreamResponse(ctx context.Context, path string, contentType string, postBody io.Reader) (io.ReadCloser, error) {
	res, err := ipfs.doPostCtx(ctx, ipfs.client, ipfs.apiURL(), path, contentType, postBody)
	if err != nil {
		return nil, err
	}

	_, err = checkResponse(path, res)
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}

// postCtx makes a POST request against
// the ipfs daemon, reads the full body of the response and
// returns it after checking for errors.
func (ipfs *Connector) postCtx(ctx context.Context, path string, contentType string, postBody io.Reader) ([]byte, error) {
	rdr, err := ipfs.postCtxStreamResponse(ctx, path, contentType, postBody)
	if err != nil {
		return nil, err
	}
	defer rdr.Close()

	body, err := io.ReadAll(rdr)
	if err != nil {
		logger.Errorf("error reading response body: %s", err)
		return nil, err
	}
	return body, nil
}
