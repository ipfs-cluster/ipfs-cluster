package ipfscluster

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	host "gx/ipfs/QmPTGbC34bPKaUm9wTxBo7zSCac7pDuG42ZmnXC718CKZZ/go-libp2p-host"
	multiaddr "gx/ipfs/QmUAQaWbKxGCUTuoQVvvicbQNZ9APF5pDGWyAZSe93AtKH/go-multiaddr"
	swarm "gx/ipfs/QmWfxnAiQ5TnnCgiX9ikVUKFNHRgGhbgKdx5DoKPELD7P4/go-libp2p-swarm"
	basichost "gx/ipfs/QmbzCT1CwxVZ2ednptC9RavuJe7Bv8DDi2Ne89qUrA37XM/go-libp2p/p2p/host/basic"
	peerstore "gx/ipfs/QmeXj9VAjmYQZxpmVz7VzccbJrpmr8qkCDSjfVNsPTWTYU/go-libp2p-peerstore"
	peer "gx/ipfs/QmfMmLGoKzCHDN7cGgk64PJr4iipzidDRME8HABSJqvmhC/go-libp2p-peer"
	crypto "gx/ipfs/QmfWDLQjGjVe4fr5CoztYW2DYYjRysMJrFe1RCsXLPTf46/go-libp2p-crypto"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

// Cluster is the main IPFS cluster component. It provides
// the go-API for it and orchestrates the componenets that make up the system.
type Cluster struct {
	ctx context.Context

	config *Config
	host   host.Host

	consensus *Consensus
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
func NewCluster(cfg *Config, api API, ipfs IPFSConnector, state State, tracker PinTracker) (*Cluster, error) {
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

	cluster := &Cluster{
		ctx:        ctx,
		config:     cfg,
		host:       host,
		consensus:  consensus,
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
			case op = <-c.rpcCh:
				goto HANDLEOP
			case <-c.shutdownCh:
				return
			}
		HANDLEOP:
			switch op.(type) {
			case *CidRPC:
				crpc := op.(*CidRPC)
				go c.handleCidNewRPC(crpc)
			case *GenericRPC:
				grpc := op.(*GenericRPC)
				go c.handleGenericNewRPC(grpc)
			default:
				logger.Error("unknown RPC type")
			}
		}
	}()
}

func (c *Cluster) handleGenericNewRPC(grpc *GenericRPC) {
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
	case LeaderRPC:
		// Leader RPC is a RPC that needs to be run
		// by the Consensus Leader. Arguments is a wrapped RPC.
		rpc, ok := grpc.Argument.(*RPC)
		if !ok {
			err = errors.New("bad LeaderRPC type")
		}
		data, err = c.leaderNewRPC(rpc)
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
func (c *Cluster) handleCidNewRPC(crpc *CidRPC) {
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
	case StatusCidRPC:
		data = c.StatusCid(crpc.CID)
	case LocalSyncCidRPC:
		data, err = c.LocalSyncCid(crpc.CID)
	case GlobalSyncCidRPC:
		data, err = c.GlobalSyncCid(crpc.CID)
	default:
		logger.Error("unknown operation for CidRPC. Ignoring.")
	}

	resp := RPCResponse{
		Data:  data,
		Error: err,
	}

	crpc.ResponseCh() <- resp
}

// This uses libp2p to contact the cluster leader and ask him to do something
func (c *Cluster) leaderNewRPC(rpc *RPC) (interface{}, error) {
	return nil, errors.New("not implemented yet")
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
