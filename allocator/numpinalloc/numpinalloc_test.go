package numpinalloc

import (
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/informer/numpin"

	peer "gx/ipfs/QmZcUPvPhD1Xvk6mwijYF8AfR3mG31S1YsEfHG4khrFPRr/go-libp2p-peer"
	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

type testcase struct {
	candidates map[peer.ID]api.Metric
	current    map[peer.ID]api.Metric
	expected   []peer.ID
}

var (
	peer0      = peer.ID("QmUQ6Nsejt1SuZAu8yL8WgqQZHHAYreLVYYa4VPsLUCed7")
	peer1      = peer.ID("QmUZ13osndQ5uL4tPWHXe3iBgBgq9gfewcBMSCAuMBsDJ6")
	peer2      = peer.ID("QmPrSBATWGAN56fiiEWEhKX3L1F3mTghEQR7vQwaeo7zHi")
	peer3      = peer.ID("QmPGDFvBkgWhvzEK9qaTWrWurSwqXNmhnK3hgELPdZZNPa")
	testCid, _ = cid.Decode("QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq")
)

var inAMinute = time.Now().Add(time.Minute).Format(time.RFC1123)

var testCases = []testcase{
	{ // regular sort
		candidates: map[peer.ID]api.Metric{
			peer0: api.Metric{
				Name:   numpin.MetricName,
				Value:  "5",
				Expire: inAMinute,
				Valid:  true,
			},
			peer1: api.Metric{
				Name:   numpin.MetricName,
				Value:  "1",
				Expire: inAMinute,
				Valid:  true,
			},
			peer2: api.Metric{
				Name:   numpin.MetricName,
				Value:  "3",
				Expire: inAMinute,
				Valid:  true,
			},
			peer3: api.Metric{
				Name:   numpin.MetricName,
				Value:  "2",
				Expire: inAMinute,
				Valid:  true,
			},
		},
		current:  map[peer.ID]api.Metric{},
		expected: []peer.ID{peer1, peer3, peer2, peer0},
	},
	{ // filter invalid
		candidates: map[peer.ID]api.Metric{
			peer0: api.Metric{
				Name:   numpin.MetricName,
				Value:  "1",
				Expire: inAMinute,
				Valid:  false,
			},
			peer1: api.Metric{
				Name:   numpin.MetricName,
				Value:  "5",
				Expire: inAMinute,
				Valid:  true,
			},
		},
		current:  map[peer.ID]api.Metric{},
		expected: []peer.ID{peer1},
	},
	{ // filter bad metric name
		candidates: map[peer.ID]api.Metric{
			peer0: api.Metric{
				Name:   "lalala",
				Value:  "1",
				Expire: inAMinute,
				Valid:  true,
			},
			peer1: api.Metric{
				Name:   numpin.MetricName,
				Value:  "5",
				Expire: inAMinute,
				Valid:  true,
			},
		},
		current:  map[peer.ID]api.Metric{},
		expected: []peer.ID{peer1},
	},
	{ // filter bad value
		candidates: map[peer.ID]api.Metric{
			peer0: api.Metric{
				Name:   numpin.MetricName,
				Value:  "abc",
				Expire: inAMinute,
				Valid:  true,
			},
			peer1: api.Metric{
				Name:   numpin.MetricName,
				Value:  "5",
				Expire: inAMinute,
				Valid:  true,
			},
		},
		current:  map[peer.ID]api.Metric{},
		expected: []peer.ID{peer1},
	},
}

func Test(t *testing.T) {
	alloc := &Allocator{}
	for i, tc := range testCases {
		t.Logf("Test case %d", i)
		res, err := alloc.Allocate(testCid, tc.current, tc.candidates)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) == 0 {
			t.Fatal("0 allocations")
		}
		for i, r := range res {
			if e := tc.expected[i]; r != e {
				t.Errorf("Expect r[%d]=%s but got %s", i, r, e)
			}
		}
	}
}
