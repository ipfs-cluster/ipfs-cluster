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

// RPCMaxQueue can be used to set the size of the ClusterRPC channels.
var RPCMaxQueue = 128

// MakeRPCRetryInterval specifies how long to wait before retrying
// to put a ClusterRPC request in the channel in MakeRPC().
var MakeRPCRetryInterval time.Duration = 1

// ClusterComponent represents a piece of ipfscluster. Cluster components
// usually run their own goroutines (a http server for example) which can
// be controlled via start and stop via Start() and Stop(). A ClusterRPC
// channel is used by Cluster to perform operations requested by the
// component.
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
// an IPFS daemon.
type IPFSConnector interface {
	ClusterComponent
	Pin(*cid.Cid) error
	Unpin(*cid.Cid) error
}

// Peered represents a component which needs to be aware of the peers
// in the Cluster.
type Peered interface {
	AddPeer(p peer.ID)
	RmPeer(p peer.ID)
	SetPeers(peers []peer.ID)
}

// ClusterState represents the shared state of the cluster and it
// is used by the ClusterConsensus component to keep track of
// objects which objects are pinned. This component should be thread safe.
type ClusterState interface {
	AddPin(*cid.Cid) error
	RmPin(*cid.Cid) error
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
	// GetPin returns a pin and ok if it is found.
	GetPin(*cid.Cid) (Pin, bool)
}

// MakeRPC sends a ClusterRPC object over a channel and waits for an answer on
// ClusterRPC.ResponseCh channel. It can be used by any ClusterComponent to
// simplify making RPC requests to Cluster. The ctx parameter must be a
// cancellable context, and can be used to timeout requests.
// If the message cannot be placed in the ClusterRPC channel, retries will be
// issued every MakeRPCRetryInterval.
func MakeRPC(ctx context.Context, ch chan ClusterRPC, r ClusterRPC, waitForResponse bool) RPCResponse {
	logger.Debugf("Sending RPC %d", r.Op())
	exitLoop := false
	for !exitLoop {
		select {
		case ch <- r:
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
		return RPCResponse{}
	}

	logger.Debug("Waiting for response")
	select {
	case resp, ok := <-r.ResponseCh():
		logger.Debug("Response received")
		if !ok { // Not interested in response
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
