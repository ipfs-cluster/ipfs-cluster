// Package disk implements an ipfs-cluster informer which can provide different
// disk-related metrics from the IPFS daemon as an api.Metric.
package disk

import (
	"fmt"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	logging "github.com/ipfs/go-log"

	"github.com/ipfs/ipfs-cluster/api"
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

// metricToRPC maps from a specified metric name to the corrresponding RPC call
var metricToRPC = map[MetricType]string{
	MetricFreeSpace: "IPFSFreeSpace",
	MetricRepoSize:  "IPFSRepoSize",
}

// Informer is a simple object to implement the ipfscluster.Informer
// and Component interfaces.
type Informer struct {
	config    *Config
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
	return disk.config.Type.String()
}

// SetClient provides us with an rpc.Client which allows
// contacting other components in the cluster.
func (disk *Informer) SetClient(c *rpc.Client) {
	disk.rpcClient = c
}

// Shutdown is called on cluster shutdown. We just invalidate
// any metrics from this point.
func (disk *Informer) Shutdown() error {
	disk.rpcClient = nil
	return nil
}

// GetMetric returns the metric obtained by this
// Informer.
func (disk *Informer) GetMetric() api.Metric {
	if disk.rpcClient == nil {
		return api.Metric{
			Name:  disk.Name(),
			Valid: false,
		}
	}

	var metric uint64
	valid := true
	err := disk.rpcClient.Call("",
		"Cluster",
		metricToRPC[disk.config.Type],
		struct{}{},
		&metric)
	if err != nil {
		logger.Error(err)
		valid = false
	}

	m := api.Metric{
		Name:  disk.Name(),
		Value: fmt.Sprintf("%d", metric),
		Valid: valid,
	}

	m.SetTTLDuration(disk.config.MetricTTL)
	return m
}
