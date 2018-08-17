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

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	host "github.com/libp2p/go-libp2p-host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	peer "github.com/libp2p/go-libp2p-peer"
	routedhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	ma "github.com/multiformats/go-multiaddr"
)

// ReadyTimeout specifies the time before giving up
// during startup (waiting for consensus to be ready)
// It may need adjustment according to timeouts in the
// consensus layer.
var ReadyTimeout = 30 * time.Second

// Cluster is the main IPFS cluster component. It provides
// the go-API for it and orchestrates the components that make up the system.
type Cluster struct {
	ctx    context.Context
	cancel func()

	id          peer.ID
	config      *Config
	host        host.Host
	dht         *dht.IpfsDHT
	rpcServer   *rpc.Server
	rpcClient   *rpc.Client
	peerManager *pstoremgr.Manager

	consensus Consensus
	api       API
	ipfs      IPFSConnector
	state     state.State
	tracker   PinTracker
	monitor   PeerMonitor
	allocator PinAllocator
	informer  Informer

	shutdownLock sync.Mutex
	shutdownB    bool
	removed      bool
	doneCh       chan struct{}
	readyCh      chan struct{}
	readyB       bool
	wg           sync.WaitGroup

	paMux sync.Mutex
}

// NewCluster builds a new IPFS Cluster peer. It initializes a LibP2P host,
// creates and RPC Server and client and sets up all components.
//
// The new cluster peer may still be performing initialization tasks when
// this call returns (consensus may still be bootstrapping). Use Cluster.Ready()
// if you need to wait until the peer is fully up.
func NewCluster(
	host host.Host,
	cfg *Config,
	consensus Consensus,
	api API,
	ipfs IPFSConnector,
	st state.State,
	tracker PinTracker,
	monitor PeerMonitor,
	allocator PinAllocator,
	informer Informer,
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

	if c := Commit; len(c) >= 8 {
		logger.Infof("IPFS Cluster v%s-%s listening on:\n%s\n", Version, Commit[0:8], listenAddrs)
	} else {
		logger.Infof("IPFS Cluster v%s listening on:\n%s\n", Version, listenAddrs)
	}

	// Note, we already loaded peers from peerstore into the host
	// in daemon.go.
	peerManager := pstoremgr.New(host, cfg.GetPeerstorePath())

	idht, err := dht.New(ctx, host)
	if err != nil {
		cancel()
		return nil, err
	}

	// Let the DHT be maintained regularly
	err = idht.Bootstrap(ctx)
	if err != nil {
		cancel()
		return nil, err
	}

	rHost := routedhost.Wrap(host, idht)

	c := &Cluster{
		ctx:         ctx,
		cancel:      cancel,
		id:          host.ID(),
		config:      cfg,
		host:        rHost,
		dht:         idht,
		consensus:   consensus,
		api:         api,
		ipfs:        ipfs,
		state:       st,
		tracker:     tracker,
		monitor:     monitor,
		allocator:   allocator,
		informer:    informer,
		peerManager: peerManager,
		shutdownB:   false,
		removed:     false,
		doneCh:      make(chan struct{}),
		readyCh:     make(chan struct{}),
		readyB:      false,
	}

	err = c.setupRPC()
	if err != nil {
		c.Shutdown()
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
	rpcServer := rpc.NewServer(c.host, RPCProtocol)
	err := rpcServer.RegisterName("Cluster", &RPCAPI{c})
	if err != nil {
		return err
	}
	c.rpcServer = rpcServer
	rpcClient := rpc.NewClientWithServer(c.host, RPCProtocol, rpcServer)
	c.rpcClient = rpcClient
	return nil
}

func (c *Cluster) setupRPCClients() {
	c.tracker.SetClient(c.rpcClient)
	c.ipfs.SetClient(c.rpcClient)
	c.api.SetClient(c.rpcClient)
	c.consensus.SetClient(c.rpcClient)
	c.monitor.SetClient(c.rpcClient)
	c.allocator.SetClient(c.rpcClient)
	c.informer.SetClient(c.rpcClient)
}

// syncWatcher loops and triggers StateSync and SyncAllLocal from time to time
func (c *Cluster) syncWatcher() {
	stateSyncTicker := time.NewTicker(c.config.StateSyncInterval)
	syncTicker := time.NewTicker(c.config.IPFSSyncInterval)

	for {
		select {
		case <-stateSyncTicker.C:
			logger.Debug("auto-triggering StateSync()")
			c.StateSync()
		case <-syncTicker.C:
			logger.Debug("auto-triggering SyncAllLocal()")
			c.SyncAllLocal()
		case <-c.ctx.Done():
			stateSyncTicker.Stop()
			return
		}
	}
}

func (c *Cluster) sendInformerMetric() (api.Metric, error) {
	metric := c.informer.GetMetric()
	metric.Peer = c.id
	return metric, c.monitor.PublishMetric(metric)
}

// pushInformerMetrics loops and publishes informers metrics using the
// cluster monitor. Metrics are pushed normally at a TTL/2 rate. If an error
// occurs, they are pushed at a TTL/4 rate.
func (c *Cluster) pushInformerMetrics() {
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
		case <-c.ctx.Done():
			return
		case <-timer.C:
			// wait
		}

		metric, err := c.sendInformerMetric()

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

func (c *Cluster) pushPingMetrics() {
	ticker := time.NewTicker(c.config.MonitorPingInterval)
	for {
		metric := api.Metric{
			Name:  "ping",
			Peer:  c.id,
			Valid: true,
		}
		metric.SetTTL(c.config.MonitorPingInterval * 2)
		c.monitor.PublishMetric(metric)

		select {
		case <-c.ctx.Done():
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
			leader, err := c.consensus.Leader()
			if err == nil && leader == c.id {
				logger.Warningf("Peer %s received alert for %s in %s", c.id, alrt.MetricName, alrt.Peer.Pretty())
				switch alrt.MetricName {
				case "ping":
					c.repinFromPeer(alrt.Peer)
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
			peers, err := c.consensus.Peers()
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
				logger.Infof("%s: removed from raft. Initiating shutdown", c.id.Pretty())
				c.removed = true
				go c.Shutdown()
				return
			}
		}
	}
}

// find all Cids pinned to a given peer and triggers re-pins on them.
func (c *Cluster) repinFromPeer(p peer.ID) {
	if c.config.DisableRepinning {
		logger.Warningf("repinning is disabled. Will not re-allocate cids from %s", p.Pretty())
		return
	}

	cState, err := c.consensus.State()
	if err != nil {
		logger.Warning(err)
		return
	}
	list := cState.List()
	for _, pin := range list {
		if containsPeer(pin.Allocations, p) {
			ok, err := c.pin(pin, []peer.ID{p}, []peer.ID{}) // pin blacklisting this peer
			if ok && err == nil {
				logger.Infof("repinned %s out of %s", pin.Cid, p.Pretty())
			}
		}
	}
}

// run launches some go-routines which live throughout the cluster's life
func (c *Cluster) run() {
	go c.syncWatcher()
	go c.pushPingMetrics()
	go c.pushInformerMetrics()
	go c.watchPeers()
	go c.alertsHandler()
}

func (c *Cluster) ready(timeout time.Duration) {
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
		c.Shutdown()
		return
	case <-c.consensus.Ready():
		// Consensus ready means the state is up to date so we can sync
		// it to the tracker. We ignore errors (normal when state
		// doesn't exist in new peers).
		c.StateSync()
	case <-c.ctx.Done():
		return
	}

	// Cluster is ready.
	peers, err := c.consensus.Peers()
	if err != nil {
		logger.Error(err)
		c.Shutdown()
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
func (c *Cluster) Shutdown() error {
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
		_, err := c.consensus.Peers()
		if err == nil {
			// best effort
			logger.Warning("attempting to leave the cluster. This may take some seconds")
			err := c.consensus.RmPeer(c.id)
			if err != nil {
				logger.Error("leaving cluster: " + err.Error())
			}
		}
	}

	if con := c.consensus; con != nil {
		if err := con.Shutdown(); err != nil {
			logger.Errorf("error stopping consensus: %s", err)
			return err
		}
	}

	// We left the cluster or were removed. Destroy the Raft state.
	if c.removed && c.readyB {
		err := c.consensus.Clean()
		if err != nil {
			logger.Error("cleaning consensus: ", err)
		}
	}

	if err := c.monitor.Shutdown(); err != nil {
		logger.Errorf("error stopping monitor: %s", err)
		return err
	}

	if err := c.api.Shutdown(); err != nil {
		logger.Errorf("error stopping API: %s", err)
		return err
	}
	if err := c.ipfs.Shutdown(); err != nil {
		logger.Errorf("error stopping IPFS Connector: %s", err)
		return err
	}

	if err := c.tracker.Shutdown(); err != nil {
		logger.Errorf("error stopping PinTracker: %s", err)
		return err
	}

	c.cancel()
	c.host.Close() // Shutdown all network services
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
func (c *Cluster) ID() api.ID {
	// ignore error since it is included in response object
	ipfsID, _ := c.ipfs.ID()
	var addrs []ma.Multiaddr

	addrsSet := make(map[string]struct{}) // to filter dups
	for _, addr := range c.host.Addrs() {
		addrsSet[addr.String()] = struct{}{}
	}
	for k := range addrsSet {
		addr, _ := ma.NewMultiaddr(k)
		addrs = append(addrs, api.MustLibp2pMultiaddrJoin(addr, c.id))
	}

	peers := []peer.ID{}
	// This method might get called very early by a remote peer
	// and might catch us when consensus is not set
	if c.consensus != nil {
		peers, _ = c.consensus.Peers()
	}

	return api.ID{
		ID: c.id,
		//PublicKey:          c.host.Peerstore().PubKey(c.id),
		Addresses:             addrs,
		ClusterPeers:          peers,
		ClusterPeersAddresses: c.peerManager.PeersAddresses(peers),
		Version:               Version,
		Commit:                Commit,
		RPCProtocolVersion:    RPCProtocol,
		IPFS:                  ipfsID,
		Peername:              c.config.Peername,
	}
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
func (c *Cluster) PeerAdd(pid peer.ID) (api.ID, error) {
	// starting 10 nodes on the same box for testing
	// causes deadlock and a global lock here
	// seems to help.
	c.paMux.Lock()
	defer c.paMux.Unlock()
	logger.Debugf("peerAdd called with %s", pid.Pretty())

	// Log the new peer in the log so everyone gets it.
	err := c.consensus.AddPeer(pid)
	if err != nil {
		logger.Error(err)
		id := api.ID{ID: pid, Error: err.Error()}
		return id, err
	}

	// Ask the new peer to connect its IPFS daemon to the rest
	err = c.rpcClient.Call(pid,
		"Cluster",
		"IPFSConnectSwarms",
		struct{}{},
		&struct{}{})
	if err != nil {
		logger.Error(err)
	}

	id := api.ID{}

	// wait up to 2 seconds for new peer to catch up
	// and return an up to date api.ID object.
	// otherwise it might not contain the current cluster peers
	// as it should.
	for i := 0; i < 20; i++ {
		id, _ = c.getIDForPeer(pid)
		ownPeers, err := c.consensus.Peers()
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
func (c *Cluster) PeerRemove(pid peer.ID) error {
	// We need to repin before removing the peer, otherwise, it won't
	// be able to submit the pins.
	logger.Infof("re-allocating all CIDs directly associated to %s", pid)
	c.repinFromPeer(pid)

	err := c.consensus.RmPeer(pid)
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
func (c *Cluster) Join(addr ma.Multiaddr) error {
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
	var myID api.IDSerial
	err = c.rpcClient.Call(pid,
		"Cluster",
		"PeerAdd",
		peer.IDB58Encode(c.id),
		&myID)
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
		ch := make(chan time.Time)
		bstCfg := dht.DefaultBootstrapConfig
		dhtBstCtx, cancel := context.WithTimeout(c.ctx, bstCfg.Timeout*2)
		defer cancel()
		proc, err := c.dht.BootstrapOnSignal(bstCfg, ch)
		if err != nil {
			logger.Error(err)
		}
		ch <- time.Now() // boostrap
		defer close(ch)
		select {
		case <-dhtBstCtx.Done(): // shut down the process
			proc.Close()
		}
	}()

	// wait for leader and for state to catch up
	// then sync
	err = c.consensus.WaitForSync()
	if err != nil {
		logger.Error(err)
		return err
	}

	c.StateSync()

	logger.Infof("%s: joined %s's cluster", c.id.Pretty(), pid.Pretty())
	return nil
}

// StateSync syncs the consensus state to the Pin Tracker, ensuring
// that every Cid in the shared state is tracked and that the Pin Tracker
// is not tracking more Cids than it should.
func (c *Cluster) StateSync() error {
	cState, err := c.consensus.State()
	if err != nil {
		return err
	}

	logger.Debug("syncing state to tracker")
	clusterPins := cState.List()

	trackedPins := c.tracker.StatusAll()
	trackedPinsMap := make(map[string]int)
	for i, tpin := range trackedPins {
		trackedPinsMap[tpin.Cid.String()] = i
	}

	// Track items which are not tracked
	for _, pin := range clusterPins {
		_, tracked := trackedPinsMap[pin.Cid.String()]
		if !tracked {
			logger.Debugf("StateSync: tracking %s, part of the shared state", pin.Cid)
			c.tracker.Track(pin)
		}
	}

	// a. Untrack items which should not be tracked
	// b. Track items which should not be remote as local
	// c. Track items which should not be local as remote
	for _, p := range trackedPins {
		pCid := p.Cid
		currentPin, has := cState.Get(pCid)
		allocatedHere := containsPeer(currentPin.Allocations, c.id) || currentPin.ReplicationFactorMin == -1

		switch {
		case !has:
			logger.Debugf("StateSync: Untracking %s, is not part of shared state", pCid)
			c.tracker.Untrack(pCid)
		case p.Status == api.TrackerStatusRemote && allocatedHere:
			logger.Debugf("StateSync: Tracking %s locally (currently remote)", pCid)
			c.tracker.Track(currentPin)
		case p.Status == api.TrackerStatusPinned && !allocatedHere:
			logger.Debugf("StateSync: Tracking %s as remote (currently local)", pCid)
			c.tracker.Track(currentPin)
		}
	}

	return nil
}

// StatusAll returns the GlobalPinInfo for all tracked Cids in all peers.
// If an error happens, the slice will contain as much information as
// could be fetched from other peers.
func (c *Cluster) StatusAll() ([]api.GlobalPinInfo, error) {
	return c.globalPinInfoSlice("TrackerStatusAll")
}

// StatusAllLocal returns the PinInfo for all the tracked Cids in this peer.
func (c *Cluster) StatusAllLocal() []api.PinInfo {
	return c.tracker.StatusAll()
}

// Status returns the GlobalPinInfo for a given Cid as fetched from all
// current peers. If an error happens, the GlobalPinInfo should contain
// as much information as could be fetched from the other peers.
func (c *Cluster) Status(h *cid.Cid) (api.GlobalPinInfo, error) {
	return c.globalPinInfoCid("TrackerStatus", h)
}

// StatusLocal returns this peer's PinInfo for a given Cid.
func (c *Cluster) StatusLocal(h *cid.Cid) api.PinInfo {
	return c.tracker.Status(h)
}

// SyncAll triggers SyncAllLocal() operations in all cluster peers, making sure
// that the state of tracked items matches the state reported by the IPFS daemon
// and returning the results as GlobalPinInfo. If an error happens, the slice
// will contain as much information as could be fetched from the peers.
func (c *Cluster) SyncAll() ([]api.GlobalPinInfo, error) {
	return c.globalPinInfoSlice("SyncAllLocal")
}

// SyncAllLocal makes sure that the current state for all tracked items
// in this peer matches the state reported by the IPFS daemon.
//
// SyncAllLocal returns the list of PinInfo that where updated because of
// the operation, along with those in error states.
func (c *Cluster) SyncAllLocal() ([]api.PinInfo, error) {
	syncedItems, err := c.tracker.SyncAll()
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
func (c *Cluster) Sync(h *cid.Cid) (api.GlobalPinInfo, error) {
	return c.globalPinInfoCid("SyncLocal", h)
}

// used for RecoverLocal and SyncLocal.
func (c *Cluster) localPinInfoOp(
	h *cid.Cid,
	f func(*cid.Cid) (api.PinInfo, error),
) (pInfo api.PinInfo, err error) {
	cids, err := c.cidsFromMetaPin(h)
	if err != nil {
		return api.PinInfo{}, err
	}

	for _, ci := range cids {
		pInfo, err = f(ci)
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
func (c *Cluster) SyncLocal(h *cid.Cid) (pInfo api.PinInfo, err error) {
	return c.localPinInfoOp(h, c.tracker.Sync)
}

// RecoverAllLocal triggers a RecoverLocal operation for all Cids tracked
// by this peer.
func (c *Cluster) RecoverAllLocal() ([]api.PinInfo, error) {
	return c.tracker.RecoverAll()
}

// Recover triggers a recover operation for a given Cid in all
// cluster peers.
func (c *Cluster) Recover(h *cid.Cid) (api.GlobalPinInfo, error) {
	return c.globalPinInfoCid("TrackerRecover", h)
}

// RecoverLocal triggers a recover operation for a given Cid in this peer only.
// It returns the updated PinInfo, after recovery.
func (c *Cluster) RecoverLocal(h *cid.Cid) (pInfo api.PinInfo, err error) {
	return c.localPinInfoOp(h, c.tracker.Recover)
}

// Pins returns the list of Cids managed by Cluster and which are part
// of the current global state. This is the source of truth as to which
// pins are managed and their allocation, but does not indicate if
// the item is successfully pinned. For that, use StatusAll().
func (c *Cluster) Pins() []api.Pin {
	cState, err := c.consensus.State()
	if err != nil {
		logger.Error(err)
		return []api.Pin{}
	}
	return cState.List()

}

// PinGet returns information for a single Cid managed by Cluster.
// The information is obtained from the current global state. The
// returned api.Pin provides information about the allocations
// assigned for the requested Cid, but does not indicate if
// the item is successfully pinned. For that, use Status(). PinGet
// returns an error if the given Cid is not part of the global state.
func (c *Cluster) PinGet(h *cid.Cid) (api.Pin, error) {
	st, err := c.consensus.State()
	if err != nil {
		return api.PinCid(h), err
	}
	pin, ok := st.Get(h)
	if !ok {
		return pin, errors.New("cid is not part of the global state")
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
func (c *Cluster) Pin(pin api.Pin) error {
	_, err := c.pin(pin, []peer.ID{}, pin.Allocations)
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
func (c *Cluster) setupPin(pin *api.Pin) error {
	err := c.setupReplicationFactor(pin)
	if err != nil {
		return err
	}

	existing, err := c.PinGet(pin.Cid)
	if err == nil && existing.Type != pin.Type { // it exists
		return errors.New("cannot repin CID with different tracking method, clear state with pin rm to proceed")
	}
	return checkPinType(pin)
}

// pin performs the actual pinning and supports a blacklist to be
// able to evacuate a node and returns whether the pin was submitted
// to the consensus layer or skipped (due to error or to the fact
// that it was already valid).
func (c *Cluster) pin(pin api.Pin, blacklist []peer.ID, prioritylist []peer.ID) (bool, error) {
	if pin.Cid == nil {
		return false, errors.New("bad pin object")
	}

	// setup pin might produce some side-effects to our pin
	err := c.setupPin(&pin)
	if err != nil {
		return false, err
	}
	if pin.Type == api.MetaType {
		return true, c.consensus.LogPin(pin)
	}

	allocs, err := c.allocate(
		pin.Cid,
		pin.ReplicationFactorMin,
		pin.ReplicationFactorMax,
		blacklist,
		prioritylist,
	)
	if err != nil {
		return false, err
	}
	pin.Allocations = allocs

	if curr, _ := c.PinGet(pin.Cid); curr.Equals(pin) {
		// skip pinning
		logger.Debugf("pinning %s skipped: already correctly allocated", pin.Cid)
		return false, nil
	}

	if len(pin.Allocations) == 0 {
		logger.Infof("IPFS cluster pinning %s everywhere:", pin.Cid)
	} else {
		logger.Infof("IPFS cluster pinning %s on %s:", pin.Cid, pin.Allocations)
	}

	return true, c.consensus.LogPin(pin)
}

// Unpin makes the cluster Unpin a Cid. This implies adding the Cid
// to the IPFS Cluster peers shared-state.
//
// Unpin returns an error if the operation could not be persisted
// to the global state. Unpin does not reflect the success or failure
// of underlying IPFS daemon unpinning operations.
func (c *Cluster) Unpin(h *cid.Cid) error {
	logger.Info("IPFS cluster unpinning:", h)
	pin, err := c.PinGet(h)
	if err != nil {
		return fmt.Errorf("cannot unpin pin uncommitted to state: %s", err)
	}

	switch pin.Type {
	case api.DataType:
		return c.consensus.LogUnpin(pin)
	case api.ShardType:
		err := "cannot unpin a shard direclty. Unpin content root CID instead."
		return errors.New(err)
	case api.MetaType:
		// Unpin cluster dag and referenced shards
		err := c.unpinClusterDag(pin)
		if err != nil {
			return err
		}
		return c.consensus.LogUnpin(pin)
	case api.ClusterDAGType:
		err := "cannot unpin a Cluster DAG directly. Unpin content root CID instead."
		return errors.New(err)
	default:
		return errors.New("unrecognized pin type")
	}
}

// unpinClusterDag unpins the clusterDAG metadata node and the shard metadata
// nodes that it references.  It handles the case where multiple parents
// reference the same metadata node, only unpinning those nodes without
// existing references
func (c *Cluster) unpinClusterDag(metaPin api.Pin) error {
	cids, err := c.cidsFromMetaPin(metaPin.Cid)
	if err != nil {
		return err
	}

	// TODO: FIXME: potentially unpinning shards which are referenced
	// by other clusterDAGs.
	for _, ci := range cids {
		err = c.consensus.LogUnpin(api.PinCid(ci))
		if err != nil {
			return err
		}
	}
	return nil
}

// AddFile adds a file to the ipfs daemons of the cluster.  The ipfs importer
// pipeline is used to DAGify the file.  Depending on input parameters this
// DAG can be added locally to the calling cluster peer's ipfs repo, or
// sharded across the entire cluster.
func (c *Cluster) AddFile(reader *multipart.Reader, params *api.AddParams) (*cid.Cid, error) {
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
	return Version
}

// Peers returns the IDs of the members of this Cluster.
func (c *Cluster) Peers() []api.ID {
	members, err := c.consensus.Peers()
	if err != nil {
		logger.Error(err)
		logger.Error("an empty list of peers will be returned")
		return []api.ID{}
	}

	peersSerial := make([]api.IDSerial, len(members), len(members))
	peers := make([]api.ID, len(members), len(members))

	ctxs, cancels := rpcutil.CtxsWithCancel(c.ctx, len(members))
	defer rpcutil.MultiCancel(cancels)

	errs := c.rpcClient.MultiCall(
		ctxs,
		members,
		"Cluster",
		"ID",
		struct{}{},
		rpcutil.CopyIDSerialsToIfaces(peersSerial),
	)

	for i, err := range errs {
		if err != nil {
			peersSerial[i].ID = peer.IDB58Encode(members[i])
			peersSerial[i].Error = err.Error()
		}
	}

	for i, ps := range peersSerial {
		peers[i] = ps.ToID()
	}
	return peers
}

func (c *Cluster) globalPinInfoCid(method string, h *cid.Cid) (api.GlobalPinInfo, error) {
	pin := api.GlobalPinInfo{
		Cid:     h,
		PeerMap: make(map[peer.ID]api.PinInfo),
	}

	members, err := c.consensus.Peers()
	if err != nil {
		logger.Error(err)
		return api.GlobalPinInfo{}, err
	}

	replies := make([]api.PinInfoSerial, len(members), len(members))
	arg := api.Pin{
		Cid: h,
	}

	ctxs, cancels := rpcutil.CtxsWithCancel(c.ctx, len(members))
	defer rpcutil.MultiCancel(cancels)

	errs := c.rpcClient.MultiCall(
		ctxs,
		members,
		"Cluster",
		method,
		arg.ToSerial(),
		rpcutil.CopyPinInfoSerialToIfaces(replies),
	)

	for i, rserial := range replies {
		e := errs[i]

		// Potentially rserial is empty. But ToPinInfo ignores all
		// errors from underlying libraries. In that case .Status
		// will be TrackerStatusBug (0)
		r := rserial.ToPinInfo()

		// No error. Parse and continue
		if e == nil {
			pin.PeerMap[members[i]] = r
			continue
		}

		// Deal with error cases (err != nil): wrap errors in PinInfo

		// In this case, we had no answer at all. The contacted peer
		// must be offline or unreachable.
		if r.Status == api.TrackerStatusBug {
			logger.Errorf("%s: error in broadcast response from %s: %s ", c.id, members[i], e)
			pin.PeerMap[members[i]] = api.PinInfo{
				Cid:    h,
				Peer:   members[i],
				Status: api.TrackerStatusClusterError,
				TS:     time.Now(),
				Error:  e.Error(),
			}
		} else { // there was an rpc error, but got a valid response :S
			r.Error = e.Error()
			pin.PeerMap[members[i]] = r
			// unlikely to come down this path
		}
	}

	return pin, nil
}

func (c *Cluster) globalPinInfoSlice(method string) ([]api.GlobalPinInfo, error) {
	var infos []api.GlobalPinInfo
	fullMap := make(map[string]api.GlobalPinInfo)

	members, err := c.consensus.Peers()
	if err != nil {
		logger.Error(err)
		return []api.GlobalPinInfo{}, err
	}

	replies := make([][]api.PinInfoSerial, len(members), len(members))

	ctxs, cancels := rpcutil.CtxsWithCancel(c.ctx, len(members))
	defer rpcutil.MultiCancel(cancels)

	errs := c.rpcClient.MultiCall(
		ctxs,
		members,
		"Cluster",
		method,
		struct{}{},
		rpcutil.CopyPinInfoSerialSliceToIfaces(replies),
	)

	mergePins := func(pins []api.PinInfoSerial) {
		for _, pserial := range pins {
			p := pserial.ToPinInfo()
			item, ok := fullMap[pserial.Cid]
			if !ok {
				fullMap[pserial.Cid] = api.GlobalPinInfo{
					Cid: p.Cid,
					PeerMap: map[peer.ID]api.PinInfo{
						p.Peer: p,
					},
				}
			} else {
				item.PeerMap[p.Peer] = p
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
		for cidStr := range fullMap {
			c, _ := cid.Decode(cidStr)
			fullMap[cidStr].PeerMap[p] = api.PinInfo{
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

func (c *Cluster) getIDForPeer(pid peer.ID) (api.ID, error) {
	idSerial := api.ID{ID: pid}.ToSerial()
	err := c.rpcClient.Call(
		pid, "Cluster", "ID", struct{}{}, &idSerial)
	id := idSerial.ToID()
	if err != nil {
		logger.Error(err)
		id.Error = err.Error()
	}
	return id, err
}

// cidsFromMetaPin expands a meta-pin and returns a list of Cids that
// Cluster handles for it: the ShardPins, the ClusterDAG and the MetaPin, in
// that order (the MetaPin is the last element).
// It returns a slice with only the given Cid if it's not a known Cid or not a
// MetaPin.
func (c *Cluster) cidsFromMetaPin(h *cid.Cid) ([]*cid.Cid, error) {
	cState, err := c.consensus.State()
	if err != nil {
		return nil, err
	}

	list := []*cid.Cid{h}

	pin, ok := cState.Get(h)
	if !ok {
		return list, nil
	}

	if pin.Type != api.MetaType {
		return list, nil
	}

	list = append([]*cid.Cid{pin.Reference}, list...)
	clusterDagPin, err := c.PinGet(pin.Reference)
	if err != nil {
		return list, fmt.Errorf("could not get clusterDAG pin from state. Malformed pin?: %s", err)
	}

	clusterDagBlock, err := c.ipfs.BlockGet(clusterDagPin.Cid)
	if err != nil {
		return list, fmt.Errorf("error reading clusterDAG block from ipfs: %s", err)
	}

	clusterDagNode, err := sharding.CborDataToNode(clusterDagBlock, "cbor")
	if err != nil {
		return list, fmt.Errorf("error parsing clusterDAG block: %s", err)
	}
	for _, l := range clusterDagNode.Links() {
		list = append([]*cid.Cid{l.Cid}, list...)
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
