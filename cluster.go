package ipfscluster

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	cid "github.com/ipfs/go-cid"
	crypto "github.com/libp2p/go-libp2p-crypto"
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

	config *Config
	host   host.Host

	consensus *Consensus
	remote    Remote
	api       API
	ipfs      IPFSConnector
	state     State
	tracker   PinTracker

	rpcCh chan RPC

	shutdownLock sync.Mutex
	shutdown     bool
	shutdownCh   chan struct{}
	wg           sync.WaitGroup
}

// NewCluster builds a ready-to-start IPFS Cluster. It takes a API,
// an IPFSConnector and a State as parameters, allowing the user,
// to provide custom implementations of these components.
func NewCluster(cfg *Config, api API, ipfs IPFSConnector, state State, tracker PinTracker, remote Remote) (*Cluster, error) {
	ctx := context.Background()
	host, err := makeHost(ctx, cfg)
	if err != nil {
		return nil, err
	}

	consensus, err := NewConsensus(cfg, host, state)
	if err != nil {
		logger.Errorf("error creating consensus: %s", err)
		return nil, err
	}

	remote.SetHost(host)

	cluster := &Cluster{
		ctx:        ctx,
		config:     cfg,
		host:       host,
		consensus:  consensus,
		remote:     remote,
		api:        api,
		ipfs:       ipfs,
		state:      state,
		tracker:    tracker,
		rpcCh:      make(chan RPC, RPCMaxQueue),
		shutdownCh: make(chan struct{}),
	}

	logger.Info("starting IPFS Cluster")

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
	if err := c.remote.Shutdown(); err != nil {
		logger.Errorf("error stopping remote: %s", err)
		return err
	}
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

	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	logger.Info("syncing state to tracker")
	clusterPins := cState.ListPins()
	var changed []*cid.Cid

	// Track items which are not tracked
	for _, h := range clusterPins {
		if c.tracker.StatusCid(h).IPFS == Unpinned {
			changed = append(changed, h)
			MakeRPC(ctx, c.rpcCh, NewRPC(TrackRPC, h), false)

		}
	}

	// Untrack items which should not be tracked
	for _, p := range c.tracker.Status() {
		h, _ := cid.Decode(p.CidStr)
		if !cState.HasPin(h) {
			changed = append(changed, h)
			MakeRPC(ctx, c.rpcCh, NewRPC(UntrackRPC, h), false)
		}
	}

	var infos []PinInfo
	for _, h := range changed {
		infos = append(infos, c.tracker.StatusCid(h))
	}
	return infos, nil
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

	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()
	logger.Infof("%d items to recover after sync", len(toRecover))
	for i, h := range toRecover {
		logger.Infof("recovering in progress for %s (%d/%d", h, i, len(toRecover))
		MakeRPC(ctx, c.rpcCh, NewRPC(TrackerRecoverRPC, h), false)
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
	return c.globalPinInfoSlice(LocalSyncRPC)
}

// GlobalSyncCid triggers a LocalSyncCid() operation for a given Cid
// in all members of the Cluster.
//
// GlobalSyncCid will only return when all operations have either failed,
// succeeded or timed-out.
func (c *Cluster) GlobalSyncCid(h *cid.Cid) (GlobalPinInfo, error) {
	return c.globalPinInfoCid(LocalSyncCidRPC, h)
}

// Status returns the GlobalPinInfo for all tracked Cids. If an error happens,
// the slice will contain as much information as could be fetched.
func (c *Cluster) Status() ([]GlobalPinInfo, error) {
	return c.globalPinInfoSlice(TrackerStatusRPC)
}

// StatusCid returns the GlobalPinInfo for a given Cid. If an error happens,
// the GlobalPinInfo should contain as much information as could be fetched.
func (c *Cluster) StatusCid(h *cid.Cid) (GlobalPinInfo, error) {
	return c.globalPinInfoCid(TrackerStatusCidRPC, h)
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
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	logger.Info("pinning:", h)
	rpc := NewRPC(ConsensusLogPinRPC, h)
	wrpc := NewRPC(LeaderRPC, rpc)
	resp := MakeRPC(ctx, c.rpcCh, wrpc, true)
	if resp.Error != nil {
		logger.Error(resp.Error)
		return resp.Error
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
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	logger.Info("unpinning:", h)
	rpc := NewRPC(ConsensusLogUnpinRPC, h)
	wrpc := NewRPC(LeaderRPC, rpc)
	resp := MakeRPC(ctx, c.rpcCh, wrpc, true)
	if resp.Error != nil {
		logger.Error(resp.Error)
		return resp.Error
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
	go func() {
		defer c.wg.Done()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c.ctx = ctx

		var op RPC
		for {
			select {
			case op = <-c.ipfs.RpcChan():
			case op = <-c.consensus.RpcChan():
			case op = <-c.api.RpcChan():
			case op = <-c.tracker.RpcChan():
			case op = <-c.remote.RpcChan():
			case op = <-c.rpcCh:
			case <-c.shutdownCh:
				return
			}

			switch op.(type) {
			case *CidRPC:
				crpc := op.(*CidRPC)
				go c.handleCidRPC(crpc)
			case *GenericRPC:
				grpc := op.(*GenericRPC)
				go c.handleGenericRPC(grpc)
			case *WrappedRPC:
				wrpc := op.(*WrappedRPC)
				go c.handleWrappedRPC(wrpc)
			default:
				logger.Error("unknown RPC type")
				op.ResponseCh() <- RPCResponse{
					Data:  nil,
					Error: NewRPCError("unknown RPC type"),
				}
			}
		}
	}()
}

func (c *Cluster) handleGenericRPC(grpc *GenericRPC) {
	var data interface{} = nil
	var err error = nil
	switch grpc.Op() {
	case VersionRPC:
		data = c.Version()
	case MemberListRPC:
		data = c.Members()
	case PinListRPC:
		data = c.Pins()
	case LocalSyncRPC:
		data, err = c.LocalSync()
	case GlobalSyncRPC:
		data, err = c.GlobalSync()
	case StateSyncRPC:
		data, err = c.StateSync()
	case StatusRPC:
		data, err = c.Status()
	case TrackerStatusRPC:
		data = c.tracker.Status()
	case RollbackRPC:
		// State, ok := grpc.Argument.(State)
		// if !ok {
		// 	err = errors.New("bad RollbackRPC type")
		// 	break
		// }
		// err = c.consensus.Rollback(state)
	default:
		logger.Error("unknown operation for GenericRPC. Ignoring.")
	}

	resp := RPCResponse{
		Data:  data,
		Error: CastRPCError(err),
	}

	grpc.ResponseCh() <- resp
}

// handleOp takes care of running the necessary action for a
// clusterRPC request and sending the response.
func (c *Cluster) handleCidRPC(crpc *CidRPC) {
	var data interface{} = nil
	var err error = nil
	var h *cid.Cid = crpc.CID()
	switch crpc.Op() {
	case PinRPC:
		err = c.Pin(h)
	case UnpinRPC:
		err = c.Unpin(h)
	case StatusCidRPC:
		data, err = c.StatusCid(h)
	case LocalSyncCidRPC:
		data, err = c.LocalSyncCid(h)
	case GlobalSyncCidRPC:
		data, err = c.GlobalSyncCid(h)
	case ConsensusLogPinRPC:
		err = c.consensus.LogPin(h)
	case ConsensusLogUnpinRPC:
		err = c.consensus.LogUnpin(h)
	case TrackRPC:
		err = c.tracker.Track(h)
	case UntrackRPC:
		err = c.tracker.Untrack(h)
	case TrackerStatusCidRPC:
		data = c.tracker.StatusCid(h)
	case TrackerRecoverRPC:
		err = c.tracker.Recover(h)
	case IPFSPinRPC:
		err = c.ipfs.Pin(h)
	case IPFSUnpinRPC:
		err = c.ipfs.Unpin(h)
	case IPFSIsPinnedRPC:
		data, err = c.ipfs.IsPinned(h)

	default:
		logger.Error("unknown operation for CidRPC. Ignoring.")
	}

	resp := RPCResponse{
		Data:  data,
		Error: CastRPCError(err),
	}

	crpc.ResponseCh() <- resp
}

func (c *Cluster) handleWrappedRPC(wrpc *WrappedRPC) {
	innerRPC := wrpc.WRPC
	var resp RPCResponse
	// resp initialization
	switch innerRPC.Op() {
	case TrackerStatusRPC, LocalSyncRPC:
		resp = RPCResponse{
			Data: []PinInfo{},
		}
	case TrackerStatusCidRPC, LocalSyncCidRPC:
		resp = RPCResponse{
			Data: PinInfo{},
		}
	default:
		resp = RPCResponse{}
	}

	switch wrpc.Op() {
	case LeaderRPC:
		// This is very generic for the moment. Only used for consensus
		// LogPin/unpin.
		leader, err := c.consensus.Leader()
		if err != nil {
			resp = RPCResponse{
				Data:  nil,
				Error: CastRPCError(err),
			}
		}
		err = c.remote.MakeRemoteRPC(innerRPC, leader, &resp)
		if err != nil {
			resp = RPCResponse{
				Data:  nil,
				Error: CastRPCError(fmt.Errorf("request to %s failed with: %s", err)),
			}
		}
	case BroadcastRPC:
		var wg sync.WaitGroup
		var responses []RPCResponse
		members := c.Members()
		rch := make(chan RPCResponse, len(members))

		makeRemote := func(p peer.ID, r RPCResponse) {
			defer wg.Done()
			err := c.remote.MakeRemoteRPC(innerRPC, p, &r)
			if err != nil {
				logger.Error("Error making remote RPC: ", err)
				rch <- RPCResponse{
					Error: CastRPCError(err),
				}
			} else {
				rch <- r
			}
		}
		wg.Add(len(members))
		for _, m := range members {
			go makeRemote(m, resp)
		}
		wg.Wait()
		close(rch)

		for r := range rch {
			responses = append(responses, r)
		}

		resp = RPCResponse{
			Data:  responses,
			Error: nil,
		}
	default:
		resp = RPCResponse{
			Data:  nil,
			Error: NewRPCError("unknown WrappedRPC type"),
		}
	}

	wrpc.ResponseCh() <- resp
}

// makeHost makes a libp2p-host
func makeHost(ctx context.Context, cfg *Config) (host.Host, error) {
	ps := peerstore.NewPeerstore()
	peerID, err := peer.IDB58Decode(cfg.ID)
	if err != nil {
		logger.Error("decoding ID: ", err)
		return nil, err
	}

	pkb, err := base64.StdEncoding.DecodeString(cfg.PrivateKey)
	if err != nil {
		logger.Error("decoding private key base64: ", err)
		return nil, err
	}

	privateKey, err := crypto.UnmarshalPrivateKey(pkb)
	if err != nil {
		logger.Error("unmarshaling private key", err)
		return nil, err
	}

	publicKey := privateKey.GetPublic()

	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d",
		cfg.ClusterAddr, cfg.ClusterPort))
	if err != nil {
		return nil, err
	}

	if err := ps.AddPubKey(peerID, publicKey); err != nil {
		return nil, err
	}

	if err := ps.AddPrivKey(peerID, privateKey); err != nil {
		return nil, err
	}

	for _, cpeer := range cfg.ClusterPeers {
		addr, err := multiaddr.NewMultiaddr(cpeer)
		if err != nil {
			logger.Errorf("parsing cluster peer multiaddress %s: %s", cpeer, err)
			return nil, err
		}

		pid, err := addr.ValueForProtocol(multiaddr.P_IPFS)
		if err != nil {
			return nil, err
		}

		strAddr := strings.Split(addr.String(), "/ipfs/")[0]
		maddr, err := multiaddr.NewMultiaddr(strAddr)
		if err != nil {
			return nil, err
		}

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
		[]multiaddr.Multiaddr{addr},
		peerID,
		ps,
		nil,
	)

	if err != nil {
		return nil, err
	}

	bhost := basichost.New(network)
	return bhost, nil
}

func (c *Cluster) globalPinInfoCid(op RPCOp, h *cid.Cid) (GlobalPinInfo, error) {
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	pin := GlobalPinInfo{
		Cid:    h,
		Status: make(map[peer.ID]PinInfo),
	}

	rpc := NewRPC(op, h)
	wrpc := NewRPC(BroadcastRPC, rpc)
	resp := MakeRPC(ctx, c.rpcCh, wrpc, true)
	if resp.Error != nil {
		return pin, resp.Error
	}

	responses, ok := resp.Data.([]RPCResponse)
	if !ok {
		return pin, errors.New("unexpected responses format")
	}

	var errorMsgs string
	for _, r := range responses {
		if r.Error != nil {
			logger.Error(r.Error)
			errorMsgs += r.Error.Error() + "\n"
		}
		info, ok := r.Data.(PinInfo)
		if !ok {
			return pin, errors.New("unexpected response format")
		}
		pin.Status[info.Peer] = info
	}

	if len(errorMsgs) == 0 {
		return pin, nil
	} else {
		return pin, errors.New(errorMsgs)
	}
}

func (c *Cluster) globalPinInfoSlice(op RPCOp) ([]GlobalPinInfo, error) {
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	var infos []GlobalPinInfo
	fullMap := make(map[string]GlobalPinInfo)

	rpc := NewRPC(op, nil)
	wrpc := NewRPC(BroadcastRPC, rpc)
	resp := MakeRPC(ctx, c.rpcCh, wrpc, true)
	if resp.Error != nil {
		return nil, resp.Error
	}

	responses, ok := resp.Data.([]RPCResponse)
	if !ok {
		return infos, errors.New("unexpected responses format")
	}

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
	for _, r := range responses {
		if r.Error != nil {
			logger.Error("error in one of the broadcast responses: ", r.Error)
			errorMsgs += r.Error.Error() + "\n"
		}
		pins, ok := r.Data.([]PinInfo)
		if !ok {
			return infos, fmt.Errorf("unexpected response format: %+v", r.Data)
		}
		mergePins(pins)
	}

	for _, v := range fullMap {
		infos = append(infos, v)
	}

	if len(errorMsgs) == 0 {
		return infos, nil
	} else {
		return infos, errors.New(errorMsgs)
	}
}
