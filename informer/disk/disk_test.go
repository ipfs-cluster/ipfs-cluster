package disk

import (
	"errors"
	"testing"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"

	"github.com/ipfs/ipfs-cluster/test"
)

type badRPCService struct {
	nthCall int
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

// func (mock *badRPCService) IPFSConfigKey(in string, out *interface{}) error {
// 	mock.nthCall++
// 	switch mock.nthCall {
// 	case 1:
// 		return errors.New("fake error the first time you use this mock")
// 	case 2:
// 		// don't set out
// 		return nil
// 	case 3:
// 		// don't set to string
// 		*out = 3
// 	case 4:
// 		// non parseable string
// 		*out = "abc"
// 	default:
// 		*out = "10KB"
// 	}
// 	return nil
// }

func (mock *badRPCService) IPFSRepoSize(in struct{}, out *uint64) error {
	*out = 2
	mock.nthCall++
	return errors.New("fake error")
}

func (mock *badRPCService) IPFSFreeSpace(in struct{}, out *uint64) error {
	*out = 2
	mock.nthCall++
	return errors.New("fake error")
}

func Test(t *testing.T) {
	cfg := &Config{}
	cfg.Default()
	inf, err := NewInformer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer inf.Shutdown()
	m := inf.GetMetric()
	if m.Valid {
		t.Error("metric should be invalid")
	}
	inf.SetClient(test.NewMockRPCClient(t))
	m = inf.GetMetric()
	if !m.Valid {
		t.Error("metric should be valid")
	}
}

func TestFreeSpace(t *testing.T) {
	cfg := &Config{}
	cfg.Default()
	cfg.Type = MetricFreeSpace

	inf, err := NewInformer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer inf.Shutdown()
	m := inf.GetMetric()
	if m.Valid {
		t.Error("metric should be invalid")
	}
	inf.SetClient(test.NewMockRPCClient(t))
	m = inf.GetMetric()
	if !m.Valid {
		t.Error("metric should be valid")
	}
	// The mock client reports 100KB and 2 pins of 1 KB
	if m.Value != "98000" {
		t.Error("bad metric value")
	}
}

func TestRepoSize(t *testing.T) {
	cfg := &Config{}
	cfg.Default()
	cfg.Type = MetricRepoSize

	inf, err := NewInformer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer inf.Shutdown()
	m := inf.GetMetric()
	if m.Valid {
		t.Error("metric should be invalid")
	}
	inf.SetClient(test.NewMockRPCClient(t))
	m = inf.GetMetric()
	if !m.Valid {
		t.Error("metric should be valid")
	}
	// The mock client reports 100KB and 2 pins of 1 KB
	if m.Value != "2000" {
		t.Error("bad metric value")
	}
}

func TestWithErrors(t *testing.T) {
	cfg := &Config{}
	cfg.Default()
	inf, err := NewInformer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer inf.Shutdown()
	inf.SetClient(badRPCClient(t))
	m := inf.GetMetric()
	if m.Valid {
		t.Errorf("metric should be invalid")
	}
}
