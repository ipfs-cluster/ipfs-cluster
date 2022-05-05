package balanced

import (
	"context"
	"testing"
	"time"

	api "github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
	peer "github.com/libp2p/go-libp2p-core/peer"
)

func makeMetric(name, value string, weight int64, peer peer.ID, partitionable bool) api.Metric {
	return api.Metric{
		Name:          name,
		Value:         value,
		Weight:        weight,
		Peer:          peer,
		Valid:         true,
		Partitionable: partitionable,
		Expire:        time.Now().Add(time.Minute).UnixNano(),
	}
}

func TestAllocate(t *testing.T) {
	alloc, err := New(&Config{
		AllocateBy: []string{
			"region",
			"az",
			"pinqueue",
			"freespace",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	candidates := api.MetricsSet{
		"abc": []api.Metric{ // don't want anything in results
			makeMetric("abc", "a", 0, test.PeerID1, true),
			makeMetric("abc", "b", 0, test.PeerID2, true),
		},
		"region": []api.Metric{
			makeMetric("region", "a-us", 0, test.PeerID1, true),
			makeMetric("region", "a-us", 0, test.PeerID2, true),

			makeMetric("region", "b-eu", 0, test.PeerID3, true),
			makeMetric("region", "b-eu", 0, test.PeerID4, true),
			makeMetric("region", "b-eu", 0, test.PeerID5, true),

			makeMetric("region", "c-au", 0, test.PeerID6, true),
			makeMetric("region", "c-au", 0, test.PeerID7, true),
			makeMetric("region", "c-au", 0, test.PeerID8, true), // I don't want to see this in results
		},
		"az": []api.Metric{
			makeMetric("az", "us1", 0, test.PeerID1, true),
			makeMetric("az", "us2", 0, test.PeerID2, true),

			makeMetric("az", "eu1", 0, test.PeerID3, true),
			makeMetric("az", "eu1", 0, test.PeerID4, true),
			makeMetric("az", "eu2", 0, test.PeerID5, true),

			makeMetric("az", "au1", 0, test.PeerID6, true),
			makeMetric("az", "au1", 0, test.PeerID7, true),
		},
		"pinqueue": []api.Metric{
			makeMetric("pinqueue", "100", 0, test.PeerID1, true),
			makeMetric("pinqueue", "200", 0, test.PeerID2, true),

			makeMetric("pinqueue", "100", 0, test.PeerID3, true),
			makeMetric("pinqueue", "200", 0, test.PeerID4, true),
			makeMetric("pinqueue", "300", 0, test.PeerID5, true),

			makeMetric("pinqueue", "100", 0, test.PeerID6, true),
			makeMetric("pinqueue", "1000", -1, test.PeerID7, true),
		},
		"freespace": []api.Metric{
			makeMetric("freespace", "100", 100, test.PeerID1, false),
			makeMetric("freespace", "500", 500, test.PeerID2, false),

			makeMetric("freespace", "200", 200, test.PeerID3, false),
			makeMetric("freespace", "400", 400, test.PeerID4, false),
			makeMetric("freespace", "10", 10, test.PeerID5, false),

			makeMetric("freespace", "50", 50, test.PeerID6, false),
			makeMetric("freespace", "600", 600, test.PeerID7, false),

			makeMetric("freespace", "10000", 10000, test.PeerID8, false),
		},
	}

	// Regions weights: a-us (pids 1,2): 600. b-eu (pids 3,4,5): 610. c-au (pids 6,7): 649
	// Az weights: us1: 100. us2: 500. eu1: 600. eu2: 10. au1: 649
	// Based on the algorithm it should choose:
	//
	// - c-au (most-weight)->au1->pinqueue(0)->pid6
	// - b-eu->eu1->pid4
	// - a-us->us2->pid2
	// - <repeat regions>
	// - c-au->au1 (nowhere else to choose)->pid7 (region exausted)
	// - b-eu->eu2 (already had in eu1)->pid5
	// - a-us->us1 (already had in us2)->pid1
	// - <repeat regions>
	// - b-eu->eu1->pid3 (only peer left)

	peers, err := alloc.Allocate(context.Background(),
		test.Cid1,
		nil,
		candidates,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(peers) < 7 {
		t.Fatalf("not enough peers: %s", peers)
	}

	for i, p := range peers {
		t.Logf("%d - %s", i, p)
		switch i {
		case 0:
			if p != test.PeerID6 {
				t.Errorf("wrong id in pos %d: %s", i, p)
			}
		case 1:
			if p != test.PeerID4 {
				t.Errorf("wrong id in pos %d: %s", i, p)
			}
		case 2:
			if p != test.PeerID2 {
				t.Errorf("wrong id in pos %d: %s", i, p)
			}
		case 3:
			if p != test.PeerID7 {
				t.Errorf("wrong id in pos %d: %s", i, p)
			}
		case 4:
			if p != test.PeerID5 {
				t.Errorf("wrong id in pos %d: %s", i, p)
			}
		case 5:
			if p != test.PeerID1 {
				t.Errorf("wrong id in pos %d: %s", i, p)
			}
		case 6:
			if p != test.PeerID3 {
				t.Errorf("wrong id in pos %d: %s", i, p)
			}
		default:
			t.Error("too many peers")
		}
	}
}
