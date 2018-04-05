package basic

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

var metricCounter = 0

func testPeerMonitor(t *testing.T) *Monitor {
	mock := test.NewMockRPCClient(t)
	cfg := &Config{}
	cfg.Default()
	cfg.CheckInterval = 2 * time.Second
	mon, err := NewMonitor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	mon.SetClient(mock)
	return mon
}

func newMetric(n string, p peer.ID) api.Metric {
	m := api.Metric{
		Name:  n,
		Peer:  p,
		Value: fmt.Sprintf("%d", metricCounter),
		Valid: true,
	}
	m.SetTTL(5)
	metricCounter++
	return m
}

func TestPeerMonitorShutdown(t *testing.T) {
	pm := testPeerMonitor(t)
	err := pm.Shutdown()
	if err != nil {
		t.Error(err)
	}

	err = pm.Shutdown()
	if err != nil {
		t.Error(err)
	}
}

func TestLogMetricConcurrent(t *testing.T) {
	pm := testPeerMonitor(t)
	defer pm.Shutdown()

	var wg sync.WaitGroup
	wg.Add(3)

	f := func() {
		defer wg.Done()
		for i := 0; i < 25; i++ {
			mt := api.Metric{
				Name:  "test",
				Peer:  test.TestPeerID1,
				Value: fmt.Sprintf("%d", time.Now().UnixNano()),
				Valid: true,
			}
			mt.SetTTLDuration(150 * time.Millisecond)
			pm.LogMetric(mt)
			time.Sleep(75 * time.Millisecond)
		}
	}
	go f()
	go f()
	go f()

	time.Sleep(150 * time.Millisecond)
	last := time.Now().Add(-500 * time.Millisecond)

	for i := 0; i <= 20; i++ {
		lastMtrcs := pm.LastMetrics("test")

		if len(lastMtrcs) != 1 {
			t.Error("no valid metrics", len(lastMtrcs), i)
			time.Sleep(75 * time.Millisecond)
			continue
		}

		n, err := strconv.Atoi(lastMtrcs[0].Value)
		if err != nil {
			t.Fatal(err)
		}
		current := time.Unix(0, int64(n))
		if current.Before(last) {
			t.Errorf("expected newer metric: Current: %s, Last: %s", current, last)
		}
		last = current
		time.Sleep(75 * time.Millisecond)
	}

	wg.Wait()
}

func TestPeerMonitorLogMetric(t *testing.T) {
	pm := testPeerMonitor(t)
	defer pm.Shutdown()
	metricCounter = 0

	// dont fill window
	pm.LogMetric(newMetric("test", test.TestPeerID1))
	pm.LogMetric(newMetric("test", test.TestPeerID2))
	pm.LogMetric(newMetric("test", test.TestPeerID3))

	// fill window
	pm.LogMetric(newMetric("test2", test.TestPeerID3))
	pm.LogMetric(newMetric("test2", test.TestPeerID3))
	pm.LogMetric(newMetric("test2", test.TestPeerID3))
	pm.LogMetric(newMetric("test2", test.TestPeerID3))

	lastMetrics := pm.LastMetrics("testbad")
	if len(lastMetrics) != 0 {
		t.Logf("%+v", lastMetrics)
		t.Error("metrics should be empty")
	}

	lastMetrics = pm.LastMetrics("test")
	if len(lastMetrics) != 3 {
		t.Error("metrics should correspond to 3 hosts")
	}

	for _, v := range lastMetrics {
		switch v.Peer {
		case test.TestPeerID1:
			if v.Value != "0" {
				t.Error("bad metric value")
			}
		case test.TestPeerID2:
			if v.Value != "1" {
				t.Error("bad metric value")
			}
		case test.TestPeerID3:
			if v.Value != "2" {
				t.Error("bad metric value")
			}
		default:
			t.Error("bad peer")
		}
	}

	lastMetrics = pm.LastMetrics("test2")
	if len(lastMetrics) != 1 {
		t.Fatal("should only be one metric")
	}
	if lastMetrics[0].Value != fmt.Sprintf("%d", metricCounter-1) {
		t.Error("metric is not last")
	}
}

func TestPeerMonitorAlerts(t *testing.T) {
	pm := testPeerMonitor(t)
	defer pm.Shutdown()

	mtr := newMetric("test", test.TestPeerID1)
	mtr.SetTTL(0)
	pm.LogMetric(mtr)
	time.Sleep(time.Second)
	timeout := time.NewTimer(time.Second * 5)

	// it should alert twice at least. Alert re-occurrs.
	for i := 0; i < 2; i++ {
		select {
		case <-timeout.C:
			t.Fatal("should have thrown an alert by now")
		case alrt := <-pm.Alerts():
			if alrt.MetricName != "test" {
				t.Error("Alert should be for test")
			}
			if alrt.Peer != test.TestPeerID1 {
				t.Error("Peer should be TestPeerID1")
			}
		}
	}
}
