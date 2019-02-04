package ipfscluster

// This files has tests for Add* using multiple cluster peers.

import (
	"context"
	"mime/multipart"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

func TestAdd(t *testing.T) {
	ctx := context.Background()
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

		pinDelay()

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
