// Package numpin implements an ipfs-cluster informer which determines how many
// items this peer is pinning and returns it as api.Metric
package numpin

import (
	"context"
	"fmt"
	"sync"

	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/libp2p/go-libp2p-gorpc"

	"go.opencensus.io/trace"
)

// MetricName specifies the name of our metric
var MetricName = "numpin"

// Informer is a simple object to implement the ipfscluster.Informer
// and Component interfaces
type Informer struct {
	config *Config

	mu        sync.Mutex
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
	npi.mu.Lock()
	npi.rpcClient = c
	npi.mu.Unlock()
}

// Shutdown is called on cluster shutdown. We just invalidate
// any metrics from this point.
func (npi *Informer) Shutdown(ctx context.Context) error {
	_, span := trace.StartSpan(ctx, "informer/numpin/Shutdown")
	defer span.End()

	npi.mu.Lock()
	npi.rpcClient = nil
	npi.mu.Unlock()
	return nil
}

// Name returns the name of this informer
func (npi *Informer) Name() string {
	return MetricName
}

// GetMetrics contacts the IPFSConnector component and requests the `pin ls`
// command. We return the number of pins in IPFS. It must always return at
// least one metric.
func (npi *Informer) GetMetrics(ctx context.Context) []api.Metric {
	ctx, span := trace.StartSpan(ctx, "informer/numpin/GetMetric")
	defer span.End()

	npi.mu.Lock()
	rpcClient := npi.rpcClient
	npi.mu.Unlock()

	if rpcClient == nil {
		return []api.Metric{
			{
				Valid: false,
			},
		}
	}

	// make use of the RPC API to obtain information
	// about the number of pins in IPFS. See RPCAPI docs.
	in := make(chan []string, 1)
	in <- []string{"recursive", "direct"}
	close(in)
	out := make(chan api.IPFSPinInfo, 1024)

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		err := rpcClient.Stream(
			ctx,
			"",              // Local call
			"IPFSConnector", // Service name
			"PinLs",         // Method name
			in,
			out,
		)
		errCh <- err
	}()

	n := 0
	for range out {
		n++
	}

	err := <-errCh

	valid := err == nil

	m := api.Metric{
		Name:          MetricName,
		Value:         fmt.Sprintf("%d", n),
		Valid:         valid,
		Partitionable: false,
	}

	m.SetTTL(npi.config.MetricTTL)
	return []api.Metric{m}
}
