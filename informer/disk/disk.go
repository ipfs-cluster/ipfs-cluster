// Package disk implements an ipfs-cluster informer which uses a metric (e.g.
// RepoSize or FreeSpace of the IPFS daemon datastore) and returns it as an
// api.Metric. The supported metrics are listed as the keys in the metricToRPC
// map below.
package disk

import (
	"fmt"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	logging "github.com/ipfs/go-log"

	"github.com/ipfs/ipfs-cluster/api"
)

type MetricType int

const (
	MetricFreeSpace = iota
	MetricRepoSize
)

const DefaultMetric = MetricFreeSpace

var logger = logging.Logger("diskinfo")

// MetricTTL specifies how long our reported metric is valid in seconds.
var MetricTTL = 30

// metricToRPC maps from a specified metric name to the corrresponding RPC call
var metricToRPC = map[MetricType]string{
	MetricFreeSpace: "IPFSFreeSpace",
	MetricRepoSize:  "IPFSRepoSize",
}

// Informer is a simple object to implement the ipfscluster.Informer
// and Component interfaces.
type Informer struct {
	Type      MetricType
	name      string
	rpcClient *rpc.Client
	rpcName   string
}

// NewInformer returns an initialized Informer that uses the DefaultMetric. The
// name argument is meant as a user-facing identifier for the Informer and can
// be anything.
func NewInformer(name string) *Informer {
	return &Informer{
		name:    name,
		rpcName: metricToRPC[DefaultMetric],
	}
}

// NewInformerWithMetric returns an Informer that uses the input MetricType. The
// name argument has the same purpose as in NewInformer.
func NewInformerWithMetric(metric MetricType, name string) *Informer {
	// check whether specified metric is supported
	if rpc, valid := metricToRPC[metric]; valid {
		return &Informer{
			name:    name,
			rpcName: rpc,
		}
	}
	return nil
}

// Name returns the user-facing name of this informer.
func (disk *Informer) Name() string {
	return disk.name
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
		disk.rpcName,
		struct{}{},
		&metric)
	if err != nil {
		logger.Error(err)
		valid = false
	}

	m := api.Metric{
		Name:  disk.name,
		Value: fmt.Sprintf("%d", metric),
		Valid: valid,
	}

	m.SetTTL(MetricTTL)
	return m
}
