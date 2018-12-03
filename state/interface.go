// Package state holds the interface that any state implementation for
// IPFS Cluster must satisfy.
package state

// State represents the shared state of the cluster and it
import (
	"io"

	cid "gx/ipfs/QmPSQnBKM9g7BaUcZCvswUJVscQ1ipjmwxN5PXCjkp9EQ7/go-cid"

	"github.com/ipfs/ipfs-cluster/api"
)

// State is used by the Consensus component to keep track of
// objects which objects are pinned. This component should be thread safe.
type State interface {
	// Add adds a pin to the State
	Add(api.Pin) error
	// Rm removes a pin from the State
	Rm(cid.Cid) error
	// List lists all the pins in the state
	List() []api.Pin
	// Has returns true if the state is holding information for a Cid
	Has(cid.Cid) bool
	// Get returns the information attacthed to this pin
	Get(cid.Cid) (api.Pin, bool)
	// Migrate restores the serialized format of an outdated state to the current version
	Migrate(r io.Reader) error
	// Return the version of this state
	GetVersion() int
	// Marshal serializes the state to a byte slice
	Marshal() ([]byte, error)
	// Unmarshal deserializes the state from marshaled bytes
	Unmarshal([]byte) error
}
