// Package util provides common functionality for monitoring components.
package util

import (
	"errors"
	"sync"

	"github.com/ipfs/ipfs-cluster/api"
)

// ErrNoMetrics is returned when there are no metrics in a MetricsWindow.
var ErrNoMetrics = errors.New("no metrics have been added to this window")

// MetricsWindow implements a circular queue to store metrics.
type MetricsWindow struct {
	last int

	safe       bool
	windowLock sync.RWMutex
	window     []api.Metric
}

// NewMetricsWindow creates an instance with the given
// window capacity. The safe indicates whether we use a lock
// for concurrent operations.
func NewMetricsWindow(windowCap int, safe bool) *MetricsWindow {
	w := make([]api.Metric, 0, windowCap)
	return &MetricsWindow{
		last:   0,
		safe:   safe,
		window: w,
	}
}

// Add adds a new metric to the window. If the window capacity
// has been reached, the oldest metric (by the time it was added),
// will be discarded.
func (mw *MetricsWindow) Add(m api.Metric) {
	if mw.safe {
		mw.windowLock.Lock()
		defer mw.windowLock.Unlock()
	}
	if len(mw.window) < cap(mw.window) {
		mw.window = append(mw.window, m)
		mw.last = len(mw.window) - 1
		return
	}

	// len == cap
	mw.last = (mw.last + 1) % cap(mw.window)
	mw.window[mw.last] = m
	return
}

// Latest returns the last metric added. It returns an error
// if no metrics were added.
func (mw *MetricsWindow) Latest() (api.Metric, error) {
	if mw.safe {
		mw.windowLock.Lock()
		defer mw.windowLock.Unlock()
	}
	if len(mw.window) == 0 {
		return api.Metric{}, ErrNoMetrics
	}
	return mw.window[mw.last], nil
}

// All returns all the metrics in the window, in the inverse order
// they were Added. That is, result[0] will be the last added
// metric.
func (mw *MetricsWindow) All() []api.Metric {
	if mw.safe {
		mw.windowLock.Lock()
		defer mw.windowLock.Unlock()
	}
	wlen := len(mw.window)
	res := make([]api.Metric, 0, wlen)
	if wlen == 0 {
		return res
	}
	for i := mw.last; i >= 0; i-- {
		res = append(res, mw.window[i])
	}
	for i := wlen - 1; i > mw.last; i-- {
		res = append(res, mw.window[i])
	}
	return res
}
