// Package balanced implements an allocator that can sort allocations
// based on multiple metrics, where metrics may be an arbitrary way to
// partition a set of peers.
//
// For example, allocating by ["tag:region", "disk"] the resulting peer
// candidate order will balanced between regions and ordered by the value of
// the weight of the disk metric.
package balanced

import (
	"context"
	"fmt"
	"sort"

	api "github.com/ipfs-cluster/ipfs-cluster/api"
	logging "github.com/ipfs/go-log/v2"
	peer "github.com/libp2p/go-libp2p/core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"
)

var logger = logging.Logger("allocator")

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

type partition struct {
	value            string
	weight           int64
	aggregatedWeight int64
	peers            map[peer.ID]bool   // the bool tracks whether the peer has been picked already out of the partition when doing the final sort.
	sub              *partitionedMetric // all peers in sub-partitions will have the same value for this metric
}

// Returns a partitionedMetric which has partitions and subpartitions based
// on the metrics and values given by the "by" slice. The partitions
// are ordered based on the cumulative weight.
func partitionMetrics(set api.MetricsSet, by []string) *partitionedMetric {
	rootMetric := by[0]
	pnedMetric := &partitionedMetric{
		metricName: rootMetric,
		partitions: partitionValues(set[rootMetric]),
	}

	// For sorting based on weight (more to less)
	lessF := func(i, j int) bool {
		wi := pnedMetric.partitions[i].weight
		wj := pnedMetric.partitions[j].weight

		// if weight is equal, sort by aggregated weight of
		// all sub-partitions.
		if wi == wj {
			awi := pnedMetric.partitions[i].aggregatedWeight
			awj := pnedMetric.partitions[j].aggregatedWeight
			// If subpartitions weight the same, do strict order
			// based on value string
			if awi == awj {
				return pnedMetric.partitions[i].value < pnedMetric.partitions[j].value
			}
			return awj < awi

		}
		// Descending!
		return wj < wi
	}

	if len(by) == 1 { // we are done
		sort.Slice(pnedMetric.partitions, lessF)
		return pnedMetric
	}

	// process sub-partitions
	for _, partition := range pnedMetric.partitions {
		filteredSet := make(api.MetricsSet)
		for k, v := range set {
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

		// Add the aggregated weight of the subpartitions
		for _, subp := range partition.sub.partitions {
			partition.aggregatedWeight += subp.aggregatedWeight
		}
	}
	sort.Slice(pnedMetric.partitions, lessF)
	return pnedMetric
}

func partitionValues(metrics []api.Metric) []*partition {
	partitions := []*partition{}

	if len(metrics) <= 0 {
		return partitions
	}

	// We group peers with the same value in the same partition.
	partitionsByValue := make(map[string]*partition)

	for _, m := range metrics {
		// Sometimes two metrics have the same value / weight, but we
		// still want to put them in different partitions. Otherwise
		// their weights get added and they form a bucket and
		// therefore not they are not selected in order: 3 peers with
		// freespace=100 and one peer with freespace=200 would result
		// in one of the peers with freespace 100 being chosen first
		// because the partition's weight is 300.
		//
		// We are going to call these metrics (like free-space),
		// non-partitionable metrics. This is going to be the default
		// (for backwards compat reasons).
		//
		// The informers must set the Partitionable field accordingly
		// when two metrics with the same value must be grouped in the
		// same partition.
		//
		// Note: aggregatedWeight is the same as weight here (sum of
		// weight of all metrics in partitions), and gets updated
		// later in partitionMetrics with the aggregated weight of
		// sub-partitions.
		if !m.Partitionable {
			partitions = append(partitions, &partition{
				value:            m.Value,
				weight:           m.GetWeight(),
				aggregatedWeight: m.GetWeight(),
				peers: map[peer.ID]bool{
					m.Peer: false,
				},
			})
			continue
		}

		// Any other case, we partition by value.
		if p, ok := partitionsByValue[m.Value]; ok {
			p.peers[m.Peer] = false
			p.weight += m.GetWeight()
			p.aggregatedWeight += m.GetWeight()
		} else {
			partitionsByValue[m.Value] = &partition{
				value:            m.Value,
				weight:           m.GetWeight(),
				aggregatedWeight: m.GetWeight(),
				peers: map[peer.ID]bool{
					m.Peer: false,
				},
			}
		}

	}
	for _, p := range partitionsByValue {
		partitions = append(partitions, p)
	}
	return partitions
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
	c api.Cid,
	current, candidates, priority api.MetricsSet,
) ([]peer.ID, error) {

	// For the allocation to work well, there have to be metrics of all
	// the types for all the peers. There cannot be a metric of one type
	// for a peer that does not appear in the other types.
	//
	// Removing such occurrences is done in allocate.go, before the
	// allocator is called.
	//
	// Otherwise, the sorting might be funny.

	candidatePartition := partitionMetrics(candidates, a.config.AllocateBy)
	priorityPartition := partitionMetrics(priority, a.config.AllocateBy)

	logger.Debugf("Balanced allocator partitions:\n%s\n", printPartition(candidatePartition, 0))
	//fmt.Println(printPartition(candidatePartition, 0))

	first := priorityPartition.sortedPeers()
	last := candidatePartition.sortedPeers()

	return append(first, last...), nil
}

// Metrics returns the names of the metrics that have been registered
// with this allocator.
func (a *Allocator) Metrics() []string {
	return a.config.AllocateBy
}

func printPartition(m *partitionedMetric, ind int) string {
	str := ""
	indent := func() {
		for i := 0; i < ind+2; i++ {
			str += " "
		}
	}

	for _, p := range m.partitions {
		indent()
		str += fmt.Sprintf(" | %s:%s - %d - [", m.metricName, p.value, p.weight)
		for p, u := range p.peers {
			str += fmt.Sprintf("%s|%t, ", p, u)
		}
		str += "]\n"
		if p.sub != nil {
			str += printPartition(p.sub, ind+2)
		}
	}
	return str
}
