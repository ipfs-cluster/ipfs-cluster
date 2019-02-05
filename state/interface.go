// Package state holds the interface that any state implementation for
// IPFS Cluster must satisfy.
package state

// State represents the shared state of the cluster and it
import (
	"context"
	"io"

	cid "github.com/ipfs/go-cid"

	"github.com/ipfs/ipfs-cluster/api"
)

// State is used by the Consensus component to keep track of
// objects which objects are pinned. This component should be thread safe.
type State interface {
	// Add adds a pin to the State
	Add(context.Context, api.Pin) error
	// Rm removes a pin from the State
	Rm(context.Context, cid.Cid) error
	// List lists all the pins in the state
	List(context.Context) []api.Pin
	// Has returns true if the state is holding information for a Cid
	Has(context.Context, cid.Cid) bool
	// Get returns the information attacthed to this pin
	Get(context.Context, cid.Cid) (api.Pin, bool)
	// Migrate restores the serialized format of an outdated state to the current version
	Migrate(ctx context.Context, r io.Reader) error
	// Return the version of this state
	GetVersion() int
	// Marshal serializes the state to a byte slice
	Marshal() ([]byte, error)
	// Unmarshal deserializes the state from marshaled bytes
	Unmarshal([]byte) error
}
