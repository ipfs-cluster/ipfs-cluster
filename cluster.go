package ipfscluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"sync"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/adder"
	"github.com/ipfs-cluster/ipfs-cluster/adder/sharding"
	"github.com/ipfs-cluster/ipfs-cluster/adder/single"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/pstoremgr"
	"github.com/ipfs-cluster/ipfs-cluster/rpcutil"
	"github.com/ipfs-cluster/ipfs-cluster/state"
	"github.com/ipfs-cluster/ipfs-cluster/version"
	"go.uber.org/multierr"

	ds "github.com/ipfs/go-datastore"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	dual "github.com/libp2p/go-libp2p-kad-dht/dual"
	host "github.com/libp2p/go-libp2p/core/host"
	metrics "github.com/libp2p/go-libp2p/core/metrics"
	peer "github.com/libp2p/go-libp2p/core/peer"
	peerstore "github.com/libp2p/go-libp2p/core/peerstore"
	mdns "github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	ma "github.com/multiformats/go-multiaddr"

	ocgorpc "github.com/lanzafame/go-libp2p-ocgorpc"
	trace "go.opencensus.io/trace"
)

// ReadyTimeout specifies the time before giving up
// during startup (waiting for consensus to be ready)
// It may need adjustment according to timeouts in the
// consensus layer.
var ReadyTimeout = 30 * time.Second

const (
	pingMetricName                = "ping"
	bootstrapCount                = 3
	reBootstrapInterval           = 30 * time.Second
	priorityPeerReconnectInterval = 5 * time.Minute
	mdnsServiceTag                = "_ipfs-cluster-discovery._udp"
	maxAlerts                     = 1000
)

var errFollowerMode = errors.New("this peer is configured to be in follower mode. Write operations are disabled")

// Cluster is the main IPFS cluster component. It provides
// the go-API for it and orchestrates the components that make up the system.
type Cluster struct {
	ctx    context.Context
	cancel func()

	id                peer.ID
	config            *Config
	host              host.Host
	bandwidthReporter metrics.Reporter
	dht               *dual.DHT
	discovery         mdns.Service
	datastore         ds.Datastore

	rpcServer   *rpc.Server
	rpcClient   *rpc.Client
	peerManager *pstoremgr.Manager

	consensus Consensus
	apis      []API
	ipfs      IPFSConnector
	tracker   PinTracker
	monitor   PeerMonitor
	allocator PinAllocator
	informers []Informer
	tracer    Tracer

	alerts    []api.Alert
	alertsMux sync.Mutex

	doneCh  chan struct{}
	readyCh chan struct{}
	readyB  bool
	wg      sync.WaitGroup

	// peerAdd
	paMux sync.Mutex

	// shutdown function and related variables
	shutdownLock sync.RWMutex
	shutdownB    bool
	removed      bool

	curPingVal pingValue
}

// NewCluster builds a new IPFS Cluster peer. It initializes a LibP2P host,
// creates and RPC Server and client and sets up all components.
//
// The new cluster peer may still be performing initialization tasks when
// this call returns (consensus may still be bootstrapping). Use Cluster.Ready()
// if you need to wait until the peer is fully up.
func NewCluster(
	ctx context.Context,
	host host.Host,
	bwc metrics.Reporter,
	dht *dual.DHT,
	cfg *Config,
	datastore ds.Datastore,
	consensus Consensus,
	apis []API,
	ipfs IPFSConnector,
	tracker PinTracker,
	monitor PeerMonitor,
	allocator PinAllocator,
	informers []Informer,
	tracer Tracer,
) (*Cluster, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	if host == nil {
		return nil, errors.New("cluster host is nil")
	}

	if len(informers) == 0 {
		return nil, errors.New("no informers are passed")
	}

	ctx, cancel := context.WithCancel(ctx)

	listenAddrs := ""
	for _, addr := range host.Addrs() {
		listenAddrs += fmt.Sprintf("        %s/p2p/%s\n", addr, host.ID())
	}

	logger.Infof("IPFS Cluster v%s listening on:\n%s\n", version.Version, listenAddrs)

	peerManager := pstoremgr.New(ctx, host, cfg.GetPeerstorePath())

	var mdnsSvc mdns.Service
	if cfg.MDNSInterval > 0 {
		mdnsSvc = mdns.NewMdnsService(host, mdnsServiceTag, peerManager)
		err = mdnsSvc.Start()
		if err != nil {
			logger.Warnf("mDNS could not be started: %s", err)
		}
	}

	c := &Cluster{
		ctx:               ctx,
		cancel:            cancel,
		id:                host.ID(),
		config:            cfg,
		host:              host,
		bandwidthReporter: bwc,
		dht:               dht,
		discovery:         mdnsSvc,
		datastore:         datastore,
		consensus:         consensus,
		apis:              apis,
		ipfs:              ipfs,
		tracker:           tracker,
		monitor:           monitor,
		allocator:         allocator,
		informers:         informers,
		tracer:            tracer,
		alerts:            []api.Alert{},
		peerManager:       peerManager,
		shutdownB:         false,
		removed:           false,
		doneCh:            make(chan struct{}),
		readyCh:           make(chan struct{}),
		readyB:            false,
	}

	// PeerAddresses are assumed to be permanent and have the maximum
	// priority for bootstrapping.
	c.peerManager.ImportPeersWithPriority(c.config.PeerAddresses, false, peerstore.PermanentAddrTTL, 0)
	// Peerstore addresses come afterwards and have increasing priorities
	// for bootstrapping and non permanent TTL (1h).
	c.peerManager.ImportPeersFromPeerstore(false, peerstore.AddressTTL)

	// Attempt to connect to some peers.
	connectedPeers := c.peerManager.Bootstrap(bootstrapCount, true, true)
	// We cannot warn when count is low as this as this is normal if going
	// to Join() later.
	logger.Debugf("Bootstrapped to %d peers successfully", len(connectedPeers))
	// Log a ping metric for every connected peer. This will make them
	// visible as peers without having to wait for them to send one.
	for _, p := range connectedPeers {
		if err := c.logPingMetric(ctx, p); err != nil {
			logger.Warn(err)
		}
	}

	// After setupRPC components can do their tasks with a fully operative
	// routed libp2p host with some connections and a working DHT (hopefully).
	err = c.setupRPC()
	if err != nil {
		c.Shutdown(ctx)
		return nil, err
	}
	c.setupRPCClients()

	// Note: It is very important to first call Add() once in a non-racy
	// place
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.ready(ReadyTimeout)
		c.run()
	}()

	return c, nil
}

func (c *Cluster) setupRPC() error {
	rpcServer, err := newRPCServer(c)
	if err != nil {
		return err
	}
	c.rpcServer = rpcServer

	var rpcClient *rpc.Client
	if c.config.Tracing {
		csh := &ocgorpc.ClientHandler{}
		rpcClient = rpc.NewClientWithServer(
			c.host,
			version.RPCProtocol,
			rpcServer,
			rpc.WithClientStatsHandler(csh),
		)
	} else {
		rpcClient = rpc.NewClientWithServer(c.host, version.RPCProtocol, rpcServer)
	}
	c.rpcClient = rpcClient
	return nil
}

func (c *Cluster) setupRPCClients() {
	c.ipfs.SetClient(c.rpcClient)
	c.tracker.SetClient(c.rpcClient)
	for _, api := range c.apis {
		api.SetClient(c.rpcClient)
	}
	c.consensus.SetClient(c.rpcClient)
	c.monitor.SetClient(c.rpcClient)
	c.allocator.SetClient(c.rpcClient)
	for _, informer := range c.informers {
		informer.SetClient(c.rpcClient)
	}
}

// watchPinset triggers recurrent operations that loop on the pinset.
func (c *Cluster) watchPinset() {
	ctx, span := trace.StartSpan(c.ctx, "cluster/watchPinset")
	defer span.End()

	stateSyncTimer := time.NewTimer(c.config.StateSyncInterval)

	// Upon start, every item in the state that is not pinned will appear
	// as PinError when doing a Status, we should proceed to recover
	// (try pinning) all of those right away.
	recoverTimer := time.NewTimer(0) // 0 so that it does an initial recover right away

	// This prevents doing an StateSync while doing a RecoverAllLocal,
	// which is intended behavior as for very large pinsets
	for {
		select {
		case <-stateSyncTimer.C:
			logger.Debug("auto-triggering StateSync()")
			c.StateSync(ctx)
			stateSyncTimer.Reset(c.config.StateSyncInterval)
		case <-recoverTimer.C:
			logger.Debug("auto-triggering RecoverAllLocal()")

			out := make(chan api.PinInfo, 1024)
			go func() {
				for range out {
				}
			}()
			err := c.RecoverAllLocal(ctx, out)
			if err != nil {
				logger.Error(err)
			}
			recoverTimer.Reset(c.config.PinRecoverInterval)
		case <-c.ctx.Done():
			if !stateSyncTimer.Stop() {
				<-stateSyncTimer.C
			}
			if !recoverTimer.Stop() {
				<-recoverTimer.C
			}
			return
		}
	}
}

// returns the smallest ttl from the metrics pushed by the informer.
func (c *Cluster) sendInformerMetrics(ctx context.Context, informer Informer) (time.Duration, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/sendInformerMetric")
	defer span.End()

	var minTTL time.Duration
	var errors error
	metrics := informer.GetMetrics(ctx)
	if len(metrics) == 0 {
		logger.Errorf("informer %s produced no metrics", informer.Name())
		return minTTL, nil
	}

	for _, metric := range metrics {
		if metric.Discard() { // do not publish invalid metrics
			// the tags informer creates an invalid metric
			// when no tags are defined.
			continue
		}
		metric.Peer = c.id
		ttl := metric.GetTTL()
		if ttl > 0 && (ttl < minTTL || minTTL == 0) {
			minTTL = ttl
		}
		err := c.monitor.PublishMetric(ctx, metric)

		if multierr.AppendInto(&errors, err) {
			logger.Warnf("error sending metric %s: %s", metric.Name, err)
		}
	}
	return minTTL, errors
}

func (c *Cluster) sendInformersMetrics(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "cluster/sendInformersMetrics")
	defer span.End()

	var errors error
	for _, informer := range c.informers {
		_, err := c.sendInformerMetrics(ctx, informer)
		if multierr.AppendInto(&errors, err) {
			logger.Warnf("informer %s did not send all metrics", informer.Name())
		}
	}
	return errors
}

// pushInformerMetrics loops and publishes informers metrics using the
// cluster monitor. Metrics are pushed normally at a TTL/2 rate. If an error
// occurs, they are pushed at a TTL/4 rate.
func (c *Cluster) pushInformerMetrics(ctx context.Context, informer Informer) {
	ctx, span := trace.StartSpan(ctx, "cluster/pushInformerMetrics")
	defer span.End()

	timer := time.NewTimer(0) // fire immediately first

	// retries counts how many retries we have made
	retries := 0
	// retryWarnMod controls how often do we log
	// "error broadcasting metric".
	// It will do it in the first error, and then on every
	// 10th.
	retryWarnMod := 10

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			// wait
		}

		minTTL, err := c.sendInformerMetrics(ctx, informer)
		if minTTL == 0 {
			minTTL = 30 * time.Second
		}
		if err != nil {
			if (retries % retryWarnMod) == 0 {
				logger.Errorf("error broadcasting metric: %s", err)
				retries++
			}
			// retry sooner
			timer.Reset(minTTL / 4)
			continue
		}

		retries = 0
		// send metric again in TTL/2
		timer.Reset(minTTL / 2)
	}
}

func (c *Cluster) sendPingMetric(ctx context.Context) (api.Metric, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/sendPingMetric")
	defer span.End()

	id := c.ID(ctx)
	newPingVal := pingValue{
		Peername:      id.Peername,
		IPFSID:        id.IPFS.ID,
		IPFSAddresses: publicIPFSAddresses(id.IPFS.Addresses),
	}
	if c.curPingVal.Valid() &&
		!newPingVal.Valid() { // i.e. ipfs down
		newPingVal = c.curPingVal // use last good value
	}
	c.curPingVal = newPingVal

	v, err := json.Marshal(newPingVal)
	if err != nil {
		logger.Error(err)
		// continue anyways
	}

	metric := api.Metric{
		Name:  pingMetricName,
		Peer:  c.id,
		Valid: true,
		Value: string(v),
	}
	metric.SetTTL(c.config.MonitorPingInterval * 2)
	return metric, c.monitor.PublishMetric(ctx, metric)
}

// logPingMetric logs a ping metric as if it had been sent from PID.  It is
// used to make peers appear available as soon as we connect to them (without
// having to wait for them to broadcast a metric).
//
// We avoid specifically sending a metric to a peer when we "connect" to it
// because: a) this requires an extra. OPEN RPC endpoint (LogMetric) that can
// be called by everyone b) We have no way of verifying that the peer ID in a
// metric pushed is actually the issuer of the metric (something the regular
// "pubsub" way of pushing metrics allows (by verifying the signature on the
// message). Thus, this reduces chances of abuse until we have something
// better.
func (c *Cluster) logPingMetric(ctx context.Context, pid peer.ID) error {
	m := api.Metric{
		Name:  pingMetricName,
		Peer:  pid,
		Valid: true,
	}
	m.SetTTL(c.config.MonitorPingInterval * 2)
	return c.monitor.LogMetric(ctx, m)
}

func (c *Cluster) pushPingMetrics(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "cluster/pushPingMetrics")
	defer span.End()

	ticker := time.NewTicker(c.config.MonitorPingInterval)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c.sendPingMetric(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Alerts returns the last alerts recorded by this cluster peer with the most
// recent first.
func (c *Cluster) Alerts() []api.Alert {
	c.alertsMux.Lock()
	alerts := make([]api.Alert, len(c.alerts))
	{
		total := len(alerts)
		for i, a := range c.alerts {
			alerts[total-1-i] = a
		}
	}
	c.alertsMux.Unlock()

	return alerts
}

// read the alerts channel from the monitor and triggers repins
func (c *Cluster) alertsHandler() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case alrt := <-c.monitor.Alerts():
			// Follower peers do not care about alerts.
			// They can do nothing about them.
			if c.config.FollowerMode {
				continue
			}

			logger.Warnf("metric alert for %s: Peer: %s.", alrt.Name, alrt.Peer)
			c.alertsMux.Lock()
			{
				if len(c.alerts) > maxAlerts {
					c.alerts = c.alerts[:0]
				}

				c.alerts = append(c.alerts, alrt)
			}
			c.alertsMux.Unlock()

			if alrt.Name != pingMetricName {
				continue // only handle ping alerts
			}

			if c.config.DisableRepinning {
				logger.Debugf("repinning is disabled. Will not re-allocate pins on alerts")
				return
			}

			cState, err := c.consensus.State(c.ctx)
			if err != nil {
				logger.Warn(err)
				return
			}

			distance, err := c.distances(c.ctx, alrt.Peer)
			if err != nil {
				logger.Warn(err)
				return
			}

			pinCh := make(chan api.Pin, 1024)
			go func() {
				err = cState.List(c.ctx, pinCh)
				if err != nil {
					logger.Warn(err)
				}
			}()

			for pin := range pinCh {
				if containsPeer(pin.Allocations, alrt.Peer) && distance.isClosest(pin.Cid) {
					c.repinFromPeer(c.ctx, alrt.Peer, pin)
				}
			}
		}
	}
}

// BandwidthByProtocol returns the libp2p bandwidth metrics as provided by the
// bandwidth reporter that the peer was initialized with. Returns nil when
// unset.
func (c *Cluster) BandwidthByProtocol() api.BandwidthByProtocol {
	if c.bandwidthReporter == nil {
		return nil
	}

	bbp := make(api.BandwidthByProtocol)
	stats := c.bandwidthReporter.GetBandwidthByProtocol()
	for k, v := range stats {
		bbp[k] = api.Bandwidth{
			TotalIn:  v.TotalIn,
			TotalOut: v.TotalOut,
			RateIn:   v.RateIn,
			RateOut:  v.RateOut,
		}
	}

	return bbp
}

// detects any changes in the peerset and saves the configuration. When it
// detects that we have been removed from the peerset, it shuts down this peer.
func (c *Cluster) watchPeers() {
	ticker := time.NewTicker(c.config.PeerWatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			//logger.Debugf("%s watching peers", c.id)
			hasMe := false
			peers, err := c.consensus.Peers(c.ctx)
			if err != nil {
				logger.Error(err)
				continue
			}
			for _, p := range peers {
				if p == c.id {
					hasMe = true
					break
				}
			}

			if !hasMe {
				c.shutdownLock.Lock()
				defer c.shutdownLock.Unlock()
				logger.Info("peer no longer in peerset. Initiating shutdown")
				c.removed = true
				go c.Shutdown(c.ctx)
				return
			}
		}
	}
}

// reBootstrap regularly attempts to bootstrap (re-connect to peers from the
// peerstore). This should ensure that we auto-recover from situations in
// which the network was completely gone and we lost all peers.
func (c *Cluster) reBootstrap() {
	generalBootstrap := time.NewTicker(reBootstrapInterval)
	priorityPeerReconnect := time.NewTicker(priorityPeerReconnectInterval)

	defer generalBootstrap.Stop()
	defer priorityPeerReconnect.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-generalBootstrap.C:
			// Attempt to reach low-water setting if for some
			// reason we are not there already. The default low
			// water is 100.  On small clusters this ensures we
			// stay connected to everyone. On larger clusters this
			// will not trigger new connections when already above
			// low water. When it does, known peers will be randomly
			// selected.
			connected := c.peerManager.Bootstrap(c.config.ConnMgr.LowWater, false, false)
			for _, p := range connected {
				logger.Infof("reconnected to %s", p)
			}
		case <-priorityPeerReconnect.C:
			// This is a safeguard for clusters with many peers.
			// It is understood that PeerAddresses are stable,
			// possibly "trusted" or at least honest peers.
			//
			// We don't need to be connected to them, but in an
			// scenario where there rest of the (untrusted) peers
			// works to isolate or mislead other peers (i.e. not
			// propagating pubsub), it does not hurt to reconnect
			// to one of these peers from time to time.
			if len(c.config.PeerAddresses) == 0 {
				break
			}
			connected := c.peerManager.Bootstrap(1, true, true)
			for _, p := range connected {
				logger.Infof("reconnected to priority peer %s", p)
			}
		}
	}
}

// find all Cids pinned to a given peer and triggers re-pins on them.
func (c *Cluster) vacatePeer(ctx context.Context, p peer.ID) {
	ctx, span := trace.StartSpan(ctx, "cluster/vacatePeer")
	defer span.End()

	if c.config.DisableRepinning {
		logger.Warnf("repinning is disabled. Will not re-allocate cids from %s", p)
		return
	}

	cState, err := c.consensus.State(ctx)
	if err != nil {
		logger.Warn(err)
		return
	}

	pinCh := make(chan api.Pin, 1024)
	go func() {
		err = cState.List(ctx, pinCh)
		if err != nil {
			logger.Warn(err)
		}
	}()

	for pin := range pinCh {
		if containsPeer(pin.Allocations, p) {
			c.repinFromPeer(ctx, p, pin)
		}
	}
}

// repinFromPeer triggers a repin on a given pin object blacklisting one of the
// allocations.
func (c *Cluster) repinFromPeer(ctx context.Context, p peer.ID, pin api.Pin) {
	ctx, span := trace.StartSpan(ctx, "cluster/repinFromPeer")
	defer span.End()

	logger.Debugf("repinning %s from peer %s", pin.Cid, p)

	pin.Allocations = nil // force re-allocations
	// note that pin() should not result in different allocations
	// if we are not under the replication-factor min.
	_, ok, err := c.pin(ctx, pin, []peer.ID{p})
	if ok && err == nil {
		logger.Infof("repinned %s out of %s", pin.Cid, p)
	}
}

// run launches some go-routines which live throughout the cluster's life
func (c *Cluster) run() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.watchPinset()
	}()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.pushPingMetrics(c.ctx)
	}()

	c.wg.Add(len(c.informers))
	for _, informer := range c.informers {
		go func(inf Informer) {
			defer c.wg.Done()
			c.pushInformerMetrics(c.ctx, inf)
		}(informer)
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.watchPeers()
	}()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.alertsHandler()
	}()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.reBootstrap()
	}()
}

func (c *Cluster) ready(timeout time.Duration) {
	ctx, span := trace.StartSpan(c.ctx, "cluster/ready")
	defer span.End()

	// We bootstrapped first because with dirty state consensus
	// may have a peerset and not find a leader so we cannot wait
	// for it.
	timer := time.NewTimer(timeout)
	select {
	case <-timer.C:
		logger.Error("***** ipfs-cluster consensus start timed out (tips below) *****")
		logger.Error(`
**************************************************
This peer was not able to become part of the cluster.
This might be due to one or several causes:
  - Check the logs above this message for errors
  - Check that there is connectivity to the "peers" multiaddresses
  - Check that all cluster peers are using the same "secret"
  - Check that this peer is reachable on its "listen_multiaddress" by all peers
  - Check that the current cluster is healthy (has a leader). Otherwise make
    sure to start enough peers so that a leader election can happen.
  - Check that the peer(s) you are trying to connect to is running the
    same version of IPFS-cluster.
**************************************************
`)
		c.Shutdown(ctx)
		return
	case <-c.consensus.Ready(ctx):
		// Consensus ready means the state is up to date.
	case <-c.ctx.Done():
		return
	}

	// Cluster is ready.

	peers, err := c.consensus.Peers(ctx)
	if err != nil {
		logger.Error(err)
		c.Shutdown(ctx)
		return
	}

	logger.Info("Cluster Peers (without including ourselves):")
	if len(peers) == 1 {
		logger.Info("    - No other peers")
	}

	for _, p := range peers {
		if p != c.id {
			logger.Infof("    - %s", p)
		}
	}

	// Wait for ipfs
	logger.Info("Waiting for IPFS to be ready...")
	select {
	case <-ctx.Done():
		return
	case <-c.ipfs.Ready(ctx):
		ipfsid, err := c.ipfs.ID(ctx)
		if err != nil {
			logger.Error("IPFS signaled ready but ID() errored: ", err)
		} else {
			logger.Infof("IPFS is ready. Peer ID: %s", ipfsid.ID)
		}
	}

	close(c.readyCh)
	c.shutdownLock.Lock()
	c.readyB = true
	c.shutdownLock.Unlock()
	logger.Info("** IPFS Cluster is READY **")
}

// Ready returns a channel which signals when this peer is
// fully initialized (including consensus).
func (c *Cluster) Ready() <-chan struct{} {
	return c.readyCh
}

// Shutdown performs all the necessary operations to shutdown
// the IPFS Cluster peer:
// * Save peerstore with the current peers
// * Remove itself from consensus when LeaveOnShutdown is set
// * It Shutdowns all the components
// * Collects all goroutines
//
// Shutdown does not close the libp2p host, the DHT, the datastore or
// generally anything that Cluster did not create.
func (c *Cluster) Shutdown(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "cluster/Shutdown")
	defer span.End()

	c.shutdownLock.Lock()
	defer c.shutdownLock.Unlock()

	if c.shutdownB {
		logger.Debug("Cluster is already shutdown")
		return nil
	}

	logger.Info("shutting down Cluster")

	// Shutdown APIs first, avoids more requests coming through.
	for _, api := range c.apis {
		if err := api.Shutdown(ctx); err != nil {
			logger.Errorf("error stopping API: %s", err)
			return err
		}
	}

	// Cancel discovery service (this shutdowns announcing). Handling
	// entries is canceled along with the context below.
	if c.discovery != nil {
		c.discovery.Close()
	}

	// Try to store peerset file for all known peers whatsoever
	// if we got ready (otherwise, don't overwrite anything)
	if c.readyB {
		// Ignoring error since it's a best-effort
		c.peerManager.SavePeerstoreForPeers(c.host.Peerstore().Peers())
	}

	// Only attempt to leave if:
	// - consensus is initialized
	// - cluster was ready (no bootstrapping error)
	// - We are not removed already (means watchPeers() called us)
	if c.consensus != nil && c.config.LeaveOnShutdown && c.readyB && !c.removed {
		c.removed = true
		_, err := c.consensus.Peers(ctx)
		if err == nil {
			// best effort
			logger.Warn("attempting to leave the cluster. This may take some seconds")
			err := c.consensus.RmPeer(ctx, c.id)
			if err != nil {
				logger.Error("leaving cluster: " + err.Error())
			}
		}
	}

	if con := c.consensus; con != nil {
		if err := con.Shutdown(ctx); err != nil {
			logger.Errorf("error stopping consensus: %s", err)
			return err
		}
	}

	// We left the cluster or were removed. Remove any consensus-specific
	// state.
	if c.removed && c.readyB {
		err := c.consensus.Clean(ctx)
		if err != nil {
			logger.Error("cleaning consensus: ", err)
		}
	}

	if err := c.monitor.Shutdown(ctx); err != nil {
		logger.Errorf("error stopping monitor: %s", err)
		return err
	}

	if err := c.ipfs.Shutdown(ctx); err != nil {
		logger.Errorf("error stopping IPFS Connector: %s", err)
		return err
	}

	if err := c.tracker.Shutdown(ctx); err != nil {
		logger.Errorf("error stopping PinTracker: %s", err)
		return err
	}

	for _, inf := range c.informers {
		if err := inf.Shutdown(ctx); err != nil {
			logger.Errorf("error stopping informer: %s", err)
			return err
		}
	}

	if err := c.tracer.Shutdown(ctx); err != nil {
		logger.Errorf("error stopping Tracer: %s", err)
		return err
	}

	c.cancel()
	c.wg.Wait()

	c.shutdownB = true
	close(c.doneCh)
	return nil
}

// Done provides a way to learn if the Peer has been shutdown
// (for example, because it has been removed from the Cluster)
func (c *Cluster) Done() <-chan struct{} {
	return c.doneCh
}

// ID returns information about the Cluster peer
func (c *Cluster) ID(ctx context.Context) api.ID {
	ctx, span := trace.StartSpan(ctx, "cluster/ID")
	defer span.End()

	// ignore error since it is included in response object
	ipfsID, err := c.ipfs.ID(ctx)
	if err != nil {
		ipfsID = api.IPFSID{
			Error: err.Error(),
		}
	}

	var addrs []api.Multiaddr
	mAddrs, err := peer.AddrInfoToP2pAddrs(&peer.AddrInfo{ID: c.id, Addrs: c.host.Addrs()})
	if err == nil {
		for _, mAddr := range mAddrs {
			addrs = append(addrs, api.NewMultiaddrWithValue(mAddr))
		}
	}

	peers := []peer.ID{}
	// This method might get called very early by a remote peer
	// and might catch us when consensus is not set
	if c.consensus != nil {
		peers, _ = c.consensus.Peers(ctx)
	}

	clusterPeerInfos := c.peerManager.PeerInfos(peers)
	addresses := []api.Multiaddr{}
	for _, pinfo := range clusterPeerInfos {
		addrs, err := peer.AddrInfoToP2pAddrs(&pinfo)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			addresses = append(addresses, api.NewMultiaddrWithValue(a))
		}
	}

	id := api.ID{
		ID: c.id,
		// PublicKey:          c.host.Peerstore().PubKey(c.id),
		Addresses:             addrs,
		ClusterPeers:          peers,
		ClusterPeersAddresses: addresses,
		Version:               version.Version.String(),
		RPCProtocolVersion:    version.RPCProtocol,
		IPFS:                  ipfsID,
		Peername:              c.config.Peername,
	}
	if err != nil {
		id.Error = err.Error()
	}

	return id
}

// PeerAdd adds a new peer to this Cluster.
//
// For it to work well, the new peer should be discoverable
// (part of our peerstore or connected to one of the existing peers)
// and reachable. Since PeerAdd allows to add peers which are
// not running, or reachable, it is recommended to call Join() from the
// new peer instead.
//
// The new peer ID will be passed to the consensus
// component to be added to the peerset.
func (c *Cluster) PeerAdd(ctx context.Context, pid peer.ID) (*api.ID, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/PeerAdd")
	defer span.End()

	c.shutdownLock.RLock()
	defer c.shutdownLock.RUnlock()
	if c.shutdownB {
		return nil, errors.New("cluster is shutdown")
	}

	// starting 10 nodes on the same box for testing
	// causes deadlock and a global lock here
	// seems to help.
	c.paMux.Lock()
	defer c.paMux.Unlock()
	logger.Debugf("peerAdd called with %s", pid)

	// Let the consensus layer be aware of this peer
	err := c.consensus.AddPeer(ctx, pid)
	if err != nil {
		logger.Error(err)
		id := &api.ID{ID: pid, Error: err.Error()}
		return id, err
	}

	logger.Infof("Peer added %s", pid)
	addedID, err := c.getIDForPeer(ctx, pid)
	if err != nil {
		return addedID, err
	}
	if !containsPeer(addedID.ClusterPeers, c.id) {
		addedID.ClusterPeers = append(addedID.ClusterPeers, c.id)
	}
	return addedID, nil
}

// PeerRemove removes a peer from this Cluster.
//
// The peer will be removed from the consensus peerset.
// This may first trigger repinnings for all content if not disabled.
func (c *Cluster) PeerRemove(ctx context.Context, pid peer.ID) error {
	ctx, span := trace.StartSpan(ctx, "cluster/PeerRemove")
	defer span.End()

	// We need to repin before removing the peer, otherwise, it won't
	// be able to submit the pins.
	logger.Infof("re-allocating all CIDs directly associated to %s", pid)
	c.vacatePeer(ctx, pid)

	err := c.consensus.RmPeer(ctx, pid)
	if err != nil {
		logger.Error(err)
		return err
	}
	logger.Info("Peer removed %s", pid)
	return nil
}

// Join adds this peer to an existing cluster by bootstrapping to a
// given multiaddress. It works by calling PeerAdd on the destination
// cluster and making sure that the new peer is ready to discover and contact
// the rest.
func (c *Cluster) Join(ctx context.Context, addr ma.Multiaddr) error {
	ctx, span := trace.StartSpan(ctx, "cluster/Join")
	defer span.End()

	logger.Debugf("Join(%s)", addr)

	// Add peer to peerstore so we can talk to it
	pid, err := c.peerManager.ImportPeer(addr, false, peerstore.PermanentAddrTTL)
	if err != nil {
		return err
	}
	if pid == c.id {
		return nil
	}

	// Note that PeerAdd() on the remote peer will
	// figure out what our real address is (obviously not
	// ListenAddr).
	var myID api.ID
	err = c.rpcClient.CallContext(
		ctx,
		pid,
		"Cluster",
		"PeerAdd",
		c.id,
		&myID,
	)
	if err != nil {
		logger.Error(err)
		return err
	}

	// Log a fake but valid metric from the peer we are
	// contacting. This will signal a CRDT component that
	// we know that peer since we have metrics for it without
	// having to wait for the next metric round.
	if err := c.logPingMetric(ctx, pid); err != nil {
		logger.Warn(err)
	}

	// Broadcast our metrics to the world
	err = c.sendInformersMetrics(ctx)
	if err != nil {
		logger.Warn(err)
	}

	_, err = c.sendPingMetric(ctx)
	if err != nil {
		logger.Warn(err)
	}

	// We need to trigger a DHT bootstrap asap for this peer to not be
	// lost if the peer it bootstrapped to goes down. We do this manually
	// by triggering 1 round of bootstrap in the background.
	// Note that our regular bootstrap process is still running in the
	// background since we created the cluster.
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		select {
		case err := <-c.dht.LAN.RefreshRoutingTable():
			if err != nil {
				// this error is quite chatty
				// on single peer clusters
				logger.Debug(err)
			}
		case <-c.ctx.Done():
			return
		}

		select {
		case err := <-c.dht.WAN.RefreshRoutingTable():
			if err != nil {
				// this error is quite chatty
				// on single peer clusters
				logger.Debug(err)
			}
		case <-c.ctx.Done():
			return
		}
	}()

	// ConnectSwarms in the background after a while, when we have likely
	// received some metrics.
	time.AfterFunc(c.config.MonitorPingInterval, func() {
		c.ipfs.ConnectSwarms(c.ctx)
	})

	// wait for leader and for state to catch up
	// then sync
	err = c.consensus.WaitForSync(ctx)
	if err != nil {
		logger.Error(err)
		return err
	}

	// Start pinning items in the state that are not on IPFS yet.
	out := make(chan api.PinInfo, 1024)
	// discard outputs
	go func() {
		for range out {
		}
	}()
	go c.RecoverAllLocal(c.ctx, out)

	logger.Infof("%s: joined %s's cluster", c.id, pid)
	return nil
}

// Distances returns a distance checker using current trusted peers.
// It can optionally receive a peer ID to exclude from the checks.
func (c *Cluster) distances(ctx context.Context, exclude peer.ID) (*distanceChecker, error) {
	trustedPeers, err := c.getTrustedPeers(ctx, exclude)
	if err != nil {
		logger.Error("could not get trusted peers:", err)
		return nil, err
	}

	return &distanceChecker{
		local:      c.id,
		otherPeers: trustedPeers,
		cache:      make(map[peer.ID]distance, len(trustedPeers)+1),
	}, nil
}

// StateSync performs maintenance tasks on the global state that require
// looping through all the items. It is triggered automatically on
// StateSyncInterval. Currently it:
//   - Sends unpin for expired items for which this peer is "closest"
//     (skipped for follower peers)
func (c *Cluster) StateSync(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "cluster/StateSync")
	defer span.End()

	logger.Debug("StateSync")

	if c.config.FollowerMode {
		return nil
	}

	cState, err := c.consensus.State(ctx)
	if err != nil {
		return err
	}

	timeNow := time.Now()

	// Only trigger pin operations if we are the closest with respect to
	// other trusted peers. We cannot know if our peer ID is trusted by
	// other peers in the Cluster. This assumes yes. Setting FollowerMode
	// is a way to assume the opposite and skip this completely.
	distance, err := c.distances(ctx, "")
	if err != nil {
		return err // could not list peers
	}

	clusterPins := make(chan api.Pin, 1024)
	go func() {
		err = cState.List(ctx, clusterPins)
		if err != nil {
			logger.Error(err)
		}
	}()

	// Unpin expired items when we are the closest peer to them.
	for p := range clusterPins {
		if p.ExpiredAt(timeNow) && distance.isClosest(p.Cid) {
			logger.Infof("Unpinning %s: pin expired at %s", p.Cid, p.ExpireAt)
			if _, err := c.Unpin(ctx, p.Cid); err != nil {
				logger.Error(err)
			}
		}
	}

	return nil
}

// StatusAll returns the GlobalPinInfo for all tracked Cids in all peers on
// the out channel. This is done by broacasting a StatusAll to all peers.  If
// an error happens, it is returned. This method blocks until it finishes. The
// operation can be aborted by canceling the context.
func (c *Cluster) StatusAll(ctx context.Context, filter api.TrackerStatus, out chan<- api.GlobalPinInfo) error {
	ctx, span := trace.StartSpan(ctx, "cluster/StatusAll")
	defer span.End()

	in := make(chan api.TrackerStatus, 1)
	in <- filter
	close(in)
	return c.globalPinInfoStream(ctx, "PinTracker", "StatusAll", in, out)
}

// StatusAllLocal returns the PinInfo for all the tracked Cids in this peer on
// the out channel. It blocks until finished.
func (c *Cluster) StatusAllLocal(ctx context.Context, filter api.TrackerStatus, out chan<- api.PinInfo) error {
	ctx, span := trace.StartSpan(ctx, "cluster/StatusAllLocal")
	defer span.End()

	return c.tracker.StatusAll(ctx, filter, out)
}

// Status returns the GlobalPinInfo for a given Cid as fetched from all
// current peers. If an error happens, the GlobalPinInfo should contain
// as much information as could be fetched from the other peers.
func (c *Cluster) Status(ctx context.Context, h api.Cid) (api.GlobalPinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/Status")
	defer span.End()

	return c.globalPinInfoCid(ctx, "PinTracker", "Status", h)
}

// StatusLocal returns this peer's PinInfo for a given Cid.
func (c *Cluster) StatusLocal(ctx context.Context, h api.Cid) api.PinInfo {
	ctx, span := trace.StartSpan(ctx, "cluster/StatusLocal")
	defer span.End()

	return c.tracker.Status(ctx, h)
}

// used for RecoverLocal and SyncLocal.
func (c *Cluster) localPinInfoOp(
	ctx context.Context,
	h api.Cid,
	f func(context.Context, api.Cid) (api.PinInfo, error),
) (pInfo api.PinInfo, err error) {
	ctx, span := trace.StartSpan(ctx, "cluster/localPinInfoOp")
	defer span.End()

	cids, err := c.cidsFromMetaPin(ctx, h)
	if err != nil {
		return api.PinInfo{}, err
	}

	for _, ci := range cids {
		pInfo, err = f(ctx, ci)
		if err != nil {
			logger.Error("tracker.SyncCid() returned with error: ", err)
			logger.Error("Is the ipfs daemon running?")
			break
		}
	}
	// return the last pInfo/err, should be the root Cid if everything ok
	return pInfo, err
}

// RecoverAll triggers a RecoverAllLocal operation on all peers and returns
// GlobalPinInfo objets for all recovered items. This method blocks until
// finished. Operation can be aborted by canceling the context.
func (c *Cluster) RecoverAll(ctx context.Context, out chan<- api.GlobalPinInfo) error {
	ctx, span := trace.StartSpan(ctx, "cluster/RecoverAll")
	defer span.End()

	return c.globalPinInfoStream(ctx, "Cluster", "RecoverAllLocal", nil, out)
}

// RecoverAllLocal triggers a RecoverLocal operation for all Cids tracked
// by this peer.
//
// Recover operations ask IPFS to pin or unpin items in error state. Recover
// is faster than calling Pin on the same CID as it avoids committing an
// identical pin to the consensus layer.
//
// It returns the list of pins that were re-queued for pinning on the out
// channel. It blocks until done.
//
// RecoverAllLocal is called automatically every PinRecoverInterval.
func (c *Cluster) RecoverAllLocal(ctx context.Context, out chan<- api.PinInfo) error {
	ctx, span := trace.StartSpan(ctx, "cluster/RecoverAllLocal")
	defer span.End()

	return c.tracker.RecoverAll(ctx, out)
}

// Recover triggers a recover operation for a given Cid in all
// cluster peers.
//
// Recover operations ask IPFS to pin or unpin items in error state. Recover
// is faster than calling Pin on the same CID as it avoids committing an
// identical pin to the consensus layer.
func (c *Cluster) Recover(ctx context.Context, h api.Cid) (api.GlobalPinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/Recover")
	defer span.End()

	return c.globalPinInfoCid(ctx, "PinTracker", "Recover", h)
}

// RecoverLocal triggers a recover operation for a given Cid in this peer only.
// It returns the updated PinInfo, after recovery.
//
// Recover operations ask IPFS to pin or unpin items in error state. Recover
// is faster than calling Pin on the same CID as it avoids committing an
// identical pin to the consensus layer.
func (c *Cluster) RecoverLocal(ctx context.Context, h api.Cid) (api.PinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/RecoverLocal")
	defer span.End()

	return c.localPinInfoOp(ctx, h, c.tracker.Recover)
}

// Pins sends pins on the given out channel as it iterates the full
// pinset (current global state). This is the source of truth as to which pins
// are managed and their allocation, but does not indicate if the item is
// successfully pinned. For that, use the Status*() methods.
//
// The operation can be aborted by canceling the context. This methods blocks
// until the operation has completed.
func (c *Cluster) Pins(ctx context.Context, out chan<- api.Pin) error {
	ctx, span := trace.StartSpan(ctx, "cluster/Pins")
	defer span.End()

	cState, err := c.consensus.State(ctx)
	if err != nil {
		logger.Error(err)
		return err
	}
	return cState.List(ctx, out)
}

// pinsSlice returns the list of Cids managed by Cluster and which are part
// of the current global state. This is the source of truth as to which
// pins are managed and their allocation, but does not indicate if
// the item is successfully pinned. For that, use StatusAll().
//
// It is recommended to use PinsChannel(), as this method is equivalent to
// loading the full pinset in memory!
func (c *Cluster) pinsSlice(ctx context.Context) ([]api.Pin, error) {
	out := make(chan api.Pin, 1024)
	var err error
	go func() {
		err = c.Pins(ctx, out)
	}()

	var pins []api.Pin
	for pin := range out {
		pins = append(pins, pin)
	}
	return pins, err
}

// PinGet returns information for a single Cid managed by Cluster.
// The information is obtained from the current global state. The
// returned api.Pin provides information about the allocations
// assigned for the requested Cid, but does not indicate if
// the item is successfully pinned. For that, use Status(). PinGet
// returns an error if the given Cid is not part of the global state.
func (c *Cluster) PinGet(ctx context.Context, h api.Cid) (api.Pin, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/PinGet")
	defer span.End()

	st, err := c.consensus.State(ctx)
	if err != nil {
		return api.Pin{}, err
	}
	pin, err := st.Get(ctx, h)
	if err != nil {
		return api.Pin{}, err
	}
	return pin, nil
}

// Pin makes the cluster Pin a Cid. This implies adding the Cid
// to the IPFS Cluster peers shared-state. Depending on the cluster
// pinning strategy, the PinTracker may then request the IPFS daemon
// to pin the Cid.
//
// Pin returns the Pin as stored in the global state (with the given
// allocations and an error if the operation could not be persisted. Pin does
// not reflect the success or failure of underlying IPFS daemon pinning
// operations which happen in async fashion.
//
// If the options UserAllocations are non-empty then these peers are pinned
// with priority over other peers in the cluster.  If the max repl factor is
// less than the size of the specified peerset then peers are chosen from this
// set in allocation order.  If the minimum repl factor is greater than the
// size of this set then the remaining peers are allocated in order from the
// rest of the cluster. Priority allocations are best effort. If any priority
// peers are unavailable then Pin will simply allocate from the rest of the
// cluster.
//
// If the Update option is set, the pin options (including allocations) will
// be copied from an existing one. This is equivalent to running PinUpdate.
func (c *Cluster) Pin(ctx context.Context, h api.Cid, opts api.PinOptions) (api.Pin, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/Pin")
	defer span.End()

	pin := api.PinWithOpts(h, opts)

	result, _, err := c.pin(ctx, pin, []peer.ID{})
	return result, err
}

// sets the default replication factor in a pin when it's set to 0
func (c *Cluster) setupReplicationFactor(pin api.Pin) (api.Pin, error) {
	rplMin := pin.ReplicationFactorMin
	rplMax := pin.ReplicationFactorMax
	if rplMin == 0 {
		rplMin = c.config.ReplicationFactorMin
		pin.ReplicationFactorMin = rplMin
	}
	if rplMax == 0 {
		rplMax = c.config.ReplicationFactorMax
		pin.ReplicationFactorMax = rplMax
	}

	// When pinning everywhere, remove all allocations.
	// Allocations may have been preset by the adder
	// for the cases when the replication factor is > -1.
	// Fixes part of #1319: allocations when adding
	// are kept.
	if pin.IsPinEverywhere() {
		pin.Allocations = nil
	}

	return pin, isReplicationFactorValid(rplMin, rplMax)
}

// basic checks on the pin type to check it's well-formed.
func checkPinType(pin api.Pin) error {
	switch pin.Type {
	case api.DataType:
		if pin.Reference != nil {
			return errors.New("data pins should not reference other pins")
		}
	case api.ShardType:
		if pin.MaxDepth != 1 {
			return errors.New("must pin shards go depth 1")
		}
		// FIXME: indirect shard pins could have max-depth 2
		// FIXME: repinning a shard type will overwrite replication
		//        factor from previous:
		// if existing.ReplicationFactorMin != rplMin ||
		//	existing.ReplicationFactorMax != rplMax {
		//	return errors.New("shard update with wrong repl factors")
		//}
	case api.ClusterDAGType:
		if pin.MaxDepth != 0 {
			return errors.New("must pin roots directly")
		}
		if pin.Reference == nil {
			return errors.New("clusterDAG pins should reference a Meta pin")
		}
	case api.MetaType:
		if len(pin.Allocations) != 0 {
			return errors.New("meta pin should not specify allocations")
		}
		if pin.Reference == nil {
			return errors.New("metaPins should reference a ClusterDAG")
		}

	default:
		return errors.New("unrecognized pin type")
	}
	return nil
}

// setupPin ensures that the Pin object is fit for pinning. We check
// and set the replication factors and ensure that the pinType matches the
// metadata consistently.
func (c *Cluster) setupPin(ctx context.Context, pin, existing api.Pin) (api.Pin, error) {
	_, span := trace.StartSpan(ctx, "cluster/setupPin")
	defer span.End()
	var err error

	pin, err = c.setupReplicationFactor(pin)
	if err != nil {
		return pin, err
	}

	if !pin.ExpireAt.IsZero() && pin.ExpireAt.Before(time.Now()) {
		return pin, errors.New("pin.ExpireAt set before current time")
	}

	if !existing.Defined() {
		return pin, nil
	}

	// If an pin CID is already pin, we do a couple more checks
	if existing.Type != pin.Type {
		msg := "cannot repin CID with different tracking method, "
		msg += "clear state with pin rm to proceed. "
		msg += "New: %s. Was: %s"
		return pin, fmt.Errorf(msg, pin.Type, existing.Type)
	}

	if existing.Mode == api.PinModeRecursive && pin.Mode != api.PinModeRecursive {
		msg := "cannot repin a CID which is already pinned in "
		msg += "recursive mode (new pin is pinned as %s). Unpin it first."
		return pin, fmt.Errorf(msg, pin.Mode)
	}

	return pin, checkPinType(pin)
}

// pin performs the actual pinning and supports a blacklist to be able to
// evacuate a node and returns the pin object that it tried to pin, whether
// the pin was submitted to the consensus layer or skipped (due to error or to
// the fact that it was already valid) and error.
//
// This is the method called by the Cluster.Pin RPC endpoint.
func (c *Cluster) pin(
	ctx context.Context,
	pin api.Pin,
	blacklist []peer.ID,
) (api.Pin, bool, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/pin")
	defer span.End()

	if c.config.FollowerMode {
		return api.Pin{}, false, errFollowerMode
	}

	if !pin.Cid.Defined() {
		return pin, false, errors.New("bad pin object")
	}

	// Handle pin updates when the option is set
	if update := pin.PinUpdate; update.Defined() && !update.Equals(pin.Cid) {
		pin, err := c.PinUpdate(ctx, update, pin.Cid, pin.PinOptions)
		return pin, true, err
	}

	existing, err := c.PinGet(ctx, pin.Cid)
	if err != nil && err != state.ErrNotFound {
		return pin, false, err
	}

	pin, err = c.setupPin(ctx, pin, existing)
	if err != nil {
		return pin, false, err
	}

	// Set the Pin timestamp to now(). This is not an user-controllable
	// "option".
	pin.Timestamp = time.Now()

	if pin.Type == api.MetaType {
		return pin, true, c.consensus.LogPin(ctx, pin)
	}

	// Usually allocations are unset when pinning normally, however, the
	// allocations may have been preset by the adder in which case they
	// need to be respected. Whenever allocations are set. We don't
	// re-allocate. repinFromPeer() unsets allocations for this reason.
	// allocate() will check which peers are currently allocated
	// and try to respect them.
	if len(pin.Allocations) == 0 {
		// If replication factor is -1, this will return empty
		// allocations.
		allocs, err := c.allocate(
			ctx,
			pin.Cid,
			existing,
			pin.ReplicationFactorMin,
			pin.ReplicationFactorMax,
			blacklist,
			pin.UserAllocations,
		)
		if err != nil {
			return pin, false, err
		}
		pin.Allocations = allocs
	}

	// If this is true, replication factor should be -1.
	if len(pin.Allocations) == 0 {
		logger.Infof("pinning %s everywhere:", pin.Cid)
	} else {
		logger.Infof("pinning %s on %s:", pin.Cid, pin.Allocations)
	}

	return pin, true, c.consensus.LogPin(ctx, pin)
}

// Unpin removes a previously pinned Cid from Cluster. It returns
// the global state Pin object as it was stored before removal, or
// an error if it was not possible to update the global state.
//
// Unpin does not reflect the success or failure of underlying IPFS daemon
// unpinning operations, which happen in async fashion.
func (c *Cluster) Unpin(ctx context.Context, h api.Cid) (api.Pin, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/Unpin")
	defer span.End()

	if c.config.FollowerMode {
		return api.Pin{}, errFollowerMode
	}

	logger.Info("IPFS cluster unpinning:", h)
	pin, err := c.PinGet(ctx, h)
	if err != nil {
		return api.Pin{}, err
	}

	switch pin.Type {
	case api.DataType:
		return pin, c.consensus.LogUnpin(ctx, pin)
	case api.ShardType:
		err := "cannot unpin a shard directly. Unpin content root CID instead"
		return pin, errors.New(err)
	case api.MetaType:
		// Unpin cluster dag and referenced shards
		err := c.unpinClusterDag(ctx, pin)
		if err != nil {
			return pin, err
		}
		return pin, c.consensus.LogUnpin(ctx, pin)
	case api.ClusterDAGType:
		err := "cannot unpin a Cluster DAG directly. Unpin content root CID instead"
		return pin, errors.New(err)
	default:
		return pin, errors.New("unrecognized pin type")
	}
}

// unpinClusterDag unpins the clusterDAG metadata node and the shard metadata
// nodes that it references.  It handles the case where multiple parents
// reference the same metadata node, only unpinning those nodes without
// existing references
func (c *Cluster) unpinClusterDag(ctx context.Context, metaPin api.Pin) error {
	cids, err := c.cidsFromMetaPin(ctx, metaPin.Cid)
	if err != nil {
		return err
	}

	// TODO: FIXME: potentially unpinning shards which are referenced
	// by other clusterDAGs.
	for _, ci := range cids {
		err = c.consensus.LogUnpin(ctx, api.PinCid(ci))
		if err != nil {
			return err
		}
	}
	return nil
}

// PinUpdate pins a new CID based on an existing cluster Pin. The allocations
// and most pin options (replication factors) are copied from the existing
// Pin.  The options object can be used to set the Name for the new pin and
// might support additional options in the future.
//
// The from pin is NOT unpinned upon completion. The new pin might take
// advantage of efficient pin/update operation on IPFS-side (if the
// IPFSConnector supports it - the default one does). This may offer
// significant speed when pinning items which are similar to previously pinned
// content.
func (c *Cluster) PinUpdate(ctx context.Context, from api.Cid, to api.Cid, opts api.PinOptions) (api.Pin, error) {
	existing, err := c.PinGet(ctx, from)
	if err != nil { // including when the existing pin is not found
		return api.Pin{}, err
	}

	// Hector: I am not sure whether it has any point to update something
	// like a MetaType.
	if existing.Type != api.DataType {
		return api.Pin{}, errors.New("this pin type cannot be updated")
	}

	existing.Cid = to
	existing.PinUpdate = from
	existing.Timestamp = time.Now()
	if opts.Name != "" {
		existing.Name = opts.Name
	}
	if !opts.ExpireAt.IsZero() && opts.ExpireAt.After(time.Now()) {
		existing.ExpireAt = opts.ExpireAt
	}
	return existing, c.consensus.LogPin(ctx, existing)
}

// PinPath pins an CID resolved from its IPFS Path. It returns the resolved
// Pin object.
func (c *Cluster) PinPath(ctx context.Context, path string, opts api.PinOptions) (api.Pin, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/PinPath")
	defer span.End()

	ci, err := c.ipfs.Resolve(ctx, path)
	if err != nil {
		return api.Pin{}, err
	}

	return c.Pin(ctx, ci, opts)
}

// UnpinPath unpins a CID resolved from its IPFS Path. If returns the
// previously pinned Pin object.
func (c *Cluster) UnpinPath(ctx context.Context, path string) (api.Pin, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/UnpinPath")
	defer span.End()

	ci, err := c.ipfs.Resolve(ctx, path)
	if err != nil {
		return api.Pin{}, err
	}

	return c.Unpin(ctx, ci)
}

// AddFile adds a file to the ipfs daemons of the cluster.  The ipfs importer
// pipeline is used to DAGify the file.  Depending on input parameters this
// DAG can be added locally to the calling cluster peer's ipfs repo, or
// sharded across the entire cluster.
func (c *Cluster) AddFile(ctx context.Context, reader *multipart.Reader, params api.AddParams) (api.Cid, error) {
	// TODO: add context param and tracing

	var dags adder.ClusterDAGService
	if params.Shard {
		dags = sharding.New(ctx, c.rpcClient, params, nil)
	} else {
		dags = single.New(ctx, c.rpcClient, params, params.Local)
	}
	defer dags.Close()
	add := adder.New(dags, params, nil)
	return add.FromMultipart(ctx, reader)
}

// Version returns the current IPFS Cluster version.
func (c *Cluster) Version() string {
	return version.Version.String()
}

// Peers returns the IDs of the members of this Cluster on the out channel.
// This method blocks until it has finished.
func (c *Cluster) Peers(ctx context.Context, out chan<- api.ID) {
	ctx, span := trace.StartSpan(ctx, "cluster/Peers")
	defer span.End()

	peers, err := c.consensus.Peers(ctx)
	if err != nil {
		logger.Error(err)
		logger.Error("an empty list of peers will be returned")
		close(out)
		return
	}
	c.peersWithFilter(ctx, peers, out)
}

// requests IDs from a given number of peers.
func (c *Cluster) peersWithFilter(ctx context.Context, peers []peer.ID, out chan<- api.ID) {
	defer close(out)

	// We should be done relatively quickly with this call. Otherwise
	// report errors.
	timeout := 15 * time.Second
	ctxCall, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	in := make(chan struct{})
	close(in)
	idsOut := make(chan api.ID, len(peers))
	errCh := make(chan []error, 1)

	go func() {
		defer close(errCh)

		errCh <- c.rpcClient.MultiStream(
			ctxCall,
			peers,
			"Cluster",
			"IDStream",
			in,
			idsOut,
		)
	}()

	// Unfortunately, we need to use idsOut as intermediary channel
	// because it is closed when MultiStream ends and we cannot keep
	// adding things on it (the errors below).
	for id := range idsOut {
		select {
		case <-ctx.Done():
			logger.Errorf("Peers call aborted: %s", ctx.Err())
			return
		case out <- id:
		}
	}

	// ErrCh will always be closed on context cancellation too.
	errs := <-errCh
	for i, err := range errs {
		if err == nil {
			continue
		}
		if rpc.IsAuthorizationError(err) {
			continue
		}
		select {
		case <-ctx.Done():
			logger.Errorf("Peers call aborted: %s", ctx.Err())
		case out <- api.ID{
			ID:    peers[i],
			Error: err.Error(),
		}:
		}
	}
}

// getTrustedPeers gives listed of trusted peers except the current peer and
// the excluded peer if provided.
func (c *Cluster) getTrustedPeers(ctx context.Context, exclude peer.ID) ([]peer.ID, error) {
	peers, err := c.consensus.Peers(ctx)
	if err != nil {
		return nil, err
	}

	trustedPeers := make([]peer.ID, 0, len(peers))

	for _, p := range peers {
		if p == c.id || p == exclude || !c.consensus.IsTrustedPeer(ctx, p) {
			continue
		}
		trustedPeers = append(trustedPeers, p)
	}

	return trustedPeers, nil
}

func (c *Cluster) setTrackerStatus(gpin *api.GlobalPinInfo, h api.Cid, peers []peer.ID, status api.TrackerStatus, pin api.Pin, t time.Time) {
	for _, p := range peers {
		pv := pingValueFromMetric(c.monitor.LatestForPeer(c.ctx, pingMetricName, p))
		gpin.Add(api.PinInfo{
			Cid:         h,
			Name:        pin.Name,
			Allocations: pin.Allocations,
			Origins:     pin.Origins,
			Created:     pin.Timestamp,
			Metadata:    pin.Metadata,
			Peer:        p,
			PinInfoShort: api.PinInfoShort{
				PeerName:      pv.Peername,
				IPFS:          pv.IPFSID,
				IPFSAddresses: pv.IPFSAddresses,
				Status:        status,
				TS:            t,
			},
		})
	}
}

func (c *Cluster) globalPinInfoCid(ctx context.Context, comp, method string, h api.Cid) (api.GlobalPinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/globalPinInfoCid")
	defer span.End()

	// The object we will return
	gpin := api.GlobalPinInfo{}

	// allocated peers, we will contact them through rpc
	var dests []peer.ID
	// un-allocated peers, we will set remote status
	var remote []peer.ID

	timeNow := time.Now()

	// If pin is not part of the pinset, mark it unpinned
	pin, err := c.PinGet(ctx, h)
	if err != nil && err != state.ErrNotFound {
		logger.Error(err)
		return api.GlobalPinInfo{}, err
	}

	// When NotFound return directly with an unpinned
	// status.
	if err == state.ErrNotFound {
		var members []peer.ID
		if c.config.FollowerMode {
			members = []peer.ID{c.host.ID()}
		} else {
			members, err = c.consensus.Peers(ctx)
			if err != nil {
				logger.Error(err)
				return api.GlobalPinInfo{}, err
			}
		}

		c.setTrackerStatus(
			&gpin,
			h,
			members,
			api.TrackerStatusUnpinned,
			api.PinCid(h),
			timeNow,
		)
		return gpin, nil
	}

	// The pin exists.
	gpin.Cid = h
	gpin.Name = pin.Name

	// Make the list of peers that will receive the request.
	if c.config.FollowerMode {
		// during follower mode return only local status.
		dests = []peer.ID{c.host.ID()}
		remote = []peer.ID{}
	} else {
		members, err := c.consensus.Peers(ctx)
		if err != nil {
			logger.Error(err)
			return api.GlobalPinInfo{}, err
		}

		if !pin.IsPinEverywhere() {
			dests = pin.Allocations
			remote = peersSubtract(members, dests)
		} else {
			dests = members
			remote = []peer.ID{}
		}
	}

	// set status remote on un-allocated peers
	c.setTrackerStatus(&gpin, h, remote, api.TrackerStatusRemote, pin, timeNow)

	lenDests := len(dests)
	replies := make([]api.PinInfo, lenDests)

	// a globalPinInfo type of request should be relatively fast. We
	// cannot block response indefinitely due to an unresponsive node.
	timeout := 15 * time.Second
	ctxs, cancels := rpcutil.CtxsWithTimeout(ctx, lenDests, timeout)
	defer rpcutil.MultiCancel(cancels)

	errs := c.rpcClient.MultiCall(
		ctxs,
		dests,
		comp,
		method,
		h,
		rpcutil.CopyPinInfoToIfaces(replies),
	)

	for i, r := range replies {
		e := errs[i]

		// No error. Parse and continue
		if e == nil {
			gpin.Add(r)
			continue
		}

		if rpc.IsAuthorizationError(e) {
			logger.Debug("rpc auth error:", e)
			continue
		}

		// Deal with error cases (err != nil): wrap errors in PinInfo
		logger.Errorf("%s: error in broadcast response from %s: %s ", c.id, dests[i], e)

		pv := pingValueFromMetric(c.monitor.LatestForPeer(ctx, pingMetricName, dests[i]))
		gpin.Add(api.PinInfo{
			Cid:         h,
			Name:        pin.Name,
			Peer:        dests[i],
			Allocations: pin.Allocations,
			Origins:     pin.Origins,
			Created:     pin.Timestamp,
			Metadata:    pin.Metadata,
			PinInfoShort: api.PinInfoShort{
				PeerName:      pv.Peername,
				IPFS:          pv.IPFSID,
				IPFSAddresses: pv.IPFSAddresses,
				Status:        api.TrackerStatusClusterError,
				TS:            timeNow,
				Error:         e.Error(),
			},
		})
	}

	return gpin, nil
}

func (c *Cluster) globalPinInfoStream(ctx context.Context, comp, method string, inChan interface{}, out chan<- api.GlobalPinInfo) error {
	defer close(out)

	ctx, span := trace.StartSpan(ctx, "cluster/globalPinInfoStream")
	defer span.End()

	if inChan == nil {
		emptyChan := make(chan struct{})
		close(emptyChan)
		inChan = emptyChan
	}

	fullMap := make(map[api.Cid]api.GlobalPinInfo)

	var members []peer.ID
	var err error
	if c.config.FollowerMode {
		members = []peer.ID{c.host.ID()}
	} else {
		members, err = c.consensus.Peers(ctx)
		if err != nil {
			logger.Error(err)
			return err
		}
	}

	// We don't have a good timeout proposal for this. Depending on the
	// size of the state and the peformance of IPFS and the network, this
	// may take moderately long.
	// If we did, this is the place to put it.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	msOut := make(chan api.PinInfo)
	errsCh := make(chan []error, 1)
	go func() {
		defer close(errsCh)
		errsCh <- c.rpcClient.MultiStream(
			ctx,
			members,
			comp,
			method,
			inChan,
			msOut,
		)
	}()

	setPinInfo := func(p api.PinInfo) {
		if !p.Defined() {
			return
		}
		info, ok := fullMap[p.Cid]
		if !ok {
			info = api.GlobalPinInfo{}
		}
		info.Add(p)
		// Set the new/updated info
		fullMap[p.Cid] = info
	}

	// make the big collection.
	for pin := range msOut {
		setPinInfo(pin)
	}

	// This WAITs until MultiStream is DONE.
	erroredPeers := make(map[peer.ID]string)
	errs, ok := <-errsCh
	if ok {
		for i, err := range errs {
			if err == nil {
				continue
			}
			if rpc.IsAuthorizationError(err) {
				logger.Debug("rpc auth error", err)
				continue
			}
			logger.Errorf("%s: error in broadcast response from %s: %s ", c.id, members[i], err)
			erroredPeers[members[i]] = err.Error()
		}
	}

	// Merge any errors
	for p, msg := range erroredPeers {
		pv := pingValueFromMetric(c.monitor.LatestForPeer(ctx, pingMetricName, p))
		for c := range fullMap {
			setPinInfo(api.PinInfo{
				Cid:         c,
				Name:        "",
				Peer:        p,
				Allocations: nil,
				Origins:     nil,
				// Created:    // leave unitialized
				Metadata: nil,
				PinInfoShort: api.PinInfoShort{
					PeerName:      pv.Peername,
					IPFS:          pv.IPFSID,
					IPFSAddresses: pv.IPFSAddresses,
					Status:        api.TrackerStatusClusterError,
					TS:            time.Now(),
					Error:         msg,
				},
			})
		}
	}

	for _, v := range fullMap {
		select {
		case <-ctx.Done():
			err := fmt.Errorf("%s.%s aborted: %w", comp, method, ctx.Err())
			logger.Error(err)
			return err
		case out <- v:
		}
	}

	return nil
}

func (c *Cluster) getIDForPeer(ctx context.Context, pid peer.ID) (*api.ID, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/getIDForPeer")
	defer span.End()

	var id api.ID
	err := c.rpcClient.CallContext(
		ctx,
		pid,
		"Cluster",
		"ID",
		struct{}{},
		&id,
	)
	if err != nil {
		logger.Error(err)
		id.ID = pid
		id.Error = err.Error()
	}
	return &id, err
}

// cidsFromMetaPin expands a meta-pin and returns a list of Cids that
// Cluster handles for it: the ShardPins, the ClusterDAG and the MetaPin, in
// that order (the MetaPin is the last element).
// It returns a slice with only the given Cid if it's not a known Cid or not a
// MetaPin.
func (c *Cluster) cidsFromMetaPin(ctx context.Context, h api.Cid) ([]api.Cid, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/cidsFromMetaPin")
	defer span.End()

	cState, err := c.consensus.State(ctx)
	if err != nil {
		return nil, err
	}

	list := []api.Cid{h}

	pin, err := cState.Get(ctx, h)
	if err != nil {
		return nil, err
	}

	if pin.Type != api.MetaType {
		return list, nil
	}

	if pin.Reference == nil {
		return nil, errors.New("metaPin.Reference is unset")
	}
	list = append([]api.Cid{*pin.Reference}, list...)
	clusterDagPin, err := c.PinGet(ctx, *pin.Reference)
	if err != nil {
		return list, fmt.Errorf("could not get clusterDAG pin from state. Malformed pin?: %s", err)
	}

	clusterDagBlock, err := c.ipfs.BlockGet(ctx, clusterDagPin.Cid)
	if err != nil {
		return list, fmt.Errorf("error reading clusterDAG block from ipfs: %s", err)
	}

	clusterDagNode, err := sharding.CborDataToNode(clusterDagBlock, "cbor")
	if err != nil {
		return list, fmt.Errorf("error parsing clusterDAG block: %s", err)
	}
	for _, l := range clusterDagNode.Links() {
		list = append([]api.Cid{api.NewCid(l.Cid)}, list...)
	}

	return list, nil
}

// // diffPeers returns the peerIDs added and removed from peers2 in relation to
// // peers1
// func diffPeers(peers1, peers2 []peer.ID) (added, removed []peer.ID) {
// 	m1 := make(map[peer.ID]struct{})
// 	m2 := make(map[peer.ID]struct{})
// 	added = make([]peer.ID, 0)
// 	removed = make([]peer.ID, 0)
// 	if peers1 == nil && peers2 == nil {
// 		return
// 	}
// 	if peers1 == nil {
// 		added = peers2
// 		return
// 	}
// 	if peers2 == nil {
// 		removed = peers1
// 		return
// 	}

// 	for _, p := range peers1 {
// 		m1[p] = struct{}{}
// 	}
// 	for _, p := range peers2 {
// 		m2[p] = struct{}{}
// 	}
// 	for k := range m1 {
// 		_, ok := m2[k]
// 		if !ok {
// 			removed = append(removed, k)
// 		}
// 	}
// 	for k := range m2 {
// 		_, ok := m1[k]
// 		if !ok {
// 			added = append(added, k)
// 		}
// 	}
// 	return
// }

// RepoGC performs garbage collection sweep on all peers' IPFS repo.
func (c *Cluster) RepoGC(ctx context.Context) (api.GlobalRepoGC, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/RepoGC")
	defer span.End()

	members, err := c.consensus.Peers(ctx)
	if err != nil {
		logger.Error(err)
		return api.GlobalRepoGC{}, err
	}

	// to club `RepoGCLocal` responses of all peers into one
	globalRepoGC := api.GlobalRepoGC{PeerMap: make(map[string]api.RepoGC)}

	for _, member := range members {
		var repoGC api.RepoGC
		err = c.rpcClient.CallContext(
			ctx,
			member,
			"Cluster",
			"RepoGCLocal",
			struct{}{},
			&repoGC,
		)
		if err == nil {
			globalRepoGC.PeerMap[member.String()] = repoGC
			continue
		}

		if rpc.IsAuthorizationError(err) {
			logger.Debug("rpc auth error:", err)
			continue
		}

		logger.Errorf("%s: error in broadcast response from %s: %s ", c.id, member, err)

		pv := pingValueFromMetric(c.monitor.LatestForPeer(ctx, pingMetricName, member))

		globalRepoGC.PeerMap[member.String()] = api.RepoGC{
			Peer:     member,
			Peername: pv.Peername,
			Keys:     []api.IPFSRepoGC{},
			Error:    err.Error(),
		}
	}

	return globalRepoGC, nil
}

// RepoGCLocal performs garbage collection only on the local IPFS deamon.
func (c *Cluster) RepoGCLocal(ctx context.Context) (api.RepoGC, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/RepoGCLocal")
	defer span.End()

	resp, err := c.ipfs.RepoGC(ctx)
	if err != nil {
		return api.RepoGC{}, err
	}
	resp.Peer = c.id
	resp.Peername = c.config.Peername
	return resp, nil
}
