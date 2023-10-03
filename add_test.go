package ipfscluster

// This files has tests for Add* using multiple cluster peers.

import (
	"context"
	"mime/multipart"
	"sync"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/adder"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/test"
	files "github.com/ipfs/boxo/files"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

func TestAdd(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	waitForLeaderAndMetrics(t, clusters)

	t.Run("default", func(t *testing.T) {
		params := api.DefaultAddParams()
		params.Shard = false
		params.Name = "testlocal"
		mfr, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		r := multipart.NewReader(mfr, mfr.Boundary())
		ci, err := clusters[0].AddFile(context.Background(), r, params)
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

	t.Run("local_one_allocation", func(t *testing.T) {
		params := api.DefaultAddParams()
		params.Shard = false
		params.Name = "testlocal"
		params.ReplicationFactorMin = 1
		params.ReplicationFactorMax = 1
		params.Local = true
		mfr, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		r := multipart.NewReader(mfr, mfr.Boundary())
		ci, err := clusters[2].AddFile(context.Background(), r, params)
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
			switch c.id {
			case clusters[2].id:
				if pin.Status != api.TrackerStatusPinned {
					t.Error("item should be pinned and is", pin.Status)
				}
			default:
				if pin.Status != api.TrackerStatusRemote {
					t.Errorf("item should only be allocated to cluster2")
				}
			}
		}

		runF(t, clusters, f)
	})
}

func TestAddWithUserAllocations(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	waitForLeaderAndMetrics(t, clusters)

	t.Run("local", func(t *testing.T) {
		params := api.DefaultAddParams()
		params.ReplicationFactorMin = 2
		params.ReplicationFactorMax = 2
		params.UserAllocations = []peer.ID{clusters[0].id, clusters[1].id}
		params.Shard = false
		params.Name = "testlocal"
		mfr, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		r := multipart.NewReader(mfr, mfr.Boundary())
		ci, err := clusters[0].AddFile(context.Background(), r, params)
		if err != nil {
			t.Fatal(err)
		}

		pinDelay()

		f := func(t *testing.T, c *Cluster) {
			if c == clusters[0] || c == clusters[1] {
				pin := c.StatusLocal(ctx, ci)
				if pin.Error != "" {
					t.Error(pin.Error)
				}
				if pin.Status != api.TrackerStatusPinned {
					t.Error("item should be pinned and is", pin.Status)
				}
			} else {
				pin := c.StatusLocal(ctx, ci)
				if pin.Status != api.TrackerStatusRemote {
					t.Error("expected tracker status remote")
				}
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
		ci, err := clusters[1].AddFile(context.Background(), r, params)
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
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	waitForLeaderAndMetrics(t, clusters)

	t.Run("local", func(t *testing.T) {
		params := api.DefaultAddParams()
		params.Shard = false
		params.Name = "testlocal"
		lg, closer := sth.GetRandFileReader(t, 100000) // 100 MB
		defer closer.Close()

		mr := files.NewMultiFileReader(lg, true, false)
		r := multipart.NewReader(mr, mr.Boundary())

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := clusters[0].AddFile(context.Background(), r, params)
			if err != nil {
				t.Error(err)
			}
		}()

		// Disconnect 1 cluster (the last). Things should keep working.
		// Important that we close the hosts, otherwise the RPC
		// Servers keep working along with BlockPuts.
		time.Sleep(100 * time.Millisecond)
		c := clusters[nClusters-1]
		c.Shutdown(context.Background())
		c.dht.Close()
		c.host.Close()
		wg.Wait()
	})
}

func TestAddAllPeersFail(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	waitForLeaderAndMetrics(t, clusters)

	t.Run("local", func(t *testing.T) {
		// Prevent added content to be allocated to cluster 0
		// as it is already going to have something.
		_, err := clusters[0].Pin(ctx, test.Cid1, api.PinOptions{
			ReplicationFactorMin: 1,
			ReplicationFactorMax: 1,
			UserAllocations:      []peer.ID{clusters[0].host.ID()},
		})
		if err != nil {
			t.Fatal(err)
		}

		ttlDelay()

		params := api.DefaultAddParams()
		params.Shard = false
		params.Name = "testlocal"
		// Allocate to every peer except 0 (which already has a pin)
		params.PinOptions.ReplicationFactorMax = nClusters - 1
		params.PinOptions.ReplicationFactorMin = nClusters - 1

		lg, closer := sth.GetRandFileReader(t, 100000) // 100 MB
		defer closer.Close()
		mr := files.NewMultiFileReader(lg, true, false)
		r := multipart.NewReader(mr, mr.Boundary())

		// var cid cid.Cid
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := clusters[0].AddFile(context.Background(), r, params)
			if err != adder.ErrBlockAdder {
				t.Error("expected ErrBlockAdder. Got: ", err)
			}
		}()

		time.Sleep(100 * time.Millisecond)

		// Shutdown all clusters except 0 to see the right error.
		// Important that we shut down the hosts, otherwise
		// the RPC Servers keep working along with BlockPuts.
		// Note that this kills raft.
		runF(t, clusters[1:], func(t *testing.T, c *Cluster) {
			c.Shutdown(ctx)
			c.dht.Close()
			c.host.Close()
		})
		wg.Wait()
	})
}
