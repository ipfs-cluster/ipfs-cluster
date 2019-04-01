package metrics

import (
	"context"
	"errors"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	peer "github.com/libp2p/go-libp2p-peer"
)

// AlertChannelCap specifies how much buffer the alerts channel has.
var AlertChannelCap = 256

// ErrAlertChannelFull is returned if the alert channel is full.
var ErrAlertChannelFull = errors.New("alert channel is full")

// Checker provides utilities to find expired metrics
// for a given peerset and send alerts if it proceeds to do so.
type Checker struct {
	alertCh   chan *api.Alert
	metrics   *Store
	threshold float64
}

// NewChecker creates a Checker using the given
// MetricsStore.
func NewChecker(metrics *Store, threshold float64) *Checker {
	return &Checker{
		alertCh:   make(chan *api.Alert, AlertChannelCap),
		metrics:   metrics,
		threshold: threshold,
	}
}

// CheckPeers will trigger alerts all latest metrics from the given peerset
// when they have expired and no alert has been sent before.
func (mc *Checker) CheckPeers(peers []peer.ID) error {
	for _, peer := range peers {
		// shortcut checking all metrics based on heartbeat
		// failure detection
		if mc.Failed(peer) {
			err := mc.alert(peer, "ping")
			if err != nil {
				return err
			}
		}
		for _, metric := range mc.metrics.PeerMetrics(peer) {
			err := mc.alertIfExpired(metric)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// CheckAll will trigger alerts for all latest metrics when they have expired
// and no alert has been sent before.
func (mc Checker) CheckAll() error {
	for _, metric := range mc.metrics.AllMetrics() {
		err := mc.alertIfExpired(metric)
		if err != nil {
			return err
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
	alrt := &api.Alert{
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

// Failed returns if a peer has potentially failed. Peers
// that are not present in the metrics store will return
// as failed.
func (mc *Checker) Failed(pid peer.ID) bool {
	latest := mc.metrics.PeerLatest("ping", pid)
	if latest == nil {
		return true
	}
	v := time.Now().UnixNano() - latest.ReceivedAt
	dv := mc.metrics.Distribution("ping", pid)
	phiv := phi(float64(v), dv)
	return phiv >= mc.threshold
}
