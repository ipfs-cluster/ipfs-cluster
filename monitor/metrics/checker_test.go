package metrics

import (
	"context"
	"testing"
	"time"

	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

func TestChecker(t *testing.T) {
	metrics := NewStore()
	checker := NewChecker(metrics, 2.0)

	metr := &api.Metric{
		Name:  "test",
		Peer:  test.PeerID1,
		Value: "1",
		Valid: true,
	}
	metr.SetTTL(2 * time.Second)

	metrics.Add(metr)

	checker.CheckPeers([]peer.ID{test.PeerID1})
	select {
	case <-checker.Alerts():
		t.Error("there should not be an alert yet")
	default:
	}

	time.Sleep(3 * time.Second)
	err := checker.CheckPeers([]peer.ID{test.PeerID1})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-checker.Alerts():
	default:
		t.Error("an alert should have been triggered")
	}

	checker.CheckPeers([]peer.ID{test.PeerID2})
	select {
	case <-checker.Alerts():
		t.Error("there should not be alerts for different peer")
	default:
	}
}

func TestChecker_Watch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	metrics := NewStore()
	checker := NewChecker(metrics, 2.0)

	metr := &api.Metric{
		Name:  "test",
		Peer:  test.PeerID1,
		Value: "1",
		Valid: true,
	}
	metr.SetTTL(100 * time.Millisecond)
	metrics.Add(metr)

	peersF := func(context.Context) ([]peer.ID, error) {
		return []peer.ID{test.PeerID1}, nil
	}

	go checker.Watch(ctx, peersF, 200*time.Millisecond)

	select {
	case a := <-checker.Alerts():
		t.Log("received alert:", a)
	case <-ctx.Done():
		t.Fatal("should have received an alert")
	}
}

func TestChecker_Failed(t *testing.T) {
	t.Run("standard failure check", func(t *testing.T) {
		metrics := NewStore()
		checker := NewChecker(metrics, 2.0)

		for i := 0; i < 10; i++ {
			metrics.Add(makePeerMetric(test.PeerID1, "1"))
			time.Sleep(time.Duration(2) * time.Millisecond)
		}
		for i := 0; i < 10; i++ {
			metrics.Add(makePeerMetric(test.PeerID1, "1"))
			time.Sleep(time.Duration(i) * time.Millisecond)
			got := checker.Failed(test.PeerID1)
			// the magic number 17 represents the point at which
			// the time between metrics addition has gotten
			// so large that the probability that the service
			// has failed goes over the threshold.
			if i >= 17 && !got {
				t.Fatal("threshold should have been passed by now")
			}
		}
	})
}

func makePeerMetric(pid peer.ID, value string) *api.Metric {
	metr := &api.Metric{
		Name:  "ping",
		Peer:  pid,
		Value: value,
		Valid: true,
	}
	return metr
}
