package balanced

import "github.com/ipfs/ipfs-cluster/api"

type informer struct {
	sorter        func([]*api.Metric) []*api.Metric
	partitionable bool
}

var informers map[string]informer

// Registers an informer with this allocator so that we know how to
// sort and use metrics coming from it.
func RegisterInformer(
	name string,
	sorter func([]*api.Metric) []*api.Metric,
	partitionable bool,
) {
	if informers == nil {
		informers = make(map[string]informer)
	}
	informers[name] = informer{
		sorter:        sorter,
		partitionable: partitionable,
	}
}
