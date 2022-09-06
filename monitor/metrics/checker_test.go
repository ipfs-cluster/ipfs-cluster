package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/test"

	peer "github.com/libp2p/go-libp2p/core/peer"
)

func TestChecker_CheckPeers(t *testing.T) {
	t.Run("check with single metric", func(t *testing.T) {
		metrics := NewStore()
		checker := NewChecker(context.Background(), metrics)

		metr := api.Metric{
			Name:  "ping",
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
	})
}

func TestChecker_CheckAll(t *testing.T) {
	t.Run("checkall with single metric", func(t *testing.T) {
		metrics := NewStore()
		checker := NewChecker(context.Background(), metrics)

		metr := api.Metric{
			Name:  "ping",
			Peer:  test.PeerID1,
			Value: "1",
			Valid: true,
		}
		metr.SetTTL(2 * time.Second)

		metrics.Add(metr)

		checker.CheckAll()
		select {
		case <-checker.Alerts():
			t.Error("there should not be an alert yet")
		default:
		}

		time.Sleep(3 * time.Second)
		err := checker.CheckAll()
		if err != nil {
			t.Fatal(err)
		}

		select {
		case <-checker.Alerts():
		default:
			t.Error("an alert should have been triggered")
		}

		checker.CheckAll()
		select {
		case <-checker.Alerts():
			t.Error("there should not be alerts for different peer")
		default:
		}
	})
}

func TestChecker_Watch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	metrics := NewStore()
	checker := NewChecker(context.Background(), metrics)

	metr := api.Metric{
		Name:  "ping",
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
		checker := NewChecker(context.Background(), metrics)

		metrics.Add(makePeerMetric(test.PeerID1, "1", 100*time.Millisecond))
		time.Sleep(50 * time.Millisecond)
		got := checker.FailedMetric("ping", test.PeerID1)
		if got {
			t.Error("should not have failed so soon")
		}
		time.Sleep(100 * time.Millisecond)
		got = checker.FailedMetric("ping", test.PeerID1)
		if !got {
			t.Error("should have failed")
		}
	})
}

func TestChecker_alert(t *testing.T) {
	t.Run("remove peer from store after alert", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		metrics := NewStore()
		checker := NewChecker(ctx, metrics)

		metr := api.Metric{
			Name:  "ping",
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

		var alertCount int
		for {
			select {
			case a := <-checker.Alerts():
				t.Log("received alert:", a)
				alertCount++
				if alertCount > MaxAlertThreshold {
					t.Fatalf("there should no more than %d alert", MaxAlertThreshold)
				}
			case <-ctx.Done():
				if alertCount < 1 {
					t.Fatal("should have received an alert")
				}
				return
			}
		}
	})
}

func makePeerMetric(pid peer.ID, value string, ttl time.Duration) api.Metric {
	metr := api.Metric{
		Name:  "ping",
		Peer:  pid,
		Value: value,
		Valid: true,
	}
	metr.SetTTL(ttl)
	return metr
}
