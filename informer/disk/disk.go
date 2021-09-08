// Package disk implements an ipfs-cluster informer which can provide different
// disk-related metrics from the IPFS daemon as an api.Metric.
package disk

import (
	"context"
	"fmt"
	"sync"

	"github.com/ipfs/ipfs-cluster/api"

	logging "github.com/ipfs/go-log/v2"
	rpc "github.com/libp2p/go-libp2p-gorpc"

	"go.opencensus.io/trace"
)

// MetricType identifies the type of metric to fetch from the IPFS daemon.
type MetricType int

const (
	// MetricFreeSpace provides the available space reported by IPFS
	MetricFreeSpace = iota
	// MetricRepoSize provides the used space reported by IPFS
	MetricRepoSize
)

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

// Name returns the user-facing name of this informer.
func (disk *Informer) Name() string {
	return "reposize"
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

// GetMetric returns the metric obtained by this
// Informer.
func (disk *Informer) GetMetric(ctx context.Context) *api.Metric {
	ctx, span := trace.StartSpan(ctx, "informer/disk/GetMetric")
	defer span.End()

	disk.mu.Lock()
	rpcClient := disk.rpcClient
	disk.mu.Unlock()

	if rpcClient == nil {
		return &api.Metric{
			Name:  disk.Name(),
			Valid: false,
		}
	}

	var repoStat api.IPFSRepoStat
	var metric uint64

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
		size := repoStat.RepoSize
		total := repoStat.StorageMax
		if size < total {
			metric = total - size
		} else { // Make sure we don't underflow
			metric = 0
		}
	}

	m := &api.Metric{
		Name:  disk.Name(),
		Value: fmt.Sprintf("%d", metric),
		Valid: valid,
	}

	m.SetTTL(disk.config.MetricTTL)
	return m
}
