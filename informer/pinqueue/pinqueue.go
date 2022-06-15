// Package pinqueue implements an ipfs-cluster informer which issues the
// current size of the pinning queue.
package pinqueue

import (
	"context"
	"fmt"
	"sync"

	"github.com/ipfs-cluster/ipfs-cluster/api"

	rpc "github.com/libp2p/go-libp2p-gorpc"

	"go.opencensus.io/trace"
)

// MetricName specifies the name of our metric
var MetricName = "pinqueue"

// Informer is a simple object to implement the ipfscluster.Informer
// and Component interfaces
type Informer struct {
	config *Config

	mu        sync.Mutex
	rpcClient *rpc.Client
}

// New returns an initialized Informer.
func New(cfg *Config) (*Informer, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	return &Informer{
		config: cfg,
	}, nil
}

// SetClient provides us with an rpc.Client which allows
// contacting other components in the cluster.
func (inf *Informer) SetClient(c *rpc.Client) {
	inf.mu.Lock()
	inf.rpcClient = c
	inf.mu.Unlock()
}

// Shutdown is called on cluster shutdown. We just invalidate
// any metrics from this point.
func (inf *Informer) Shutdown(ctx context.Context) error {
	_, span := trace.StartSpan(ctx, "informer/numpin/Shutdown")
	defer span.End()

	inf.mu.Lock()
	inf.rpcClient = nil
	inf.mu.Unlock()
	return nil
}

// Name returns the name of this informer
func (inf *Informer) Name() string {
	return MetricName
}

// GetMetrics contacts the Pintracker component and requests the number of
// queued items for pinning.
func (inf *Informer) GetMetrics(ctx context.Context) []api.Metric {
	ctx, span := trace.StartSpan(ctx, "informer/pinqueue/GetMetric")
	defer span.End()

	inf.mu.Lock()
	rpcClient := inf.rpcClient
	inf.mu.Unlock()

	if rpcClient == nil {
		return []api.Metric{
			{
				Valid: false,
			},
		}
	}

	var queued int64

	err := rpcClient.CallContext(
		ctx,
		"",
		"PinTracker",
		"PinQueueSize",
		struct{}{},
		&queued,
	)
	valid := err == nil
	weight := -queued // smaller pin queues have more priority
	if div := inf.config.WeightBucketSize; div > 0 {
		weight = weight / int64(div)
	}

	m := api.Metric{
		Name:          MetricName,
		Value:         fmt.Sprintf("%d", queued),
		Valid:         valid,
		Partitionable: false,
		Weight:        weight,
	}

	m.SetTTL(inf.config.MetricTTL)
	return []api.Metric{m}
}
