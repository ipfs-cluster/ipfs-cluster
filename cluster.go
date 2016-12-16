package ipfscluster

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	crypto "github.com/libp2p/go-libp2p-crypto"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	swarm "github.com/libp2p/go-libp2p-swarm"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	multiaddr "github.com/multiformats/go-multiaddr"

	cid "github.com/ipfs/go-cid"
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
		rpcCh:      make(chan RPC),
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

// LocalSync makes sure that the current state of the Cluster matches
// and the desired IPFS daemon state are aligned. It makes sure that
// IPFS is pinning content that should be pinned locally, and not pinning
// other content. It will also try to recover any failed pin or unpin
// operations by retrigerring them.
func (c *Cluster) LocalSync() ([]Pin, error) {
	cState, err := c.consensus.State()
	if err != nil {
		return nil, err
	}
	changed := c.tracker.SyncState(cState)
	for _, p := range changed {
		logger.Debugf("recovering %s", p.Cid)
		err = c.tracker.Recover(p.Cid)
		if err != nil {
			logger.Errorf("Error recovering %s: %s", p.Cid, err)
			return nil, err
		}
	}
	return c.tracker.ListPins(), nil
}

// LocalSyncCid makes sure that the current state of the cluster
// and the desired IPFS daemon state are aligned for a given Cid. It
// makes sure that IPFS is pinning content that should be pinned locally,
// and not pinning other content. It will also try to recover any failed
// pin or unpin operations by retriggering them.
func (c *Cluster) LocalSyncCid(h *cid.Cid) (Pin, error) {
	changed := c.tracker.Sync(h)
	if changed {
		err := c.tracker.Recover(h)
		if err != nil {
			logger.Errorf("Error recovering %s: %s", h, err)
			return Pin{}, err
		}
	}
	return c.tracker.GetPin(h), nil
}

// GlobalSync triggers Sync() operations in all members of the Cluster.
func (c *Cluster) GlobalSync() ([]Pin, error) {
	return c.Status(), nil
}

// GlobalSunc triggers a Sync() operation for a given Cid in all members
// of the Cluster.
func (c *Cluster) GlobalSyncCid(h *cid.Cid) (Pin, error) {
	return c.StatusCid(h), nil
}

// Status returns the last known status for all Pins tracked by Cluster.
func (c *Cluster) Status() []Pin {
	// TODO: Global
	return c.tracker.ListPins()
}

// StatusCid returns the last known status for a given Cid
func (c *Cluster) StatusCid(h *cid.Cid) Pin {
	// TODO: Global
	return c.tracker.GetPin(h)
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
	rpc := NewRPC(ConsensusAddPinRPC, h)
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
	rpc := NewRPC(ConsensusRmPinRPC, h)
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
				goto HANDLEOP
			case op = <-c.consensus.RpcChan():
				goto HANDLEOP
			case op = <-c.api.RpcChan():
				goto HANDLEOP
			case op = <-c.tracker.RpcChan():
				goto HANDLEOP
			case op = <-c.remote.RpcChan():
				goto HANDLEOP
			case op = <-c.rpcCh:
				goto HANDLEOP
			case <-c.shutdownCh:
				return
			}
		HANDLEOP:
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
					Error: errors.New("unknown RPC type"),
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
	case StatusRPC:
		data = c.Status()
	case RollbackRPC:
		state, ok := grpc.Argument.(State)
		if !ok {
			err = errors.New("bad RollbackRPC type")
			break
		}
		err = c.consensus.Rollback(state)
	default:
		logger.Error("unknown operation for GenericRPC. Ignoring.")
	}

	resp := RPCResponse{
		Data:  data,
		Error: err,
	}

	grpc.ResponseCh() <- resp
}

// handleOp takes care of running the necessary action for a
// clusterRPC request and sending the response.
func (c *Cluster) handleCidRPC(crpc *CidRPC) {
	var data interface{} = nil
	var err error = nil
	var h *cid.Cid = crpc.CID
	switch crpc.Op() {
	case PinRPC:
		err = c.Pin(h)
	case UnpinRPC:
		err = c.Unpin(h)
	case ConsensusAddPinRPC:
		err = c.consensus.AddPin(h)
	case ConsensusRmPinRPC:
		err = c.consensus.RmPin(h)
	case IPFSPinRPC:
		c.tracker.Pinning(h)
		err = c.ipfs.Pin(h)
		if err != nil {
			c.tracker.PinError(h)
		} else {
			c.tracker.Pinned(h)
		}
	case IPFSUnpinRPC:
		c.tracker.Unpinning(h)
		err = c.ipfs.Unpin(h)
		if err != nil {
			c.tracker.UnpinError(h)
		} else {
			c.tracker.Unpinned(h)
		}
	case IPFSIsPinnedRPC:
		data, err = c.ipfs.IsPinned(h)
	case StatusCidRPC:
		data = c.StatusCid(h)
	case LocalSyncCidRPC:
		data, err = c.LocalSyncCid(h)
	case GlobalSyncCidRPC:
		data, err = c.GlobalSyncCid(h)
	default:
		logger.Error("unknown operation for CidRPC. Ignoring.")
	}

	resp := RPCResponse{
		Data:  data,
		Error: err,
	}

	crpc.ResponseCh() <- resp
}

func (c *Cluster) handleWrappedRPC(wrpc *WrappedRPC) {
	innerRPC := wrpc.WRPC
	var resp RPCResponse
	switch wrpc.Op() {
	case LeaderRPC:
		leader, err := c.consensus.Leader()
		if err != nil {
			resp = RPCResponse{
				Data:  nil,
				Error: err,
			}
		}
		resp, err = c.remote.MakeRemoteRPC(innerRPC, leader)
		if err != nil {
			resp = RPCResponse{
				Data:  nil,
				Error: fmt.Errorf("request to %s failed with: %s", err),
			}
		}
	case BroadcastRPC:
		resp = RPCResponse{
			Data:  nil,
			Error: errors.New("not implemented"),
		}
	default:
		resp = RPCResponse{
			Data:  nil,
			Error: errors.New("unknown WrappedRPC type"),
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
		cfg.ConsensusListenAddr, cfg.ConsensusListenPort))
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
