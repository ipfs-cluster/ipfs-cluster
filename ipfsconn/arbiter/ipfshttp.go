// Package arbiter implements an IPFS Cluster IPFSConnector component
// with noop Pin and Unpin methods so to provide a pure consensus node. It
// uses the IPFS HTTP API to communicate to IPFS.
package arbiter

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	madns "github.com/multiformats/go-multiaddr-dns"
	manet "github.com/multiformats/go-multiaddr/net"

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/plugin/ochttp/propagation/tracecontext"
	"go.opencensus.io/trace"
)

// DNSTimeout is used when resolving DNS multiaddresses in this module
var DNSTimeout = 5 * time.Second

var logger = logging.Logger("arbiter")

// Connector implements the IPFSConnector interface
// and provides a component which  is used to perform
// on-demand requests against the configured IPFS daemom
// (such as a pin request).
type Connector struct {
	ctx    context.Context
	cancel func()

	config *ipfshttp.Config
	*ipfshttp.Connector

	nodeAddr string

	rpcClient *rpc.Client
	rpcReady  chan struct{}

	client *http.Client // client to ipfs daemon

	shutdownLock sync.Mutex
	shutdown     bool
	wg           sync.WaitGroup
}

// NewConnector creates the component and leaves it ready to be started
func NewConnector(cfg *ipfshttp.Config) (*Connector, error) {
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

	conn, err := ipfshttp.NewConnector(cfg)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	ipfs := &Connector{
		ctx:       ctx,
		cancel:    cancel,
		config:    cfg,
		Connector: conn,

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
	_, span := trace.StartSpan(ctx, "ipfsconn/arbiter/Shutdown")
	defer span.End()

	ipfs.shutdownLock.Lock()
	defer ipfs.shutdownLock.Unlock()

	if ipfs.shutdown {
		logger.Debug("already shutdown")
		return nil
	}

	logger.Info("stopping IPFS Connector")
	if err := ipfs.Connector.Shutdown(ctx); err != nil {
		logger.Error("error shutting down ipfshttp", "err", err)
	}

	ipfs.cancel()
	close(ipfs.rpcReady)

	ipfs.wg.Wait()
	ipfs.shutdown = true

	return nil
}

// Pin performs a pin request against the configured IPFS
// daemon.
func (ipfs *Connector) Pin(ctx context.Context, pin *api.Pin) error {
	_, span := trace.StartSpan(ctx, "ipfsconn/arbiter/Pin")
	defer span.End()

	logger.Info("noop pin")
	return nil
}

// Unpin performs an unpin request against the configured IPFS
// daemon.
func (ipfs *Connector) Unpin(ctx context.Context, hash cid.Cid) error {
	_, span := trace.StartSpan(ctx, "ipfsconn/arbiter/Unpin")
	defer span.End()

	logger.Info("noop unpin")
	return nil
}

// RepoStat returns the DiskUsage and StorageMax repo/stat values from the
// ipfs daemon, in bytes, wrapped as an IPFSRepoStat object. Arbiter returns
// 0 for both values so that it is never chosen for pinning.
func (ipfs *Connector) RepoStat(ctx context.Context) (*api.IPFSRepoStat, error) {
	_, span := trace.StartSpan(ctx, "ipfsconn/arbiter/RepoStat")
	defer span.End()

	logger.Info("noop repoStat")
	return &api.IPFSRepoStat{RepoSize: 0}, nil
}
