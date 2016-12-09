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
	"errors"
	logging "gx/ipfs/QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52/go-log"
	"time"

	peer "gx/ipfs/QmfMmLGoKzCHDN7cGgk64PJr4iipzidDRME8HABSJqvmhC/go-libp2p-peer"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

var logger = logging.Logger("ipfs-cluster")

// Current Cluster version.
const Version = "0.0.1"

// RPCMaxQueue can be used to set the size of the ClusterRPC channels,
// which will start blocking on send after reaching this number.
var RPCMaxQueue = 128

// MakeRPCRetryInterval specifies how long to wait before retrying
// to put a ClusterRPC request in the channel in MakeRPC().
var MakeRPCRetryInterval time.Duration = 1

// ClusterComponent represents a piece of ipfscluster. Cluster components
// usually run their own goroutines (a http server for example). They
// communicate with Cluster via a channel carrying ClusterRPC operations.
// These operations request tasks from other components. This way all components
// are independent from each other and can be swapped as long as they maintain
// RPC compatibility with Cluster.
type ClusterComponent interface {
	Shutdown() error
	RpcChan() <-chan ClusterRPC
}

// ClusterAPI is a component which offers an API for Cluster. This is
// a base component.
type ClusterAPI interface {
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

// ClusterState represents the shared state of the cluster and it
// is used by the ClusterConsensus component to keep track of
// objects which objects are pinned. This component should be thread safe.
type ClusterState interface {
	// AddPin adds a pin to the ClusterState
	AddPin(*cid.Cid) error
	// RmPin removes a pin from the ClusterState
	RmPin(*cid.Cid) error
	// ListPins lists all the pins in the state
	ListPins() []*cid.Cid
}

// PinTracker represents a component which tracks the status of
// the pins in this cluster and ensures they are in sync with the
// IPFS daemon. This component should be thread safe.
type PinTracker interface {
	ClusterComponent
	// Pinning tells the pin tracker that a pin is being pinned by IPFS
	Pinning(*cid.Cid) error
	// Pinned tells the pin tracer is pinned by IPFS
	Pinned(*cid.Cid) error
	// Pinned tells the pin tracker is being unpinned by IPFS
	Unpinning(*cid.Cid) error
	// Unpinned tells the pin tracker that a pin has been unpinned by IFPS
	Unpinned(*cid.Cid) error
	// PinError tells the pin tracker that an IPFS pin operation has failed
	PinError(*cid.Cid) error
	// UnpinError tells the pin tracker that an IPFS unpin operation has failed
	UnpinError(*cid.Cid) error
	// ListPins returns the list of pins with their status
	ListPins() []Pin
	// GetPin returns a Pin.
	GetPin(*cid.Cid) Pin
	// Sync makes sure that the Cid status reflect the real IPFS status. If not,
	// the status is marked as error. The return value indicates if the
	// Pin status was updated.
	Sync(*cid.Cid) bool
	// Recover attempts to recover an error by re-[un]pinning the item.
	Recover(*cid.Cid) error
	// SyncAll runs Sync() on every known Pin. It returns a list of changed Pins
	SyncAll() []Pin
	// SyncState makes sure that the tracked Pins matches those in the
	// cluster state and runs SyncAll(). It returns a list of changed Pins.
	SyncState(ClusterState) []Pin
}

// MakeRPC sends a ClusterRPC object over a channel and optionally waits for a
// Response on the ClusterRPC.ResponseCh channel. It can be used by any
// ClusterComponent to simplify making RPC requests to Cluster.
// The ctx parameter must be a cancellable context, and can be used to
// timeout requests.
// If the message cannot be placed in the ClusterRPC channel, retries will be
// issued every MakeRPCRetryInterval seconds.
func MakeRPC(ctx context.Context, rpcCh chan ClusterRPC, r ClusterRPC, waitForResponse bool) RPCResponse {
	logger.Debugf("Sending RPC %d", r.Op())
	exitLoop := false
	for !exitLoop {
		select {
		case rpcCh <- r:
			exitLoop = true
		case <-ctx.Done():
			logger.Debug("Cancelling sending RPC")
			return RPCResponse{
				Data:  nil,
				Error: errors.New("Operation timed out while sending RPC"),
			}
		default:
			logger.Error("RPC channel is full. Will retry in 1 second.")
			time.Sleep(MakeRPCRetryInterval * time.Second)
		}
	}
	if !waitForResponse {
		logger.Debug("Not waiting for response. Returning directly")
		return RPCResponse{}
	}

	logger.Debug("Waiting for response")
	select {
	case resp, ok := <-r.ResponseCh():
		logger.Debug("Response received")
		if !ok {
			logger.Warning("Response channel closed. Ignoring")
			return RPCResponse{
				Data:  nil,
				Error: nil,
			}
		}
		return resp
	case <-ctx.Done():
		logger.Debug("Cancelling waiting for RPC Response")
		return RPCResponse{
			Data:  nil,
			Error: errors.New("Operation timed out while waiting for response"),
		}
	}
}
