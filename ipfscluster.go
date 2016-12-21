// package ipfscluster implements a wrapper for the IPFS deamon which
// allows to orchestrate a number of tasks between several IPFS nodes.
//
// IPFS Cluster uses a consensus algorithm and libP2P to keep a shared
// state between the different members of the cluster. This state is
// primarily used to keep track of pinned items, and ensure that an
// item is pinned in different places.
package ipfscluster

import (
	"context"
	"time"

	host "github.com/libp2p/go-libp2p-host"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
)

var logger = logging.Logger("ipfscluster")

// Current Cluster version.
const Version = "0.0.1"

// RPCMaxQueue can be used to set the size of the RPC channels,
// which will start blocking on send after reaching this number.
var RPCMaxQueue = 256

// MakeRPCRetryInterval specifies how long to wait before retrying
// to put a RPC request in the channel in MakeRPC().
var MakeRPCRetryInterval time.Duration = 1 * time.Second

// SilentRaft controls whether all Raft log messages are discarded.
var SilentRaft = true

// SetLogLevel sets the level in the logs
func SetLogLevel(l string) {
	/*
		CRITICAL Level = iota
		ERROR
		WARNING
		NOTICE
		INFO
		DEBUG
	*/
	logging.SetLogLevel("ipfs-cluster", l)
}

// ClusterComponent represents a piece of ipfscluster. Cluster components
// usually run their own goroutines (a http server for example). They
// communicate with Cluster via a channel carrying RPC operations.
// These operations request tasks from other components. This way all components
// are independent from each other and can be swapped as long as they maintain
// RPC compatibility with Cluster.
type ClusterComponent interface {
	Shutdown() error
	RpcChan() <-chan RPC
}

// API is a component which offers an API for Cluster. This is
// a base component.
type API interface {
	ClusterComponent
}

// IPFSConnector is a component which allows cluster to interact with
// an IPFS daemon. This is a base component.
type IPFSConnector interface {
	ClusterComponent
	Pin(*cid.Cid) error
	Unpin(*cid.Cid) error
	IsPinned(*cid.Cid) (bool, error)
}

// Peered represents a component which needs to be aware of the peers
// in the Cluster and of any changes to the peer set.
type Peered interface {
	AddPeer(p peer.ID)
	RmPeer(p peer.ID)
	SetPeers(peers []peer.ID)
}

// State represents the shared state of the cluster and it
// is used by the Consensus component to keep track of
// objects which objects are pinned. This component should be thread safe.
type State interface {
	// AddPin adds a pin to the State
	AddPin(*cid.Cid) error
	// RmPin removes a pin from the State
	RmPin(*cid.Cid) error
	// ListPins lists all the pins in the state
	ListPins() []*cid.Cid
	// HasPin returns true if the state is holding a Cid
	HasPin(*cid.Cid) bool
}

// PinTracker represents a component which tracks the status of
// the pins in this cluster and ensures they are in sync with the
// IPFS daemon. This component should be thread safe.
type PinTracker interface {
	ClusterComponent
	// Track tells the tracker that a Cid is now under its supervision
	// The tracker may decide to perform an IPFS pin.
	Track(*cid.Cid) error
	// Untrack tells the tracker that a Cid is to be forgotten. The tracker
	// may perform an IPFS unpin operation.
	Untrack(*cid.Cid) error
	// Status returns the list of pins with their local status.
	Status() []PinInfo
	// StatusCid returns the local status of a given Cid.
	StatusCid(*cid.Cid) PinInfo
	// Sync makes sure that the Cid status reflect the real IPFS status.
	// The return value indicates if the Cid status deserved attention,
	// either because its state was updated or because it is in error state.
	Sync(*cid.Cid) bool
	// Recover retriggers a Pin/Unpin operation in Cids with error status.
	Recover(*cid.Cid) error
}

// Remote represents a component which takes care of
// executing RPC requests in a different cluster member as well as
// handling any incoming remote requests from other nodes.
type Remote interface {
	ClusterComponent
	// MakeRemoteRPC performs an RPC requests to a remote peer.
	// The response is decoded onto the RPCResponse provided.
	MakeRemoteRPC(RPC, peer.ID, *RPCResponse) error
	// SetHost provides a libp2p host to use by this remote
	SetHost(host.Host)
}

// MakeRPC sends a RPC object over a channel and optionally waits for a
// Response on the RPC.ResponseCh channel. It can be used by any
// ClusterComponent to simplify making RPC requests to Cluster.
// The ctx parameter must be a cancellable context, and can be used to
// timeout requests.
// If the message cannot be placed in the RPC channel, retries will be
// issued every MakeRPCRetryInterval.
func MakeRPC(ctx context.Context, rpcCh chan RPC, r RPC, waitForResponse bool) RPCResponse {
	logger.Debugf("sending RPC %d", r.Op())
	exitLoop := false
	for !exitLoop {
		select {
		case rpcCh <- r:
			exitLoop = true
		case <-ctx.Done():
			logger.Debug("cancelling sending RPC")
			return RPCResponse{
				Data:  nil,
				Error: NewRPCError("operation timed out while sending RPC"),
			}
		default:
			logger.Errorf("RPC channel is full. Will retry request %d", r.Op())
			time.Sleep(MakeRPCRetryInterval)
		}
	}
	if !waitForResponse {
		logger.Debug("not waiting for response. Returning directly")
		return RPCResponse{}
	}

	logger.Debug("waiting for response")
	select {
	case resp, ok := <-r.ResponseCh():
		logger.Debug("response received")
		if !ok {
			logger.Warning("response channel closed. Ignoring")
			return RPCResponse{
				Data:  nil,
				Error: nil,
			}
		}
		return resp
	case <-ctx.Done():
		logger.Debug("cancelling waiting for RPC Response")
		return RPCResponse{
			Data:  nil,
			Error: NewRPCError("operation timed out while waiting for response"),
		}
	}
}
