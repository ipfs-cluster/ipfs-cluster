package ipfscluster

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	swarm "github.com/libp2p/go-libp2p-swarm"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	ma "github.com/multiformats/go-multiaddr"
)

// Cluster is the main IPFS cluster component. It provides
// the go-API for it and orchestrates the components that make up the system.
type Cluster struct {
	ctx    context.Context
	cancel func()

	id          peer.ID
	config      *Config
	host        host.Host
	rpcServer   *rpc.Server
	rpcClient   *rpc.Client
	peerManager *peerManager

	consensus *Consensus
	api       API
	ipfs      IPFSConnector
	state     State
	tracker   PinTracker
	monitor   PeerMonitor
	allocator PinAllocator
	informer  Informer

	shutdownLock sync.Mutex
	shutdown     bool
	doneCh       chan struct{}
	readyCh      chan struct{}
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
	cfg *Config,
	api API,
	ipfs IPFSConnector,
	state State,
	tracker PinTracker,
	monitor PeerMonitor,
	allocator PinAllocator,
	informer Informer) (*Cluster, error) {

	ctx, cancel := context.WithCancel(context.Background())
	host, err := makeHost(ctx, cfg)
	if err != nil {
		cancel()
		return nil, err
	}

	logger.Infof("IPFS Cluster v%s listening on:", Version)
	for _, addr := range host.Addrs() {
		logger.Infof("        %s/ipfs/%s", addr, host.ID().Pretty())
	}

	c := &Cluster{
		ctx:       ctx,
		cancel:    cancel,
		id:        host.ID(),
		config:    cfg,
		host:      host,
		api:       api,
		ipfs:      ipfs,
		state:     state,
		tracker:   tracker,
		monitor:   monitor,
		allocator: allocator,
		informer:  informer,
		doneCh:    make(chan struct{}),
		readyCh:   make(chan struct{}),
	}

	c.setupPeerManager()
	err = c.setupRPC()
	if err != nil {
		c.Shutdown()
		return nil, err
	}

	err = c.setupConsensus()
	if err != nil {
		c.Shutdown()
		return nil, err
	}
	c.setupRPCClients()
	c.bootstrap()
	ok := c.bootstrap()
	if !ok {
		logger.Error("Bootstrap unsuccessful")
		c.Shutdown()
		return nil, errors.New("bootstrap unsuccessful")
	}
	go func() {
		c.ready()
		c.run()
	}()
	return c, nil
}

func (c *Cluster) setupPeerManager() {
	pm := newPeerManager(c)
	c.peerManager = pm
	if len(c.config.ClusterPeers) > 0 {
		c.peerManager.addFromMultiaddrs(c.config.ClusterPeers)
	} else {
		c.peerManager.addFromMultiaddrs(c.config.Bootstrap)
	}

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

func (c *Cluster) setupConsensus() error {
	var startPeers []peer.ID
	if len(c.config.ClusterPeers) > 0 {
		startPeers = peersFromMultiaddrs(c.config.ClusterPeers)
	} else {
		startPeers = peersFromMultiaddrs(c.config.Bootstrap)
	}

	consensus, err := NewConsensus(
		append(startPeers, c.id),
		c.host,
		c.config.ConsensusDataFolder,
		c.state)
	if err != nil {
		logger.Errorf("error creating consensus: %s", err)
		return err
	}
	c.consensus = consensus
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

// stateSyncWatcher loops and triggers StateSync from time to time
func (c *Cluster) stateSyncWatcher() {
	stateSyncTicker := time.NewTicker(
		time.Duration(c.config.StateSyncSeconds) * time.Second)
	for {
		select {
		case <-stateSyncTicker.C:
			c.StateSync()
		case <-c.ctx.Done():
			stateSyncTicker.Stop()
			return
		}
	}
}

// push metrics loops and pushes metrics to the leader's monitor
func (c *Cluster) pushInformerMetrics() {
	timer := time.NewTimer(0) // fire immediately first
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-timer.C:
			// wait
		}

		leader, err := c.consensus.Leader()

		if err != nil {
			// retry in 1 second
			timer.Stop()
			timer.Reset(1 * time.Second)
			continue
		}

		metric := c.informer.GetMetric()
		metric.Peer = c.id

		err = c.rpcClient.Call(
			leader,
			"Cluster", "PeerMonitorLogMetric",
			metric, &struct{}{})
		if err != nil {
			logger.Errorf("error pushing metric to %s", leader.Pretty())
		}

		logger.Debugf("pushed metric %s to %s", metric.Name, metric.Peer.Pretty())

		timer.Stop() // no need to drain C if we are here
		timer.Reset(metric.GetTTL() / 2)
	}
}

// run provides a cancellable context and launches some goroutines
// before signaling readyCh
func (c *Cluster) run() {
	go c.stateSyncWatcher()
	go c.pushInformerMetrics()
}

func (c *Cluster) ready() {
	// We bootstrapped first because with dirty state consensus
	// may have a peerset and not find a leader so we cannot wait
	// for it.
	timer := time.NewTimer(30 * time.Second)
	select {
	case <-timer.C:
		logger.Error("consensus start timed out")
		c.Shutdown()
		return
	case <-c.consensus.Ready():
	case <-c.ctx.Done():
		return
	}

	// Cluster is ready.
	logger.Info("Cluster Peers (not including ourselves):")
	peers := c.peerManager.peersAddrs()
	if len(peers) == 0 {
		logger.Info("    - No other peers")
	}
	for _, a := range c.peerManager.peersAddrs() {
		logger.Infof("    - %s", a)
	}
	close(c.readyCh)
	logger.Info("IPFS Cluster is ready")
}

func (c *Cluster) bootstrap() bool {
	// Cases in which we do not bootstrap
	if len(c.config.Bootstrap) == 0 || len(c.config.ClusterPeers) > 0 {
		return true
	}

	for _, b := range c.config.Bootstrap {
		logger.Infof("Bootstrapping to %s", b)
		err := c.Join(b)
		if err == nil {
			return true
		}
		logger.Error(err)
	}
	return false
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

	if c.shutdown {
		logger.Warning("Cluster is already shutdown")
		return nil
	}

	logger.Info("shutting down IPFS Cluster")

	if c.config.LeaveOnShutdown {
		// best effort
		logger.Warning("Attempting to leave Cluster. This may take some seconds")
		err := c.consensus.LogRmPeer(c.id)
		if err != nil {
			logger.Error("leaving cluster: " + err.Error())
		} else {
			time.Sleep(2 * time.Second)
		}
		c.peerManager.resetPeers()
	}

	// Cancel contexts
	c.cancel()

	if con := c.consensus; con != nil {
		if err := con.Shutdown(); err != nil {
			logger.Errorf("error stopping consensus: %s", err)
			return err
		}
	}

	c.peerManager.savePeers()

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
	c.wg.Wait()
	c.host.Close() // Shutdown all network services
	c.shutdown = true
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
		addrs = append(addrs, multiaddrJoin(addr, c.id))
	}

	return api.ID{
		ID: c.id,
		//PublicKey:          c.host.Peerstore().PubKey(c.id),
		Addresses:          addrs,
		ClusterPeers:       c.peerManager.peersAddrs(),
		Version:            Version,
		Commit:             Commit,
		RPCProtocolVersion: RPCProtocol,
		IPFS:               ipfsID,
	}
}

// PeerAdd adds a new peer to this Cluster.
//
// The new peer must be reachable. It will be added to the
// consensus and will receive the shared state (including the
// list of peers). The new peer should be a single-peer cluster,
// preferable without any relevant state.
func (c *Cluster) PeerAdd(addr ma.Multiaddr) (api.ID, error) {
	// starting 10 nodes on the same box for testing
	// causes deadlock and a global lock here
	// seems to help.
	c.paMux.Lock()
	defer c.paMux.Unlock()
	logger.Debugf("peerAdd called with %s", addr)
	pid, decapAddr, err := multiaddrSplit(addr)
	if err != nil {
		id := api.ID{
			Error: err.Error(),
		}
		return id, err
	}

	// Figure out its real address if we have one
	remoteAddr := getRemoteMultiaddr(c.host, pid, decapAddr)

	err = c.peerManager.addPeer(remoteAddr)
	if err != nil {
		logger.Error(err)
		id := api.ID{ID: pid, Error: err.Error()}
		return id, err
	}

	// Figure out our address to that peer. This also
	// ensures that it is reachable
	var addrSerial api.MultiaddrSerial
	err = c.rpcClient.Call(pid, "Cluster",
		"RemoteMultiaddrForPeer", c.id, &addrSerial)
	if err != nil {
		logger.Error(err)
		id := api.ID{ID: pid, Error: err.Error()}
		c.peerManager.rmPeer(pid, false)
		return id, err
	}

	// Log the new peer in the log so everyone gets it.
	err = c.consensus.LogAddPeer(remoteAddr)
	if err != nil {
		logger.Error(err)
		id := api.ID{ID: pid, Error: err.Error()}
		c.peerManager.rmPeer(pid, false)
		return id, err
	}

	// Send cluster peers to the new peer.
	clusterPeers := append(c.peerManager.peersAddrs(),
		addrSerial.ToMultiaddr())
	err = c.rpcClient.Call(pid,
		"Cluster",
		"PeerManagerAddFromMultiaddrs",
		api.MultiaddrsToSerial(clusterPeers),
		&struct{}{})
	if err != nil {
		logger.Error(err)
	}

	id, err := c.getIDForPeer(pid)
	return id, nil
}

// PeerRemove removes a peer from this Cluster.
//
// The peer will be removed from the consensus peer set,
// it will be shut down after this happens.
func (c *Cluster) PeerRemove(pid peer.ID) error {
	if !c.peerManager.isPeer(pid) {
		return fmt.Errorf("%s is not a peer", pid.Pretty())
	}

	err := c.consensus.LogRmPeer(pid)
	if err != nil {
		logger.Error(err)
		return err
	}

	// This is a best effort. It may fail
	// if that peer is down
	err = c.rpcClient.Call(pid,
		"Cluster",
		"PeerManagerRmPeerShutdown",
		pid,
		&struct{}{})
	if err != nil {
		logger.Error(err)
	}

	return nil
}

// Join adds this peer to an existing cluster. The calling peer should
// be a single-peer cluster node. This is almost equivalent to calling
// PeerAdd on the destination cluster.
func (c *Cluster) Join(addr ma.Multiaddr) error {
	logger.Debugf("Join(%s)", addr)

	//if len(c.peerManager.peers()) > 1 {
	//	logger.Error(c.peerManager.peers())
	//	return errors.New("only single-node clusters can be joined")
	//}

	pid, _, err := multiaddrSplit(addr)
	if err != nil {
		logger.Error(err)
		return err
	}

	// Bootstrap to myself
	if pid == c.id {
		return nil
	}

	// Add peer to peerstore so we can talk to it
	c.peerManager.addPeer(addr)

	// Note that PeerAdd() on the remote peer will
	// figure out what our real address is (obviously not
	// ClusterAddr).
	var myID api.IDSerial
	err = c.rpcClient.Call(pid,
		"Cluster",
		"PeerAdd",
		api.MultiaddrToSerial(multiaddrJoin(c.config.ClusterAddr, c.id)),
		&myID)
	if err != nil {
		logger.Error(err)
		return err
	}

	// wait for leader and for state to catch up
	// then sync
	err = c.consensus.WaitForSync()
	if err != nil {
		logger.Error(err)
		return err
	}
	c.StateSync()

	logger.Infof("joined %s's cluster", addr)
	return nil
}

// StateSync syncs the consensus state to the Pin Tracker, ensuring
// that every Cid that should be tracked is tracked. It returns
// PinInfo for Cids which were added or deleted.
func (c *Cluster) StateSync() ([]api.PinInfo, error) {
	cState, err := c.consensus.State()
	if err != nil {
		return nil, err
	}

	logger.Debug("syncing state to tracker")
	clusterPins := cState.List()
	var changed []*cid.Cid

	// Track items which are not tracked
	for _, carg := range clusterPins {
		if c.tracker.Status(carg.Cid).Status == api.TrackerStatusUnpinned {
			changed = append(changed, carg.Cid)
			go c.tracker.Track(carg)
		}
	}

	// Untrack items which should not be tracked
	for _, p := range c.tracker.StatusAll() {
		if !cState.Has(p.Cid) {
			changed = append(changed, p.Cid)
			go c.tracker.Untrack(p.Cid)
		}
	}

	var infos []api.PinInfo
	for _, h := range changed {
		infos = append(infos, c.tracker.Status(h))
	}
	return infos, nil
}

// StatusAll returns the GlobalPinInfo for all tracked Cids. If an error
// happens, the slice will contain as much information as could be fetched.
func (c *Cluster) StatusAll() ([]api.GlobalPinInfo, error) {
	return c.globalPinInfoSlice("TrackerStatusAll")
}

// Status returns the GlobalPinInfo for a given Cid. If an error happens,
// the GlobalPinInfo should contain as much information as could be fetched.
func (c *Cluster) Status(h *cid.Cid) (api.GlobalPinInfo, error) {
	return c.globalPinInfoCid("TrackerStatus", h)
}

// SyncAllLocal makes sure that the current state for all tracked items
// matches the state reported by the IPFS daemon.
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

// SyncLocal performs a local sync operation for the given Cid. This will
// tell the tracker to verify the status of the Cid against the IPFS daemon.
// It returns the updated PinInfo for the Cid.
func (c *Cluster) SyncLocal(h *cid.Cid) (api.PinInfo, error) {
	var err error
	pInfo, err := c.tracker.Sync(h)
	// Despite errors, trackers provides an updated PinInfo so
	// we just log it.
	if err != nil {
		logger.Error("tracker.SyncCid() returned with error: ", err)
		logger.Error("Is the ipfs daemon running?")
	}
	return pInfo, err
}

// SyncAll triggers LocalSync() operations in all cluster peers.
func (c *Cluster) SyncAll() ([]api.GlobalPinInfo, error) {
	return c.globalPinInfoSlice("SyncAllLocal")
}

// Sync triggers a LocalSyncCid() operation for a given Cid
// in all cluster peers.
func (c *Cluster) Sync(h *cid.Cid) (api.GlobalPinInfo, error) {
	return c.globalPinInfoCid("SyncLocal", h)
}

// RecoverLocal triggers a recover operation for a given Cid
func (c *Cluster) RecoverLocal(h *cid.Cid) (api.PinInfo, error) {
	return c.tracker.Recover(h)
}

// Recover triggers a recover operation for a given Cid in all
// cluster peers.
func (c *Cluster) Recover(h *cid.Cid) (api.GlobalPinInfo, error) {
	return c.globalPinInfoCid("TrackerRecover", h)
}

// Pins returns the list of Cids managed by Cluster and which are part
// of the current global state. This is the source of truth as to which
// pins are managed, but does not indicate if the item is successfully pinned.
func (c *Cluster) Pins() []api.CidArg {
	cState, err := c.consensus.State()
	if err != nil {
		logger.Error(err)
		return []api.CidArg{}
	}

	return cState.List()
}

// Pin makes the cluster Pin a Cid. This implies adding the Cid
// to the IPFS Cluster peers shared-state. Depending on the cluster
// pinning strategy, the PinTracker may then request the IPFS daemon
// to pin the Cid.
//
// Pin returns an error if the operation could not be persisted
// to the global state. Pin does not reflect the success or failure
// of underlying IPFS daemon pinning operations.
func (c *Cluster) Pin(h *cid.Cid) error {
	logger.Info("pinning:", h)

	cidArg := api.CidArg{
		Cid: h,
	}

	rpl := c.config.ReplicationFactor
	switch {
	case rpl == 0:
		return errors.New("replication factor is 0")
	case rpl < 0:
		cidArg.Everywhere = true
	case rpl > 0:
		allocs, err := c.allocate(h)
		if err != nil {
			return err
		}
		cidArg.Allocations = allocs
	}

	err := c.consensus.LogPin(cidArg)
	if err != nil {
		return err
	}
	return nil
}

// Unpin makes the cluster Unpin a Cid. This implies adding the Cid
// to the IPFS Cluster peers shared-state.
//
// Unpin returns an error if the operation could not be persisted
// to the global state. Unpin does not reflect the success or failure
// of underlying IPFS daemon unpinning operations.
func (c *Cluster) Unpin(h *cid.Cid) error {
	logger.Info("unpinning:", h)

	carg := api.CidArg{
		Cid: h,
	}

	err := c.consensus.LogUnpin(carg)
	if err != nil {
		return err
	}
	return nil
}

// Version returns the current IPFS Cluster version
func (c *Cluster) Version() string {
	return Version
}

// Peers returns the IDs of the members of this Cluster
func (c *Cluster) Peers() []api.ID {
	members := c.peerManager.peers()
	peersSerial := make([]api.IDSerial, len(members), len(members))
	peers := make([]api.ID, len(members), len(members))

	errs := c.multiRPC(members, "Cluster", "ID", struct{}{},
		copyIDSerialsToIfaces(peersSerial))

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

// makeHost makes a libp2p-host
func makeHost(ctx context.Context, cfg *Config) (host.Host, error) {
	ps := peerstore.NewPeerstore()
	privateKey := cfg.PrivateKey
	publicKey := privateKey.GetPublic()

	if err := ps.AddPubKey(cfg.ID, publicKey); err != nil {
		return nil, err
	}

	if err := ps.AddPrivKey(cfg.ID, privateKey); err != nil {
		return nil, err
	}

	network, err := swarm.NewNetwork(
		ctx,
		[]ma.Multiaddr{cfg.ClusterAddr},
		cfg.ID,
		ps,
		nil,
	)

	if err != nil {
		return nil, err
	}

	bhost := basichost.New(network)
	return bhost, nil
}

// Perform an RPC request to multiple destinations
func (c *Cluster) multiRPC(dests []peer.ID, svcName, svcMethod string, args interface{}, reply []interface{}) []error {
	if len(dests) != len(reply) {
		panic("must have matching dests and replies")
	}
	var wg sync.WaitGroup
	errs := make([]error, len(dests), len(dests))

	for i := range dests {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := c.rpcClient.Call(
				dests[i],
				svcName,
				svcMethod,
				args,
				reply[i])
			errs[i] = err
		}(i)
	}
	wg.Wait()
	return errs

}

func (c *Cluster) globalPinInfoCid(method string, h *cid.Cid) (api.GlobalPinInfo, error) {
	pin := api.GlobalPinInfo{
		Cid:     h,
		PeerMap: make(map[peer.ID]api.PinInfo),
	}

	members := c.peerManager.peers()
	replies := make([]api.PinInfoSerial, len(members), len(members))
	arg := api.CidArg{
		Cid: h,
	}
	errs := c.multiRPC(members,
		"Cluster",
		method, arg.ToSerial(),
		copyPinInfoSerialToIfaces(replies))

	for i, rserial := range replies {
		r := rserial.ToPinInfo()
		if e := errs[i]; e != nil {
			if r.Status == api.TrackerStatusBug {
				// This error must come from not being able to contact that cluster member
				logger.Errorf("%s: error in broadcast response from %s: %s ", c.id, members[i], e)
				r = api.PinInfo{
					Cid:    r.Cid,
					Peer:   members[i],
					Status: api.TrackerStatusClusterError,
					TS:     time.Now(),
					Error:  e.Error(),
				}
			} else {
				r.Error = e.Error()
			}
		}
		pin.PeerMap[members[i]] = r
	}

	return pin, nil
}

func (c *Cluster) globalPinInfoSlice(method string) ([]api.GlobalPinInfo, error) {
	var infos []api.GlobalPinInfo
	fullMap := make(map[string]api.GlobalPinInfo)

	members := c.peerManager.peers()
	replies := make([][]api.PinInfoSerial, len(members), len(members))
	errs := c.multiRPC(members,
		"Cluster",
		method, struct{}{},
		copyPinInfoSerialSliceToIfaces(replies))

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

// allocate finds peers to allocate a hash using the informer and the monitor
// it should only be used with a positive replication factor
func (c *Cluster) allocate(hash *cid.Cid) ([]peer.ID, error) {
	if c.config.ReplicationFactor <= 0 {
		return nil, errors.New("cannot decide allocation for replication factor <= 0")
	}

	// Figure out who is currently holding this
	var currentlyAllocatedPeers []peer.ID
	st, err := c.consensus.State()
	if err != nil {
		// no state we assume it is empty. If there was other
		// problem, we would fail to commit anyway.
		currentlyAllocatedPeers = []peer.ID{}
	} else {
		carg := st.Get(hash)
		currentlyAllocatedPeers = carg.Allocations
	}

	// initialize a candidate metrics map with all current clusterPeers
	// (albeit with invalid metrics)
	clusterPeers := c.peerManager.peers()
	metricsMap := make(map[peer.ID]api.Metric)
	for _, cp := range clusterPeers {
		metricsMap[cp] = api.Metric{Valid: false}
	}

	// Request latest metrics logged by informers from the leader
	metricName := c.informer.Name()
	l, err := c.consensus.Leader()
	if err != nil {
		return nil, errors.New("cannot determine leading Monitor")
	}
	var metrics []api.Metric
	err = c.rpcClient.Call(l,
		"Cluster", "PeerMonitorLastMetrics",
		metricName,
		&metrics)
	if err != nil {
		return nil, err
	}

	// put metrics in the metricsMap if they belong to a current clusterPeer
	for _, m := range metrics {
		_, ok := metricsMap[m.Peer]
		if !ok {
			continue
		}
		metricsMap[m.Peer] = m
	}

	// Remove any invalid metric. This will clear any cluster peers
	// for which we did not receive metrics.
	for p, m := range metricsMap {
		if m.Discard() {
			delete(metricsMap, p)
		}
	}

	// Move metrics from currentlyAllocatedPeers to a new map
	currentlyAllocatedPeersMetrics := make(map[peer.ID]api.Metric)
	for _, p := range currentlyAllocatedPeers {
		m, ok := metricsMap[p]
		if !ok {
			continue
		}
		currentlyAllocatedPeersMetrics[p] = m
		delete(metricsMap, p)

	}

	// how many allocations do we need (note we will re-allocate if we did
	// not receive good metrics for currently allocated peeers)
	needed := c.config.ReplicationFactor - len(currentlyAllocatedPeersMetrics)

	// if we are already good (note invalid metrics would trigger
	// re-allocations as they are not included in currentAllocMetrics)
	if needed <= 0 {
		return nil, fmt.Errorf("CID is already correctly allocated to %s", currentlyAllocatedPeers)
	}

	// Allocate is called with currentAllocMetrics which contains
	// only currentlyAllocatedPeers when they have provided valid metrics.
	candidateAllocs, err := c.allocator.Allocate(hash, currentlyAllocatedPeersMetrics, metricsMap)
	if err != nil {
		return nil, logError(err.Error())
	}

	// we don't have enough peers to pin
	if len(candidateAllocs) < needed {
		err = logError("cannot find enough allocations for this CID: needed: %d. Got: %s",
			needed, candidateAllocs)
		return nil, err
	}

	// return as many as needed
	return candidateAllocs[0:needed], nil
}
