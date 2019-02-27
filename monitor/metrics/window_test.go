package metrics

import (
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
)

func makeMetric(value string) *api.Metric {
	metr := &api.Metric{
		Name:  "test",
		Peer:  "peer1",
		Value: value,
		Valid: true,
	}
	metr.SetTTL(5 * time.Second)
	return metr
}

func TestMetricsWindow(t *testing.T) {
	mw := NewWindow(4)

	_, err := mw.Latest()
	if err != ErrNoMetrics {
		t.Error("expected ErrNoMetrics")
	}

	if len(mw.All()) != 0 {
		t.Error("expected 0 metrics")
	}

	mw.Add(makeMetric("1"))

	metr2, err := mw.Latest()
	if err != nil {
		t.Fatal(err)
	}

	if metr2.Value != "1" {
		t.Error("expected different value")
	}

	mw.Add(makeMetric("2"))
	mw.Add(makeMetric("3"))

	all := mw.All()
	if len(all) != 3 {
		t.Fatal("should only be storing 3 metrics")
	}

	if all[0].Value != "3" {
		t.Error("newest metric should be first")
	}

	if all[1].Value != "2" {
		t.Error("older metric should be second")
	}

	mw.Add(makeMetric("4"))
	mw.Add(makeMetric("5"))

	all = mw.All()
	if len(all) != 4 {
		t.Fatal("should only be storing 4 metrics")
	}

	if all[len(all)-1].Value != "2" {
		t.Error("oldest metric should be 2")
	}
}
