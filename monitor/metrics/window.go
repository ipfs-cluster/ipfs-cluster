// Package metrics provides common functionality for working with metrics,
// particulary useful for monitoring components. It includes types to store,
// check and filter metrics.
package metrics

import (
	"container/ring"
	"errors"
	"sync"
	"time"

	logging "github.com/ipfs/go-log"
	"github.com/ipfs/ipfs-cluster/api"
)

var logger = logging.Logger("metricwin")

// DefaultWindowCap sets the amount of metrics to store per peer.
var DefaultWindowCap = 25

// ErrNoMetrics is returned when there are no metrics in a Window.
var ErrNoMetrics = errors.New("no metrics have been added to this window")

// Window implements a circular queue to store metrics.
type Window struct {
	lMu  sync.RWMutex
	last *api.Metric

	wMu    sync.RWMutex
	window *ring.Ring
}

// NewWindow creates an instance with the given
// window capacity.
func NewWindow(windowCap int) *Window {
	if windowCap <= 0 {
		panic("invalid windowCap")
	}

	w := ring.New(windowCap)
	return &Window{
		last:   nil,
		window: w,
	}
}

// Add adds a new metric to the window. If the window capacity
// has been reached, the oldest metric (by the time it was added),
// will be discarded. Add leaves the cursor on the next spot,
// which is either empty or the oldest record.
func (mw *Window) Add(m *api.Metric) {
	m.TS = time.Now().UnixNano()

	mw.wMu.Lock()
	mw.window.Value = m
	mw.window = mw.window.Next()
	mw.wMu.Unlock()

	mw.lMu.Lock()
	mw.last = m
	mw.lMu.Unlock()

	return
}

// Latest returns the last metric added. It returns an error
// if no metrics were added.
func (mw *Window) Latest() (*api.Metric, error) {
	mw.lMu.RLock()
	if mw.last == nil {
		return nil, ErrNoMetrics
	}
	last := mw.last
	mw.lMu.RUnlock()
	return last, nil
}

// All returns all the metrics in the window, in the inverse order
// they were Added. That is, result[0] will be the last added
// metric.
func (mw *Window) All() []*api.Metric {
	mw.wMu.RLock()
	values := make([]*api.Metric, 0, mw.window.Len())
	mw.window.Do(func(v interface{}) {
		if i, ok := v.(*api.Metric); ok {
			// append younger values to older value
			values = append([]*api.Metric{i}, values...)
		}
	})
	mw.wMu.RUnlock()
	return values
}

// Distribution returns the deltas between all the current
// values contained in the current window. This will
// only return values if the api.Metric.Type() is "ping",
// which are used for accural failure detection.
func (mw *Window) Distribution() []int64 {
	ms := mw.All()
	dist := make([]int64, 0, len(ms)-1)
	for i, v := range ms {
		// the last value can't be used to calculate a delta
		if i == len(ms)-1 {
			break
		}
		// All() provides an order slice, where ms[i] is younger than ms[i+1]
		delta := v.TS - ms[i+1].TS
		dist = append(dist, delta)
	}

	return dist
}
