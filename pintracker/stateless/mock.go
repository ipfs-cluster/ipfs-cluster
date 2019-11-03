package stateless

import (
	"context"
	"errors"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/state"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
)

var pinOpts = api.PinOptions{
	ReplicationFactorMax: -1,
	ReplicationFactorMin: -1,
}

type mockState struct {
	slow bool
}

// NewMockState return a mock implementation of State.
func NewMockState(slow bool) state.ReadOnly {
	return mockState{slow: slow}
}

func (st mockState) List(ctx context.Context) ([]*api.Pin, error) {
	if st.slow {
		return []*api.Pin{
			api.PinWithOpts(test.Cid1, pinOpts),
			api.PinWithOpts(test.Cid3, pinOpts),
		}, nil
	}
	return []*api.Pin{
		api.PinWithOpts(test.Cid1, pinOpts),
		api.PinCid(test.Cid2),
		api.PinWithOpts(test.Cid3, pinOpts),
	}, nil
}

func (st mockState) Has(context.Context, cid.Cid) (bool, error) {
	return false, nil
}

func (st mockState) Get(ctx context.Context, in cid.Cid) (*api.Pin, error) {
	if st.slow {
		switch in.String() {
		case test.ErrorCid.String():
			return nil, errors.New("expected error when using ErrorCid")
		case test.Cid1.String(), test.Cid2.String():
			pin := api.PinWithOpts(in, pinOpts)
			return pin, nil
		}
		return api.PinCid(in), nil
	}

	switch in.String() {
	case test.ErrorCid.String():
		return nil, errors.New("this is an expected error when using ErrorCid")
	case test.Cid1.String(), test.Cid3.String():
		p := api.PinCid(in)
		p.ReplicationFactorMin = -1
		p.ReplicationFactorMax = -1
		return p, nil
	case test.Cid2.String(): // This is a remote pin
		p := api.PinCid(in)
		p.ReplicationFactorMin = 1
		p.ReplicationFactorMax = 1
		return p, nil
	default:
		return nil, state.ErrNotFound
	}
}
