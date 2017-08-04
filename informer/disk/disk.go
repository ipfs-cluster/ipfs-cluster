// Package disk implements an ipfs-cluster informer which uses a metric (e.g.
// RepoSize or FreeSpace of the IPFS daemon datastore) and returns it as an
// api.Metric. The supported metrics are listed as the keys in the nameToRPC
// map below.
package disk

import (
	"fmt"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	logging "github.com/ipfs/go-log"

	"github.com/ipfs/ipfs-cluster/api"
)

const DefaultMetric = "disk-freespace"

var logger = logging.Logger("diskinfo")

// MetricTTL specifies how long our reported metric is valid in seconds.
var MetricTTL = 30

// nameToRPC maps from a specified metric name to the corrresponding RPC call
var nameToRPC = map[string]string{
	"disk-freespace": "IPFSFreeSpace",
	"disk-reposize":  "IPFSRepoSize",
}

// Informer is a simple object to implement the ipfscluster.Informer
// and Component interfaces.
type Informer struct {
	metricName string
	rpcClient  *rpc.Client
}

// NewInformer returns an initialized Informer that uses the DefaultMetric
func NewInformer() *Informer {
	return &Informer{
		metricName: DefaultMetric,
	}
}

// NewInformerWithMetric returns an initialized Informer with a specified metric
// name, if that name is valid, or nil. A metric name is valid if it is a key in
// the nameToRPC map.
func NewInformerWithMetric(metric string) *Informer {
	// check whether specified metric is supported
	if _, valid := nameToRPC[metric]; valid {
		return &Informer{
			metricName: metric,
		}
	}
	return nil
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
	return disk.metricName
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
		nameToRPC[disk.metricName],
		struct{}{},
		&metric)
	if err != nil {
		logger.Error(err)
		valid = false
	}

	m := api.Metric{
		Name:  disk.metricName,
		Value: fmt.Sprintf("%d", metric),
		Valid: valid,
	}

	m.SetTTL(MetricTTL)
	return m
}
