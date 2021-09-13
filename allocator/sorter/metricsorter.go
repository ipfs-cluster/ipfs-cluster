// Package sorter is a utility package used by the allocator
// implementations. This package provides the sort function for metrics of
// different value types.
package sorter

import (
	"sort"
	"strconv"

	"github.com/ipfs/ipfs-cluster/api"
)

// SortNumeric sorts a list of metrics of Uint values from smallest to
// largest. Invalid or non-numeric metrics are discarded.
func SortNumeric(metrics []*api.Metric) []*api.Metric {
	return sortNumeric(metrics, false)
}

// SortNumericReverse sorts a list of metrics of Uint values from largest to
// smallest. Invalid or non-numeric metrics are discarded.
func SortNumericReverse(metrics []*api.Metric) []*api.Metric {
	return sortNumeric(metrics, true)
}

func sortNumeric(metrics []*api.Metric, reverse bool) []*api.Metric {
	filteredMetrics := make([]*api.Metric, 0, len(metrics))
	values := make([]uint64, 0, len(metrics))

	// Parse and discard
	for _, m := range metrics {
		if m.Discard() {
			continue
		}
		val, err := strconv.ParseUint(m.Value, 10, 64)
		if err != nil {
			continue
		}
		filteredMetrics = append(filteredMetrics, m)
		values = append(values, val)
	}

	sorter := &numericSorter{
		values:  values,
		metrics: filteredMetrics,
		reverse: reverse,
	}
	sort.Sort(sorter)
	return sorter.metrics
}

// SortText sorts a list of metrics of string values.
// Invalid metrics are discarded.
func SortText(metrics []*api.Metric) []*api.Metric {
	filteredMetrics := make([]*api.Metric, 0, len(metrics))

	// Parse and discard
	for _, m := range metrics {
		if m.Discard() {
			continue
		}
		filteredMetrics = append(filteredMetrics, m)
	}

	sorter := &textSorter{
		metrics: filteredMetrics,
		reverse: false,
	}
	sort.Sort(sorter)
	return sorter.metrics
}

// numericSorter implements the sort.Sort interface
type numericSorter struct {
	values  []uint64
	metrics []*api.Metric
	reverse bool
}

func (s numericSorter) Len() int {
	return len(s.values)
}

func (s numericSorter) Swap(i, j int) {
	tempVal := s.values[i]
	s.values[i] = s.values[j]
	s.values[j] = tempVal

	tempMetric := s.metrics[i]
	s.metrics[i] = s.metrics[j]
	s.metrics[j] = tempMetric
}

// We make it a strict order by ordering by peer ID.
func (s numericSorter) Less(i, j int) bool {
	if s.values[i] == s.values[j] {
		if s.reverse {
			return string(s.metrics[i].Peer) > string(s.metrics[j].Peer)
		}
		return string(s.metrics[i].Peer) < string(s.metrics[j].Peer)
	}

	if s.reverse {
		return s.values[i] > s.values[j]
	}
	return s.values[i] < s.values[j]
}

// textSorter implements the sort.Sort interface
type textSorter struct {
	metrics []*api.Metric
	reverse bool
}

func (s textSorter) Len() int {
	return len(s.metrics)
}

func (s textSorter) Swap(i, j int) {
	tempMetric := s.metrics[i]
	s.metrics[i] = s.metrics[j]
	s.metrics[j] = tempMetric
}

// We make it a strict order by ordering by peer ID.
func (s textSorter) Less(i, j int) bool {
	if s.metrics[i].Value == s.metrics[j].Value {
		if s.reverse {
			return string(s.metrics[i].Peer) > string(s.metrics[j].Peer)
		}
		return string(s.metrics[i].Peer) < string(s.metrics[j].Peer)
	}

	if s.reverse {
		return s.metrics[i].Value > s.metrics[j].Value
	}
	return s.metrics[i].Value < s.metrics[j].Value
}
