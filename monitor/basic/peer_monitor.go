// Package basic implements a basic PeerMonitor component for IPFS Cluster. This
// component is in charge of logging metrics and triggering alerts when a peer
// goes down.
package basic

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/monitor/util"
	"github.com/ipfs/ipfs-cluster/rpcutil"
)

var logger = logging.Logger("monitor")

// AlertChannelCap specifies how much buffer the alerts channel has.
var AlertChannelCap = 256

// DefaultWindowCap sets the amount of metrics to store per peer.
var DefaultWindowCap = 25

type metricsByPeer map[peer.ID]*util.MetricsWindow

// Monitor is a component in charge of monitoring peers, logging
// metrics and detecting failures
type Monitor struct {
	ctx       context.Context
	cancel    func()
	rpcClient *rpc.Client
	rpcReady  chan struct{}

	metrics    map[string]metricsByPeer
	metricsMux sync.RWMutex
	windowCap  int

	alerts chan api.Alert

	config *Config

	shutdownLock sync.Mutex
	shutdown     bool
	wg           sync.WaitGroup
}

// NewMonitor creates a new monitor. It receives the window capacity
// (how many metrics to keep for each peer and type of metric) and the
// monitoringInterval (interval between the checks that produce alerts)
// as parameters
func NewMonitor(cfg *Config) (*Monitor, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	if DefaultWindowCap <= 0 {
		panic("windowCap too small")
	}

	ctx, cancel := context.WithCancel(context.Background())

	mon := &Monitor{
		ctx:      ctx,
		cancel:   cancel,
		rpcReady: make(chan struct{}, 1),

		metrics:   make(map[string]metricsByPeer),
		windowCap: DefaultWindowCap,
		alerts:    make(chan api.Alert, AlertChannelCap),

		config: cfg,
	}

	go mon.run()
	return mon, nil
}

func (mon *Monitor) run() {
	select {
	case <-mon.rpcReady:
		go mon.monitor()
	case <-mon.ctx.Done():
	}
}

// SetClient saves the given rpc.Client  for later use
func (mon *Monitor) SetClient(c *rpc.Client) {
	mon.rpcClient = c
	mon.rpcReady <- struct{}{}
}

// Shutdown stops the peer monitor. It particular, it will
// not deliver any alerts.
func (mon *Monitor) Shutdown() error {
	mon.shutdownLock.Lock()
	defer mon.shutdownLock.Unlock()

	if mon.shutdown {
		logger.Warning("Monitor already shut down")
		return nil
	}

	logger.Info("stopping Monitor")
	close(mon.rpcReady)
	mon.cancel()
	mon.wg.Wait()
	mon.shutdown = true
	return nil
}

// LogMetric stores a metric so it can later be retrieved.
func (mon *Monitor) LogMetric(m api.Metric) error {
	mon.metricsMux.Lock()
	defer mon.metricsMux.Unlock()
	name := m.Name
	peer := m.Peer
	mbyp, ok := mon.metrics[name]
	if !ok {
		mbyp = make(metricsByPeer)
		mon.metrics[name] = mbyp
	}
	window, ok := mbyp[peer]
	if !ok {
		// We always lock the outer map, so we can use unsafe
		// MetricsWindow.
		window = util.NewMetricsWindow(mon.windowCap, false)
		mbyp[peer] = window
	}

	logger.Debugf("logged '%s' metric from '%s'. Expires on %d", name, peer, m.Expire)
	window.Add(m)
	return nil
}

// PublishMetric broadcasts a metric to all current cluster peers.
func (mon *Monitor) PublishMetric(m api.Metric) error {
	if m.Discard() {
		logger.Warningf("discarding invalid metric: %+v", m)
		return nil
	}

	peers, err := mon.getPeers()
	if err != nil {
		logger.Error("PublishPeers could not list peers:", err)
	}

	ctxs, cancels := rpcutil.CtxsWithTimeout(mon.ctx, len(peers), m.GetTTL()/2)
	defer rpcutil.Multicancel(cancels)

	logger.Debugf(
		"broadcasting metric %s to %s. Expires: %d",
		m.Name,
		peers,
		m.Expire,
	)

	// This may hang if one of the calls does, but we will return when the
	// context expires.
	errs := mon.rpcClient.MultiCall(
		ctxs,
		peers,
		"Cluster",
		"PeerMonitorLogMetric",
		m,
		rpcutil.RPCDiscardReplies(len(peers)),
	)

	var errStrs []string

	for i, e := range errs {
		if e != nil {
			errStr := fmt.Sprintf(
				"error pushing metric to %s: %s",
				peers[i].Pretty(),
				e,
			)
			logger.Errorf(errStr)
			errStrs = append(errStrs, errStr)
		}
	}

	if len(errStrs) > 0 {
		return errors.New(strings.Join(errStrs, "\n"))
	}

	logger.Debugf(
		"broadcasted metric %s to [%s]. Expires: %d",
		m.Name,
		peers,
		m.Expire,
	)
	return nil
}

// getPeers gets the current list of peers from the consensus component
func (mon *Monitor) getPeers() ([]peer.ID, error) {
	// Ger current list of peers
	var peers []peer.ID
	err := mon.rpcClient.Call("",
		"Cluster",
		"ConsensusPeers",
		struct{}{},
		&peers)
	return peers, err
}

func copyEmptyStructToIfaces(in []struct{}) []interface{} {
	ifaces := make([]interface{}, len(in), len(in))
	for i := range in {
		ifaces[i] = &in[i]
	}
	return ifaces
}

// func (mon *Monitor) getLastMetric(name string, p peer.ID) api.Metric {
// 	mon.metricsMux.RLock()
// 	defer mon.metricsMux.RUnlock()

// 	emptyMetric := api.Metric{
// 		Name:  name,
// 		Peer:  p,
// 		Valid: false,
// 	}

// 	mbyp, ok := mon.metrics[name]
// 	if !ok {
// 		return emptyMetric
// 	}

// 	window, ok := mbyp[p]
// 	if !ok {
// 		return emptyMetric
// 	}
// 	metric, err := window.Latest()
// 	if err != nil {
// 		return emptyMetric
// 	}
// 	return metric
// }

// LastMetrics returns last known VALID metrics of a given type. A metric
// is only valid if it has not expired and belongs to a current cluster peer.
func (mon *Monitor) LastMetrics(name string) []api.Metric {
	peers, err := mon.getPeers()
	if err != nil {
		logger.Errorf("LastMetrics could not list peers: %s", err)
		return []api.Metric{}
	}

	mon.metricsMux.RLock()
	defer mon.metricsMux.RUnlock()

	mbyp, ok := mon.metrics[name]
	if !ok {
		logger.Warningf("LastMetrics: No %s metrics", name)
		return []api.Metric{}
	}

	metrics := make([]api.Metric, 0, len(mbyp))

	// only show metrics for current set of peers
	for _, peer := range peers {
		window, ok := mbyp[peer]
		if !ok {
			continue
		}
		last, err := window.Latest()
		if err != nil || last.Discard() {
			logger.Warningf("no valid last metric for peer: %+v", last)
			continue
		}
		metrics = append(metrics, last)

	}
	return metrics
}

// Alerts returns a channel on which alerts are sent when the
// monitor detects a failure.
func (mon *Monitor) Alerts() <-chan api.Alert {
	return mon.alerts
}

// monitor creates a ticker which fetches current
// cluster peers and checks that the last metric for a peer
// has not expired.
func (mon *Monitor) monitor() {
	ticker := time.NewTicker(mon.config.CheckInterval)
	for {
		select {
		case <-ticker.C:
			logger.Debug("monitoring tick")
			// Get current peers
			var peers []peer.ID
			err := mon.rpcClient.Call("",
				"Cluster",
				"ConsensusPeers",
				struct{}{},
				&peers)
			if err != nil {
				logger.Error(err)
				break
			}

			for k := range mon.metrics {
				logger.Debug("check metrics ", k)
				mon.checkMetrics(peers, k)
			}
		case <-mon.ctx.Done():
			ticker.Stop()
			return
		}
	}
}

// This is probably the place to implement some advanced ways of detecting down
// peers.
// Currently easy logic, just check that all peers have a valid metric.
func (mon *Monitor) checkMetrics(peers []peer.ID, metricName string) {
	mon.metricsMux.RLock()
	defer mon.metricsMux.RUnlock()

	// get metric windows for peers
	metricsByPeer := mon.metrics[metricName]

	// for each of the given current peers
	for _, p := range peers {
		// get metrics for that peer
		window, ok := metricsByPeer[p]
		if !ok { // no metrics from this peer
			continue
		}
		last, err := window.Latest()
		if err != nil { // no metrics for this peer
			continue
		}
		// send alert if metric is expired (but was valid at some point)
		if last.Valid && last.Expired() {
			logger.Debugf("Metric %s from peer %s expired at %s", metricName, p, last.Expire)
			mon.sendAlert(p, metricName)
		}
	}
}

func (mon *Monitor) sendAlert(p peer.ID, metricName string) {
	alrt := api.Alert{
		Peer:       p,
		MetricName: metricName,
	}
	select {
	case mon.alerts <- alrt:
	default:
		logger.Error("alert channel is full")
	}
}
