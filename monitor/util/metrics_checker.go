package util

import (
	"errors"

	"github.com/ipfs/ipfs-cluster/api"

	peer "github.com/libp2p/go-libp2p-peer"
)

// ErrAlertChannelFull is returned if the alert channel is full.
var ErrAlertChannelFull = errors.New("alert channel is full")

// Metrics maps metric names to PeerMetrics
type Metrics map[string]PeerMetrics

// PeerMetrics maps a peer IDs to a metric window.
type PeerMetrics map[peer.ID]*MetricsWindow

// MetricsChecker provides utilities to find expired metrics
// for a given peerset and send alerts if it proceeds to do so.
type MetricsChecker struct {
	alertCh chan api.Alert
	metrics Metrics
}

// NewMetricsChecker creates a MetricsChecker using the given
// Metrics and alert channel. MetricsChecker assumes non-concurrent
// access to the Metrics map. It's the caller's responsability
// to it lock otherwise while calling CheckMetrics().
func NewMetricsChecker(metrics Metrics, alertCh chan api.Alert) *MetricsChecker {
	return &MetricsChecker{
		alertCh: alertCh,
		metrics: metrics,
	}
}

// CheckMetrics triggers Check() on all metrics known for the given peerset.
func (mc *MetricsChecker) CheckMetrics(peers []peer.ID) {
	for name, peerMetrics := range mc.metrics {
		for _, pid := range peers {
			window, ok := peerMetrics[pid]
			if !ok { // no metrics for this peer
				continue
			}
			mc.Check(pid, name, window)
		}
	}
}

// Check sends an alert on the alert channel for the given peer and metric name
// if the last metric in the window was valid but expired.
func (mc *MetricsChecker) Check(pid peer.ID, metricName string, mw *MetricsWindow) error {
	last, err := mw.Latest()
	if err != nil { // no metrics
		return nil
	}

	if last.Valid && last.Expired() {
		return mc.alert(pid, metricName)
	}
	return nil
}

func (mc *MetricsChecker) alert(pid peer.ID, metricName string) error {
	alrt := api.Alert{
		Peer:       pid,
		MetricName: metricName,
	}
	select {
	case mc.alertCh <- alrt:
	default:
		return ErrAlertChannelFull
	}
	return nil
}
