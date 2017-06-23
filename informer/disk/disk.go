// Package disk implements an ipfs-cluster informer which determines
// the current RepoSize of the ipfs daemon datastore and returns it as an
// api.Metric.
package disk

import (
	"fmt"

	logging "gx/ipfs/QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52/go-log"
	rpc "gx/ipfs/QmYqnvVzUjjVddWPLGMAErUjNBqnyjoeeCgZUZFsAJeGHr/go-libp2p-gorpc"

	"github.com/ipfs/ipfs-cluster/api"
)

var logger = logging.Logger("diskinfo")

// MetricTTL specifies how long our reported metric is valid in seconds.
var MetricTTL = 30

// MetricName specifies the name of our metric
var MetricName = "disk"

// Informer is a simple object to implement the ipfscluster.Informer
// and Component interfaces.
type Informer struct {
	rpcClient *rpc.Client
}

// NewInformer returns an initialized Informer.
func NewInformer() *Informer {
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

// GetMetric uses the IPFSConnector the current
// repository size and returns it in a metric.
func (disk *Informer) GetMetric() api.Metric {
	if disk.rpcClient == nil {
		return api.Metric{
			Valid: false,
		}
	}

	var repoSize int
	valid := true
	err := disk.rpcClient.Call("",
		"Cluster",
		"IPFSRepoSize",
		struct{}{},
		&repoSize)
	if err != nil {
		logger.Error(err)
		valid = false
	}

	m := api.Metric{
		Name:  MetricName,
		Value: fmt.Sprintf("%d", repoSize),
		Valid: valid,
	}

	m.SetTTL(MetricTTL)
	return m
}
