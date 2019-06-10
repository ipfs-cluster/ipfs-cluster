package metrics

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/observations"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	peer "github.com/libp2p/go-libp2p-peer"
)

// AlertChannelCap specifies how much buffer the alerts channel has.
var AlertChannelCap = 256

// ErrAlertChannelFull is returned if the alert channel is full.
var ErrAlertChannelFull = errors.New("alert channel is full")

// Checker provides utilities to find expired metrics
// for a given peerset and send alerts if it proceeds to do so.
type Checker struct {
	ctx       context.Context
	alertCh   chan *api.Alert
	metrics   *Store
	threshold float64

	failedPeersMu sync.Mutex
	failedPeers   map[peer.ID]bool
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
		failedPeers: make(map[peer.ID]bool),
	}
}

// CheckPeers will trigger alerts based on the latest metrics from the given peerset
// when they have expired and no alert has been sent before.
func (mc *Checker) CheckPeers(peers []peer.ID) error {
	for _, peer := range peers {
		for _, metric := range mc.metrics.PeerMetrics(peer) {
			if mc.FailedMetric(metric.Name, peer) {
				err := mc.alert(peer, metric.Name)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// CheckAll will trigger alerts for all latest metrics when they have expired
// and no alert has been sent before.
func (mc Checker) CheckAll() error {
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

func (mc *Checker) alertIfExpired(metric *api.Metric) error {
	if !metric.Expired() {
		return nil
	}

	err := mc.alert(metric.Peer, metric.Name)
	if err != nil {
		return err
	}
	metric.Valid = false
	mc.metrics.Add(metric) // invalidate so we don't alert again
	return nil
}

func (mc *Checker) alert(pid peer.ID, metricName string) error {
	mc.failedPeersMu.Lock()
	if mc.failedPeers[pid] {
		mc.metrics.RemovePeer(pid)
		delete(mc.failedPeers, pid)
		return nil
	}
	mc.failedPeers[pid] = true
	mc.failedPeersMu.Unlock()

	alrt := &api.Alert{
		Peer:       pid,
		MetricName: metricName,
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

// Failed returns true if a peer has potentially failed.
// Peers that are not present in the metrics store will return
// as failed.
func (mc *Checker) Failed(pid peer.ID) bool {
	_, _, _, result := mc.failed("ping", pid)
	return result
}

// FailedMetric is the same as Failed but can use any metric type,
// not just ping.
func (mc *Checker) FailedMetric(metric string, pid peer.ID) bool {
	_, _, _, result := mc.failed(metric, pid)
	return result
}

// failed returns all the values involved in making the decision
// as to whether a peer has failed or not. This mainly for debugging
// purposes.
func (mc *Checker) failed(metric string, pid peer.ID) (float64, []float64, float64, bool) {
	latest := mc.metrics.PeerLatest(metric, pid)
	if latest == nil {
		return 0.0, nil, 0.0, true
	}
	v := time.Now().UnixNano() - latest.ReceivedAt
	dv := mc.metrics.Distribution(metric, pid)
	// one metric isn't enough to calculate a distribution
	// alerting/failure detection will fallback to the metric-expiring
	// method
	switch {
	case len(dv) < 5 && !latest.Expired():
		return float64(v), dv, 0.0, false
	default:
		phiv := phi(float64(v), dv)
		return float64(v), dv, phiv, phiv >= mc.threshold
	}
}
