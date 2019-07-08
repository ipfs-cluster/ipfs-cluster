package state

import (
	"context"

	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
)

type empty struct{}

func (e *empty) List(ctx context.Context) ([]*api.Pin, error) {
	return []*api.Pin{}, nil
}

func (e *empty) Has(ctx context.Context, c cid.Cid) (bool, error) {
	return false, nil
}

func (e *empty) Get(ctx context.Context, c cid.Cid) (*api.Pin, error) {
	return nil, ErrNotFound
}

// Empty returns an empty read-only state.
func Empty() ReadOnly {
	return &empty{}
}
