// Package disk implements an ipfs-cluster informer which determines
// the current RepoSize of the ipfs daemon datastore and returns it as an
// api.Metric.
package disk

import (
	"fmt"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	logging "github.com/ipfs/go-log"

	"github.com/ipfs/ipfs-cluster/api"
)

// TODO: switch default to disk-freespace
const DefaultMetric = "disk-reposize"

var logger = logging.Logger("diskinfo")

// MetricTTL specifies how long our reported metric is valid in seconds.
var MetricTTL = 30

// MetricName specifies the name of our metric
var MetricName string

var nameToRPC = map[string]string{
	"disk-freespace": "IPFSFreeSpace",
	"disk-reposize":  "IPFSRepoSize",
}

// Informer is a simple object to implement the ipfscluster.Informer
// and Component interfaces.
type Informer struct {
	rpcClient *rpc.Client
}

// NewInformer returns an initialized Informer.
func NewInformer() *Informer {
	MetricName = DefaultMetric
	return &Informer{}
}

// NewInformer returns an initialized Informer.
func NewInformerWithMetric(metric string) *Informer {
	// assume `metric` has been checked already
	MetricName = metric
	return &Informer{}
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

// Name returns the name of this informer.
func (disk *Informer) Name() string {
	return MetricName
}

func (disk *Informer) GetMetric() api.Metric {
	if disk.rpcClient == nil {
		return api.Metric{
			Valid: false,
		}
	}

	var metric int
	valid := true
	err := disk.rpcClient.Call("",
		"Cluster",
		nameToRPC[MetricName],
		struct{}{},
		&metric)
	if err != nil {
		logger.Error(err)
		valid = false
	}

	m := api.Metric{
		Name:  MetricName,
		Value: fmt.Sprintf("%d", metric),
		Valid: valid,
	}

	m.SetTTL(MetricTTL)
	return m
}
