package ipfscluster

import (
	"fmt"
	"sync"
	"testing"
	"time"

	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	ma "github.com/multiformats/go-multiaddr"
)

func peerManagerClusters(t *testing.T) ([]*Cluster, []*test.IpfsMock) {
	cls := make([]*Cluster, nClusters, nClusters)
	mocks := make([]*test.IpfsMock, nClusters, nClusters)
	var wg sync.WaitGroup
	for i := 0; i < nClusters; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cl, m := createOnePeerCluster(t, i, testingClusterSecret)
			cls[i] = cl
			mocks[i] = m
		}(i)
	}
	wg.Wait()
	return cls, mocks
}

func clusterAddr(c *Cluster) ma.Multiaddr {
	cAddr, _ := ma.NewMultiaddr(fmt.Sprintf("%s/ipfs/%s", c.host.Addrs()[0], c.id.Pretty()))
	return cAddr
}

func TestClustersPeerAdd(t *testing.T) {
	clusters, mocks := peerManagerClusters(t)
	defer shutdownClusters(t, clusters, mocks)

	if len(clusters) < 2 {
		t.Skip("need at least 2 nodes for this test")
	}

	for i := 1; i < len(clusters); i++ {
		addr := clusterAddr(clusters[i])
		id, err := clusters[0].PeerAdd(addr)
		if err != nil {
			t.Fatal(err)
		}

		if len(id.ClusterPeers) != i+1 {
			// ClusterPeers is originally empty and contains nodes as we add them
			t.Log(i, id.ClusterPeers)
			t.Fatal("cluster peers should be up to date with the cluster")
		}
	}

	h, _ := cid.Decode(test.TestCid1)
	err := clusters[1].Pin(api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}
	pinDelay()

	f := func(t *testing.T, c *Cluster) {
		ids := c.Peers()

		// check they are tracked by the peer manager
		if len(ids) != nClusters {
			//t.Log(ids)
			t.Error("added clusters are not part of clusters")
		}

		// Check that they are part of the consensus
		pins := c.Pins()
		if len(pins) != 1 {
			t.Log(pins)
			t.Error("expected 1 pin everywhere")
		}

		if len(c.ID().ClusterPeers) != nClusters {
			t.Log(c.ID().ClusterPeers)
			t.Error("By now cluster peers should reflect all peers")
		}

		// check that all peers are part of the peerstore
		// (except ourselves)
		addrs := c.peerManager.LoadPeerstore()
		peerMap := make(map[peer.ID]struct{})
		for _, a := range addrs {
			pid, _, err := api.Libp2pMultiaddrSplit(a)
			if err != nil {
				t.Fatal(err)
			}
			peerMap[pid] = struct{}{}
		}

		if len(peerMap) != nClusters-1 {
			t.Error(c.peerManager.LoadPeerstore())
			t.Errorf("%s: expected different cluster peers in the peerstore", c.id)
		}
	}
	runF(t, clusters, f)
}

func TestClustersPeerAddBadPeer(t *testing.T) {
	clusters, mocks := peerManagerClusters(t)
	defer shutdownClusters(t, clusters, mocks)

	if len(clusters) < 2 {
		t.Skip("need at least 2 nodes for this test")
	}

	badClusterAddr := clusterAddr(clusters[1])

	// We add a cluster that has been shutdown
	// (closed transports)
	clusters[1].Shutdown()

	// Let the OS actually close the ports.
	// Sometimes we hang otherwise.
	delay()

	_, err := clusters[0].PeerAdd(badClusterAddr)
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
		t.Skip("need at least 3 nodes for this test")
	}

	_, err := clusters[0].PeerAdd(clusterAddr(clusters[1]))
	ids := clusters[1].Peers()
	if len(ids) != 2 {
		t.Error("expected 2 peers")
	}

	// Now we shutdown one member of the running cluster
	// and try to add someone else.
	err = clusters[1].Shutdown()
	if err != nil {
		t.Error("Shutdown should be clean: ", err)
	}
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
	clusters, mocks := createClusters(t)
	defer shutdownClusters(t, clusters, mocks)

	if len(clusters) < 2 {
		t.Skip("test needs at least 2 clusters")
	}

	p := clusters[1].ID().ID
	err := clusters[0].PeerRemove(p)
	if err != nil {
		t.Error(err)
	}

	delay()

	f := func(t *testing.T, c *Cluster) {
		if c.ID().ID == p { //This is the removed cluster
			_, ok := <-c.Done()
			if ok {
				t.Error("removed peer should have exited")
			}
		} else {
			ids := c.Peers()
			if len(ids) != nClusters-1 {
				t.Error("should have removed 1 peer")
			}
		}
	}

	runF(t, clusters, f)
}

func TestClustersPeerRemoveSelf(t *testing.T) {
	// this test hangs sometimes if there are problems
	clusters, mocks := createClusters(t)
	defer shutdownClusters(t, clusters, mocks)

	for i := 0; i < len(clusters); i++ {
		waitForLeaderAndMetrics(t, clusters)
		peers := clusters[i].Peers()
		t.Logf("Current cluster size: %d", len(peers))
		if len(peers) != (len(clusters) - i) {
			t.Fatal("Previous peers not removed correctly")
		}
		err := clusters[i].PeerRemove(clusters[i].ID().ID)
		// Last peer member won't be able to remove itself
		// In this case, we shut it down.
		if err != nil {
			if i != len(clusters)-1 { //not last
				t.Error(err)
			} else {
				err := clusters[i].Shutdown()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		_, more := <-clusters[i].Done()
		if more {
			t.Error("should be done")
		}
	}
}

func TestClustersPeerRemoveLeader(t *testing.T) {
	// this test is like the one above, except it always
	// removes the current leader.
	// this test hangs sometimes if there are problems
	clusters, mocks := createClusters(t)
	defer shutdownClusters(t, clusters, mocks)

	findLeader := func() *Cluster {
		var l peer.ID
		for _, c := range clusters {
			if !c.shutdownB {
				waitForLeaderAndMetrics(t, clusters)
				l, _ = c.consensus.Leader()
			}
		}
		for _, c := range clusters {
			if c.id == l {
				return c
			}
		}
		return nil
	}

	for i := 0; i < len(clusters); i++ {
		leader := findLeader()
		peers := leader.Peers()
		t.Logf("Current cluster size: %d", len(peers))
		if len(peers) != (len(clusters) - i) {
			t.Fatal("Previous peers not removed correctly")
		}
		err := leader.PeerRemove(leader.id)
		// Last peer member won't be able to remove itself
		// In this case, we shut it down.
		if err != nil {
			if i != len(clusters)-1 { //not last
				t.Error(err)
			} else {
				err := leader.Shutdown()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		_, more := <-leader.Done()
		if more {
			t.Error("should be done")
		}
		time.Sleep(time.Second / 2)
	}
}

func TestClustersPeerRemoveReallocsPins(t *testing.T) {
	clusters, mocks := createClusters(t)
	defer shutdownClusters(t, clusters, mocks)

	if len(clusters) < 3 {
		t.Skip("test needs at least 3 clusters")
	}

	// Adjust the replication factor for re-allocation
	for _, c := range clusters {
		c.config.ReplicationFactorMin = nClusters - 1
		c.config.ReplicationFactorMax = nClusters - 1
	}

	// We choose to remove the leader, to make things even more interesting
	leaderID, err := clusters[0].consensus.Leader()
	if err != nil {
		t.Fatal(err)
	}

	var leader *Cluster
	var leaderi int
	for i, cl := range clusters {
		if id := cl.ID().ID; id == leaderID {
			leader = cl
			leaderi = i
			break
		}
	}
	if leader == nil {
		t.Fatal("did not find a leader?")
	}

	leaderMock := mocks[leaderi]

	// Remove leader from set
	clusters = append(clusters[:leaderi], clusters[leaderi+1:]...)
	mocks = append(mocks[:leaderi], mocks[leaderi+1:]...)
	defer leader.Shutdown()
	defer leaderMock.Close()

	tmpCid, _ := cid.Decode(test.TestCid1)
	prefix := tmpCid.Prefix()

	// Pin nCluster random pins. This ensures each peer will
	// pin the same number of Cids.
	for i := 0; i < nClusters; i++ {
		h, err := prefix.Sum(randomBytes())
		checkErr(t, err)
		err = leader.Pin(api.PinCid(h))
		checkErr(t, err)
		ttlDelay()
	}

	pinDelay()

	// At this point, all peers must have 1 pin associated to them.
	// Find out which pin is associated to leader.
	interestingCids := []*cid.Cid{}

	pins := leader.Pins()
	if len(pins) != nClusters {
		t.Fatal("expected number of tracked pins to be nClusters")
	}
	for _, p := range pins {
		if containsPeer(p.Allocations, leaderID) {
			//t.Logf("%s pins %s", leaderID, p.Cid)
			interestingCids = append(interestingCids, p.Cid)
		}
	}

	if len(interestingCids) != nClusters-1 {
		//t.Fatal("The number of allocated Cids is not expected")
		t.Fatalf("Expected %d allocated CIDs but got %d", nClusters-1,
			len(interestingCids))
	}

	// Now the leader removes itself
	err = leader.PeerRemove(leaderID)
	if err != nil {
		t.Fatal("error removing peer:", err)
	}

	delay()
	waitForLeaderAndMetrics(t, clusters)
	delay() // this seems to fail when not waiting enough...

	for _, icid := range interestingCids {
		// Now check that the allocations are new.
		newPin, err := clusters[1].PinGet(icid)
		if err != nil {
			t.Fatal("error getting the new allocations for", icid)
		}
		if containsPeer(newPin.Allocations, leaderID) {
			t.Fatal("pin should not be allocated to the removed peer")
		}
	}
}

func TestClustersPeerJoin(t *testing.T) {
	clusters, mocks := peerManagerClusters(t)
	defer shutdownClusters(t, clusters, mocks)

	if len(clusters) < 3 {
		t.Skip("test needs at least 3 clusters")
	}

	for i := 1; i < len(clusters); i++ {
		err := clusters[i].Join(clusterAddr(clusters[0]))
		if err != nil {
			t.Fatal(err)
		}
	}
	hash, _ := cid.Decode(test.TestCid1)
	clusters[0].Pin(api.PinCid(hash))
	pinDelay()

	f := func(t *testing.T, c *Cluster) {
		peers := c.Peers()
		if len(peers) != nClusters {
			t.Error("all peers should be connected")
		}
		pins := c.Pins()
		if len(pins) != 1 || !pins[0].Cid.Equals(hash) {
			t.Error("all peers should have pinned the cid")
		}
	}
	runF(t, clusters, f)
}

func TestClustersPeerJoinAllAtOnce(t *testing.T) {
	clusters, mocks := peerManagerClusters(t)
	defer shutdownClusters(t, clusters, mocks)

	if len(clusters) < 2 {
		t.Skip("test needs at least 2 clusters")
	}

	f := func(t *testing.T, c *Cluster) {
		err := c.Join(clusterAddr(clusters[0]))
		if err != nil {
			t.Fatal(err)
		}
	}
	runF(t, clusters[1:], f)

	hash, _ := cid.Decode(test.TestCid1)
	clusters[0].Pin(api.PinCid(hash))
	pinDelay()

	f2 := func(t *testing.T, c *Cluster) {
		peers := c.Peers()
		if len(peers) != nClusters {
			t.Error("all peers should be connected")
		}
		pins := c.Pins()
		if len(pins) != 1 || !pins[0].Cid.Equals(hash) {
			t.Error("all peers should have pinned the cid")
		}
	}
	runF(t, clusters, f2)
}

// This test fails a lot when re-use port is not available (MacOS, Windows)
// func TestClustersPeerJoinAllAtOnceWithRandomBootstrap(t *testing.T) {
// 	clusters, mocks := peerManagerClusters(t)
// 	defer shutdownClusters(t, clusters, mocks)

// 	if len(clusters) < 3 {
// 		t.Skip("test needs at least 3 clusters")
// 	}

// 	delay()

// 	// We have a 2 node cluster and the rest of nodes join
// 	// one of the two seeds randomly

// 	err := clusters[1].Join(clusterAddr(clusters[0]))
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	f := func(t *testing.T, c *Cluster) {
// 		j := rand.Intn(2)
// 		err := c.Join(clusterAddr(clusters[j]))
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 	}
// 	runF(t, clusters[2:], f)

// 	hash, _ := cid.Decode(test.TestCid1)
// 	clusters[0].Pin(api.PinCid(hash))
// 	delay()

// 	f2 := func(t *testing.T, c *Cluster) {
// 		peers := c.Peers()
// 		if len(peers) != nClusters {
// 			peersIds := []peer.ID{}
// 			for _, p := range peers {
// 				peersIds = append(peersIds, p.ID)
// 			}
// 			t.Errorf("%s sees %d peers: %s", c.id, len(peers), peersIds)
// 		}
// 		pins := c.Pins()
// 		if len(pins) != 1 || !pins[0].Cid.Equals(hash) {
// 			t.Error("all peers should have pinned the cid")
// 		}
// 	}
// 	runF(t, clusters, f2)
// }

// Tests that a peer catches up on the state correctly after rejoining
func TestClustersPeerRejoin(t *testing.T) {
	clusters, mocks := peerManagerClusters(t)
	defer shutdownClusters(t, clusters, mocks)

	// pin something in c0
	pin1, _ := cid.Decode(test.TestCid1)
	err := clusters[0].Pin(api.PinCid(pin1))
	if err != nil {
		t.Fatal(err)
	}

	// add all clusters
	for i := 1; i < len(clusters); i++ {
		addr := clusterAddr(clusters[i])
		_, err := clusters[0].PeerAdd(addr)
		if err != nil {
			t.Fatal(err)
		}
	}

	delay()

	// all added peers should have the content
	for i := 1; i < len(clusters); i++ {
		pinfo := clusters[i].tracker.Status(pin1)
		if pinfo.Status != api.TrackerStatusPinned {
			t.Error("Added peers should pin the content")
		}
	}

	clusters[0].config.LeaveOnShutdown = true
	err = clusters[0].Shutdown()
	if err != nil {
		t.Fatal(err)
	}
	mocks[0].Close()

	delay()

	// Forget peer so we can re-add one in same address/port
	f := func(t *testing.T, c *Cluster) {
		c.peerManager.RmPeer(clusters[0].id)
	}
	runF(t, clusters[1:], f)

	// Pin something on the rest
	pin2, _ := cid.Decode(test.TestCid2)
	err = clusters[1].Pin(api.PinCid(pin2))
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	// Rejoin c0
	c0, m0 := createOnePeerCluster(t, 0, testingClusterSecret)
	clusters[0] = c0
	mocks[0] = m0
	addr := clusterAddr(c0)
	_, err = clusters[1].PeerAdd(addr)
	if err != nil {
		t.Fatal(err)
	}

	delay()

	pinfo := clusters[0].tracker.Status(pin2)
	if pinfo.Status != api.TrackerStatusPinned {
		t.Error("re-joined cluster should have caught up")
	}

	pinfo = clusters[0].tracker.Status(pin1)
	if pinfo.Status != api.TrackerStatusPinned {
		t.Error("re-joined cluster should have original pin")
	}
}
