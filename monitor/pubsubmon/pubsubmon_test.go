package pubsubmon

import (
	"context"
	"fmt"

	"strconv"
	"sync"
	"testing"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

func init() {
	// GossipSub needs to heartbeat to discover newly connected hosts
	// This speeds things up a little.
	pubsub.GossipSubHeartbeatInterval = 50 * time.Millisecond
}

type metricFactory struct {
	l       sync.Mutex
	counter int
}

func newMetricFactory() *metricFactory {
	return &metricFactory{
		counter: 0,
	}
}

func (mf *metricFactory) newMetric(n string, p peer.ID) api.Metric {
	mf.l.Lock()
	defer mf.l.Unlock()
	m := api.Metric{
		Name:  n,
		Peer:  p,
		Value: fmt.Sprintf("%d", mf.counter),
		Valid: true,
	}
	m.SetTTL(5 * time.Second)
	mf.counter++
	return m
}

func (mf *metricFactory) count() int {
	mf.l.Lock()
	defer mf.l.Unlock()
	return mf.counter
}

func testPeerMonitor(t *testing.T) (*Monitor, func()) {
	ctx := context.Background()
	h, err := libp2p.New(
		context.Background(),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	if err != nil {
		t.Fatal(err)
	}

	mock := test.NewMockRPCClientWithHost(t, h)
	cfg := &Config{}
	cfg.Default()
	cfg.CheckInterval = 2 * time.Second
	mon, err := New(h, cfg)
	if err != nil {
		t.Fatal(err)
	}
	mon.SetClient(mock)

	shutdownF := func() {
		mon.Shutdown(ctx)
		h.Close()
	}

	return mon, shutdownF
}

func TestPeerMonitorShutdown(t *testing.T) {
	ctx := context.Background()
	pm, shutdown := testPeerMonitor(t)
	defer shutdown()

	err := pm.Shutdown(ctx)
	if err != nil {
		t.Error(err)
	}

	err = pm.Shutdown(ctx)
	if err != nil {
		t.Error(err)
	}
}

func TestLogMetricConcurrent(t *testing.T) {
	ctx := context.Background()
	pm, shutdown := testPeerMonitor(t)
	defer shutdown()

	var wg sync.WaitGroup
	wg.Add(3)

	// Insert 25 metrics
	f := func() {
		defer wg.Done()
		for i := 0; i < 25; i++ {
			mt := api.Metric{
				Name:  "test",
				Peer:  test.TestPeerID1,
				Value: fmt.Sprintf("%d", time.Now().UnixNano()),
				Valid: true,
			}
			mt.SetTTL(150 * time.Millisecond)
			pm.LogMetric(ctx, mt)
			time.Sleep(75 * time.Millisecond)
		}
	}
	go f()
	go f()
	go f()

	// Wait for at least two metrics to be inserted
	time.Sleep(200 * time.Millisecond)
	last := time.Now().Add(-500 * time.Millisecond)

	for i := 0; i <= 20; i++ {
		lastMtrcs := pm.LatestMetrics(ctx, "test")

		// There should always 1 valid LatestMetric "test"
		if len(lastMtrcs) != 1 {
			t.Error("no valid metrics", len(lastMtrcs), i)
			time.Sleep(75 * time.Millisecond)
			continue
		}

		n, err := strconv.Atoi(lastMtrcs[0].Value)
		if err != nil {
			t.Fatal(err)
		}

		// The timestamp of the metric cannot be older than
		// the timestamp from the last
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
	ctx := context.Background()
	pm, shutdown := testPeerMonitor(t)
	defer shutdown()
	mf := newMetricFactory()

	// dont fill window
	pm.LogMetric(ctx, mf.newMetric("test", test.TestPeerID1))
	pm.LogMetric(ctx, mf.newMetric("test", test.TestPeerID2))
	pm.LogMetric(ctx, mf.newMetric("test", test.TestPeerID3))

	// fill window
	pm.LogMetric(ctx, mf.newMetric("test2", test.TestPeerID3))
	pm.LogMetric(ctx, mf.newMetric("test2", test.TestPeerID3))
	pm.LogMetric(ctx, mf.newMetric("test2", test.TestPeerID3))
	pm.LogMetric(ctx, mf.newMetric("test2", test.TestPeerID3))

	latestMetrics := pm.LatestMetrics(ctx, "testbad")
	if len(latestMetrics) != 0 {
		t.Logf("%+v", latestMetrics)
		t.Error("metrics should be empty")
	}

	latestMetrics = pm.LatestMetrics(ctx, "test")
	if len(latestMetrics) != 3 {
		t.Error("metrics should correspond to 3 hosts")
	}

	for _, v := range latestMetrics {
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

	latestMetrics = pm.LatestMetrics(ctx, "test2")
	if len(latestMetrics) != 1 {
		t.Fatal("should only be one metric")
	}
	if latestMetrics[0].Value != fmt.Sprintf("%d", mf.count()-1) {
		t.Error("metric is not last")
	}
}

func TestPeerMonitorPublishMetric(t *testing.T) {
	ctx := context.Background()
	pm, shutdown := testPeerMonitor(t)
	defer shutdown()

	pm2, shutdown2 := testPeerMonitor(t)
	defer shutdown2()

	time.Sleep(200 * time.Millisecond)

	err := pm.host.Connect(
		context.Background(),
		peerstore.PeerInfo{
			ID:    pm2.host.ID(),
			Addrs: pm2.host.Addrs(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	mf := newMetricFactory()

	metric := mf.newMetric("test", test.TestPeerID1)
	err = pm.PublishMetric(ctx, metric)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)

	checkMetric := func(t *testing.T, pm *Monitor) {
		latestMetrics := pm.LatestMetrics(ctx, "test")
		if len(latestMetrics) != 1 {
			t.Fatal(pm.host.ID(), "expected 1 published metric")
		}
		t.Log(pm.host.ID(), "received metric")

		receivedMetric := latestMetrics[0]
		if receivedMetric.Peer != metric.Peer ||
			receivedMetric.Expire != metric.Expire ||
			receivedMetric.Value != metric.Value ||
			receivedMetric.Valid != metric.Valid ||
			receivedMetric.Name != metric.Name {
			t.Fatal("it should be exactly the same metric we published")
		}
	}

	t.Log("pm1")
	checkMetric(t, pm)
	t.Log("pm2")
	checkMetric(t, pm2)
}

func TestPeerMonitorAlerts(t *testing.T) {
	ctx := context.Background()
	pm, shutdown := testPeerMonitor(t)
	defer shutdown()
	mf := newMetricFactory()

	mtr := mf.newMetric("test", test.TestPeerID1)
	mtr.SetTTL(0)
	pm.LogMetric(ctx, mtr)
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
