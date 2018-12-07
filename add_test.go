package ipfscluster

// This files has tests for Add* using multiple cluster peers.

import (
	"mime/multipart"
	"testing"

	"github.com/ipfs/ipfs-cluster/adder/sharding"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

func TestAdd(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

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

		pinDelay()

		f := func(t *testing.T, c *Cluster) {
			pin := c.StatusLocal(ci)
			if pin.Error != "" {
				t.Error(pin.Error)
			}
			if pin.Status != api.TrackerStatusPinned {
				t.Error("item should be pinned")
			}
		}

		runF(t, clusters, f)

		// for next test, this needs to be unpinned.
		err = clusters[0].Unpin(ci)
		if err != nil {
			t.Fatal(err)
		}
		pinDelay()
	})

	t.Run("shard", func(t *testing.T) {
		params := api.DefaultAddParams()
		params.ShardSize = 1024 * 300 // 300kB
		params.Shard = true
		params.Name = "testsharding"
		// replication factor is -1, which doesn't make sense
		// but allows to check things more easily.
		mfr, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		r := multipart.NewReader(mfr, mfr.Boundary())
		ci, err := clusters[0].AddFile(r, params)
		if err != nil {
			t.Fatal(err)
		}
		if ci.String() != test.ShardingDirBalancedRootCID {
			t.Fatal("unexpected root CID for sharding add")
		}

		pinDelay()

		// shardBlocks ignored, as tested in /sharding
		_, err = sharding.VerifyShards(t, ci, clusters[0], clusters[0].ipfs, 14)
		if err != nil {
			t.Fatal(err)
		}
		err = clusters[0].Unpin(ci)
		pinDelay()
		f := func(t *testing.T, c *Cluster) {
			pins := c.Pins()
			if len(pins) > 0 {
				t.Error("should have removed all pins from the state")
			}
		}
		runF(t, clusters, f)
	})
}

func TestShardingRandFile(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	params := api.DefaultAddParams()
	params.ShardSize = 1024 * 1024 * 5 // 5 MB
	params.Name = "testingFile"
	params.Shard = true

	// Add a random 50MB file. Note size in kbs below.
	mr, closer := sth.GetRandFileMultiReader(t, 1024*50) // 50 MB
	defer closer.Close()
	r := multipart.NewReader(mr, mr.Boundary())

	ci, err := clusters[0].AddFile(r, params)
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	// shardBlocks ignored, as tested in /sharding
	// 11 shards as due to chunking and extra nodes
	// things don't fit perfectly in 10*5MB shards.
	_, err = sharding.VerifyShards(t, ci, clusters[0], clusters[0].ipfs, 11)
	if err != nil {
		t.Fatal(err)
	}
	err = clusters[0].Unpin(ci)
	pinDelay()
	f := func(t *testing.T, c *Cluster) {
		pins := c.Pins()
		if len(pins) > 0 {
			t.Error("should have removed all pins from the state")
		}
	}
	runF(t, clusters, f)
}

func TestShardingUnpin(t *testing.T) {
	countShards := func(pins []api.Pin) int {
		n := 0
		for _, p := range pins {
			if p.Type == api.ShardType {
				n++
			}
		}
		return n
	}

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	params := api.DefaultAddParams()
	params.ShardSize = 1024 * 500 // 500 KB
	params.Name = "testingFile"
	params.Shard = true

	// Add a random file
	mr, closer := sth.GetRandFileMultiReader(t, 1024*50) // 50 MB
	defer closer.Close()
	r := multipart.NewReader(mr, mr.Boundary())

	ci, err := clusters[0].AddFile(r, params)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("file1: ", ci)
	shards1 := countShards(clusters[0].Pins())

	pinDelay()

	// Add the same file again, except only the first half
	params.Name = "testingFile2"
	mr2, closer2 := sth.GetRandFileMultiReader(t, 1024*25) // half the size
	defer closer2.Close()
	r2 := multipart.NewReader(mr2, mr2.Boundary())
	ci2, err := clusters[0].AddFile(r2, params)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("file2: ", ci2)
	pinDelay()

	shards2 := countShards(clusters[0].Pins())
	if shards2 != shards1+1 {
		// The last shard is different because the file
		// was cut sooner in it.
		t.Error("There should be only one new shard")
	}

	// Unpin the first file:
	// The shards from the second should stay pinned.
	err = clusters[0].Unpin(ci)
	if err != nil {
		t.Fatal(err)
	}
	pinDelay()

	shards3 := countShards(clusters[0].Pins())

	t.Logf("shards1: %d. 2: %d. 3: %d", shards1, shards2, shards3)
	if shards3 != shards1/2 {
		t.Error("half of the shards should still be pinned")
	}
}

func TestAddPeerDown(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	err := clusters[0].Shutdown()
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

		pinDelay()

		f := func(t *testing.T, c *Cluster) {
			if c.id == clusters[0].id {
				return
			}

			pin := c.StatusLocal(ci)
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
