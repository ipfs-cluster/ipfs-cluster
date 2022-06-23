// Package disk implements an ipfs-cluster informer which can provide different
// disk-related metrics from the IPFS daemon as an api.Metric.
package disk

import (
	"context"
	"fmt"
	"sync"

	"github.com/ipfs-cluster/ipfs-cluster/api"

	logging "github.com/ipfs/go-log/v2"
	rpc "github.com/libp2p/go-libp2p-gorpc"

	"go.opencensus.io/trace"
)

// MetricType identifies the type of metric to fetch from the IPFS daemon.
type MetricType int

const (
	// MetricFreeSpace provides the available space reported by IPFS
	MetricFreeSpace MetricType = iota
	// MetricRepoSize provides the used space reported by IPFS
	MetricRepoSize
)

// String returns a string representation for MetricType.
func (t MetricType) String() string {
	switch t {
	case MetricFreeSpace:
		return "freespace"
	case MetricRepoSize:
		return "reposize"
	}
	return ""
}

var logger = logging.Logger("diskinfo")

// Informer is a simple object to implement the ipfscluster.Informer
// and Component interfaces.
type Informer struct {
	config *Config // set when created, readonly

	mu        sync.Mutex // guards access to following fields
	rpcClient *rpc.Client
}

// NewInformer returns an initialized informer using the given InformerConfig.
func NewInformer(cfg *Config) (*Informer, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	return &Informer{
		config: cfg,
	}, nil
}

// Name returns the name of the metric issued by this informer.
func (disk *Informer) Name() string {
	return disk.config.MetricType.String()
}

// SetClient provides us with an rpc.Client which allows
// contacting other components in the cluster.
func (disk *Informer) SetClient(c *rpc.Client) {
	disk.mu.Lock()
	defer disk.mu.Unlock()
	disk.rpcClient = c
}

// Shutdown is called on cluster shutdown. We just invalidate
// any metrics from this point.
func (disk *Informer) Shutdown(ctx context.Context) error {
	_, span := trace.StartSpan(ctx, "informer/disk/Shutdown")
	defer span.End()

	disk.mu.Lock()
	defer disk.mu.Unlock()

	disk.rpcClient = nil
	return nil
}

// GetMetrics returns the metric obtained by this Informer. It must always
// return at least one metric.
func (disk *Informer) GetMetrics(ctx context.Context) []api.Metric {
	ctx, span := trace.StartSpan(ctx, "informer/disk/GetMetric")
	defer span.End()

	disk.mu.Lock()
	rpcClient := disk.rpcClient
	disk.mu.Unlock()

	if rpcClient == nil {
		return []api.Metric{
			{
				Name:  disk.Name(),
				Valid: false,
			},
		}
	}

	var repoStat api.IPFSRepoStat
	var weight uint64
	var value string

	valid := true

	err := rpcClient.CallContext(
		ctx,
		"",
		"IPFSConnector",
		"RepoStat",
		struct{}{},
		&repoStat,
	)
	if err != nil {
		logger.Error(err)
		valid = false
	} else {
		switch disk.config.MetricType {
		case MetricFreeSpace:
			size := repoStat.RepoSize
			total := repoStat.StorageMax
			if size < total {
				weight = total - size
			} else {
				// Make sure we don't underflow and stop
				// sending this metric when space is exhausted.
				weight = 0
				valid = false
				logger.Warn("reported freespace is 0")
			}
			value = fmt.Sprintf("%d", weight)
		case MetricRepoSize:
			// smaller repositories have more priority
			weight = -repoStat.RepoSize
			value = fmt.Sprintf("%d", repoStat.RepoSize)
		}
	}

	m := api.Metric{
		Name:          disk.Name(),
		Value:         value,
		Valid:         valid,
		Weight:        int64(weight),
		Partitionable: false,
	}

	m.SetTTL(disk.config.MetricTTL)
	return []api.Metric{m}
}
