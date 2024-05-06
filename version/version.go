// Package version stores version information for IPFS Cluster.
package version

import (
	semver "github.com/blang/semver"
	protocol "github.com/libp2p/go-libp2p/core/protocol"
)

// Version is the current cluster version.
var Version = semver.MustParse("1.1.0")

// RPCProtocol is protocol handler used to send libp2p-rpc messages between
// cluster peers.  All peers in the cluster need to speak the same protocol
// version.
//
// The RPC Protocol is not linked to the IPFS Cluster version (though it once
// was). The protocol version will be updated as needed when breaking changes
// are introduced, though at this point we aim to minimize those as much as
// possible.
var RPCProtocol = protocol.ID("/ipfscluster/1.0/rpc")
