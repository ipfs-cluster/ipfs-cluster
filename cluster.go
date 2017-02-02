package ipfscluster

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

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
	ctx context.Context

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

	shutdownLock sync.Mutex
	shutdown     bool
	shutdownCh   chan struct{}
	doneCh       chan struct{}
	readyCh      chan struct{}
	wg           sync.WaitGroup
}

// NewCluster builds a new IPFS Cluster peer. It initializes a LibP2P host,
// creates and RPC Server and client and sets up all components.
//
// The new cluster peer may still be performing initialization tasks when
// this call returns (consensus may still be bootstrapping). Use Cluster.Ready()
// if you need to wait until the peer is fully up.
func NewCluster(cfg *Config, api API, ipfs IPFSConnector, state State, tracker PinTracker) (*Cluster, error) {
	ctx := context.Background()
	host, err := makeHost(ctx, cfg)
	if err != nil {
		return nil, err
	}

	logger.Infof("IPFS Cluster v%s listening on:", Version)
	for _, addr := range host.Addrs() {
		logger.Infof("        %s/ipfs/%s", addr, host.ID().Pretty())
	}

	cluster := &Cluster{
		ctx:        ctx,
		config:     cfg,
		host:       host,
		api:        api,
		ipfs:       ipfs,
		state:      state,
		tracker:    tracker,
		shutdownCh: make(chan struct{}, 1),
		doneCh:     make(chan struct{}, 1),
		readyCh:    make(chan struct{}, 1),
	}

	// Setup peer manager
	pm := newPeerManager(cluster)
	cluster.peerManager = pm
	err = pm.addFromConfig(cfg)
	if err != nil {
		cluster.Shutdown()
		return nil, err
	}

	// Workaround for https://github.com/libp2p/go-libp2p-swarm/issues/15
	cluster.openConns()

	// Setup RPC
	rpcServer := rpc.NewServer(host, RPCProtocol)
	err = rpcServer.RegisterName("Cluster", &RPCAPI{cluster: cluster})
	if err != nil {
		cluster.Shutdown()
		return nil, err
	}
	cluster.rpcServer = rpcServer

	// Setup RPC client that components from this peer will use
	rpcClient := rpc.NewClientWithServer(host, RPCProtocol, rpcServer)
	cluster.rpcClient = rpcClient

	// Setup Consensus
	consensus, err := NewConsensus(pm.peers(), host, cfg.ConsensusDataFolder, state)
	if err != nil {
		logger.Errorf("error creating consensus: %s", err)
		cluster.Shutdown()
		return nil, err
	}
	cluster.consensus = consensus

	tracker.SetClient(rpcClient)
	ipfs.SetClient(rpcClient)
	api.SetClient(rpcClient)
	consensus.SetClient(rpcClient)

	cluster.run()
	return cluster, nil
}

// Ready returns a channel which signals when this peer is
// fully initialized (including consensus).
func (c *Cluster) Ready() <-chan struct{} {
	return c.consensus.readyCh
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
	if con := c.consensus; con != nil {
		if err := con.Shutdown(); err != nil {
			logger.Errorf("error stopping consensus: %s", err)
			return err
		}
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
	c.shutdownCh <- struct{}{}
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
func (c *Cluster) ID() ID {
	// ignore error since it is included in response object
	ipfsID, _ := c.ipfs.ID()
	var addrs []ma.Multiaddr
	for _, addr := range c.host.Addrs() {
		ipfsAddr, _ := ma.NewMultiaddr("/ipfs/" + c.host.ID().Pretty())
		addrs = append(addrs, addr.Encapsulate(ipfsAddr))
	}

	return ID{
		ID:                 c.host.ID(),
		PublicKey:          c.host.Peerstore().PubKey(c.host.ID()),
		Addresses:          addrs,
		ClusterPeers:       c.config.ClusterPeers,
		Version:            Version,
		Commit:             Commit,
		RPCProtocolVersion: RPCProtocol,
		IPFS:               ipfsID,
	}
}

// PeerAdd adds a new peer to this Cluster.
//
// The current peer will first attempt to contact the provided
// peer at the given multiaddress. If the connection is successful,
// the new peer, with the given multiaddress will be added to the
// cluster_peers and the configuration saved with the updated set.
// All other Cluster peers will be asked to do the same.
//
// Finally, the list of cluster peers is sent to the new
// peer, which will update its configuration and join the cluster.
//
// PeerAdd will fail if any of the peers is not reachable.
func (c *Cluster) PeerAdd(addr ma.Multiaddr) (ID, error) {
	p, decapAddr, err := multiaddrSplit(addr)
	if err != nil {
		id := ID{
			Error: err.Error(),
		}
		return id, err
	}

	// only add reachable nodes
	err = c.host.Connect(c.ctx, peerstore.PeerInfo{
		ID:    p,
		Addrs: []ma.Multiaddr{decapAddr},
	})
	if err != nil {
		err = fmt.Errorf("Peer unreachable. Aborting operation: %s", err)
		id := ID{
			ID:    p,
			Error: err.Error(),
		}
		logger.Error(err)
		return id, err
	}

	// Find which local address we use to connect
	conns := c.host.Network().ConnsToPeer(p)
	if len(conns) == 0 {
		err := errors.New("No connections to peer available")
		logger.Error(err)
		id := ID{
			ID:    p,
			Error: err.Error(),
		}

		return id, err
	}
	pidMAddr, _ := ma.NewMultiaddr("/ipfs/" + c.host.ID().Pretty())
	localMAddr := conns[0].LocalMultiaddr().Encapsulate(pidMAddr)

	// Let all peer managers know they need to add this peer
	peers := c.peerManager.peers()
	replies := make([]peer.ID, len(peers), len(peers))
	errs := c.multiRPC(peers, "Cluster", "PeerManagerAddPeer",
		MultiaddrToSerial(addr), copyPIDsToIfaces(replies))
	errorMsgs := ""
	for i, err := range errs {
		if err != nil {
			logger.Error(err)
			errorMsgs += fmt.Sprintf("%s: %s\n",
				peers[i].Pretty(),
				err.Error())
		}
	}
	if errorMsgs != "" {
		logger.Error("There were errors adding peer. Trying to rollback the operation")
		c.PeerRemove(p)
		id := ID{
			ID:    p,
			Error: "Error adding peer: " + errorMsgs,
		}
		return id, errors.New(errorMsgs)
	}

	// Inform the peer of the current cluster peers
	clusterPeers := MultiaddrsToSerial(c.config.ClusterPeers)
	clusterPeers = append(clusterPeers, MultiaddrToSerial(localMAddr))
	err = c.rpcClient.Call(
		p, "Cluster", "PeerManagerAddFromMultiaddrs",
		clusterPeers, &struct{}{})
	if err != nil {
		logger.Errorf("Error sending back the list of peers: %s")
		id := ID{
			ID:    p,
			Error: err.Error(),
		}
		return id, err
	}
	idSerial := ID{
		ID: p,
	}.ToSerial()
	err = c.rpcClient.Call(
		p, "Cluster", "ID", struct{}{}, &idSerial)

	logger.Infof("peer %s has been added to the Cluster", addr)
	return idSerial.ToID(), err
}

// PeerRemove removes a peer from this Cluster.
//
// The peer will be removed from the consensus peer set,
// remove all cluster peers from its configuration and
// shut itself down.
func (c *Cluster) PeerRemove(p peer.ID) error {
	peers := c.peerManager.peers()
	replies := make([]struct{}, len(peers), len(peers))
	errs := c.multiRPC(peers, "Cluster", "PeerManagerRmPeer",
		p, copyEmptyStructToIfaces(replies))
	errorMsgs := ""
	for i, err := range errs {
		if err != nil && peers[i] != p {
			logger.Error(err)
			errorMsgs += fmt.Sprintf("%s: %s\n",
				peers[i].Pretty(),
				err.Error())
		}
	}
	if errorMsgs != "" {
		return errors.New(errorMsgs)
	}
	logger.Infof("peer %s has been removed from the Cluster", p.Pretty())
	return nil
}

// StateSync syncs the consensus state to the Pin Tracker, ensuring
// that every Cid that should be tracked is tracked. It returns
// PinInfo for Cids which were added or deleted.
func (c *Cluster) StateSync() ([]PinInfo, error) {
	cState, err := c.consensus.State()
	if err != nil {
		return nil, err
	}

	logger.Debug("syncing state to tracker")
	clusterPins := cState.ListPins()
	var changed []*cid.Cid

	// Track items which are not tracked
	for _, h := range clusterPins {
		if c.tracker.Status(h).Status == TrackerStatusUnpinned {
			changed = append(changed, h)
			err := c.rpcClient.Go("",
				"Cluster",
				"Track",
				NewCidArg(h),
				&struct{}{},
				nil)
			if err != nil {
				return []PinInfo{}, err
			}

		}
	}

	// Untrack items which should not be tracked
	for _, p := range c.tracker.StatusAll() {
		h, _ := cid.Decode(p.CidStr)
		if !cState.HasPin(h) {
			changed = append(changed, h)
			err := c.rpcClient.Go("",
				"Cluster",
				"Track",
				&CidArg{p.CidStr},
				&struct{}{},
				nil)
			if err != nil {
				return []PinInfo{}, err
			}
		}
	}

	var infos []PinInfo
	for _, h := range changed {
		infos = append(infos, c.tracker.Status(h))
	}
	return infos, nil
}

// StatusAll returns the GlobalPinInfo for all tracked Cids. If an error
// happens, the slice will contain as much information as could be fetched.
func (c *Cluster) StatusAll() ([]GlobalPinInfo, error) {
	return c.globalPinInfoSlice("TrackerStatusAll")
}

// Status returns the GlobalPinInfo for a given Cid. If an error happens,
// the GlobalPinInfo should contain as much information as could be fetched.
func (c *Cluster) Status(h *cid.Cid) (GlobalPinInfo, error) {
	return c.globalPinInfoCid("TrackerStatus", h)
}

// SyncAllLocal makes sure that the current state for all tracked items
// matches the state reported by the IPFS daemon.
//
// SyncAllLocal returns the list of PinInfo that where updated because of
// the operation, along with those in error states.
func (c *Cluster) SyncAllLocal() ([]PinInfo, error) {
	syncedItems, err := c.tracker.SyncAll()
	// Despite errors, tracker provides synced items that we can provide.
	// They encapsulate the error.
	if err != nil {
		logger.Error("tracker.Sync() returned with error: ", err)
		logger.Error("Is the ipfs daemon running?")
		logger.Error("LocalSync returning without attempting recovers")
	}
	return syncedItems, err
}

// SyncLocal performs a local sync operation for the given Cid. This will
// tell the tracker to verify the status of the Cid against the IPFS daemon.
// It returns the updated PinInfo for the Cid.
func (c *Cluster) SyncLocal(h *cid.Cid) (PinInfo, error) {
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
func (c *Cluster) SyncAll() ([]GlobalPinInfo, error) {
	return c.globalPinInfoSlice("SyncAllLocal")
}

// Sync triggers a LocalSyncCid() operation for a given Cid
// in all cluster peers.
func (c *Cluster) Sync(h *cid.Cid) (GlobalPinInfo, error) {
	return c.globalPinInfoCid("SyncLocal", h)
}

// RecoverLocal triggers a recover operation for a given Cid
func (c *Cluster) RecoverLocal(h *cid.Cid) (PinInfo, error) {
	return c.tracker.Recover(h)
}

// Recover triggers a recover operation for a given Cid in all
// cluster peers.
func (c *Cluster) Recover(h *cid.Cid) (GlobalPinInfo, error) {
	return c.globalPinInfoCid("TrackerRecover", h)
}

// Pins returns the list of Cids managed by Cluster and which are part
// of the current global state. This is the source of truth as to which
// pins are managed, but does not indicate if the item is successfully pinned.
func (c *Cluster) Pins() []*cid.Cid {
	cState, err := c.consensus.State()
	if err != nil {
		logger.Error(err)
		return []*cid.Cid{}
	}
	return cState.ListPins()
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
	err := c.consensus.LogPin(h)
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
	err := c.consensus.LogUnpin(h)
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
func (c *Cluster) Peers() []ID {
	members := c.peerManager.peers()
	peersSerial := make([]IDSerial, len(members), len(members))
	peers := make([]ID, len(members), len(members))

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

// run provides a cancellable context and waits for all components to be ready
// before signaling readyCh
func (c *Cluster) run() {
	c.wg.Add(1)

	// Currently we do nothing other than waiting to
	// cancel our context.
	go func() {
		defer c.wg.Done()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c.ctx = ctx

		for {
			select {
			case <-c.shutdownCh:
				return
			case <-c.consensus.Ready():
				close(c.readyCh)
				logger.Info("IPFS Cluster is ready")
			}
		}
	}()
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

func (c *Cluster) globalPinInfoCid(method string, h *cid.Cid) (GlobalPinInfo, error) {
	pin := GlobalPinInfo{
		Cid:     h,
		PeerMap: make(map[peer.ID]PinInfo),
	}

	members := c.peerManager.peers()
	replies := make([]PinInfo, len(members), len(members))
	args := NewCidArg(h)
	errs := c.multiRPC(members, "Cluster", method, args, copyPinInfoToIfaces(replies))

	for i, r := range replies {
		if e := errs[i]; e != nil { // This error must come from not being able to contact that cluster member
			logger.Errorf("%s: error in broadcast response from %s: %s ", c.host.ID(), members[i], e)
			if r.Status == TrackerStatusBug {
				r = PinInfo{
					CidStr: h.String(),
					Peer:   members[i],
					Status: TrackerStatusClusterError,
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

func (c *Cluster) globalPinInfoSlice(method string) ([]GlobalPinInfo, error) {
	var infos []GlobalPinInfo
	fullMap := make(map[string]GlobalPinInfo)

	members := c.peerManager.peers()
	replies := make([][]PinInfo, len(members), len(members))
	errs := c.multiRPC(members, "Cluster", method, struct{}{}, copyPinInfoSliceToIfaces(replies))

	mergePins := func(pins []PinInfo) {
		for _, p := range pins {
			item, ok := fullMap[p.CidStr]
			c, _ := cid.Decode(p.CidStr)
			if !ok {
				fullMap[p.CidStr] = GlobalPinInfo{
					Cid: c,
					PeerMap: map[peer.ID]PinInfo{
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
			logger.Errorf("%s: error in broadcast response from %s: %s ", c.host.ID(), members[i], e)
			erroredPeers[members[i]] = e.Error()
		} else {
			mergePins(r)
		}
	}

	// Merge any errors
	for p, msg := range erroredPeers {
		for c := range fullMap {
			fullMap[c].PeerMap[p] = PinInfo{
				CidStr: c,
				Peer:   p,
				Status: TrackerStatusClusterError,
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

// openConns is a workaround for
// https://github.com/libp2p/go-libp2p-swarm/issues/15
// which break our tests.
// It runs when consensus is initialized so we can assume
// that the cluster is more or less up.
// It should open connections for peers where they haven't
// yet been opened. By randomly sleeping we reduce the
// chance that peers will open 2 connections simultaneously.
func (c *Cluster) openConns() {
	rand.Seed(time.Now().UnixNano())
	time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
	peers := c.host.Peerstore().Peers()
	for _, p := range peers {
		peerInfo := c.host.Peerstore().PeerInfo(p)
		if p == c.host.ID() {
			continue // do not connect to ourselves
		}
		// ignore any errors here
		c.host.Connect(c.ctx, peerInfo)
	}
}
