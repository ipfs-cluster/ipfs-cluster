// Package pubsubmon implements a PeerMonitor component for IPFS Cluster that
// uses PubSub to send and receive metrics.
package pubsubmon

import (
	"bytes"
	"context"

	"sync"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/monitor/metrics"

	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	msgpack "github.com/multiformats/go-multicodec/msgpack"

	"go.opencensus.io/trace"
)

var logger = logging.Logger("monitor")

// PubsubTopic specifies the topic used to publish Cluster metrics.
var PubsubTopic = "monitor.metrics"

var msgpackHandle = msgpack.DefaultMsgpackHandle()

// Monitor is a component in charge of monitoring peers, logging
// metrics and detecting failures
type Monitor struct {
	ctx       context.Context
	cancel    func()
	rpcClient *rpc.Client
	rpcReady  chan struct{}

	pubsub       *pubsub.PubSub
	subscription *pubsub.Subscription
	peers        PeersFunc

	metrics *metrics.Store
	checker *metrics.Checker

	config *Config

	shutdownLock sync.Mutex
	shutdown     bool
	wg           sync.WaitGroup
}

// PeersFunc allows the Monitor to filter and discard metrics
// that do not belong to a given peerset.
type PeersFunc func(context.Context) ([]peer.ID, error)

// New creates a new PubSub monitor, using the given host, config and
// PeersFunc. The PeersFunc can be nil. In this case, no metric filtering is
// done based on peers (any peer is considered part of the peerset).
func New(
	ctx context.Context,
	cfg *Config,
	psub *pubsub.PubSub,
	peers PeersFunc,
) (*Monitor, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)

	mtrs := metrics.NewStore()
	checker := metrics.NewChecker(ctx, mtrs, cfg.FailureThreshold)

	subscription, err := psub.Subscribe(PubsubTopic)
	if err != nil {
		cancel()
		return nil, err
	}

	mon := &Monitor{
		ctx:      ctx,
		cancel:   cancel,
		rpcReady: make(chan struct{}, 1),

		pubsub:       psub,
		subscription: subscription,
		peers:        peers,

		metrics: mtrs,
		checker: checker,
		config:  cfg,
	}

	go mon.run()
	return mon, nil
}

func (mon *Monitor) run() {
	select {
	case <-mon.rpcReady:
		go mon.logFromPubsub()
		go mon.checker.Watch(mon.ctx, mon.peers, mon.config.CheckInterval)
	case <-mon.ctx.Done():
	}
}

// logFromPubsub logs metrics received in the subscribed topic.
func (mon *Monitor) logFromPubsub() {
	ctx, span := trace.StartSpan(mon.ctx, "monitor/pubsub/logFromPubsub")
	defer span.End()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := mon.subscription.Next(ctx)
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

			err = mon.LogMetric(ctx, &metric)
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
func (mon *Monitor) Shutdown(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "monitor/pubsub/Shutdown")
	defer span.End()

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
func (mon *Monitor) LogMetric(ctx context.Context, m *api.Metric) error {
	ctx, span := trace.StartSpan(ctx, "monitor/pubsub/LogMetric")
	defer span.End()

	mon.metrics.Add(m)
	logger.Debugf("pubsub mon logged '%s' metric from '%s'. Expires on %d", m.Name, m.Peer, m.Expire)
	return nil
}

// PublishMetric broadcasts a metric to all current cluster peers.
func (mon *Monitor) PublishMetric(ctx context.Context, m *api.Metric) error {
	ctx, span := trace.StartSpan(ctx, "monitor/pubsub/PublishMetric")
	defer span.End()

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

// LatestMetrics returns last known VALID metrics of a given type. A metric
// is only valid if it has not expired and belongs to a current cluster peer.
func (mon *Monitor) LatestMetrics(ctx context.Context, name string) []*api.Metric {
	ctx, span := trace.StartSpan(ctx, "monitor/pubsub/LatestMetrics")
	defer span.End()

	latest := mon.metrics.LatestValid(name)

	if mon.peers == nil {
		return latest
	}

	// Make sure we only return metrics in the current peerset if we have
	// a peerset provider.
	peers, err := mon.peers(ctx)
	if err != nil {
		return []*api.Metric{}
	}

	return metrics.PeersetFilter(latest, peers)
}

// Alerts returns a channel on which alerts are sent when the
// monitor detects a failure.
func (mon *Monitor) Alerts() <-chan *api.Alert {
	return mon.checker.Alerts()
}
