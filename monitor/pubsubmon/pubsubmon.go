// Package pubsubmon implements a PeerMonitor component for IPFS Cluster that
// uses PubSub to send and receive metrics.
package pubsubmon

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/monitor/util"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	logging "github.com/ipfs/go-log"
	floodsub "github.com/libp2p/go-floodsub"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	msgpack "github.com/multiformats/go-multicodec/msgpack"
)

var logger = logging.Logger("monitor")

// PubsubTopic specifies the topic used to publish Cluster metrics.
var PubsubTopic = "pubsubmon"

// AlertChannelCap specifies how much buffer the alerts channel has.
var AlertChannelCap = 256

// DefaultWindowCap sets the amount of metrics to store per peer.
var DefaultWindowCap = 25

var msgpackHandle = msgpack.DefaultMsgpackHandle()

// Monitor is a component in charge of monitoring peers, logging
// metrics and detecting failures
type Monitor struct {
	ctx       context.Context
	cancel    func()
	rpcClient *rpc.Client
	rpcReady  chan struct{}

	host         host.Host
	pubsub       *floodsub.PubSub
	subscription *floodsub.Subscription

	metrics    util.Metrics
	metricsMux sync.RWMutex
	windowCap  int

	checker *util.MetricsChecker
	alerts  chan api.Alert

	config *Config

	shutdownLock sync.Mutex
	shutdown     bool
	wg           sync.WaitGroup
}

// New creates a new monitor. It receives the window capacity
// (how many metrics to keep for each peer and type of metric) and the
// monitoringInterval (interval between the checks that produce alerts)
// as parameters
func New(h host.Host, cfg *Config) (*Monitor, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	if DefaultWindowCap <= 0 {
		panic("windowCap too small")
	}

	ctx, cancel := context.WithCancel(context.Background())

	alertCh := make(chan api.Alert, AlertChannelCap)
	metrics := make(util.Metrics)

	pubsub, err := floodsub.NewFloodSub(ctx, h)
	if err != nil {
		cancel()
		return nil, err
	}

	subscription, err := pubsub.Subscribe(PubsubTopic)
	if err != nil {
		cancel()
		return nil, err
	}

	mon := &Monitor{
		ctx:      ctx,
		cancel:   cancel,
		rpcReady: make(chan struct{}, 1),

		host:         h,
		pubsub:       pubsub,
		subscription: subscription,

		metrics:   metrics,
		windowCap: DefaultWindowCap,
		checker:   util.NewMetricsChecker(metrics, alertCh),
		alerts:    alertCh,
		config:    cfg,
	}

	go mon.run()
	return mon, nil
}

func (mon *Monitor) run() {
	select {
	case <-mon.rpcReady:
		go mon.monitor()
		go mon.logFromPubsub()
	case <-mon.ctx.Done():
	}
}

// logFromPubsub logs metrics received in the subscribed topic.
func (mon *Monitor) logFromPubsub() {
	for {
		select {
		case <-mon.ctx.Done():
			return
		default:
			msg, err := mon.subscription.Next(mon.ctx)
			if err != nil { // context cancelled enters here
				continue
			}

			data := msg.GetData()
			buf := bytes.NewBuffer(data)
			dec := msgpack.Multicodec(msgpackHandle).Decoder(buf)
			metric := api.Metric{}
			err = dec.Decode(&metric)
			if err != nil {
				logger.Error(err)
				continue
			}
			logger.Debugf(
				"received pubsub metric '%s' from '%s'",
				metric.Name,
				metric.Peer,
			)

			err = mon.LogMetric(metric)
			if err != nil {
				logger.Error(err)
				continue
			}
		}
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

	// not necessary as this just removes subscription
	// mon.subscription.Cancel()
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
		mbyp = make(util.PeerMetrics)
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

	var b bytes.Buffer

	enc := msgpack.Multicodec(msgpackHandle).Encoder(&b)
	err := enc.Encode(m)
	if err != nil {
		logger.Error(err)
		return err
	}

	logger.Debugf(
		"publishing metric %s to pubsub. Expires: %d",
		m.Name,
		m.Expire,
	)

	err = mon.pubsub.Publish(PubsubTopic, b.Bytes())
	if err != nil {
		logger.Error(err)
		return err
	}

	return nil
}

// getPeers gets the current list of peers from the consensus component
func (mon *Monitor) getPeers() ([]peer.ID, error) {
	// Ger current list of peers
	var peers []peer.ID
	err := mon.rpcClient.Call(
		"",
		"Cluster",
		"ConsensusPeers",
		struct{}{},
		&peers,
	)
	return peers, err
}

// LastMetrics returns last known VALID metrics of a given type. A metric
// is only valid if it has not expired and belongs to a current cluster peers.
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
			peers, err := mon.getPeers()
			if err != nil {
				logger.Error(err)
				break
			}
			mon.metricsMux.RLock()
			mon.checker.CheckMetrics(peers)
			mon.metricsMux.RUnlock()
		case <-mon.ctx.Done():
			ticker.Stop()
			return
		}
	}
}
