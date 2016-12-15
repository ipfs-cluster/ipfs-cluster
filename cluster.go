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

	config *ClusterConfig
	host   host.Host

	consensus *ClusterConsensus
	api       ClusterAPI
	ipfs      IPFSConnector
	state     ClusterState
	tracker   PinTracker

	shutdownLock sync.Mutex
	shutdown     bool
	shutdownCh   chan struct{}
	wg           sync.WaitGroup
}

// NewCluster builds a ready-to-start IPFS Cluster. It takes a ClusterAPI,
// an IPFSConnector and a ClusterState as parameters, allowing the user,
// to provide custom implementations of these components.
func NewCluster(cfg *ClusterConfig, api ClusterAPI, ipfs IPFSConnector, state ClusterState, tracker PinTracker) (*Cluster, error) {
	ctx := context.Background()
	host, err := makeHost(ctx, cfg)
	if err != nil {
		return nil, err
	}

	consensus, err := NewClusterConsensus(cfg, host, state)
	if err != nil {
		logger.Errorf("error creating consensus: %s", err)
		return nil, err
	}

	cluster := &Cluster{
		ctx:        ctx,
		config:     cfg,
		host:       host,
		consensus:  consensus,
		api:        api,
		ipfs:       ipfs,
		state:      state,
		tracker:    tracker,
		shutdownCh: make(chan struct{}),
	}

	logger.Info("starting IPFS Cluster")

	cluster.run()
	logger.Info("performing State synchronization")
	cluster.Sync()
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

func (c *Cluster) Sync() error {
	cState, err := c.consensus.State()
	if err != nil {
		return err
	}
	changed := c.tracker.SyncState(cState)
	for _, p := range changed {
		logger.Debugf("recovering %s", p.Cid)
		c.tracker.Recover(p.Cid)
	}
	return nil
}

// Pin makes the cluster Pin a Cid. This implies adding the Cid
// to the IPFS Cluster peers shared-state. Depending on the cluster
// pinning strategy, the IPFSConnector may then request the IPFS daemon
// to pin the Cid.
//
// Pin returns an error if the operation could not be persisted
// to the global state. Pin does not reflect the success or failure
// of underlying IPFS daemon pinning operations.
func (c *Cluster) Pin(h *cid.Cid) error {
	// TODO: Check this hash makes any sense
	logger.Info("pinning:", h)
	err := c.consensus.AddPin(h)
	if err != nil {
		logger.Error(err)
		return err
	}
	return nil
}

// Unpin makes the cluster Unpin a Cid. This implies adding the Cid
// to the IPFS Cluster peers shared-state. Depending on the cluster
// unpinning strategy, the IPFSConnector may then request the IPFS daemon
// to unpin the Cid.
//
// Unpin returns an error if the operation could not be persisted
// to the global state. Unpin does not reflect the success or failure
// of underlying IPFS daemon unpinning operations.
func (c *Cluster) Unpin(h *cid.Cid) error {
	// TODO: Check this hash makes any sense
	logger.Info("unpinning:", h)
	err := c.consensus.RmPin(h)
	if err != nil {
		logger.Error(err)
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
	go func() {
		defer c.wg.Done()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c.ctx = ctx
		ipfsCh := c.ipfs.RpcChan()
		consensusCh := c.consensus.RpcChan()
		apiCh := c.api.RpcChan()
		trackerCh := c.tracker.RpcChan()

		var op ClusterRPC
		for {
			select {
			case op = <-ipfsCh:
				goto HANDLEOP
			case op = <-consensusCh:
				goto HANDLEOP
			case op = <-apiCh:
				goto HANDLEOP
			case op = <-trackerCh:
				goto HANDLEOP
			case <-c.shutdownCh:
				return
			}
		HANDLEOP:
			switch op.(type) {
			case *CidClusterRPC:
				crpc := op.(*CidClusterRPC)
				go c.handleCidRPC(crpc)
			case *GenericClusterRPC:
				grpc := op.(*GenericClusterRPC)
				go c.handleGenericRPC(grpc)
			default:
				logger.Error("unknown ClusterRPC type")
			}
		}
	}()
}

func (c *Cluster) handleGenericRPC(grpc *GenericClusterRPC) {
	var data interface{} = nil
	var err error = nil
	switch grpc.Op() {
	case VersionRPC:
		data = c.Version()
	case MemberListRPC:
		data = c.Members()
	case PinListRPC:
		data = c.tracker.ListPins()
	case RollbackRPC:
		state, ok := grpc.Argument.(ClusterState)
		if !ok {
			err = errors.New("bad RollbackRPC type")
			break
		}
		err = c.consensus.Rollback(state)
	case LeaderRPC:
		// Leader RPC is a RPC that needs to be run
		// by the Consensus Leader. Arguments is a wrapped RPC.
		rpc, ok := grpc.Argument.(*ClusterRPC)
		if !ok {
			err = errors.New("bad LeaderRPC type")
		}
		data, err = c.leaderRPC(rpc)
	default:
		logger.Error("unknown operation for GenericClusterRPC. Ignoring.")
	}

	resp := RPCResponse{
		Data:  data,
		Error: err,
	}

	grpc.ResponseCh() <- resp
}

// handleOp takes care of running the necessary action for a
// clusterRPC request and sending the response.
func (c *Cluster) handleCidRPC(crpc *CidClusterRPC) {
	var data interface{} = nil
	var err error = nil
	switch crpc.Op() {
	case PinRPC:
		err = c.Pin(crpc.CID)
	case UnpinRPC:
		err = c.Unpin(crpc.CID)
	case IPFSPinRPC:
		c.tracker.Pinning(crpc.CID)
		err = c.ipfs.Pin(crpc.CID)
		if err != nil {
			c.tracker.PinError(crpc.CID)
		} else {
			c.tracker.Pinned(crpc.CID)
		}
	case IPFSUnpinRPC:
		c.tracker.Unpinning(crpc.CID)
		err = c.ipfs.Unpin(crpc.CID)
		if err != nil {
			c.tracker.UnpinError(crpc.CID)
		} else {
			c.tracker.Unpinned(crpc.CID)
		}
	case IPFSIsPinnedRPC:
		data, err = c.ipfs.IsPinned(crpc.CID)
	default:
		logger.Error("unknown operation for CidClusterRPC. Ignoring.")
	}

	resp := RPCResponse{
		Data:  data,
		Error: err,
	}

	crpc.ResponseCh() <- resp
}

// This uses libp2p to contact the cluster leader and ask him to do something
func (c *Cluster) leaderRPC(rpc *ClusterRPC) (interface{}, error) {
	return nil, errors.New("not implemented yet")
}

// makeHost makes a libp2p-host
func makeHost(ctx context.Context, cfg *ClusterConfig) (host.Host, error) {
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
