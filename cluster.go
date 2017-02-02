package ipfscluster

import (
	"context"
	"errors"
	"fmt"
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

	openConns(ctx, host)

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
		addrs = append(addrs, multiaddrJoin(addr, c.host.ID()))
	}

	return ID{
		ID:                 c.host.ID(),
		PublicKey:          c.host.Peerstore().PubKey(c.host.ID()),
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
	logger.Debugf("peerAdd called with %s", addr)
	pid, decapAddr, err := multiaddrSplit(addr)
	if err != nil {
		id := ID{
			Error: err.Error(),
		}
		return id, err
	}

	if c.peerManager.isPeer(pid) {
		id, _ := c.getIDForPeer(pid)
		return id, errors.New("peer is already in the cluster")
	}

	// Attempt connection and find which is the multiaddress
	// we use to connect to that peer
	localMAddr, err := getLocalMultiaddrTo(c.ctx, c.host, pid, decapAddr)
	if err != nil {
		logger.Error(err)
		id := ID{ID: pid, Error: err.Error()}
		return id, err
	}
	logger.Debugf("local multiaddress to %s is %s", pid, localMAddr)

	c.peerManager.addPeer(addr)

	// Let new peer join the cluster
	err = c.rpcClient.Call(pid, "Cluster", "Join",
		MultiaddrToSerial(localMAddr), &struct{}{})
	if err != nil {
		msg := "error adding peer: " + err.Error()
		logger.Error(msg)
		c.peerManager.rmPeer(pid, false)
		id := ID{ID: pid, Error: msg}
		return id, errors.New(msg)
	}
	logger.Infof("added peer to cluster: %s", addr)
	return c.getIDForPeer(pid)
}

// PeerRemove removes a peer from this Cluster.
//
// The peer will be removed from the consensus peer set,
// remove all cluster peers from its configuration and
// shut itself down.
func (c *Cluster) PeerRemove(pid peer.ID) error {
	if !c.peerManager.isPeer(pid) {
		return fmt.Errorf("%s is not a peer", pid.Pretty())
	}
	peers := c.peerManager.peers()
	replies := make([]struct{}, len(peers), len(peers))
	errs := c.multiRPC(peers, "Cluster", "PeerManagerRmPeer",
		pid, copyEmptyStructToIfaces(replies))
	errorMsgs := ""
	for i, err := range errs {
		if err != nil && peers[i] != pid {
			logger.Error(err)
			errorMsgs += fmt.Sprintf("%s: %s\n",
				peers[i].Pretty(),
				err.Error())
		}
	}
	if errorMsgs != "" {
		return errors.New(errorMsgs)
	}
	return nil
}

// Join adds this peer to an existing cluster by fetching the cluster members
// from the given multiaddress and then proceeding to contact each of them
// and registering with them.
func (c *Cluster) Join(addr ma.Multiaddr) error {
	logger.Debugf("Join(%s)", addr)

	if len(c.peerManager.peers()) > 1 {
		return errors.New("only single-node clusters can be joined")
	}

	// Add bootstrapping node so we can talk to it
	pid, decapAddr, err := multiaddrSplit(addr)
	if err != nil {
		return errors.New("bad address: " + err.Error())
	}

	c.host.Peerstore().AddAddr(pid, decapAddr, peerstore.TempAddrTTL)

	// If there is no connection open, the first thing is to register
	// ourselves with the new node, otherwise we assume this is done.
	// The main problem is that we cannot know which IP/interface we use
	// for a connection unless we opened it ourselves (i.e. otherwise
	// localAddr() is /ip4/0.0.0.0/. And we cannot force close/re-open
	// in Join because we might have been called remotely by PeerAdd().
	// But in that case, the caller will have added us so we can skip
	// the step.
	if len(c.host.Network().ConnsToPeer(pid)) == 0 {
		whoAmI, err := getLocalMultiaddrTo(c.ctx, c.host, pid, decapAddr)
		if err != nil {
			logger.Error(err)
			return errors.New("cannot contact cluster: " + err.Error())
		}
		var tmp peer.ID
		err = c.rpcClient.Call(pid, "Cluster", "PeerManagerAddPeer",
			MultiaddrToSerial(whoAmI), &tmp)
		if err != nil {
			msg := fmt.Sprintf("error joining %s: %s", pid.Pretty(), err)
			logger.Error(msg)
			return errors.New(msg)
		}
	}

	// At this point, either we registered ourselves or we assume
	// we were registered because we were contacted by that node
	// so we get some cluster peers and check-in with them all

	id, err := c.getIDForPeer(pid)
	if err != nil {
		return err
	}
	err = c.checkInWithPeers(append(id.ClusterPeers, addr))
	if err != nil {
		return err
	}
	logger.Infof("peer joined %s's cluster", pid.Pretty())
	return nil
}

// checkInWithPeers peerManager.addPeer locally and remove for
// the peers in the given list, provided that:
// * They are not in our peer list
// * We have no open connections to them (if so we just add them locally)
// Then, we fetch the ID()s from all previously-unknown cluster peers and
// repeat with their list of Cluster peers.
// This allows to checkIn with all peers in the cluster even if they
// were still bootstrapping to a different node at the same time as us.
func (c *Cluster) checkInWithPeers(clusterPeers []ma.Multiaddr) error {
	logger.Debugf("checkInWithPeers %s", clusterPeers)

	if len(clusterPeers) == 0 {
		return nil
	}
	newClusterPeersMap := make(map[peer.ID]ma.Multiaddr)
	errs := make([]string, len(clusterPeers), len(clusterPeers))
	var mux sync.Mutex
	var wg sync.WaitGroup
	for i, cp := range clusterPeers {
		wg.Add(1)
		go func(cp ma.Multiaddr, i int) {
			defer wg.Done()
			logger.Debugf("starting checking with %s", cp)
			pid, addr, _ := multiaddrSplit(cp)
			if c.peerManager.isPeer(pid) {
				return
			}
			if len(c.host.Network().ConnsToPeer(pid)) > 0 {
				logger.Debugf("no checkin: already connected to %s", pid.Pretty())
				// They are connected. They must have us.
				// We don't check in, just add them
				c.peerManager.addPeer(cp)
				return
			}

			whoAmI, err := getLocalMultiaddrTo(c.ctx, c.host, pid, addr)
			if err != nil {
				msg := fmt.Sprintf("cannot connect to %s. ", cp)
				logger.Error(msg)
				errs[i] = msg
				return
			}
			logger.Debugf("checin: I am %s", whoAmI)

			var tmp peer.ID
			err = c.rpcClient.Call(pid, "Cluster", "PeerManagerAddPeer",
				MultiaddrToSerial(whoAmI), &tmp)
			if err != nil {
				msg := fmt.Sprintf("error checking-in with %s: %s", pid.Pretty(), err)
				logger.Error(msg)
				errs[i] = msg
				return
			}
			c.peerManager.addPeer(cp)

			id, err := c.getIDForPeer(pid)
			if err != nil {
				msg := fmt.Sprintf("error getting peers from %s: %s", pid.Pretty(), err)
				logger.Error(msg)
				errs[i] = msg
				return
			}
			mux.Lock()
			for _, newcp := range id.ClusterPeers {
				newpid, _, _ := multiaddrSplit(newcp)
				newClusterPeersMap[newpid] = newcp
			}
			mux.Unlock()
		}(cp, i)
	}
	wg.Wait()

	// Collect errors and roll back
	errorMsgs := ""
	for _, e := range errs {
		if e != "" {
			errorMsgs += e + "\n"
		}
	}

	if errorMsgs != "" {
		// Broadcast PeerManagerRmPeer without shutting ourselves down
		peers := c.peerManager.peers()
		replies := make([]struct{}, len(peers), len(peers))
		c.multiRPC(peers, "Cluster", "PeerManagerRmPeerNoShutdown",
			c.host.ID(), copyEmptyStructToIfaces(replies))
		return errors.New(errorMsgs)
	}

	newPeers := make([]ma.Multiaddr, 0, len(newClusterPeersMap))
	for _, v := range newClusterPeersMap {
		newPeers = append(newPeers, v)
	}
	// Recursively check with all retrieved peers
	return c.checkInWithPeers(newPeers)
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
				c.readyCh <- struct{}{}
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

func (c *Cluster) getIDForPeer(pid peer.ID) (ID, error) {
	idSerial := ID{ID: pid}.ToSerial()
	err := c.rpcClient.Call(
		pid, "Cluster", "ID", struct{}{}, &idSerial)
	id := idSerial.ToID()
	if err != nil {
		logger.Error(err)
		id.Error = err.Error()
	}
	return id, err
}
