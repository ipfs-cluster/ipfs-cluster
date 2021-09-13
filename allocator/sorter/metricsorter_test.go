package sorter

import (
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
)

func TestSortNumeric(t *testing.T) {
	metrics := []*api.Metric{
		&api.Metric{
			Name:  "num",
			Value: "5",
			Valid: true,
		},
		&api.Metric{
			Name:  "num",
			Value: "0",
			Valid: true,
		},
		&api.Metric{
			Name:  "num",
			Value: "-1",
			Valid: true,
		},
		&api.Metric{
			Name:  "num3",
			Value: "abc",
			Valid: true,
		},
		&api.Metric{
			Name:  "num2",
			Value: "10",
			Valid: true,
		},
		&api.Metric{
			Name:  "num2",
			Value: "4",
			Valid: true,
		},
	}

	for _, m := range metrics {
		m.SetTTL(time.Minute)
	}
	metrics[4].Expire = 0 // manually expire

	sorted := SortNumeric(metrics)
	if len(sorted) != 3 {
		t.Fatal("sorter did not remove invalid metrics:")
	}
	if sorted[0].Value != "0" ||
		sorted[1].Value != "4" ||
		sorted[2].Value != "5" {
		t.Error("not sorted properly")
	}

	sortedRev := SortNumericReverse(metrics)
	if len(sortedRev) != 3 {
		t.Fatal("sorted did not remove invalid metrics")
	}

	if sortedRev[0].Value != "5" ||
		sortedRev[1].Value != "4" ||
		sortedRev[2].Value != "0" {
		t.Error("not sorted properly")
	}

}
