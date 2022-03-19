// Package tags implements an ipfs-cluster informer publishes user-defined
// tags as metrics.
package tags

import (
	"context"
	"sync"

	"github.com/ipfs/ipfs-cluster/api"

	logging "github.com/ipfs/go-log/v2"
	rpc "github.com/libp2p/go-libp2p-gorpc"
)

var logger = logging.Logger("tags")

// MetricName specifies the name of our metric
var MetricName = "tags"

// Informer is a simple object to implement the ipfscluster.Informer
// and Component interfaces.
type Informer struct {
	config *Config // set when created, readonly

	mu        sync.Mutex // guards access to following fields
	rpcClient *rpc.Client
}

// New returns an initialized informer using the given InformerConfig.
func New(cfg *Config) (*Informer, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	return &Informer{
		config: cfg,
	}, nil
}

// Name returns the name of this informer. Note the informer issues metrics
// with custom names.
func (tags *Informer) Name() string {
	return MetricName
}

// SetClient provides us with an rpc.Client which allows
// contacting other components in the cluster.
func (tags *Informer) SetClient(c *rpc.Client) {
	tags.mu.Lock()
	defer tags.mu.Unlock()
	tags.rpcClient = c
}

// Shutdown is called on cluster shutdown. We just invalidate
// any metrics from this point.
func (tags *Informer) Shutdown(ctx context.Context) error {
	tags.mu.Lock()
	defer tags.mu.Unlock()

	tags.rpcClient = nil
	return nil
}

// GetMetrics returns one metric for each tag defined in the configuration.
// The metric name is set as "tags:<tag_name>". When no tags are defined,
// a single invalid metric is returned.
func (tags *Informer) GetMetrics(ctx context.Context) []api.Metric {
	// Note we could potentially extend the tag:value syntax to include manual weights
	// ie: { "region": "us:100", ... }
	// This would potentially allow to always give priority to peers of a certain group

	if len(tags.config.Tags) == 0 {
		logger.Debug("no tags defined in tags informer")
		m := api.Metric{
			Name:          "tag:none",
			Value:         "",
			Valid:         false,
			Partitionable: true,
		}
		m.SetTTL(tags.config.MetricTTL)
		return []api.Metric{m}
	}

	metrics := make([]api.Metric, 0, len(tags.config.Tags))
	for n, v := range tags.config.Tags {
		m := api.Metric{
			Name:          "tag:" + n,
			Value:         v,
			Valid:         true,
			Partitionable: true,
		}
		m.SetTTL(tags.config.MetricTTL)
		metrics = append(metrics, m)
	}

	return metrics
}
