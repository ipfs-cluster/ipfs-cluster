package pinqueue

import (
	"context"
	"testing"

	rpc "github.com/libp2p/go-libp2p-gorpc"
)

type mockService struct{}

func (mock *mockService) PinQueueSize(ctx context.Context, in struct{}, out *int64) error {
	*out = 42
	return nil
}

func mockRPCClient(t *testing.T) *rpc.Client {
	s := rpc.NewServer(nil, "mock")
	c := rpc.NewClientWithServer(nil, "mock", s)
	err := s.RegisterName("PinTracker", &mockService{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func Test(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{}
	cfg.Default()
	cfg.WeightBucketSize = 0
	inf, err := New(cfg)
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
	if m.Value != "42" {
		t.Error("bad metric value", m.Value)
	}
	if m.Partitionable {
		t.Error("should not be a partitionable metric")
	}
	if m.Weight != -42 {
		t.Error("weight should be -42")
	}

	cfg.WeightBucketSize = 5
	inf, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	inf.SetClient(mockRPCClient(t))
	metrics = inf.GetMetrics(ctx)
	if len(metrics) != 1 {
		t.Fatal("expected 1 metric")
	}
	m = metrics[0]
	if m.Weight != -8 {
		t.Error("weight should be -8, not", m.Weight)
	}
}
