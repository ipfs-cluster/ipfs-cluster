package ipfscluster

import (
	"sync"
	"testing"

	cid "github.com/ipfs/go-cid"
	ma "github.com/multiformats/go-multiaddr"
)

func peerManagerClusters(t *testing.T) ([]*Cluster, []*ipfsMock) {
	cls := make([]*Cluster, nClusters, nClusters)
	mocks := make([]*ipfsMock, nClusters, nClusters)
	var wg sync.WaitGroup
	for i := 0; i < nClusters; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cl, m := createOnePeerCluster(t, i)
			cls[i] = cl
			mocks[i] = m
		}(i)
	}
	wg.Wait()
	return cls, mocks
}

func clusterAddr(c *Cluster) ma.Multiaddr {
	addr := c.config.ClusterAddr
	pidAddr, _ := ma.NewMultiaddr("/ipfs/" + c.ID().ID.Pretty())
	return addr.Encapsulate(pidAddr)
}

func TestClustersPeerAdd(t *testing.T) {
	clusters, mocks := peerManagerClusters(t)
	defer shutdownClusters(t, clusters, mocks)

	if len(clusters) < 2 {
		t.Fatal("need at least 2 nodes for this test")
	}

	for i := 1; i < len(clusters); i++ {
		addr := clusterAddr(clusters[i])
		id, err := clusters[0].PeerAdd(addr)
		if err != nil {
			t.Fatal(err)
		}
		if len(id.ClusterPeers) != i {
			// ClusterPeers is originally empty and contains nodes as we add them
			t.Log(id.ClusterPeers)
			t.Fatal("cluster peers should be up to date with the cluster")
		}
	}

	h, _ := cid.Decode(testCid)
	clusters[1].Pin(h)
	delay()

	f := func(t *testing.T, c *Cluster) {
		ids := c.Peers()

		// check they are tracked by the peer manager
		if len(ids) != nClusters {
			t.Error("added clusters are not part of clusters")
		}

		// Check that they are part of the consensus
		pins := c.Pins()
		if len(pins) != 1 {
			t.Error("expected 1 pin everywhere")
		}

		// check that its part of the configuration
		if len(c.config.ClusterPeers) != nClusters-1 {
			t.Error("expected different cluster peers in the configuration")
		}

		for _, peer := range c.config.ClusterPeers {
			if peer == nil {
				t.Error("something went wrong adding peer to config")
			}
		}
	}
	runF(t, clusters, f)
}

func TestClustersPeerAddBadPeer(t *testing.T) {
	clusters, mocks := peerManagerClusters(t)
	defer shutdownClusters(t, clusters, mocks)

	if len(clusters) < 2 {
		t.Fatal("need at least 2 nodes for this test")
	}

	// We add a cluster that has been shutdown
	// (closed transports)
	clusters[1].Shutdown()
	_, err := clusters[0].PeerAdd(clusterAddr(clusters[1]))
	if err == nil {
		t.Error("expected an error")
	}
	ids := clusters[0].Peers()
	if len(ids) != 1 {
		t.Error("cluster should have only one member")
	}
}

func TestClustersPeerAddInUnhealthyCluster(t *testing.T) {
	clusters, mocks := peerManagerClusters(t)
	defer shutdownClusters(t, clusters, mocks)

	if len(clusters) < 3 {
		t.Fatal("need at least 3 nodes for this test")
	}

	_, err := clusters[0].PeerAdd(clusterAddr(clusters[1]))
	ids := clusters[1].Peers()
	if len(ids) != 2 {
		t.Error("expected 2 peers")
	}

	// Now we shutdown one member of the running cluster
	// and try to add someone else.
	clusters[1].Shutdown()
	_, err = clusters[0].PeerAdd(clusterAddr(clusters[2]))

	if err == nil {
		t.Error("expected an error")
	}

	ids = clusters[0].Peers()
	if len(ids) != 2 {
		t.Error("cluster should still have 2 peers")
	}
}

func TestClustersPeerRemove(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)

	p := clusters[1].ID().ID
	err := clusters[0].PeerRemove(p)
	if err != nil {
		t.Error(err)
	}

	f := func(t *testing.T, c *Cluster) {
		if c.ID().ID == p { //This is the removed cluster
			_, ok := <-c.Done()
			if ok {
				t.Error("removed peer should have exited")
			}
			if len(c.config.ClusterPeers) != 0 {
				t.Error("cluster peers should be empty")
			}
		} else {
			ids := c.Peers()
			if len(ids) != nClusters-1 {
				t.Error("should have removed 1 peer")
			}
			if len(c.config.ClusterPeers) != nClusters-2 {
				t.Error("should have removed peer from config")
			}
		}
	}

	runF(t, clusters, f)
}
