package ipfscluster

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

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
	ctx    context.Context
	cancel context.CancelFunc

	config *ClusterConfig
	host   host.Host

	consensus *ClusterConsensus
	api       ClusterAPI
	ipfs      IPFSConnector
	state     ClusterState
}

// NewCluster builds a ready-to-start IPFS Cluster. It takes a ClusterAPI,
// an IPFSConnector and a ClusterState as parameters, allowing the user,
// to provide custom implementations of these components.
func NewCluster(cfg *ClusterConfig, api ClusterAPI, ipfs IPFSConnector, state ClusterState) (*Cluster, error) {
	ctx, cancel := context.WithCancel(context.Background())
	host, err := makeHost(ctx, cfg)
	if err != nil {
		return nil, err
	}

	consensus, err := NewClusterConsensus(cfg, host, state)
	if err != nil {
		logger.Errorf("Error creating consensus: %s", err)
		return nil, err
	}

	cluster := &Cluster{
		ctx:       ctx,
		cancel:    cancel,
		config:    cfg,
		host:      host,
		consensus: consensus,
		api:       api,
		ipfs:      ipfs,
		state:     state,
	}

	logger.Info("Starting IPFS Cluster")
	go cluster.run()
	return cluster, nil
}

// Shutdown stops the IPFS cluster components
func (c *Cluster) Shutdown() error {
	logger.Info("Shutting down IPFS Cluster")
	if err := c.consensus.Shutdown(); err != nil {
		logger.Errorf("Error stopping consensus: %s", err)
		return err
	}
	if err := c.api.Shutdown(); err != nil {
		logger.Errorf("Error stopping API: %s", err)
		return err
	}
	if err := c.ipfs.Shutdown(); err != nil {
		logger.Errorf("Error stopping IPFS Connector: %s", err)
		return err
	}
	c.cancel()
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
	logger.Info("Pinning:", h)
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
	logger.Info("Unpinning:", h)
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
	ipfsCh := c.ipfs.RpcChan()
	consensusCh := c.consensus.RpcChan()
	apiCh := c.api.RpcChan()

	for {
		select {
		case ipfsOp := <-ipfsCh:
			go c.handleOp(&ipfsOp)
		case consensusOp := <-consensusCh:
			go c.handleOp(&consensusOp)
		case apiOp := <-apiCh:
			go c.handleOp(&apiOp)
		case <-c.ctx.Done():
			logger.Debug("Cluster is Done()")
			return
		}
	}
}

// handleOp takes care of running the necessary action for a
// clusterRPC request and sending the response.
func (c *Cluster) handleOp(op *ClusterRPC) {
	var data interface{} = nil
	var err error = nil
	switch op.Method {
	case PinRPC:
		hash, ok := op.Arguments.(cid.Cid)
		if !ok {
			err = errors.New("Bad PinRPC type")
			break
		}
		err = c.Pin(&hash)
	case UnpinRPC:
		hash, ok := op.Arguments.(cid.Cid)
		if !ok {
			err = errors.New("Bad UnpinRPC type")
			break
		}
		err = c.Unpin(&hash)
	case PinListRPC:
		data, err = c.consensus.ListPins()
	case IPFSPinRPC:
		hash, ok := op.Arguments.(cid.Cid)
		if !ok {
			err = errors.New("Bad IPFSPinRPC type")
			break
		}
		err = c.ipfs.Pin(&hash)
	case IPFSUnpinRPC:
		hash, ok := op.Arguments.(cid.Cid)
		if !ok {
			err = errors.New("Bad IPFSUnpinRPC type")
			break
		}
		err = c.ipfs.Unpin(&hash)
	case VersionRPC:
		data = c.Version()
	case MemberListRPC:
		data = c.Members()
	case RollbackRPC:
		state, ok := op.Arguments.(ClusterState)
		if !ok {
			err = errors.New("Bad RollbackRPC type")
			break
		}
		err = c.consensus.Rollback(state)
	case LeaderRPC:
		// Leader RPC is a RPC that needs to be run
		// by the Consensus Leader. Arguments is a wrapped RPC.
		rpc, ok := op.Arguments.(*ClusterRPC)
		if !ok {
			err = errors.New("Bad LeaderRPC type")
		}
		data, err = c.leaderRPC(rpc)
	default:
		logger.Error("Unknown operation. Ignoring")
	}

	resp := RPCResponse{
		Data:  data,
		Error: err,
	}

	op.ResponseCh <- resp
}

// This uses libp2p to contact the cluster leader and ask him to do something
func (c *Cluster) leaderRPC(rpc *ClusterRPC) (interface{}, error) {
	return nil, errors.New("Not implemented yet")
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
