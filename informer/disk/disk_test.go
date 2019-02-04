package disk

import (
	"context"
	"errors"
	"testing"

	rpc "github.com/libp2p/go-libp2p-gorpc"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

type badRPCService struct {
}

func badRPCClient(t *testing.T) *rpc.Client {
	s := rpc.NewServer(nil, "mock")
	c := rpc.NewClientWithServer(nil, "mock", s)
	err := s.RegisterName("Cluster", &badRPCService{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (mock *badRPCService) IPFSRepoStat(ctx context.Context, in struct{}, out *api.IPFSRepoStat) error {
	return errors.New("fake error")
}

func Test(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	cfg.Default()
	inf, err := NewInformer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer inf.Shutdown(ctx)
	m := inf.GetMetric(ctx)
	if m.Valid {
		t.Error("metric should be invalid")
	}
	inf.SetClient(test.NewMockRPCClient(t))
	m = inf.GetMetric(ctx)
	if !m.Valid {
		t.Error("metric should be valid")
	}
}

func TestFreeSpace(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	cfg.Default()
	cfg.Type = MetricFreeSpace

	inf, err := NewInformer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer inf.Shutdown(ctx)
	m := inf.GetMetric(ctx)
	if m.Valid {
		t.Error("metric should be invalid")
	}
	inf.SetClient(test.NewMockRPCClient(t))
	m = inf.GetMetric(ctx)
	if !m.Valid {
		t.Error("metric should be valid")
	}
	// The mock client reports 100KB and 2 pins of 1 KB
	if m.Value != "98000" {
		t.Error("bad metric value")
	}
}

func TestRepoSize(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	cfg.Default()
	cfg.Type = MetricRepoSize

	inf, err := NewInformer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer inf.Shutdown(ctx)
	m := inf.GetMetric(ctx)
	if m.Valid {
		t.Error("metric should be invalid")
	}
	inf.SetClient(test.NewMockRPCClient(t))
	m = inf.GetMetric(ctx)
	if !m.Valid {
		t.Error("metric should be valid")
	}
	// The mock client reports 100KB and 2 pins of 1 KB
	if m.Value != "2000" {
		t.Error("bad metric value")
	}
}

func TestWithErrors(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	cfg.Default()
	inf, err := NewInformer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer inf.Shutdown(ctx)
	inf.SetClient(badRPCClient(t))
	m := inf.GetMetric(ctx)
	if m.Valid {
		t.Errorf("metric should be invalid")
	}
}
