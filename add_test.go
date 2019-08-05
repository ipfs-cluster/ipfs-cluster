package ipfscluster

// This files has tests for Add* using multiple cluster peers.

import (
	"context"
	"mime/multipart"
	"sync"
	"testing"
	"time"

	cid "github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"
)

func TestAdd(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	waitForLeaderAndMetrics(t, clusters)

	t.Run("local", func(t *testing.T) {
		params := api.DefaultAddParams()
		params.Shard = false
		params.Name = "testlocal"
		mfr, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		r := multipart.NewReader(mfr, mfr.Boundary())
		ci, err := clusters[0].AddFile(r, params)
		if err != nil {
			t.Fatal(err)
		}
		if ci.String() != test.ShardingDirBalancedRootCID {
			t.Fatal("unexpected root CID for local add")
		}

		// We need to sleep a lot because it takes time to
		// catch up on a first/single pin on crdts
		time.Sleep(10 * time.Second)

		f := func(t *testing.T, c *Cluster) {
			pin := c.StatusLocal(ctx, ci)
			if pin.Error != "" {
				t.Error(pin.Error)
			}
			if pin.Status != api.TrackerStatusPinned {
				t.Error("item should be pinned and is", pin.Status)
			}
		}

		runF(t, clusters, f)
	})
}

func TestAddPeerDown(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)
	err := clusters[0].Shutdown(ctx)
	if err != nil {
		t.Fatal(err)
	}

	waitForLeaderAndMetrics(t, clusters)

	t.Run("local", func(t *testing.T) {
		params := api.DefaultAddParams()
		params.Shard = false
		params.Name = "testlocal"
		mfr, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		r := multipart.NewReader(mfr, mfr.Boundary())
		ci, err := clusters[1].AddFile(r, params)
		if err != nil {
			t.Fatal(err)
		}
		if ci.String() != test.ShardingDirBalancedRootCID {
			t.Fatal("unexpected root CID for local add")
		}

		// We need to sleep a lot because it takes time to
		// catch up on a first/single pin on crdts
		time.Sleep(10 * time.Second)

		f := func(t *testing.T, c *Cluster) {
			if c.id == clusters[0].id {
				return
			}
			pin := c.StatusLocal(ctx, ci)
			if pin.Error != "" {
				t.Error(pin.Error)
			}
			if pin.Status != api.TrackerStatusPinned {
				t.Error("item should be pinned")
			}
		}

		runF(t, clusters, f)
	})
}

func TestAddOnePeerFails(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	waitForLeaderAndMetrics(t, clusters)

	t.Run("local", func(t *testing.T) {
		params := api.DefaultAddParams()
		params.Shard = false
		params.Name = "testlocal"
		lg, closer := sth.GetRandFileReader(t, 50000) // 50 MB
		defer closer.Close()

		mr := files.NewMultiFileReader(lg, true)

		r := multipart.NewReader(mr, mr.Boundary())

		var err error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err = clusters[0].AddFile(r, params)
			if err != nil {
				t.Fatal(err)
			}
		}()

		err = clusters[1].Shutdown(ctx)
		if err != nil {
			t.Fatal(err)
		}

		wg.Wait()
	})
}

// Kishan: Uncomment after https://github.com/ipfs/ipfs-cluster/issues/761
// is resolved. This test would pass, but not for the reason we want it to.
// Add fails waiting for the leader.
func TestAddAllPeersFail(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	waitForLeaderAndMetrics(t, clusters)

	t.Run("local", func(t *testing.T) {
		params := api.DefaultAddParams()
		params.Shard = false
		params.Name = "testlocal"
		lg, closer := sth.GetRandFileReader(t, 50000) // 50 MB
		defer closer.Close()

		mr := files.NewMultiFileReader(lg, true)

		r := multipart.NewReader(mr, mr.Boundary())
		params.PinOptions.ReplicationFactorMax = 2
		params.PinOptions.ReplicationFactorMin = 2
		clusters[0].allocator = &mockPinAllocator{
			peers: []peer.ID{clusters[1].id, clusters[2].id},
		}

		var err error
		// var cid cid.Cid
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err = clusters[0].AddFile(r, params)
			if err == nil {
				t.Fatalf("expected error")
			}
		}()

		for i := 1; i < 3; i++ {
			err = clusters[i].Shutdown(ctx)
			if err != nil {
				t.Fatal(err)
			}
		}

		wg.Wait()

		// pinDelay()

		// pini, err := clusters[0].Status(ctx, cid)
		// if err != nil {
		// 	t.Error(err)
		// }
		// fmt.Println(pini.String())

		// pin, err := clusters[0].PinGet(ctx, cid)
		// if err != nil {
		// 	t.Fatal(err)
		// }

		// fmt.Println(pin.Allocations)
	})
}

type mockPinAllocator struct {
	peers []peer.ID
}

// SetClient does nothing in this allocator
func (alloc mockPinAllocator) SetClient(c *rpc.Client) {}

// Shutdown does nothing in this allocator
func (alloc mockPinAllocator) Shutdown(_ context.Context) error { return nil }

func (alloc mockPinAllocator) Allocate(ctx context.Context, c cid.Cid, current, candidates, priority map[peer.ID]*api.Metric) ([]peer.ID, error) {
	return alloc.peers, nil
}
