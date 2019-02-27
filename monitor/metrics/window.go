// Package metrics provides common functionality for working with metrics,
// particulary useful for monitoring components. It includes types to store,
// check and filter metrics.
package metrics

import (
	"errors"

	"github.com/ipfs/ipfs-cluster/api"
)

// DefaultWindowCap sets the amount of metrics to store per peer.
var DefaultWindowCap = 25

// ErrNoMetrics is returned when there are no metrics in a Window.
var ErrNoMetrics = errors.New("no metrics have been added to this window")

// Window implements a circular queue to store metrics.
type Window struct {
	last   int
	window []*api.Metric
}

// NewWindow creates an instance with the given
// window capacity.
func NewWindow(windowCap int) *Window {
	if windowCap <= 0 {
		panic("invalid windowCap")
	}

	w := make([]*api.Metric, 0, windowCap)
	return &Window{
		last:   0,
		window: w,
	}
}

// Add adds a new metric to the window. If the window capacity
// has been reached, the oldest metric (by the time it was added),
// will be discarded.
func (mw *Window) Add(m *api.Metric) {
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
func (mw *Window) Latest() (*api.Metric, error) {
	if len(mw.window) == 0 {
		return nil, ErrNoMetrics
	}
	return mw.window[mw.last], nil
}

// All returns all the metrics in the window, in the inverse order
// they were Added. That is, result[0] will be the last added
// metric.
func (mw *Window) All() []*api.Metric {
	wlen := len(mw.window)
	res := make([]*api.Metric, 0, wlen)
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
