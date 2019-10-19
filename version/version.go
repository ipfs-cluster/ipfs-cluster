package version

import (
	"fmt"

	semver "github.com/blang/semver"
	protocol "github.com/libp2p/go-libp2p-protocol"
)

// Version is the current cluster version. Version alignment between
// components, apis and tools ensures compatibility among them.
var Version = semver.MustParse("0.11.0")

// RPCProtocol is used to send libp2p messages between cluster peers
var RPCProtocol = protocol.ID(
	fmt.Sprintf("/ipfscluster/%d.%d/rpc", Version.Major, Version.Minor),
)
