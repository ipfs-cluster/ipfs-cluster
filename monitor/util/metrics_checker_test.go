package util

import (
	"testing"
	"time"

	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

func TestMetricsChecker(t *testing.T) {
	metrics := make(Metrics)
	alerts := make(chan api.Alert, 1)

	checker := NewMetricsChecker(metrics, alerts)

	metr := api.Metric{
		Name:  "test",
		Peer:  test.TestPeerID1,
		Value: "1",
		Valid: true,
	}
	metr.SetTTL(2 * time.Second)

	metrics["test"] = make(PeerMetrics)
	metrics["test"][test.TestPeerID1] = NewMetricsWindow(5, true)
	metrics["test"][test.TestPeerID1].Add(metr)

	checker.CheckMetrics([]peer.ID{test.TestPeerID1})
	select {
	case <-alerts:
		t.Error("there should not be an alert yet")
	default:
	}

	time.Sleep(3 * time.Second)
	checker.CheckMetrics([]peer.ID{test.TestPeerID1})

	select {
	case <-alerts:
	default:
		t.Error("an alert should have been triggered")
	}

	checker.CheckMetrics([]peer.ID{test.TestPeerID2})
	select {
	case <-alerts:
		t.Error("there should not be alerts for different peer")
	default:
	}
}
