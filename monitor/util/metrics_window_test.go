package util

import (
	"testing"

	"github.com/ipfs/ipfs-cluster/api"
)

func TestMetricsWindow(t *testing.T) {
	mw := NewMetricsWindow(4)

	_, err := mw.Latest()
	if err != ErrNoMetrics {
		t.Error("expected ErrNoMetrics")
	}

	if len(mw.All()) != 0 {
		t.Error("expected 0 metrics")
	}

	metr := api.Metric{
		Name:  "test",
		Peer:  "peer1",
		Value: "1",
		Valid: true,
	}
	metr.SetTTL(5)

	mw.Add(metr)

	metr2, err := mw.Latest()
	if err != nil {
		t.Fatal(err)
	}

	if metr2.Value != "1" {
		t.Error("expected different value")
	}

	metr.Value = "2"
	mw.Add(metr)
	metr.Value = "3"
	mw.Add(metr)

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

	metr.Value = "4"
	mw.Add(metr)
	metr.Value = "5"
	mw.Add(metr)

	all = mw.All()
	if len(all) != 4 {
		t.Fatal("should only be storing 4 metrics")
	}

	if all[len(all)-1].Value != "2" {
		t.Error("oldest metric should be 2")
	}
}
