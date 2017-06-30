package ipfscluster

import "testing"

func TestSimplePNet(t *testing.T) {
    clusters, mocks := peerManagerClusters(t)
    defer cleanRaft()
    defer shutdownClusters(t, clusters, mocks)

	if len(clusters) < 2 {
		t.Skip("need at least 2 nodes for this test")
	}

    _, err := clusters[0].PeerAdd(clusterAddr(clusters[1]))
    if err != nil {
        t.Fatal(err)
    }

    if len(clusters[0].Peers()) != len(clusters[1].Peers()) {
        t.Fatal("Expected same number of peers")
    }
    if len(clusters[0].Peers()) != 2 {
        t.Fatal("Expected 2 peers")
    }
}

func TestSwarmKeyRequired(t *testing.T) {
    cl1, _ := createOnePeerCluster(t, 1, true)
    cl2, _ := createOnePeerCluster(t, 2, false)
    defer cleanRaft()
    defer cl1.Shutdown()
    defer cl2.Shutdown()
    peers1 := cl1.Peers()
    peers2 := cl2.Peers()

    _, err := cl1.PeerAdd(clusterAddr(cl2))
    if err == nil {
        t.Fatal("Peer entered private cluster without key.")
    }

    if len(peers1) != len(peers2) {
        t.Fatal("Expected same number of peers")
    }
    if len(peers1) != 1 {
        t.Fatal("Expected no peers other than self")
    }
}

