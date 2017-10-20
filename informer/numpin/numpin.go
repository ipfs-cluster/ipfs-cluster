// Package numpin implements an ipfs-cluster informer which determines how many
// items this peer is pinning and returns it as api.Metric
package numpin

import (
	"fmt"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"

	"github.com/ipfs/ipfs-cluster/api"
)

// MetricName specifies the name of our metric
var MetricName = "numpin"

// Informer is a simple object to implement the ipfscluster.Informer
// and Component interfaces
type Informer struct {
	config    *Config
	rpcClient *rpc.Client
}

// NewInformer returns an initialized Informer.
func NewInformer(cfg *Config) (*Informer, error) {
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
func (npi *Informer) SetClient(c *rpc.Client) {
	npi.rpcClient = c
}

// Shutdown is called on cluster shutdown. We just invalidate
// any metrics from this point.
func (npi *Informer) Shutdown() error {
	npi.rpcClient = nil
	return nil
}

// Name returns the name of this informer
func (npi *Informer) Name() string {
	return MetricName
}

// GetMetric contacts the IPFSConnector component and
// requests the `pin ls` command. We return the number
// of pins in IPFS.
func (npi *Informer) GetMetric() api.Metric {
	if npi.rpcClient == nil {
		return api.Metric{
			Valid: false,
		}
	}

	pinMap := make(map[string]api.IPFSPinStatus)

	// make use of the RPC API to obtain information
	// about the number of pins in IPFS. See RPCAPI docs.
	err := npi.rpcClient.Call("", // Local call
		"Cluster",   // Service name
		"IPFSPinLs", // Method name
		"recursive", // in arg
		&pinMap)     // out arg

	valid := err == nil

	m := api.Metric{
		Name:  MetricName,
		Value: fmt.Sprintf("%d", len(pinMap)),
		Valid: valid,
	}

	m.SetTTLDuration(npi.config.MetricTTL)
	return m
}
