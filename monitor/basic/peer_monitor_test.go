package basic

import (
	"fmt"
	"testing"
	"time"

	peer "gx/ipfs/QmZcUPvPhD1Xvk6mwijYF8AfR3mG31S1YsEfHG4khrFPRr/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

var metricCounter = 0

func testPeerMonitor(t *testing.T) *StdPeerMonitor {
	mock := test.NewMockRPCClient(t)
	mon := NewStdPeerMonitor(2)
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
