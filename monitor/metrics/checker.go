package metrics

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"

	peer "github.com/libp2p/go-libp2p/core/peer"
)

// AlertChannelCap specifies how much buffer the alerts channel has.
var AlertChannelCap = 256

// MaxAlertThreshold specifies how many alerts will occur per a peer is
// removed from the list of monitored peers.
var MaxAlertThreshold = 1

// ErrAlertChannelFull is returned if the alert channel is full.
var ErrAlertChannelFull = errors.New("alert channel is full")

// Checker provides utilities to find expired metrics
// for a given peerset and send alerts if it proceeds to do so.
type Checker struct {
	ctx     context.Context
	alertCh chan api.Alert
	metrics *Store

	failedPeersMu sync.Mutex
	failedPeers   map[peer.ID]map[string]int
}

// NewChecker creates a Checker using the given
// MetricsStore. The threshold value indicates when a
// monitored component should be considered to have failed.
// The greater the threshold value the more leniency is granted.
//
// A value between 2.0 and 4.0 is suggested for the threshold.
func NewChecker(ctx context.Context, metrics *Store) *Checker {
	return &Checker{
		ctx:         ctx,
		alertCh:     make(chan api.Alert, AlertChannelCap),
		metrics:     metrics,
		failedPeers: make(map[peer.ID]map[string]int),
	}
}

// CheckPeers will trigger alerts based on the latest metrics from the given peerset
// when they have expired and no alert has been sent before.
func (mc *Checker) CheckPeers(peers []peer.ID) error {
	for _, name := range mc.metrics.MetricNames() {
		for _, peer := range peers {
			for _, metric := range mc.metrics.PeerMetricAll(name, peer) {
				if mc.FailedMetric(metric.Name, peer) {
					err := mc.alert(peer, metric.Name)
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// CheckAll will trigger alerts for all latest metrics when they have expired
// and no alert has been sent before.
func (mc *Checker) CheckAll() error {
	for _, metric := range mc.metrics.AllMetrics() {
		if mc.FailedMetric(metric.Name, metric.Peer) {
			err := mc.alert(metric.Peer, metric.Name)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// ResetAlerts clears up how many time a peer alerted for a given metric.
// Thus, if it was over the threshold, it will start alerting again.
func (mc *Checker) ResetAlerts(pid peer.ID, metricName string) {
	mc.failedPeersMu.Lock()
	defer mc.failedPeersMu.Unlock()

	failedMetrics, ok := mc.failedPeers[pid]
	if !ok {
		return
	}
	delete(failedMetrics, metricName)
	if len(mc.failedPeers[pid]) == 0 {
		delete(mc.failedPeers, pid)
	}
}

func (mc *Checker) alert(pid peer.ID, metricName string) error {
	mc.failedPeersMu.Lock()
	defer mc.failedPeersMu.Unlock()

	if _, ok := mc.failedPeers[pid]; !ok {
		mc.failedPeers[pid] = make(map[string]int)
	}
	failedMetrics := mc.failedPeers[pid]
	lastMetric := mc.metrics.PeerLatest(metricName, pid)
	if !lastMetric.Defined() {
		lastMetric = api.Metric{
			Name: metricName,
			Peer: pid,
		}
	}

	failedMetrics[metricName]++
	// If above threshold, do not send alert
	if failedMetrics[metricName] > MaxAlertThreshold {
		// Cleanup old metrics eventually
		if failedMetrics[metricName] >= 300 {
			delete(failedMetrics, metricName)
			if len(mc.failedPeers[pid]) == 0 {
				delete(mc.failedPeers, pid)
			}
		}
		return nil
	}

	alrt := api.Alert{
		Metric:      lastMetric,
		TriggeredAt: time.Now(),
	}
	select {
	case mc.alertCh <- alrt:
	default:
		return ErrAlertChannelFull
	}
	return nil
}

// Alerts returns a channel which gets notified by CheckPeers.
func (mc *Checker) Alerts() <-chan api.Alert {
	return mc.alertCh
}

// Watch will trigger regular CheckPeers on the given interval. It will call
// peersF to obtain a peerset. It can be stopped by canceling the context.
// Usually you want to launch this in a goroutine.
func (mc *Checker) Watch(ctx context.Context, peersF func(context.Context) ([]peer.ID, error), interval time.Duration) {
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			if peersF != nil {
				peers, err := peersF(ctx)
				if err != nil {
					continue
				}
				mc.CheckPeers(peers)
			} else {
				mc.CheckAll()
			}
		case <-ctx.Done():
			ticker.Stop()
			return
		}
	}
}

// FailedMetric returns if a peer is marked as failed for a particular metric.
func (mc *Checker) FailedMetric(metric string, pid peer.ID) bool {
	latest := mc.metrics.PeerLatest(metric, pid)
	return latest.Expired()
}
