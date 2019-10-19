package util

import (
	"errors"

	"github.com/ipfs/ipfs-cluster/api"

	peer "github.com/libp2p/go-libp2p-core/peer"
)

var (
	// ErrFullQueue is the error used when pin or unpin operation channel is full.
	ErrFullQueue = errors.New("pin/unpin operation queue is full (too many operations), increasing max_pin_queue_size would help")
)

// IsRemotePin determines whether a Pin's ReplicationFactor has
// been met, so as to either pin or unpin it from the peer.
func IsRemotePin(c *api.Pin, pid peer.ID) bool {
	if c.ReplicationFactorMax < 0 {
		return false
	}

	for _, p := range c.Allocations {
		if p == pid {
			return false
		}
	}
	return true
}
