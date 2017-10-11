// Package basic implements a basic PeerMonitor component for IPFS Cluster. This
// component is in charge of logging metrics and triggering alerts when a peer
// goes down.
package basic

import (
	"context"
	"errors"
	"sync"
	"time"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
)

var logger = logging.Logger("monitor")

// AlertChannelCap specifies how much buffer the alerts channel has.
var AlertChannelCap = 256

// WindowCap specifies how many metrics to keep for given host and metric type
var WindowCap = 10

// peerMetrics is just a circular queue
type peerMetrics struct {
	last   int
	window []api.Metric
	//	mux    sync.RWMutex
}

func newPeerMetrics(windowCap int) *peerMetrics {
	w := make([]api.Metric, 0, windowCap)
	return &peerMetrics{0, w}
}

func (pmets *peerMetrics) add(m api.Metric) {
	//	pmets.mux.Lock()
	//	defer pmets.mux.Unlock()
	if len(pmets.window) < cap(pmets.window) {
		pmets.window = append(pmets.window, m)
		pmets.last = len(pmets.window) - 1
		return
	}

	// len == cap
	pmets.last = (pmets.last + 1) % cap(pmets.window)
	pmets.window[pmets.last] = m
	return
}

func (pmets *peerMetrics) latest() (api.Metric, error) {
	//	pmets.mux.RLock()
	//	defer pmets.mux.RUnlock()
	if len(pmets.window) == 0 {
		return api.Metric{}, errors.New("no metrics")
	}
	return pmets.window[pmets.last], nil
}

// ordered from newest to oldest
func (pmets *peerMetrics) all() []api.Metric {
	//	pmets.mux.RLock()
	//	pmets.mux.RUnlock()
	wlen := len(pmets.window)
	res := make([]api.Metric, 0, wlen)
	if wlen == 0 {
		return res
	}
	for i := pmets.last; i >= 0; i-- {
		res = append(res, pmets.window[i])
	}
	for i := wlen; i > pmets.last; i-- {
		res = append(res, pmets.window[i])
	}
	return res
}

type metricsByPeer map[peer.ID]*peerMetrics

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

	if WindowCap <= 0 {
		panic("windowCap too small")
	}

	ctx, cancel := context.WithCancel(context.Background())

	mon := &Monitor{
		ctx:      ctx,
		cancel:   cancel,
		rpcReady: make(chan struct{}, 1),

		metrics:   make(map[string]metricsByPeer),
		windowCap: WindowCap,
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
func (mon *Monitor) LogMetric(m api.Metric) {
	mon.metricsMux.Lock()
	defer mon.metricsMux.Unlock()
	name := m.Name
	peer := m.Peer
	mbyp, ok := mon.metrics[name]
	if !ok {
		mbyp = make(metricsByPeer)
		mon.metrics[name] = mbyp
	}
	pmets, ok := mbyp[peer]
	if !ok {
		pmets = newPeerMetrics(mon.windowCap)
		mbyp[peer] = pmets
	}

	logger.Debugf("logged '%s' metric from '%s'. Expires on %s", name, peer, m.Expire)
	pmets.add(m)
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

// 	pmets, ok := mbyp[p]
// 	if !ok {
// 		return emptyMetric
// 	}
// 	metric, err := pmets.latest()
// 	if err != nil {
// 		return emptyMetric
// 	}
// 	return metric
// }

// LastMetrics returns last known VALID metrics of a given type. A metric
// is only valid if it has not expired and belongs to a current cluster peer.
func (mon *Monitor) LastMetrics(name string) []api.Metric {
	// Ger current list of peers
	var peers []peer.ID
	err := mon.rpcClient.Call("",
		"Cluster",
		"PeerManagerPeers",
		struct{}{},
		&peers)
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
		peerMetrics, ok := mbyp[peer]
		if !ok {
			continue
		}
		last, err := peerMetrics.latest()
		if err != nil || last.Discard() {
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
				"PeerManagerPeers",
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
		pMetrics, ok := metricsByPeer[p]
		if !ok { // no metrics from this peer
			continue
		}
		last, err := pMetrics.latest()
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
