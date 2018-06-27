package numpin

import (
	"context"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/libp2p/go-libp2p-gorpc"
)

type mockService struct{}

func mockRPCClient(t *testing.T) *rpc.Client {
	s := rpc.NewServer(nil, "mock")
	c := rpc.NewClientWithServer(nil, "mock", s)
	err := s.RegisterName("Cluster", &mockService{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (mock *mockService) IPFSPinLs(ctx context.Context, in string, out *map[string]api.IPFSPinStatus) error {
	*out = map[string]api.IPFSPinStatus{
		"QmPGDFvBkgWhvzEK9qaTWrWurSwqXNmhnK3hgELPdZZNPa": api.IPFSPinStatusRecursive,
		"QmUZ13osndQ5uL4tPWHXe3iBgBgq9gfewcBMSCAuMBsDJ6": api.IPFSPinStatusRecursive,
	}
	return nil
}

func Test(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	cfg.Default()
	inf, err := NewInformer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m := inf.GetMetric(ctx)
	if m.Valid {
		t.Error("metric should be invalid")
	}
	inf.SetClient(mockRPCClient(t))
	m = inf.GetMetric(ctx)
	if !m.Valid {
		t.Error("metric should be valid")
	}
	if m.Value != "2" {
		t.Error("bad metric value")
	}
}
