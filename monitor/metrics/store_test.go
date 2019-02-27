package metrics

import (
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

func TestStoreLatest(t *testing.T) {
	store := NewStore()

	metr := &api.Metric{
		Name:  "test",
		Peer:  test.TestPeerID1,
		Value: "1",
		Valid: true,
	}
	metr.SetTTL(200 * time.Millisecond)
	store.Add(metr)

	latest := store.Latest("test")
	if len(latest) != 1 {
		t.Error("expected 1 metric")
	}

	time.Sleep(220 * time.Millisecond)

	latest = store.Latest("test")
	if len(latest) != 0 {
		t.Error("expected no metrics")
	}
}
