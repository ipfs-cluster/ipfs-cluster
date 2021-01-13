package metrics

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/observations"

	peer "github.com/libp2p/go-libp2p-core/peer"

	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
)

// AlertChannelCap specifies how much buffer the alerts channel has.
var AlertChannelCap = 256

// MaxAlertThreshold specifies how many alerts will occur per a peer is
// removed from the list of monitored peers.
var MaxAlertThreshold = 1

// ErrAlertChannelFull is returned if the alert channel is full.
var ErrAlertChannelFull = errors.New("alert channel is full")

// accrualMetricsNum represents the number metrics required for
// accrual to function appropriately, and under which we use
// TTL to determine whether a peer may have failed.
var accrualMetricsNum = 6

// Checker provides utilities to find expired metrics
// for a given peerset and send alerts if it proceeds to do so.
type Checker struct {
	ctx       context.Context
	alertCh   chan *api.Alert
	metrics   *Store
	threshold float64

	failedPeersMu sync.Mutex
	failedPeers   map[peer.ID]map[string]int
}

// NewChecker creates a Checker using the given
// MetricsStore. The threshold value indicates when a
// monitored component should be considered to have failed.
// The greater the threshold value the more leniency is granted.
//
// A value between 2.0 and 4.0 is suggested for the threshold.
func NewChecker(ctx context.Context, metrics *Store, threshold float64) *Checker {
	return &Checker{
		ctx:         ctx,
		alertCh:     make(chan *api.Alert, AlertChannelCap),
		metrics:     metrics,
		threshold:   threshold,
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

func (mc *Checker) alert(pid peer.ID, metricName string) error {
	mc.failedPeersMu.Lock()
	defer mc.failedPeersMu.Unlock()

	if _, ok := mc.failedPeers[pid]; !ok {
		mc.failedPeers[pid] = make(map[string]int)
	}
	failedMetrics := mc.failedPeers[pid]

	// If above threshold, remove all metrics for that peer
	// and clean up failedPeers when no failed metrics are left.
	if failedMetrics[metricName] >= MaxAlertThreshold {
		mc.metrics.RemovePeerMetrics(pid, metricName)
		delete(failedMetrics, metricName)
		if len(mc.failedPeers[pid]) == 0 {
			delete(mc.failedPeers, pid)
		}
		return nil
	}

	failedMetrics[metricName]++

	alrt := &api.Alert{
		Peer:       pid,
		MetricName: metricName,
		Expiry:     time.Now().Add(30 * time.Second).UnixNano(),
	}
	select {
	case mc.alertCh <- alrt:
		stats.RecordWithTags(
			mc.ctx,
			[]tag.Mutator{tag.Upsert(observations.RemotePeerKey, pid.Pretty())},
			observations.Alerts.M(1),
		)
	default:
		return ErrAlertChannelFull
	}
	return nil
}

// Alerts returns a channel which gets notified by CheckPeers.
func (mc *Checker) Alerts() <-chan *api.Alert {
	return mc.alertCh
}

// Watch will trigger regular CheckPeers on the given interval. It will call
// peersF to obtain a peerset. It can be stopped by cancelling the context.
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
	_, _, _, result := mc.failed(metric, pid)
	return result
}

// failed returns all the values involved in making the decision
// as to whether a peer has failed or not. The debugging parameter
// enables a more computation heavy path of the function but
// allows for insight into the return phi value.
func (mc *Checker) failed(metric string, pid peer.ID) (float64, []float64, float64, bool) {
	latest := mc.metrics.PeerLatest(metric, pid)
	if latest == nil {
		return 0.0, nil, 0.0, true
	}

	// A peer is never failed if the latest metric from it has
	// not expired or we do not have enough number of metrics
	// for accrual detection
	if !latest.Expired() {
		return 0.0, nil, 0.0, false
	}
	// The latest metric has expired

	pmtrs := mc.metrics.PeerMetricAll(metric, pid)
	// Not enough values for accrual and metric expired. Peer failed.
	if len(pmtrs) < accrualMetricsNum {
		return 0.0, nil, 0.0, true
	}

	v := time.Now().UnixNano() - latest.ReceivedAt
	dv := mc.metrics.Distribution(metric, pid)
	phiv := phi(float64(v), dv)
	return float64(v), dv, phiv, phiv >= mc.threshold
}
