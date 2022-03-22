package numpin

import (
	"context"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"

	rpc "github.com/libp2p/go-libp2p-gorpc"
)

type mockService struct{}

func mockRPCClient(t *testing.T) *rpc.Client {
	s := rpc.NewServer(nil, "mock")
	c := rpc.NewClientWithServer(nil, "mock", s)
	err := s.RegisterName("IPFSConnector", &mockService{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (mock *mockService) PinLs(ctx context.Context, in <-chan []string, out chan<- api.IPFSPinInfo) error {
	out <- api.IPFSPinInfo{Cid: api.Cid(test.Cid1), Type: api.IPFSPinStatusRecursive}
	out <- api.IPFSPinInfo{Cid: api.Cid(test.Cid2), Type: api.IPFSPinStatusRecursive}
	close(out)
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
	metrics := inf.GetMetrics(ctx)
	if len(metrics) != 1 {
		t.Fatal("expected 1 metric")
	}
	m := metrics[0]

	if m.Valid {
		t.Error("metric should be invalid")
	}
	inf.SetClient(mockRPCClient(t))

	metrics = inf.GetMetrics(ctx)
	if len(metrics) != 1 {
		t.Fatal("expected 1 metric")
	}
	m = metrics[0]
	if !m.Valid {
		t.Error("metric should be valid")
	}
	if m.Value != "2" {
		t.Error("bad metric value")
	}
}
