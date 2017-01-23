package ipfscluster

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	rpc "github.com/hsanjuan/go-libp2p-rpc"
	cid "github.com/ipfs/go-cid"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	swarm "github.com/libp2p/go-libp2p-swarm"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	multiaddr "github.com/multiformats/go-multiaddr"
)

// Cluster is the main IPFS cluster component. It provides
// the go-API for it and orchestrates the componenets that make up the system.
type Cluster struct {
	ctx context.Context

	config    *Config
	host      host.Host
	rpcServer *rpc.Server
	rpcClient *rpc.Client

	consensus *Consensus
	api       API
	ipfs      IPFSConnector
	state     State
	tracker   PinTracker

	shutdownLock sync.Mutex
	shutdown     bool
	shutdownCh   chan struct{}
	wg           sync.WaitGroup
}

// NewCluster builds a new IPFS Cluster. It initializes a LibP2P host, creates
// and RPC Server and client and sets up all components.
func NewCluster(cfg *Config, api API, ipfs IPFSConnector, state State, tracker PinTracker) (*Cluster, error) {
	ctx := context.Background()
	host, err := makeHost(ctx, cfg)
	if err != nil {
		return nil, err
	}

	rpcServer := rpc.NewServer(host, RPCProtocol)
	rpcClient := rpc.NewClientWithServer(host, RPCProtocol, rpcServer)

	consensus, err := NewConsensus(cfg, host, state)
	if err != nil {
		logger.Errorf("error creating consensus: %s", err)
		return nil, err
	}

	cluster := &Cluster{
		ctx:        ctx,
		config:     cfg,
		host:       host,
		rpcServer:  rpcServer,
		rpcClient:  rpcClient,
		consensus:  consensus,
		api:        api,
		ipfs:       ipfs,
		state:      state,
		tracker:    tracker,
		shutdownCh: make(chan struct{}),
	}

	err = rpcServer.RegisterName(
		"Cluster",
		&RPCAPI{cluster: cluster})
	if err != nil {
		return nil, err
	}

	// Workaround for https://github.com/libp2p/go-libp2p-swarm/issues/15
	cluster.openConns()

	defer func() {
		tracker.SetClient(rpcClient)
		ipfs.SetClient(rpcClient)
		api.SetClient(rpcClient)
		consensus.SetClient(rpcClient)
	}()

	logger.Infof("starting IPFS Cluster v%s", Version)

	cluster.run()
	return cluster, nil
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
	if err := c.consensus.Shutdown(); err != nil {
		logger.Errorf("error stopping consensus: %s", err)
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
	c.shutdownCh <- struct{}{}
	c.wg.Wait()
	c.host.Close() // Shutdown all network services
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

	logger.Info("syncing state to tracker")
	clusterPins := cState.ListPins()
	var changed []*cid.Cid

	// Track items which are not tracked
	for _, h := range clusterPins {
		if c.tracker.StatusCid(h).IPFS == Unpinned {
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
	for _, p := range c.tracker.Status() {
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
		infos = append(infos, c.tracker.StatusCid(h))
	}
	return infos, nil
}

// Status returns the GlobalPinInfo for all tracked Cids. If an error happens,
// the slice will contain as much information as could be fetched.
func (c *Cluster) Status() ([]GlobalPinInfo, error) {
	return c.globalPinInfoSlice("TrackerStatus")
}

// StatusCid returns the GlobalPinInfo for a given Cid. If an error happens,
// the GlobalPinInfo should contain as much information as could be fetched.
func (c *Cluster) StatusCid(h *cid.Cid) (GlobalPinInfo, error) {
	return c.globalPinInfoCid("TrackerStatusCid", h)
}

// LocalSync makes sure that the current state the Tracker matches
// the IPFS daemon state by triggering a Tracker.Sync() and Recover()
// on all items that need it. Returns PinInfo for items changed on Sync().
//
// LocalSync triggers recoveries asynchronously, and will not wait for
// them to fail or succeed before returning.
func (c *Cluster) LocalSync() ([]PinInfo, error) {
	status := c.tracker.Status()
	var toRecover []*cid.Cid

	for _, p := range status {
		h, _ := cid.Decode(p.CidStr)
		modified := c.tracker.Sync(h)
		if modified {
			toRecover = append(toRecover, h)
		}
	}

	logger.Infof("%d items to recover after sync", len(toRecover))
	for i, h := range toRecover {
		logger.Infof("recovering in progress for %s (%d/%d",
			h, i, len(toRecover))
		go func(h *cid.Cid) {
			c.tracker.Recover(h)
		}(h)
	}

	var changed []PinInfo
	for _, h := range toRecover {
		changed = append(changed, c.tracker.StatusCid(h))
	}
	return changed, nil
}

// LocalSyncCid performs a Tracker.Sync() operation followed by a
// Recover() when needed. It returns the latest known PinInfo for the Cid.
//
// LocalSyncCid will wait for the Recover operation to fail or succeed before
// returning.
func (c *Cluster) LocalSyncCid(h *cid.Cid) (PinInfo, error) {
	var err error
	if c.tracker.Sync(h) {
		err = c.tracker.Recover(h)
	}
	return c.tracker.StatusCid(h), err
}

// GlobalSync triggers Sync() operations in all members of the Cluster.
func (c *Cluster) GlobalSync() ([]GlobalPinInfo, error) {
	return c.globalPinInfoSlice("LocalSync")
}

// GlobalSyncCid triggers a LocalSyncCid() operation for a given Cid
// in all members of the Cluster.
//
// GlobalSyncCid will only return when all operations have either failed,
// succeeded or timed-out.
func (c *Cluster) GlobalSyncCid(h *cid.Cid) (GlobalPinInfo, error) {
	return c.globalPinInfoCid("LocalSyncCid", h)
}

// Pins returns the list of Cids managed by Cluster and which are part
// of the current global state. This is the source of truth as to which
// pins are managed, but does not indicate if the item is successfully pinned.
func (c *Cluster) Pins() []*cid.Cid {
	cState, err := c.consensus.State()
	if err != nil {
		return []*cid.Cid{}
	}
	return cState.ListPins()
}

// Pin makes the cluster Pin a Cid. This implies adding the Cid
// to the IPFS Cluster peers shared-state. Depending on the cluster
// pinning strategy, the PinTracker may then request the IPFS daemon
// to pin the Cid. When the current node is not the cluster leader,
// the request is forwarded to the leader.
//
// Pin returns an error if the operation could not be persisted
// to the global state. Pin does not reflect the success or failure
// of underlying IPFS daemon pinning operations.
func (c *Cluster) Pin(h *cid.Cid) error {
	logger.Info("pinning:", h)
	leader, err := c.consensus.Leader()
	if err != nil {
		return err
	}
	err = c.rpcClient.Call(
		leader,
		"Cluster",
		"ConsensusLogPin",
		NewCidArg(h),
		&struct{}{})

	if err != nil {
		return err
	}
	return nil
}

// Unpin makes the cluster Unpin a Cid. This implies adding the Cid
// to the IPFS Cluster peers shared-state. When the current node is
// not the cluster leader, the request is forwarded to the leader.
//
// Unpin returns an error if the operation could not be persisted
// to the global state. Unpin does not reflect the success or failure
// of underlying IPFS daemon unpinning operations.
func (c *Cluster) Unpin(h *cid.Cid) error {
	logger.Info("pinning:", h)
	leader, err := c.consensus.Leader()
	if err != nil {
		return err
	}
	err = c.rpcClient.Call(
		leader,
		"Cluster",
		"ConsensusLogUnpin",
		NewCidArg(h),
		&struct{}{})

	if err != nil {
		return err
	}
	return nil
}

// Version returns the current IPFS Cluster version
func (c *Cluster) Version() string {
	return Version
}

// Members returns the IDs of the members of this Cluster
func (c *Cluster) Members() []peer.ID {
	return c.host.Peerstore().Peers()
}

// run reads from the RPC channels of the different components and launches
// short-lived go-routines to handle any requests.
func (c *Cluster) run() {
	c.wg.Add(1)

	// Currently we do nothing other than waiting to
	// cancel our context.
	go func() {
		defer c.wg.Done()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c.ctx = ctx
		<-c.shutdownCh
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

	for _, addr := range cfg.ClusterPeers {
		pid, err := addr.ValueForProtocol(multiaddr.P_IPFS)
		if err != nil {
			return nil, err
		}

		ipfs, _ := multiaddr.NewMultiaddr("/ipfs/" + pid)
		maddr := addr.Decapsulate(ipfs)

		peerID, err := peer.IDB58Decode(pid)
		if err != nil {
			return nil, err
		}

		ps.AddAddrs(
			peerID,
			[]multiaddr.Multiaddr{maddr},
			peerstore.PermanentAddrTTL)
	}

	network, err := swarm.NewNetwork(
		ctx,
		[]multiaddr.Multiaddr{cfg.ClusterAddr},
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

// Perform a sync rpc request to multiple destinations
func (c *Cluster) multiRPC(dests []peer.ID, svcName, svcMethod string, args interface{}, reply []interface{}) []error {
	if len(dests) != len(reply) {
		panic("must have mathing dests and replies")
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
		Cid:    h,
		Status: make(map[peer.ID]PinInfo),
	}

	members := c.Members()
	replies := make([]PinInfo, len(members), len(members))
	ifaceReplies := make([]interface{}, len(members), len(members))
	for i := range replies {
		ifaceReplies[i] = &replies[i]
	}
	args := NewCidArg(h)
	errs := c.multiRPC(members, "Cluster", method, args, ifaceReplies)

	var errorMsgs string
	for i, r := range replies {
		if e := errs[i]; e != nil {
			logger.Errorf("%s: error in broadcast response from %s: %s ", c.host.ID(), members[i], e)
			errorMsgs += e.Error() + "\n"
		}
		pin.Status[r.Peer] = r
	}

	if len(errorMsgs) == 0 {
		return pin, nil
	}

	return pin, errors.New(errorMsgs)
}

func (c *Cluster) globalPinInfoSlice(method string) ([]GlobalPinInfo, error) {
	var infos []GlobalPinInfo
	fullMap := make(map[string]GlobalPinInfo)

	members := c.Members()
	replies := make([][]PinInfo, len(members), len(members))
	ifaceReplies := make([]interface{}, len(members), len(members))
	for i := range replies {
		ifaceReplies[i] = &replies[i]
	}
	errs := c.multiRPC(members, "Cluster", method, struct{}{}, ifaceReplies)

	mergePins := func(pins []PinInfo) {
		for _, p := range pins {
			item, ok := fullMap[p.CidStr]
			c, _ := cid.Decode(p.CidStr)
			if !ok {
				fullMap[p.CidStr] = GlobalPinInfo{
					Cid: c,
					Status: map[peer.ID]PinInfo{
						p.Peer: p,
					},
				}
			} else {
				item.Status[p.Peer] = p
			}
		}
	}

	var errorMsgs string
	for i, r := range replies {
		if e := errs[i]; e != nil {
			logger.Errorf("%s: error in broadcast response from %s: %s ", c.host.ID(), members[i], e)
			errorMsgs += e.Error() + "\n"
		}
		mergePins(r)
	}

	for _, v := range fullMap {
		infos = append(infos, v)
	}

	if len(errorMsgs) == 0 {
		return infos, nil
	}
	return infos, errors.New(errorMsgs)
}

// openConns is a workaround for
// https://github.com/libp2p/go-libp2p-swarm/issues/15
// It runs when consensus is initialized so we can assume
// that the cluster is more or less up.
// It should open connections for peers where they haven't
// yet been opened. By randomly sleeping we reduce the
// chance that members will open 2 connections simultaneously.
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
