package balanced

import (
	"context"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/allocator/sorter"
	api "github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
	peer "github.com/libp2p/go-libp2p-core/peer"
)

func makeMetric(name, value string, peer peer.ID) *api.Metric {
	return &api.Metric{
		Name:   name,
		Value:  value,
		Peer:   peer,
		Valid:  true,
		Expire: time.Now().Add(time.Minute).UnixNano(),
	}
}

func TestAllocate(t *testing.T) {
	RegisterInformer("region", sorter.SortText, true)
	RegisterInformer("az", sorter.SortText, true)
	RegisterInformer("freespace", sorter.SortNumericReverse, false)

	alloc, err := New(&Config{
		AllocateBy: []string{
			"region",
			"az",
			"freespace",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	candidates := api.MetricsSet{
		"abc": []*api.Metric{ // don't want anything in results
			makeMetric("abc", "a", test.PeerID1),
			makeMetric("abc", "b", test.PeerID2),
		},
		"region": []*api.Metric{
			makeMetric("region", "a-us", test.PeerID1),
			makeMetric("region", "a-us", test.PeerID2),

			makeMetric("region", "b-eu", test.PeerID3),
			makeMetric("region", "b-eu", test.PeerID4),
			makeMetric("region", "b-eu", test.PeerID5),

			makeMetric("region", "c-au", test.PeerID6),
			makeMetric("region", "c-au", test.PeerID7),
			makeMetric("region", "c-au", test.PeerID8), // I don't want to see this in results
		},
		"az": []*api.Metric{
			makeMetric("az", "us1", test.PeerID1),
			makeMetric("az", "us2", test.PeerID2),

			makeMetric("az", "eu1", test.PeerID3),
			makeMetric("az", "eu1", test.PeerID4),
			makeMetric("az", "eu2", test.PeerID5),

			makeMetric("az", "au1", test.PeerID6),
			makeMetric("az", "au1", test.PeerID7),
		},
		"freespace": []*api.Metric{
			makeMetric("freespace", "100", test.PeerID1),
			makeMetric("freespace", "500", test.PeerID2),

			makeMetric("freespace", "200", test.PeerID3),
			makeMetric("freespace", "400", test.PeerID4),
			makeMetric("freespace", "10", test.PeerID5),

			makeMetric("freespace", "50", test.PeerID6),
			makeMetric("freespace", "600", test.PeerID7),

			makeMetric("freespace", "10000", test.PeerID8),
		},
	}

	// Based on the algorithm it should choose:
	//
	// - For the region us, az us1, it should select the peer with most
	// freespace: ID2
	// - Then switch to next region: ID4
	// - And so on:
	// - us-us1-100 ID1 - only peer in az
	// - eu-eu1-400 ID4 - over ID3
	// - au-au1-600 ID7 - over ID6
	// - us-us2-500 ID2 - only peer in az
	// - eu-eu2-10 ID5  - only peer in az
	// - au-au1-50 ID6  - id7 already used
	// - // no more in us
	// - eu-eu1-ID3     - ID4 already used
	// - // no more peers in au
	// - // no more peers

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
			if p != test.PeerID1 {
				t.Errorf("wrong id in pos %d: %s", i, p)
			}
		case 1:
			if p != test.PeerID4 {
				t.Errorf("wrong id in pos %d: %s", i, p)
			}
		case 2:
			if p != test.PeerID7 {
				t.Errorf("wrong id in pos %d: %s", i, p)
			}
		case 3:
			if p != test.PeerID2 {
				t.Errorf("wrong id in pos %d: %s", i, p)
			}
		case 4:
			if p != test.PeerID5 {
				t.Errorf("wrong id in pos %d: %s", i, p)
			}
		case 5:
			if p != test.PeerID6 {
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
