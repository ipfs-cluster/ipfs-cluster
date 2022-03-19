package metrics

import (
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

func TestStoreLatest(t *testing.T) {
	store := NewStore()

	metr := api.Metric{
		Name:  "test",
		Peer:  test.PeerID1,
		Value: "1",
		Valid: true,
	}
	metr.SetTTL(200 * time.Millisecond)
	store.Add(metr)

	latest := store.LatestValid("test")
	if len(latest) != 1 {
		t.Error("expected 1 metric")
	}

	time.Sleep(220 * time.Millisecond)

	latest = store.LatestValid("test")
	if len(latest) != 0 {
		t.Error("expected no metrics")
	}
}

func TestRemovePeer(t *testing.T) {
	store := NewStore()

	metr := api.Metric{
		Name:  "test",
		Peer:  test.PeerID1,
		Value: "1",
		Valid: true,
	}
	metr.SetTTL(200 * time.Millisecond)
	store.Add(metr)

	if pmtrs := store.PeerMetrics(test.PeerID1); len(pmtrs) <= 0 {
		t.Errorf("there should be one peer metric; got: %v", pmtrs)
	}
	store.RemovePeer(test.PeerID1)
	if pmtrs := store.PeerMetrics(test.PeerID1); len(pmtrs) > 0 {
		t.Errorf("there should be no peer metrics; got: %v", pmtrs)
	}
}
