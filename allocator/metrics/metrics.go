// Package metrics implements an allocator that can sort allocations
// based on multiple metrics, where metrics may be an arbitrary way to
// partition a set of peers.
//
// For example, allocating by ["tag:region", "disk"] will
// first order candidate peers by tag metric, and then by "disk" metric.
// The final list will pick up allocations from each tag metric group.
// based on the given order of metrics.
package metrics

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/ipfs-cluster/api"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"
)

// Allocator is an allocator that partitions metrics and orders
// the final list of allocation by selecting for each partition.
type Allocator struct {
	config    *Config
	rpcClient *rpc.Client
}

// New returns an initialized Allocator.
func New(cfg *Config) (*Allocator, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	return &Allocator{
		config: cfg,
	}, nil
}

// SetClient provides us with an rpc.Client which allows
// contacting other components in the cluster.
func (a *Allocator) SetClient(c *rpc.Client) {
	a.rpcClient = c
}

// Shutdown is called on cluster shutdown. We just invalidate
// any metrics from this point.
func (a *Allocator) Shutdown(ctx context.Context) error {
	a.rpcClient = nil
	return nil
}

type partitionedMetric struct {
	metricName       string
	curChoosingIndex int
	noMore           bool
	partitions       []*partition // they are in order of their values
}

// Returns a list of peers sorted by never choosing twice from the same
// partition if there is some other partition to choose from.
func (pnedm *partitionedMetric) sortedPeers() []peer.ID {
	peers := []peer.ID{}
	for {
		peer := pnedm.chooseNext()
		if peer == "" { // This means we are done.
			break
		}
		peers = append(peers, peer)
	}
	return peers
}

func (pnedm *partitionedMetric) chooseNext() peer.ID {
	lenp := len(pnedm.partitions)
	if lenp == 0 {
		return ""
	}

	if pnedm.noMore {
		return ""
	}

	var peer peer.ID

	curPartition := pnedm.partitions[pnedm.curChoosingIndex]
	done := 0
	for {
		if curPartition.sub != nil {
			// Choose something from the sub-partitionedMetric
			peer = curPartition.sub.chooseNext()
		} else {
			// We are a bottom-partition. Choose one of our peers
			for pid, used := range curPartition.peers {
				if !used {
					peer = pid
					curPartition.peers[pid] = true // mark as used
					break
				}
			}
		}
		// look in next partition next time
		pnedm.curChoosingIndex = (pnedm.curChoosingIndex + 1) % lenp
		curPartition = pnedm.partitions[pnedm.curChoosingIndex]
		done++

		if peer != "" {
			break
		}

		// no peer and we have looked in as many partitions as we have
		if done == lenp {
			pnedm.noMore = true
			break
		}
	}

	return peer
}

type partition struct {
	value string
	peers map[peer.ID]bool   // the bool tracks whether the peer has been picked already out of the partition when doing the final sort.
	sub   *partitionedMetric // all peers in sub-partitions will have the same value for this metric
}

func partitionMetrics(sortedSet api.MetricsSet, by []string) *partitionedMetric {
	rootMetric := by[0]
	informer := informers[rootMetric]
	pnedMetric := &partitionedMetric{
		metricName: rootMetric,
		partitions: partitionValues(sortedSet[rootMetric], informer),
	}
	if len(by) == 1 { // we are done
		return pnedMetric
	}

	// process sub-partitions
	for _, partition := range pnedMetric.partitions {
		filteredSet := make(api.MetricsSet)
		for k, v := range sortedSet {
			if k == rootMetric { // not needed anymore
				continue
			}
			for _, m := range v {
				// only leave metrics for peers in current partition
				if _, ok := partition.peers[m.Peer]; ok {
					filteredSet[k] = append(filteredSet[k], m)
				}
			}
		}

		partition.sub = partitionMetrics(filteredSet, by[1:])
	}
	return pnedMetric
}

func partitionValues(sortedMetrics []*api.Metric, inf informer) []*partition {
	partitions := []*partition{}

	if len(sortedMetrics) <= 0 {
		return partitions
	}

	// For not partitionable metrics we create one partition
	// per value, even if two values are the same.
	groupable := inf.partitionable

	curPartition := &partition{
		value: sortedMetrics[0].Value,
		peers: map[peer.ID]bool{
			sortedMetrics[0].Peer: false,
		},
	}
	partitions = append(partitions, curPartition)

	for _, m := range sortedMetrics[1:] {
		if groupable && m.Value == curPartition.value {
			curPartition.peers[m.Peer] = false
		} else {
			curPartition = &partition{
				value: m.Value,
				peers: map[peer.ID]bool{
					m.Peer: false,
				},
			}
			partitions = append(partitions, curPartition)
		}
	}
	return partitions
}

// Allocate produces a sorted list of cluster peer IDs based on different
// metrics provided for those peer IDs.
// It works as follows:
//
//   - First, it buckets each peer metrics based on the AllocateBy list. The
//   metric name must match the bucket name, otherwise they are put at the end.
//   - Second, based on the AllocateBy order, it orders the first bucket and
//   groups peers by ordered value.
//   - Third, it selects metrics on the second bucket for the most prioritary
//   peers of the first bucket and orders their metrics. Then for the peers in
//   second position etc.
//   - It repeats the process until there is no more buckets to sort.
//   - Finally, it returns the first peer of the first
//   - Third, based on the AllocateBy order, it select the first metric
func (a *Allocator) Allocate(
	ctx context.Context,
	c cid.Cid,
	current, candidates, priority api.MetricsSet,
) ([]peer.ID, error) {

	// sort all metrics. TODO: it should not remove invalids.
	for _, arg := range []api.MetricsSet{current, candidates, priority} {
		if arg == nil {
			continue
		}
		for _, by := range a.config.AllocateBy {
			sorter := informers[by].sorter
			if sorter == nil {
				return nil, fmt.Errorf("allocate_by contains an unknown metric name: %s", by)
			}
			arg[by] = sorter(arg[by])
		}
	}

	// For the allocation to work well, there have to be metrics of all
	// the types for all the peers. There cannot be a metric of one type
	// for a peer that does not appear in the other types.
	//
	// Removing such occurences is done in allocate.go, before the
	// allocator is called.
	//
	// Otherwise, the sorting might be funny.

	candidatePartition := partitionMetrics(candidates, a.config.AllocateBy)
	priorityPartition := partitionMetrics(priority, a.config.AllocateBy)

	//fmt.Println("---")
	//printPartition(candidatePartition)

	first := priorityPartition.sortedPeers()
	last := candidatePartition.sortedPeers()

	return append(first, last...), nil
}

// Metrics returns the names of the metrics that have been registered
// with this allocator.
func (a *Allocator) Metrics() []string {
	return a.config.AllocateBy
}

// func printPartition(p *partitionedMetric) {
// 	fmt.Println(p.metricName)
// 	for _, p := range p.partitions {
// 		fmt.Printf("%s - [", p.value)
// 		for p, u := range p.peers {
// 			fmt.Printf("%s|%t, ", p, u)
// 		}
// 		fmt.Println("]")
// 		if p.sub != nil {
// 			printPartition(p.sub)
// 		}
// 	}
// }
