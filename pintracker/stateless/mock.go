package stateless

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/datastore/inmem"
	"github.com/ipfs/ipfs-cluster/state"
	"github.com/ipfs/ipfs-cluster/state/dsstate"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	rpc "github.com/libp2p/go-libp2p-gorpc"
)

var (
	pinCancelCid      = test.Cid3
	unpinCancelCid    = test.Cid2
	errPinCancelCid   = errors.New("should not have received rpc.IPFSPin operation")
	errUnpinCancelCid = errors.New("should not have received rpc.IPFSUnpin operation")
	pinOpts           = api.PinOptions{
		ReplicationFactorMax: -1,
		ReplicationFactorMin: -1,
	}
)

type mockIPFS struct{}

// MockRPCClient returns a mock rpc client that has delays introduced in IPFS
// calls.
func MockRPCClient(t testing.TB) *rpc.Client {
	s := rpc.NewServer(nil, "mock")
	c := rpc.NewClientWithServer(nil, "mock", s)

	err := s.RegisterName("IPFSConnector", &mockIPFS{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (mock *mockIPFS) Pin(ctx context.Context, in *api.Pin, out *struct{}) error {
	switch in.Cid.String() {
	case test.SlowCid1.String():
		time.Sleep(3 * time.Second)
	case pinCancelCid.String():
		return errPinCancelCid
	}
	return nil
}

func (mock *mockIPFS) Unpin(ctx context.Context, in *api.Pin, out *struct{}) error {
	switch in.Cid.String() {
	case test.SlowCid1.String():
		time.Sleep(3 * time.Second)
	case unpinCancelCid.String():
		return errUnpinCancelCid
	}
	return nil
}

func (mock *mockIPFS) PinLsCid(ctx context.Context, in cid.Cid, out *api.IPFSPinStatus) error {
	switch in.String() {
	case test.Cid1.String(), test.Cid2.String():
		*out = api.IPFSPinStatusRecursive
	case test.Cid4.String():
		*out = api.IPFSPinStatusError
		return errors.New("an ipfs error")
	default:
		*out = api.IPFSPinStatusUnpinned
	}
	return nil
}

func (mock *mockIPFS) PinLs(ctx context.Context, in string, out *map[string]api.IPFSPinStatus) error {
	m := map[string]api.IPFSPinStatus{
		test.Cid1.String(): api.IPFSPinStatusRecursive,
	}
	*out = m
	return nil
}

// SlowPinTracker returns a stateless pintracker that uses MockRPCClient (has
// delays introduced in some methods) and state with inmem datastore.
func SlowPinTracker(t testing.TB) (*Tracker, state.State) {
	cfg := &Config{}
	cfg.Default()
	st, err := dsstate.New(inmem.New(), "", dsstate.DefaultHandle())
	if err != nil {
		t.Fatal()
	}
	getState := func(ctx context.Context) (state.ReadOnly, error) {
		return st, nil
	}
	spt := New(cfg, test.PeerID1, test.PeerName1, getState)
	spt.SetClient(MockRPCClient(t))
	return spt, st
}

// PinTracker returns a stateless pintracker that uses test.MockRPCClient
// and state with inmem datastore.
func PinTracker(t testing.TB) (*Tracker, state.State) {
	cfg := &Config{}
	cfg.Default()
	st, err := dsstate.New(inmem.New(), "", dsstate.DefaultHandle())
	if err != nil {
		t.Fatal()
	}
	getState := func(ctx context.Context) (state.ReadOnly, error) {
		return st, nil
	}
	spt := New(cfg, test.PeerID1, test.PeerName1, getState)
	spt.SetClient(test.NewMockRPCClient(t))
	return spt, st
}
