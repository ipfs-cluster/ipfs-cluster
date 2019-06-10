package metrics

import (
	"context"
	"errors"
	"fmt"
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
// removed the list of monitored peers.
var MaxAlertThreshold = 1

// ErrAlertChannelFull is returned if the alert channel is full.
var ErrAlertChannelFull = errors.New("alert channel is full")

// Checker provides utilities to find expired metrics
// for a given peerset and send alerts if it proceeds to do so.
type Checker struct {
	ctx       context.Context
	alertCh   chan *api.Alert
	metrics   *Store
	threshold float64

	alertThreshold int

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
	defer mc.failedPeersMu.Unlock()

	_, ok := mc.failedPeers[pid]
	if !ok {
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
	_, _, _, result := mc.failed("ping", pid, false)
	return result
}

// FailedMetric is the same as Failed but can use any metric type,
// not just ping.
func (mc *Checker) FailedMetric(metric string, pid peer.ID) bool {
	_, _, _, result := mc.failed(metric, pid, false)
	return result
}

// failed returns all the values involved in making the decision
// as to whether a peer has failed or not. The debugging parameter
// enables a more computation heavy path of the function but
// allows for insight into the return phi value.
// This should not be used for anything other than testing.
func (mc *Checker) failed(metric string, pid peer.ID, debugging bool) (float64, []float64, float64, bool) {
	latest := mc.metrics.PeerLatest(metric, pid)
	if latest == nil {
		return 0.0, nil, 0.0, true
	}

	// the invalidTTL check prevents false-positive results
	// where multiple metrics closer together skew the distribution
	// to be less than that of the TTL value of the metrics
	pmtrs := mc.metrics.PeerMetricAll(metric, pid)
	var withinTTL bool
	switch {
	case len(pmtrs) == 1:
		if pmtrs[0].Expired() {
			return 0.0, nil, 0.0, true
		}
		return 0.0, nil, 0.0, false
	case len(pmtrs) >= 2:
		currMetricExpiry := time.Unix(0, pmtrs[1].Expire)
		prevMetricReceived := time.Unix(0, pmtrs[0].ReceivedAt)
		withinTTL = prevMetricReceived.Before(currMetricExpiry)
		if withinTTL && !debugging {
			return 0.0, nil, 0.0, false
		}
		if debugging {
			fmt.Printf("validTTL: %v\texp: %v,\tra: %v\n", withinTTL, currMetricExpiry, prevMetricReceived)
		}
	}

	v := time.Now().UnixNano() - latest.ReceivedAt
	dv := mc.metrics.Distribution(metric, pid)
	// one metric isn't enough to calculate a distribution
	// alerting/failure detection will fallback to the metric-expiring
	// method
	switch {
	case withinTTL && debugging:
		phiv := phi(float64(v), dv)
		return float64(v), dv, phiv, false
	case len(dv) < 5 && !latest.Expired():
		return float64(v), dv, 0.0, false
	default:
		phiv := phi(float64(v), dv)
		return float64(v), dv, phiv, phiv >= mc.threshold
	}
}
