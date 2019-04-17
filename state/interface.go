// Package state holds the interface that any state implementation for
// IPFS Cluster must satisfy.
package state

// State represents the shared state of the cluster and it
import (
	"context"
	"errors"
	"io"

	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
)

// ErrNotFound should be returned when a pin is not part of the state.
var ErrNotFound = errors.New("pin is not part of the pinset")

// State is a wrapper to the Cluster shared state so that Pin objects can
// easily read, written and queried. The state can be marshaled and
// unmarshaled. Implementation should be thread-safe.
type State interface {
	ReadOnly
	WriteOnly
	// Migrate restores the serialized format of an outdated state to the
	// current version.
	Migrate(ctx context.Context, r io.Reader) error
	// Marshal serializes the state to a byte slice.
	Marshal(io.Writer) error
	// Unmarshal deserializes the state from marshaled bytes.
	Unmarshal(io.Reader) error
	// Commit writes any batched operations.
}

// ReadOnly represents a the read side of a State.
type ReadOnly interface {
	// List lists all the pins in the state.
	List(context.Context) ([]*api.Pin, error)
	// Has returns true if the state is holding information for a Cid.
	Has(context.Context, cid.Cid) (bool, error)
	// Get returns the information attacthed to this pin, if any. If the
	// pin is not part of the state, it should return ErrNotFound.
	Get(context.Context, cid.Cid) (*api.Pin, error)
}

// WriteOnly represents the write side of a State.
type WriteOnly interface {
	// Add adds a pin to the State
	Add(context.Context, *api.Pin) error
	// Rm removes a pin from the State.
	Rm(context.Context, cid.Cid) error
}

// BatchingState represents a state which batches write operations.
type BatchingState interface {
	State
	// Commit writes any batched operations.
	Commit(context.Context) error
}
