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

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/monitor/metrics"
	"github.com/ipfs/ipfs-cluster/rpcutil"
	"go.opencensus.io/trace"

	logging "github.com/ipfs/go-log"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p-peer"
)

var logger = logging.Logger("monitor")

// Monitor is a component in charge of monitoring peers, logging
// metrics and detecting failures
type Monitor struct {
	ctx       context.Context
	cancel    func()
	rpcClient *rpc.Client
	rpcReady  chan struct{}

	metrics *metrics.Store
	checker *metrics.Checker

	config *Config

	shutdownLock sync.Mutex
	shutdown     bool
	wg           sync.WaitGroup
}

// NewMonitor creates a new monitor using the given config.
func NewMonitor(cfg *Config) (*Monitor, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	mtrs := metrics.NewStore()
	checker := metrics.NewChecker(mtrs)

	mon := &Monitor{
		ctx:      ctx,
		cancel:   cancel,
		rpcReady: make(chan struct{}, 1),

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
		go mon.checker.Watch(mon.ctx, mon.getPeers, mon.config.CheckInterval)
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
func (mon *Monitor) Shutdown(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "monitor/basic/Shutdown")
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
func (mon *Monitor) LogMetric(ctx context.Context, m api.Metric) error {
	ctx, span := trace.StartSpan(ctx, "monitor/basic/LogMetric")
	defer span.End()

	mon.metrics.Add(m)
	logger.Debugf("basic monitor logged '%s' metric from '%s'. Expires on %d", m.Name, m.Peer, m.Expire)
	return nil
}

// PublishMetric broadcasts a metric to all current cluster peers.
func (mon *Monitor) PublishMetric(ctx context.Context, m api.Metric) error {
	ctx, span := trace.StartSpan(ctx, "monitor/basic/PublishMetric")
	defer span.End()

	if m.Discard() {
		logger.Warningf("discarding invalid metric: %+v", m)
		return nil
	}

	peers, err := mon.getPeers(ctx)
	if err != nil {
		return err
	}

	ctxs, cancels := rpcutil.CtxsWithTimeout(ctx, len(peers), m.GetTTL()/2)
	defer rpcutil.MultiCancel(cancels)

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
func (mon *Monitor) getPeers(ctx context.Context) ([]peer.ID, error) {
	ctx, span := trace.StartSpan(ctx, "monitor/basic/getPeers")
	defer span.End()

	var peers []peer.ID
	err := mon.rpcClient.CallContext(
		ctx,
		"",
		"Cluster",
		"ConsensusPeers",
		struct{}{},
		&peers,
	)
	if err != nil {
		logger.Error(err)
	}
	return peers, err
}

// LatestMetrics returns last known VALID metrics of a given type. A metric
// is only valid if it has not expired and belongs to a current cluster peers.
func (mon *Monitor) LatestMetrics(ctx context.Context, name string) []api.Metric {
	ctx, span := trace.StartSpan(ctx, "monitor/basic/LatestMetrics")
	defer span.End()

	latest := mon.metrics.Latest(name)

	// Make sure we only return metrics in the current peerset
	peers, err := mon.getPeers(ctx)
	if err != nil {
		return []api.Metric{}
	}

	return metrics.PeersetFilter(latest, peers)
}

// Alerts returns a channel on which alerts are sent when the
// monitor detects a failure.
func (mon *Monitor) Alerts() <-chan api.Alert {
	return mon.checker.Alerts()
}
