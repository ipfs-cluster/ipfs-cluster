package ipfscluster

import (
	"context"
	"errors"
	"sync"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
)

// AlertChannelCap specifies how much buffer the alerts channel has.
var AlertChannelCap = 256

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

// StdPeerMonitor is a component in charge of monitoring peers, logging
// metrics and detecting failures
type StdPeerMonitor struct {
	ctx       context.Context
	cancel    func()
	rpcClient *rpc.Client
	rpcReady  chan struct{}

	metrics    map[string]metricsByPeer
	metricsMux sync.RWMutex
	windowCap  int

	alerts chan api.Alert

	shutdownLock sync.Mutex
	shutdown     bool
	wg           sync.WaitGroup
}

// NewStdPeerMonitor creates a new monitor.
func NewStdPeerMonitor(windowCap int) *StdPeerMonitor {
	if windowCap <= 0 {
		panic("windowCap too small")
	}

	ctx, cancel := context.WithCancel(context.Background())

	mon := &StdPeerMonitor{
		ctx:      ctx,
		cancel:   cancel,
		rpcReady: make(chan struct{}, 1),

		metrics:   make(map[string]metricsByPeer),
		windowCap: windowCap,
		alerts:    make(chan api.Alert),
	}

	go mon.run()
	return mon
}

func (mon *StdPeerMonitor) run() {
	select {
	case <-mon.rpcReady:
		//go mon.Heartbeat()
	case <-mon.ctx.Done():
	}
}

// SetClient saves the given rpc.Client  for later use
func (mon *StdPeerMonitor) SetClient(c *rpc.Client) {
	mon.rpcClient = c
	mon.rpcReady <- struct{}{}
}

// Shutdown stops the peer monitor. It particular, it will
// not deliver any alerts.
func (mon *StdPeerMonitor) Shutdown() error {
	mon.shutdownLock.Lock()
	defer mon.shutdownLock.Unlock()

	if mon.shutdown {
		logger.Warning("StdPeerMonitor already shut down")
		return nil
	}

	logger.Info("stopping StdPeerMonitor")
	close(mon.rpcReady)
	mon.cancel()
	mon.wg.Wait()
	mon.shutdown = true
	return nil
}

// LogMetric stores a metric so it can later be retrieved.
func (mon *StdPeerMonitor) LogMetric(m api.Metric) {
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

	logger.Debugf("logged '%s' metric from '%s'", name, peer)
	pmets.add(m)
}

// func (mon *StdPeerMonitor) getLastMetric(name string, p peer.ID) api.Metric {
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

// LastMetrics returns last known VALID metrics of a given type
func (mon *StdPeerMonitor) LastMetrics(name string) []api.Metric {
	mon.metricsMux.RLock()
	defer mon.metricsMux.RUnlock()

	mbyp, ok := mon.metrics[name]
	if !ok {
		return []api.Metric{}
	}

	metrics := make([]api.Metric, 0, len(mbyp))

	for _, peerMetrics := range mbyp {
		last, err := peerMetrics.latest()
		if err != nil || last.Discard() {
			continue
		}
		metrics = append(metrics, last)
	}
	return metrics
}

// Alerts() returns a channel on which alerts are sent when the
// monitor detects a failure.
func (mon *StdPeerMonitor) Alerts() <-chan api.Alert {
	return mon.alerts
}
