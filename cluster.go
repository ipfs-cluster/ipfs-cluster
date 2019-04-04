package ipfscluster

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/adder/local"
	"github.com/ipfs/ipfs-cluster/adder/sharding"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/pstoremgr"
	"github.com/ipfs/ipfs-cluster/rpcutil"
	"github.com/ipfs/ipfs-cluster/state"
	"github.com/ipfs/ipfs-cluster/version"

	cid "github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	host "github.com/libp2p/go-libp2p-host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"

	ocgorpc "github.com/lanzafame/go-libp2p-ocgorpc"
	trace "go.opencensus.io/trace"
)

// ReadyTimeout specifies the time before giving up
// during startup (waiting for consensus to be ready)
// It may need adjustment according to timeouts in the
// consensus layer.
var ReadyTimeout = 30 * time.Second

var pingMetricName = "ping"

// Cluster is the main IPFS cluster component. It provides
// the go-API for it and orchestrates the components that make up the system.
type Cluster struct {
	ctx    context.Context
	cancel func()

	id        peer.ID
	config    *Config
	host      host.Host
	dht       *dht.IpfsDHT
	datastore ds.Datastore

	rpcServer   *rpc.Server
	rpcClient   *rpc.Client
	peerManager *pstoremgr.Manager

	consensus Consensus
	apis      []API
	ipfs      IPFSConnector
	tracker   PinTracker
	monitor   PeerMonitor
	allocator PinAllocator
	informer  Informer
	tracer    Tracer

	doneCh  chan struct{}
	readyCh chan struct{}
	readyB  bool
	wg      sync.WaitGroup

	// peerAdd
	paMux sync.Mutex

	// shutdown function and related variables
	shutdownLock sync.Mutex
	shutdownB    bool
	removed      bool
}

// NewCluster builds a new IPFS Cluster peer. It initializes a LibP2P host,
// creates and RPC Server and client and sets up all components.
//
// The new cluster peer may still be performing initialization tasks when
// this call returns (consensus may still be bootstrapping). Use Cluster.Ready()
// if you need to wait until the peer is fully up.
func NewCluster(
	host host.Host,
	dht *dht.IpfsDHT,
	cfg *Config,
	datastore ds.Datastore,
	consensus Consensus,
	apis []API,
	ipfs IPFSConnector,
	tracker PinTracker,
	monitor PeerMonitor,
	allocator PinAllocator,
	informer Informer,
	tracer Tracer,
) (*Cluster, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	if host == nil {
		return nil, errors.New("cluster host is nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	listenAddrs := ""
	for _, addr := range host.Addrs() {
		listenAddrs += fmt.Sprintf("        %s/ipfs/%s\n", addr, host.ID().Pretty())
	}

	logger.Infof("IPFS Cluster v%s listening on:\n%s\n", version.Version, listenAddrs)

	// Note, we already loaded peers from peerstore into the host
	// in daemon.go.
	peerManager := pstoremgr.New(host, cfg.GetPeerstorePath())

	c := &Cluster{
		ctx:         ctx,
		cancel:      cancel,
		id:          host.ID(),
		config:      cfg,
		host:        host,
		dht:         dht,
		datastore:   datastore,
		consensus:   consensus,
		apis:        apis,
		ipfs:        ipfs,
		tracker:     tracker,
		monitor:     monitor,
		allocator:   allocator,
		informer:    informer,
		tracer:      tracer,
		peerManager: peerManager,
		shutdownB:   false,
		removed:     false,
		doneCh:      make(chan struct{}),
		readyCh:     make(chan struct{}),
		readyB:      false,
	}

	err = c.setupRPC()
	if err != nil {
		c.Shutdown(ctx)
		return nil, err
	}
	c.setupRPCClients()
	go func() {
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
	c.tracker.SetClient(c.rpcClient)
	c.ipfs.SetClient(c.rpcClient)
	for _, api := range c.apis {
		api.SetClient(c.rpcClient)
	}
	c.consensus.SetClient(c.rpcClient)
	c.monitor.SetClient(c.rpcClient)
	c.allocator.SetClient(c.rpcClient)
	c.informer.SetClient(c.rpcClient)
}

// syncWatcher loops and triggers StateSync and SyncAllLocal from time to time
func (c *Cluster) syncWatcher() {
	ctx, span := trace.StartSpan(c.ctx, "cluster/syncWatcher")
	defer span.End()

	stateSyncTicker := time.NewTicker(c.config.StateSyncInterval)
	syncTicker := time.NewTicker(c.config.IPFSSyncInterval)

	for {
		select {
		case <-stateSyncTicker.C:
			logger.Debug("auto-triggering StateSync()")
			c.StateSync(ctx)
		case <-syncTicker.C:
			logger.Debug("auto-triggering SyncAllLocal()")
			c.SyncAllLocal(ctx)
		case <-c.ctx.Done():
			stateSyncTicker.Stop()
			return
		}
	}
}

func (c *Cluster) sendInformerMetric(ctx context.Context) (*api.Metric, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/sendInformerMetric")
	defer span.End()

	metric := c.informer.GetMetric(ctx)
	metric.Peer = c.id
	return metric, c.monitor.PublishMetric(ctx, metric)
}

// pushInformerMetrics loops and publishes informers metrics using the
// cluster monitor. Metrics are pushed normally at a TTL/2 rate. If an error
// occurs, they are pushed at a TTL/4 rate.
func (c *Cluster) pushInformerMetrics(ctx context.Context) {
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

		metric, err := c.sendInformerMetric(ctx)

		if err != nil {
			if (retries % retryWarnMod) == 0 {
				logger.Errorf("error broadcasting metric: %s", err)
				retries++
			}
			// retry sooner
			timer.Reset(metric.GetTTL() / 4)
			continue
		}

		retries = 0
		// send metric again in TTL/2
		timer.Reset(metric.GetTTL() / 2)
	}
}

func (c *Cluster) pushPingMetrics(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "cluster/pushPingMetrics")
	defer span.End()

	ticker := time.NewTicker(c.config.MonitorPingInterval)
	for {
		metric := &api.Metric{
			Name:  pingMetricName,
			Peer:  c.id,
			Valid: true,
		}
		metric.SetTTL(c.config.MonitorPingInterval * 2)
		c.monitor.PublishMetric(ctx, metric)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// read the alerts channel from the monitor and triggers repins
func (c *Cluster) alertsHandler() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case alrt := <-c.monitor.Alerts():
			// only the leader handles alerts
			leader, err := c.consensus.Leader(c.ctx)
			if err == nil && leader == c.id {
				logger.Warningf(
					"Peer %s received alert for %s in %s",
					c.id, alrt.MetricName, alrt.Peer,
				)
				switch alrt.MetricName {
				case pingMetricName:
					c.repinFromPeer(c.ctx, alrt.Peer)
				}
			}
		}
	}
}

// detects any changes in the peerset and saves the configuration. When it
// detects that we have been removed from the peerset, it shuts down this peer.
func (c *Cluster) watchPeers() {
	ticker := time.NewTicker(c.config.PeerWatchInterval)

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			logger.Debugf("%s watching peers", c.id)
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
				logger.Infof("%s: removed from raft. Initiating shutdown", c.id.Pretty())
				c.removed = true
				go c.Shutdown(c.ctx)
				return
			}
		}
	}
}

// find all Cids pinned to a given peer and triggers re-pins on them.
func (c *Cluster) repinFromPeer(ctx context.Context, p peer.ID) {
	ctx, span := trace.StartSpan(ctx, "cluster/repinFromPeer")
	defer span.End()

	if c.config.DisableRepinning {
		logger.Warningf("repinning is disabled. Will not re-allocate cids from %s", p.Pretty())
		return
	}

	cState, err := c.consensus.State(ctx)
	if err != nil {
		logger.Warning(err)
		return
	}
	list, err := cState.List(ctx)
	if err != nil {
		logger.Warning(err)
		return
	}
	for _, pin := range list {
		if containsPeer(pin.Allocations, p) {
			_, ok, err := c.pin(ctx, pin, []peer.ID{p}, []peer.ID{}) // pin blacklisting this peer
			if ok && err == nil {
				logger.Infof("repinned %s out of %s", pin.Cid, p.Pretty())
			}
		}
	}
}

// run launches some go-routines which live throughout the cluster's life
func (c *Cluster) run() {
	go c.syncWatcher()
	go c.pushPingMetrics(c.ctx)
	go c.pushInformerMetrics(c.ctx)
	go c.watchPeers()
	go c.alertsHandler()
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
		// Consensus ready means the state is up to date so we can sync
		// it to the tracker. We ignore errors (normal when state
		// doesn't exist in new peers).
		c.StateSync(ctx)
	case <-c.ctx.Done():
		return
	}

	// Cluster is ready.

	// Bootstrap the DHT now that we possibly have some connections
	c.dht.Bootstrap(c.ctx)

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
			logger.Infof("    - %s", p.Pretty())
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

// Shutdown stops the IPFS cluster components
func (c *Cluster) Shutdown(ctx context.Context) error {
	_, span := trace.StartSpan(ctx, "cluster/Shutdown")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	c.shutdownLock.Lock()
	defer c.shutdownLock.Unlock()

	if c.shutdownB {
		logger.Debug("Cluster is already shutdown")
		return nil
	}

	logger.Info("shutting down Cluster")

	// Try to store peerset file for all known peers whatsoever
	// if we got ready (otherwise, don't overwrite anything)
	if c.readyB {
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
			logger.Warning("attempting to leave the cluster. This may take some seconds")
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

	for _, api := range c.apis {
		if err := api.Shutdown(ctx); err != nil {
			logger.Errorf("error stopping API: %s", err)
			return err
		}
	}

	if err := c.ipfs.Shutdown(ctx); err != nil {
		logger.Errorf("error stopping IPFS Connector: %s", err)
		return err
	}

	if err := c.tracker.Shutdown(ctx); err != nil {
		logger.Errorf("error stopping PinTracker: %s", err)
		return err
	}

	if err := c.tracer.Shutdown(ctx); err != nil {
		logger.Errorf("error stopping Tracer: %s", err)
		return err
	}

	c.cancel()
	c.host.Close() // Shutdown all network services
	c.wg.Wait()

	// Cleanly close the datastore
	if err := c.datastore.Close(); err != nil {
		logger.Errorf("error closing Datastore: %s", err)
		return err
	}

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
func (c *Cluster) ID(ctx context.Context) *api.ID {
	_, span := trace.StartSpan(ctx, "cluster/ID")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	// ignore error since it is included in response object
	ipfsID, err := c.ipfs.ID(ctx)
	if err != nil {
		ipfsID = &api.IPFSID{
			Error: err.Error(),
		}
	}
	var addrs []api.Multiaddr

	addrsSet := make(map[string]struct{}) // to filter dups
	for _, addr := range c.host.Addrs() {
		addrsSet[addr.String()] = struct{}{}
	}
	for k := range addrsSet {
		addr, _ := api.NewMultiaddr(k)
		addrs = append(addrs, api.MustLibp2pMultiaddrJoin(addr, c.id))
	}

	peers := []peer.ID{}
	// This method might get called very early by a remote peer
	// and might catch us when consensus is not set
	if c.consensus != nil {
		peers, _ = c.consensus.Peers(ctx)
	}

	return &api.ID{
		ID: c.id,
		//PublicKey:          c.host.Peerstore().PubKey(c.id),
		Addresses:             addrs,
		ClusterPeers:          peers,
		ClusterPeersAddresses: c.peerManager.PeersAddresses(peers),
		Version:               version.Version.String(),
		RPCProtocolVersion:    version.RPCProtocol,
		IPFS:                  ipfsID,
		Peername:              c.config.Peername,
	}
}

// RepoGC performs garbage collection in the cluster
func (c *Cluster) RepoGC(ctx context.Context) *api.IPFSRepoGc {
	_, span := trace.StartSpan(ctx, "cluster/GC")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	// ignore error since it is included in response object
	ipfsGC, err := c.ipfs.RepoGC(ctx)
	if err != nil {
		ipfsGC = &api.IPFSRepoGC{
			Error: err.Error(),
		}
	}

	return ipfsGC
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
	_, span := trace.StartSpan(ctx, "cluster/PeerAdd")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	// starting 10 nodes on the same box for testing
	// causes deadlock and a global lock here
	// seems to help.
	c.paMux.Lock()
	defer c.paMux.Unlock()
	logger.Debugf("peerAdd called with %s", pid.Pretty())

	// Let the consensus layer be aware of this peer
	err := c.consensus.AddPeer(ctx, pid)
	if err != nil {
		logger.Error(err)
		id := &api.ID{ID: pid, Error: err.Error()}
		return id, err
	}

	// Send a ping metric to the new node directly so
	// it knows about this one at least
	m := &api.Metric{
		Name:  pingMetricName,
		Peer:  c.id,
		Valid: true,
	}
	m.SetTTL(c.config.MonitorPingInterval * 2)
	err = c.rpcClient.CallContext(
		ctx,
		pid,
		"PeerMonitor",
		"LogMetric",
		m,
		&struct{}{},
	)
	if err != nil {
		logger.Warning(err)
	}

	// Ask the new peer to connect its IPFS daemon to the rest
	err = c.rpcClient.CallContext(
		ctx,
		pid,
		"IPFSConnector",
		"ConnectSwarms",
		struct{}{},
		&struct{}{},
	)
	if err != nil {
		logger.Warning(err)
	}

	id := &api.ID{}

	// wait up to 2 seconds for new peer to catch up
	// and return an up to date api.ID object.
	// otherwise it might not contain the current cluster peers
	// as it should.
	for i := 0; i < 20; i++ {
		id, _ = c.getIDForPeer(ctx, pid)
		ownPeers, err := c.consensus.Peers(ctx)
		if err != nil {
			break
		}
		newNodePeers := id.ClusterPeers
		added, removed := diffPeers(ownPeers, newNodePeers)
		if len(added) == 0 && len(removed) == 0 && containsPeer(ownPeers, pid) {
			break // the new peer has fully joined
		}
		time.Sleep(200 * time.Millisecond)
		logger.Debugf("%s addPeer: retrying to get ID from %s",
			c.id.Pretty(), pid.Pretty())
	}
	logger.Info("Peer added ", pid.Pretty())
	return id, nil
}

// PeerRemove removes a peer from this Cluster.
//
// The peer will be removed from the consensus peerset.
// This may first trigger repinnings for all content if not disabled.
func (c *Cluster) PeerRemove(ctx context.Context, pid peer.ID) error {
	_, span := trace.StartSpan(ctx, "cluster/PeerRemove")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	// We need to repin before removing the peer, otherwise, it won't
	// be able to submit the pins.
	logger.Infof("re-allocating all CIDs directly associated to %s", pid)
	c.repinFromPeer(ctx, pid)

	err := c.consensus.RmPeer(ctx, pid)
	if err != nil {
		logger.Error(err)
		return err
	}
	logger.Info("Peer removed ", pid.Pretty())
	return nil
}

// Join adds this peer to an existing cluster by bootstrapping to a
// given multiaddress. It works by calling PeerAdd on the destination
// cluster and making sure that the new peer is ready to discover and contact
// the rest.
func (c *Cluster) Join(ctx context.Context, addr ma.Multiaddr) error {
	_, span := trace.StartSpan(ctx, "cluster/Join")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	logger.Debugf("Join(%s)", addr)

	pid, _, err := api.Libp2pMultiaddrSplit(addr)
	if err != nil {
		logger.Error(err)
		return err
	}

	// Bootstrap to myself
	if pid == c.id {
		return nil
	}

	// Add peer to peerstore so we can talk to it
	c.peerManager.ImportPeer(addr, true)

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

	// We need to trigger a DHT bootstrap asap for this peer to not be
	// lost if the peer it bootstrapped to goes down. We do this manually
	// by triggering 1 round of bootstrap in the background.
	// Note that our regular bootstrap process is still running in the
	// background since we created the cluster.
	go func() {
		c.dht.BootstrapOnce(ctx, dht.DefaultBootstrapConfig)
	}()

	// wait for leader and for state to catch up
	// then sync
	err = c.consensus.WaitForSync(ctx)
	if err != nil {
		logger.Error(err)
		return err
	}

	c.StateSync(ctx)

	logger.Infof("%s: joined %s's cluster", c.id.Pretty(), pid.Pretty())
	return nil
}

// StateSync syncs the consensus state to the Pin Tracker, ensuring
// that every Cid in the shared state is tracked and that the Pin Tracker
// is not tracking more Cids than it should.
func (c *Cluster) StateSync(ctx context.Context) error {
	_, span := trace.StartSpan(ctx, "cluster/StateSync")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	cState, err := c.consensus.State(ctx)
	if err != nil {
		return err
	}

	logger.Debug("syncing state to tracker")
	clusterPins, err := cState.List(ctx)
	if err != nil {
		return err
	}

	trackedPins := c.tracker.StatusAll(ctx)
	trackedPinsMap := make(map[string]int)
	for i, tpin := range trackedPins {
		trackedPinsMap[tpin.Cid.String()] = i
	}

	// Track items which are not tracked
	for _, pin := range clusterPins {
		_, tracked := trackedPinsMap[pin.Cid.String()]
		if !tracked {
			logger.Debugf("StateSync: tracking %s, part of the shared state", pin.Cid)
			c.tracker.Track(ctx, pin)
		}
	}

	// a. Untrack items which should not be tracked
	// b. Track items which should not be remote as local
	// c. Track items which should not be local as remote
	for _, p := range trackedPins {
		pCid := p.Cid
		currentPin, err := cState.Get(ctx, pCid)
		if err != nil && err != state.ErrNotFound {
			return err
		}

		if err == state.ErrNotFound {
			logger.Debugf("StateSync: untracking %s: not part of shared state", pCid)
			c.tracker.Untrack(ctx, pCid)
			continue
		}

		allocatedHere := containsPeer(currentPin.Allocations, c.id) || currentPin.ReplicationFactorMin == -1

		switch {
		case p.Status == api.TrackerStatusRemote && allocatedHere:
			logger.Debugf("StateSync: Tracking %s locally (currently remote)", pCid)
			c.tracker.Track(ctx, currentPin)
		case p.Status == api.TrackerStatusPinned && !allocatedHere:
			logger.Debugf("StateSync: Tracking %s as remote (currently local)", pCid)
			c.tracker.Track(ctx, currentPin)
		}
	}

	return nil
}

// StatusAll returns the GlobalPinInfo for all tracked Cids in all peers.
// If an error happens, the slice will contain as much information as
// could be fetched from other peers.
func (c *Cluster) StatusAll(ctx context.Context) ([]*api.GlobalPinInfo, error) {
	_, span := trace.StartSpan(ctx, "cluster/StatusAll")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	return c.globalPinInfoSlice(ctx, "PinTracker", "StatusAll")
}

// StatusAllLocal returns the PinInfo for all the tracked Cids in this peer.
func (c *Cluster) StatusAllLocal(ctx context.Context) []*api.PinInfo {
	_, span := trace.StartSpan(ctx, "cluster/StatusAllLocal")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	return c.tracker.StatusAll(ctx)
}

// Status returns the GlobalPinInfo for a given Cid as fetched from all
// current peers. If an error happens, the GlobalPinInfo should contain
// as much information as could be fetched from the other peers.
func (c *Cluster) Status(ctx context.Context, h cid.Cid) (*api.GlobalPinInfo, error) {
	_, span := trace.StartSpan(ctx, "cluster/Status")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	return c.globalPinInfoCid(ctx, "PinTracker", "Status", h)
}

// StatusLocal returns this peer's PinInfo for a given Cid.
func (c *Cluster) StatusLocal(ctx context.Context, h cid.Cid) *api.PinInfo {
	_, span := trace.StartSpan(ctx, "cluster/StatusLocal")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	return c.tracker.Status(ctx, h)
}

// SyncAll triggers SyncAllLocal() operations in all cluster peers, making sure
// that the state of tracked items matches the state reported by the IPFS daemon
// and returning the results as GlobalPinInfo. If an error happens, the slice
// will contain as much information as could be fetched from the peers.
func (c *Cluster) SyncAll(ctx context.Context) ([]*api.GlobalPinInfo, error) {
	_, span := trace.StartSpan(ctx, "cluster/SyncAll")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	return c.globalPinInfoSlice(ctx, "Cluster", "SyncAllLocal")
}

// SyncAllLocal makes sure that the current state for all tracked items
// in this peer matches the state reported by the IPFS daemon.
//
// SyncAllLocal returns the list of PinInfo that where updated because of
// the operation, along with those in error states.
func (c *Cluster) SyncAllLocal(ctx context.Context) ([]*api.PinInfo, error) {
	_, span := trace.StartSpan(ctx, "cluster/SyncAllLocal")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	syncedItems, err := c.tracker.SyncAll(ctx)
	// Despite errors, tracker provides synced items that we can provide.
	// They encapsulate the error.
	if err != nil {
		logger.Error("tracker.Sync() returned with error: ", err)
		logger.Error("Is the ipfs daemon running?")
	}
	return syncedItems, err
}

// Sync triggers a SyncLocal() operation for a given Cid.
// in all cluster peers.
func (c *Cluster) Sync(ctx context.Context, h cid.Cid) (*api.GlobalPinInfo, error) {
	_, span := trace.StartSpan(ctx, "cluster/Sync")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	return c.globalPinInfoCid(ctx, "Cluster", "SyncLocal", h)
}

// used for RecoverLocal and SyncLocal.
func (c *Cluster) localPinInfoOp(
	ctx context.Context,
	h cid.Cid,
	f func(context.Context, cid.Cid) (*api.PinInfo, error),
) (pInfo *api.PinInfo, err error) {
	ctx, span := trace.StartSpan(ctx, "cluster/localPinInfoOp")
	defer span.End()

	cids, err := c.cidsFromMetaPin(ctx, h)
	if err != nil {
		return nil, err
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

// SyncLocal performs a local sync operation for the given Cid. This will
// tell the tracker to verify the status of the Cid against the IPFS daemon.
// It returns the updated PinInfo for the Cid.
func (c *Cluster) SyncLocal(ctx context.Context, h cid.Cid) (pInfo *api.PinInfo, err error) {
	_, span := trace.StartSpan(ctx, "cluster/SyncLocal")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	return c.localPinInfoOp(ctx, h, c.tracker.Sync)
}

// RecoverAllLocal triggers a RecoverLocal operation for all Cids tracked
// by this peer.
func (c *Cluster) RecoverAllLocal(ctx context.Context) ([]*api.PinInfo, error) {
	_, span := trace.StartSpan(ctx, "cluster/RecoverAllLocal")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	return c.tracker.RecoverAll(ctx)
}

// Recover triggers a recover operation for a given Cid in all
// cluster peers.
func (c *Cluster) Recover(ctx context.Context, h cid.Cid) (*api.GlobalPinInfo, error) {
	_, span := trace.StartSpan(ctx, "cluster/Recover")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	return c.globalPinInfoCid(ctx, "PinTracker", "Recover", h)
}

// RecoverLocal triggers a recover operation for a given Cid in this peer only.
// It returns the updated PinInfo, after recovery.
func (c *Cluster) RecoverLocal(ctx context.Context, h cid.Cid) (pInfo *api.PinInfo, err error) {
	_, span := trace.StartSpan(ctx, "cluster/RecoverLocal")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	return c.localPinInfoOp(ctx, h, c.tracker.Recover)
}

// Pins returns the list of Cids managed by Cluster and which are part
// of the current global state. This is the source of truth as to which
// pins are managed and their allocation, but does not indicate if
// the item is successfully pinned. For that, use StatusAll().
func (c *Cluster) Pins(ctx context.Context) ([]*api.Pin, error) {
	_, span := trace.StartSpan(ctx, "cluster/Pins")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	cState, err := c.consensus.State(ctx)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	return cState.List(ctx)
}

// PinGet returns information for a single Cid managed by Cluster.
// The information is obtained from the current global state. The
// returned api.Pin provides information about the allocations
// assigned for the requested Cid, but does not indicate if
// the item is successfully pinned. For that, use Status(). PinGet
// returns an error if the given Cid is not part of the global state.
func (c *Cluster) PinGet(ctx context.Context, h cid.Cid) (*api.Pin, error) {
	_, span := trace.StartSpan(ctx, "cluster/PinGet")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	st, err := c.consensus.State(ctx)
	if err != nil {
		return nil, err
	}
	pin, err := st.Get(ctx, h)
	if err != nil {
		return nil, err
	}
	return pin, nil
}

// Pin makes the cluster Pin a Cid. This implies adding the Cid
// to the IPFS Cluster peers shared-state. Depending on the cluster
// pinning strategy, the PinTracker may then request the IPFS daemon
// to pin the Cid.
//
// Pin returns an error if the operation could not be persisted
// to the global state. Pin does not reflect the success or failure
// of underlying IPFS daemon pinning operations.
//
// If the argument's allocations are non-empty then these peers are pinned with
// priority over other peers in the cluster.  If the max repl factor is less
// than the size of the specified peerset then peers are chosen from this set
// in allocation order.  If the min repl factor is greater than the size of
// this set then the remaining peers are allocated in order from the rest of
// the cluster.  Priority allocations are best effort.  If any priority peers
// are unavailable then Pin will simply allocate from the rest of the cluster.
func (c *Cluster) Pin(ctx context.Context, pin *api.Pin) error {
	_, span := trace.StartSpan(ctx, "cluster/Pin")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)
	_, _, err := c.pin(ctx, pin, []peer.ID{}, pin.UserAllocations)
	return err
}

// sets the default replication factor in a pin when it's set to 0
func (c *Cluster) setupReplicationFactor(pin *api.Pin) error {
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

	return isReplicationFactorValid(rplMin, rplMax)
}

// basic checks on the pin type to check it's well-formed.
func checkPinType(pin *api.Pin) error {
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
		if pin.Allocations != nil && len(pin.Allocations) != 0 {
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
func (c *Cluster) setupPin(ctx context.Context, pin *api.Pin) error {
	ctx, span := trace.StartSpan(ctx, "cluster/setupPin")
	defer span.End()

	err := c.setupReplicationFactor(pin)
	if err != nil {
		return err
	}

	existing, err := c.PinGet(ctx, pin.Cid)
	if err != nil && err != state.ErrNotFound {
		return err
	}

	if existing != nil && existing.Type != pin.Type {
		msg := "cannot repin CID with different tracking method, "
		msg += "clear state with pin rm to proceed. "
		msg += "New: %s. Was: %s"
		return fmt.Errorf(msg, pin.Type, existing.Type)
	}

	return checkPinType(pin)
}

// pin performs the actual pinning and supports a blacklist to be
// able to evacuate a node and returns the pin object that it tried to pin, whether the pin was submitted
// to the consensus layer or skipped (due to error or to the fact
// that it was already valid) and errror.
func (c *Cluster) pin(ctx context.Context, pin *api.Pin, blacklist []peer.ID, prioritylist []peer.ID) (*api.Pin, bool, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/pin")
	defer span.End()

	if pin.Cid == cid.Undef {
		return pin, false, errors.New("bad pin object")
	}

	// setup pin might produce some side-effects to our pin
	err := c.setupPin(ctx, pin)
	if err != nil {
		return pin, false, err
	}
	if pin.Type == api.MetaType {
		return pin, true, c.consensus.LogPin(ctx, pin)
	}

	allocs, err := c.allocate(
		ctx,
		pin.Cid,
		pin.ReplicationFactorMin,
		pin.ReplicationFactorMax,
		blacklist,
		prioritylist,
	)
	if err != nil {
		return pin, false, err
	}
	pin.Allocations = allocs

	// Equals can handle nil objects.
	if curr, _ := c.PinGet(ctx, pin.Cid); curr.Equals(pin) {
		// skip pinning
		logger.Debugf("pinning %s skipped: already correctly allocated", pin.Cid)
		return pin, false, nil
	}

	if len(pin.Allocations) == 0 {
		logger.Infof("pinning %s everywhere:", pin.Cid)
	} else {
		logger.Infof("pinning %s on %s:", pin.Cid, pin.Allocations)
	}

	return pin, true, c.consensus.LogPin(ctx, pin)
}

func (c *Cluster) unpin(ctx context.Context, h cid.Cid) (*api.Pin, error) {
	_, span := trace.StartSpan(ctx, "cluster/unpin")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	logger.Info("IPFS cluster unpinning:", h)
	pin, err := c.PinGet(ctx, h)
	if err != nil {
		return nil, err
	}

	switch pin.Type {
	case api.DataType:
		return pin, c.consensus.LogUnpin(ctx, pin)
	case api.ShardType:
		err := "cannot unpin a shard direclty. Unpin content root CID instead."
		return pin, errors.New(err)
	case api.MetaType:
		// Unpin cluster dag and referenced shards
		err := c.unpinClusterDag(pin)
		if err != nil {
			return pin, err
		}
		return pin, c.consensus.LogUnpin(ctx, pin)
	case api.ClusterDAGType:
		err := "cannot unpin a Cluster DAG directly. Unpin content root CID instead."
		return pin, errors.New(err)
	default:
		return pin, errors.New("unrecognized pin type")
	}
}

// Unpin makes the cluster Unpin a Cid. This implies adding the Cid
// to the IPFS Cluster peers shared-state.
//
// Unpin returns an error if the operation could not be persisted
// to the global state. Unpin does not reflect the success or failure
// of underlying IPFS daemon unpinning operations.
func (c *Cluster) Unpin(ctx context.Context, h cid.Cid) error {
	_, span := trace.StartSpan(ctx, "cluster/Unpin")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)
	_, err := c.unpin(ctx, h)
	return err
}

// unpinClusterDag unpins the clusterDAG metadata node and the shard metadata
// nodes that it references.  It handles the case where multiple parents
// reference the same metadata node, only unpinning those nodes without
// existing references
func (c *Cluster) unpinClusterDag(metaPin *api.Pin) error {
	ctx, span := trace.StartSpan(c.ctx, "cluster/unpinClusterDag")
	defer span.End()

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

// PinPath pins an CID resolved from its IPFS Path. It returns the resolved
// Pin object.
func (c *Cluster) PinPath(ctx context.Context, path *api.PinPath) (*api.Pin, error) {
	_, span := trace.StartSpan(ctx, "cluster/PinPath")
	defer span.End()

	ctx = trace.NewContext(c.ctx, span)
	ci, err := c.ipfs.Resolve(ctx, path.Path)
	if err != nil {
		return nil, err
	}

	p := api.PinCid(ci)
	p.PinOptions = path.PinOptions
	p, _, err = c.pin(ctx, p, []peer.ID{}, p.UserAllocations)
	return p, err
}

// UnpinPath unpins a CID resolved from its IPFS Path. If returns the
// previously pinned Pin object.
func (c *Cluster) UnpinPath(ctx context.Context, path string) (*api.Pin, error) {
	_, span := trace.StartSpan(ctx, "cluster/UnpinPath")
	defer span.End()

	ctx = trace.NewContext(c.ctx, span)
	ci, err := c.ipfs.Resolve(ctx, path)
	if err != nil {
		return nil, err
	}

	return c.unpin(ctx, ci)
}

// AddFile adds a file to the ipfs daemons of the cluster.  The ipfs importer
// pipeline is used to DAGify the file.  Depending on input parameters this
// DAG can be added locally to the calling cluster peer's ipfs repo, or
// sharded across the entire cluster.
func (c *Cluster) AddFile(reader *multipart.Reader, params *api.AddParams) (cid.Cid, error) {
	// TODO: add context param and tracing
	var dags adder.ClusterDAGService
	if params.Shard {
		dags = sharding.New(c.rpcClient, params.PinOptions, nil)
	} else {
		dags = local.New(c.rpcClient, params.PinOptions)
	}
	add := adder.New(dags, params, nil)
	return add.FromMultipart(c.ctx, reader)
}

// Version returns the current IPFS Cluster version.
func (c *Cluster) Version() string {
	return version.Version.String()
}

// Peers returns the IDs of the members of this Cluster.
func (c *Cluster) Peers(ctx context.Context) []*api.ID {
	_, span := trace.StartSpan(ctx, "cluster/Peers")
	defer span.End()
	ctx = trace.NewContext(c.ctx, span)

	members, err := c.consensus.Peers(ctx)
	if err != nil {
		logger.Error(err)
		logger.Error("an empty list of peers will be returned")
		return []*api.ID{}
	}
	lenMembers := len(members)

	peers := make([]*api.ID, lenMembers, lenMembers)

	ctxs, cancels := rpcutil.CtxsWithCancel(ctx, lenMembers)
	defer rpcutil.MultiCancel(cancels)

	errs := c.rpcClient.MultiCall(
		ctxs,
		members,
		"Cluster",
		"ID",
		struct{}{},
		rpcutil.CopyIDsToIfaces(peers),
	)

	for i, err := range errs {
		if err != nil {
			peers[i] = &api.ID{}
			peers[i].ID = members[i]
			peers[i].Error = err.Error()
		}
	}

	return peers
}

func (c *Cluster) globalPinInfoCid(ctx context.Context, comp, method string, h cid.Cid) (*api.GlobalPinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/globalPinInfoCid")
	defer span.End()

	pin := &api.GlobalPinInfo{
		Cid:     h,
		PeerMap: make(map[string]*api.PinInfo),
	}

	members, err := c.consensus.Peers(ctx)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	lenMembers := len(members)

	replies := make([]*api.PinInfo, lenMembers, lenMembers)
	ctxs, cancels := rpcutil.CtxsWithCancel(ctx, lenMembers)
	defer rpcutil.MultiCancel(cancels)

	errs := c.rpcClient.MultiCall(
		ctxs,
		members,
		comp,
		method,
		h,
		rpcutil.CopyPinInfoToIfaces(replies),
	)

	for i, r := range replies {
		e := errs[i]

		// No error. Parse and continue
		if e == nil {
			pin.PeerMap[peer.IDB58Encode(members[i])] = r
			continue
		}

		// Deal with error cases (err != nil): wrap errors in PinInfo
		logger.Errorf("%s: error in broadcast response from %s: %s ", c.id, members[i], e)
		pin.PeerMap[peer.IDB58Encode(members[i])] = &api.PinInfo{
			Cid:      h,
			Peer:     members[i],
			PeerName: members[i].String(),
			Status:   api.TrackerStatusClusterError,
			TS:       time.Now(),
			Error:    e.Error(),
		}
	}

	return pin, nil
}

func (c *Cluster) globalPinInfoSlice(ctx context.Context, comp, method string) ([]*api.GlobalPinInfo, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/globalPinInfoSlice")
	defer span.End()

	infos := make([]*api.GlobalPinInfo, 0)
	fullMap := make(map[cid.Cid]*api.GlobalPinInfo)

	members, err := c.consensus.Peers(ctx)
	if err != nil {
		logger.Error(err)
		return nil, err
	}
	lenMembers := len(members)

	replies := make([][]*api.PinInfo, lenMembers, lenMembers)

	ctxs, cancels := rpcutil.CtxsWithCancel(ctx, lenMembers)
	defer rpcutil.MultiCancel(cancels)

	errs := c.rpcClient.MultiCall(
		ctxs,
		members,
		comp,
		method,
		struct{}{},
		rpcutil.CopyPinInfoSliceToIfaces(replies),
	)

	mergePins := func(pins []*api.PinInfo) {
		for _, p := range pins {
			if p == nil {
				continue
			}
			item, ok := fullMap[p.Cid]
			if !ok {
				fullMap[p.Cid] = &api.GlobalPinInfo{
					Cid: p.Cid,
					PeerMap: map[string]*api.PinInfo{
						peer.IDB58Encode(p.Peer): p,
					},
				}
			} else {
				item.PeerMap[peer.IDB58Encode(p.Peer)] = p
			}
		}
	}

	erroredPeers := make(map[peer.ID]string)
	for i, r := range replies {
		if e := errs[i]; e != nil { // This error must come from not being able to contact that cluster member
			logger.Errorf("%s: error in broadcast response from %s: %s ", c.id, members[i], e)
			erroredPeers[members[i]] = e.Error()
		} else {
			mergePins(r)
		}
	}

	// Merge any errors
	for p, msg := range erroredPeers {
		for c := range fullMap {
			fullMap[c].PeerMap[peer.IDB58Encode(p)] = &api.PinInfo{
				Cid:    c,
				Peer:   p,
				Status: api.TrackerStatusClusterError,
				TS:     time.Now(),
				Error:  msg,
			}
		}
	}

	for _, v := range fullMap {
		infos = append(infos, v)
	}

	return infos, nil
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
func (c *Cluster) cidsFromMetaPin(ctx context.Context, h cid.Cid) ([]cid.Cid, error) {
	ctx, span := trace.StartSpan(ctx, "cluster/cidsFromMetaPin")
	defer span.End()

	cState, err := c.consensus.State(ctx)
	if err != nil {
		return nil, err
	}

	list := []cid.Cid{h}

	pin, err := cState.Get(ctx, h)
	if err != nil {
		return nil, err
	}
	if pin == nil {
		return list, nil
	}

	if pin.Type != api.MetaType {
		return list, nil
	}

	if pin.Reference == nil {
		return nil, errors.New("metaPin.Reference is unset")
	}
	list = append([]cid.Cid{*pin.Reference}, list...)
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
		list = append([]cid.Cid{l.Cid}, list...)
	}

	return list, nil
}

// diffPeers returns the peerIDs added and removed from peers2 in relation to
// peers1
func diffPeers(peers1, peers2 []peer.ID) (added, removed []peer.ID) {
	m1 := make(map[peer.ID]struct{})
	m2 := make(map[peer.ID]struct{})
	added = make([]peer.ID, 0)
	removed = make([]peer.ID, 0)
	if peers1 == nil && peers2 == nil {
		return
	}
	if peers1 == nil {
		added = peers2
		return
	}
	if peers2 == nil {
		removed = peers1
		return
	}

	for _, p := range peers1 {
		m1[p] = struct{}{}
	}
	for _, p := range peers2 {
		m2[p] = struct{}{}
	}
	for k := range m1 {
		_, ok := m2[k]
		if !ok {
			removed = append(removed, k)
		}
	}
	for k := range m2 {
		_, ok := m1[k]
		if !ok {
			added = append(added, k)
		}
	}
	return
}
