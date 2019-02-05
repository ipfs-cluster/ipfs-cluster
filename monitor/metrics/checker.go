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
	alertCh chan api.Alert
	metrics *Store
}

// NewChecker creates a Checker using the given
// MetricsStore.
func NewChecker(metrics *Store) *Checker {
	return &Checker{
		alertCh: make(chan api.Alert, AlertChannelCap),
		metrics: metrics,
	}
}

// CheckPeers will trigger alerts for expired metrics belonging to the
// given peerset.
func (mc *Checker) CheckPeers(peers []peer.ID) error {
	for _, peer := range peers {
		for _, metric := range mc.metrics.PeerMetrics(peer) {
			if metric.Valid && metric.Expired() {
				err := mc.alert(metric.Peer, metric.Name)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (mc *Checker) alert(pid peer.ID, metricName string) error {
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

// Alerts returns a channel which gets notified by CheckPeers.
func (mc *Checker) Alerts() <-chan api.Alert {
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
			peers, err := peersF(ctx)
			if err != nil {
				continue
			}
			mc.CheckPeers(peers)
		case <-ctx.Done():
			ticker.Stop()
			return
		}
	}
}
