package ipfscluster

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/allocator/descendalloc"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/informer/disk"
	"github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs/ipfs-cluster/monitor/basic"
	"github.com/ipfs/ipfs-cluster/pintracker/maptracker"
	"github.com/ipfs/ipfs-cluster/sharder"
	"github.com/ipfs/ipfs-cluster/state"
	"github.com/ipfs/ipfs-cluster/state/mapstate"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	crypto "github.com/libp2p/go-libp2p-crypto"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
)

//TestClusters*
var (
	// number of clusters to create
	nClusters = 6

	// number of pins to pin/unpin/check
	nPins = 500

	logLevel = "CRITICAL"

	// When testing with fixed ports...
	// clusterPort   = 10000
	// apiPort       = 10100
	// ipfsProxyPort = 10200
)

func init() {
	flag.StringVar(&logLevel, "loglevel", logLevel, "default log level for tests")
	flag.IntVar(&nClusters, "nclusters", nClusters, "number of clusters to use")
	flag.IntVar(&nPins, "npins", nPins, "number of pins to pin/unpin/check")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	for f := range LoggingFacilities {
		SetFacilityLogLevel(f, logLevel)
	}

	for f := range LoggingFacilitiesExtra {
		SetFacilityLogLevel(f, logLevel)
	}
}

func checkErr(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

func randomBytes() []byte {
	bs := make([]byte, 64, 64)
	for i := 0; i < len(bs); i++ {
		b := byte(rand.Int())
		bs[i] = b
	}
	return bs
}

func createComponents(t *testing.T, i int, clusterSecret []byte) (*Config, *raft.Config, API, IPFSConnector, state.State, PinTracker, PeerMonitor, PinAllocator, Informer, Sharder, *test.IpfsMock) {
	mock := test.NewIpfsMock()
	//
	//clusterAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", clusterPort+i))
	// Bind on port 0
	clusterAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	//apiAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", apiPort+i))
	// Bind on port 0
	apiAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	// Bind on Port 0
	// proxyAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", ipfsProxyPort+i))
	proxyAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	nodeAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", mock.Addr, mock.Port))
	priv, pub, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
	checkErr(t, err)
	pid, err := peer.IDFromPublicKey(pub)
	checkErr(t, err)
	peername := fmt.Sprintf("peer_%d", i)

	clusterCfg, apiCfg, ipfshttpCfg, consensusCfg, trackerCfg, monCfg, diskInfCfg, sharderCfg := testingConfigs()

	clusterCfg.ID = pid
	clusterCfg.Peername = peername
	clusterCfg.PrivateKey = priv
	clusterCfg.Secret = clusterSecret
	clusterCfg.ListenAddr = clusterAddr
	clusterCfg.LeaveOnShutdown = false
	apiCfg.ListenAddr = apiAddr
	ipfshttpCfg.ProxyAddr = proxyAddr
	ipfshttpCfg.NodeAddr = nodeAddr
	consensusCfg.DataFolder = "./e2eTestRaft/" + pid.Pretty()

	api, err := rest.NewAPI(apiCfg)
	checkErr(t, err)
	ipfs, err := ipfshttp.NewConnector(ipfshttpCfg)
	checkErr(t, err)
	state := mapstate.NewMapState()
	tracker := maptracker.NewMapPinTracker(trackerCfg, clusterCfg.ID)
	mon, err := basic.NewMonitor(monCfg)
	checkErr(t, err)
	alloc := descendalloc.NewAllocator()
	inf, err := disk.NewInformer(diskInfCfg)
	checkErr(t, err)
	sharder, err := sharder.NewSharder(sharderCfg)
	checkErr(t, err)

	return clusterCfg, consensusCfg, api, ipfs, state, tracker, mon, alloc, inf, sharder, mock
}

func createCluster(t *testing.T, clusterCfg *Config, consensusCfg *raft.Config, api API, ipfs IPFSConnector, state state.State, tracker PinTracker, mon PeerMonitor, alloc PinAllocator, inf Informer, sharder Sharder) *Cluster {
	cl, err := NewCluster(clusterCfg, consensusCfg, api, ipfs, state, tracker, mon, alloc, inf, sharder)
	checkErr(t, err)
	<-cl.Ready()
	return cl
}

func createOnePeerCluster(t *testing.T, nth int, clusterSecret []byte) (*Cluster, *test.IpfsMock) {
	clusterCfg, consensusCfg, api, ipfs, state, tracker, mon, alloc, inf, sharder, mock := createComponents(t, nth, clusterSecret)
	cl := createCluster(t, clusterCfg, consensusCfg, api, ipfs, state, tracker, mon, alloc, inf, sharder)
	return cl, mock
}

func createClusters(t *testing.T) ([]*Cluster, []*test.IpfsMock) {
	os.RemoveAll("./e2eTestRaft")
	cfgs := make([]*Config, nClusters, nClusters)
	concfgs := make([]*raft.Config, nClusters, nClusters)
	apis := make([]API, nClusters, nClusters)
	ipfss := make([]IPFSConnector, nClusters, nClusters)
	states := make([]state.State, nClusters, nClusters)
	trackers := make([]PinTracker, nClusters, nClusters)
	mons := make([]PeerMonitor, nClusters, nClusters)
	allocs := make([]PinAllocator, nClusters, nClusters)
	infs := make([]Informer, nClusters, nClusters)
	sharders := make([]Sharder, nClusters, nClusters)
	ipfsMocks := make([]*test.IpfsMock, nClusters, nClusters)
	clusters := make([]*Cluster, nClusters, nClusters)

	// Uncomment when testing with fixed ports
	// clusterPeers := make([]ma.Multiaddr, nClusters, nClusters)

	for i := 0; i < nClusters; i++ {
		clusterCfg, consensusCfg, api, ipfs, state, tracker, mon, alloc, inf, sharder, mock := createComponents(t, i, testingClusterSecret)
		cfgs[i] = clusterCfg
		concfgs[i] = consensusCfg
		apis[i] = api
		ipfss[i] = ipfs
		states[i] = state
		trackers[i] = tracker
		mons[i] = mon
		allocs[i] = alloc
		infs[i] = inf
		sharders[i] = sharder
		ipfsMocks[i] = mock

		// Uncomment with testing with fixed ports and ClusterPeers
		// addr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s",
		// 	clusterPort+i,
		// 	clusterCfg.ID.Pretty()))
		// clusterPeers[i] = addr
	}

	// ----------------------------------------------------------

	// // Set up the cluster using ClusterPeers
	// for i := 0; i < nClusters; i++ {
	// 	cfgs[i].Peers = make([]ma.Multiaddr, nClusters, nClusters)
	// 	for j := 0; j < nClusters; j++ {
	// 		cfgs[i].Peers[j] = clusterPeers[j]
	// 	}
	// }

	// var wg sync.WaitGroup
	// for i := 0; i < nClusters; i++ {
	// 	wg.Add(1)
	// 	go func(i int) {
	// 		clusters[i] = createCluster(t, cfgs[i], concfgs[i], apis[i], ipfss[i], states[i], trackers[i], mons[i], allocs[i], infs[i])
	// 		wg.Done()
	// 	}(i)
	// }
	// wg.Wait()

	// ----------------------------------------------

	// Alternative way of starting using bootstrap
	// Start first node
	clusters[0] = createCluster(t, cfgs[0], concfgs[0], apis[0], ipfss[0], states[0], trackers[0], mons[0], allocs[0], infs[0], sharders[0])
	// Find out where it binded
	bootstrapAddr, _ := ma.NewMultiaddr(fmt.Sprintf("%s/ipfs/%s", clusters[0].host.Addrs()[0], clusters[0].id.Pretty()))
	// Use first node to bootstrap
	for i := 1; i < nClusters; i++ {
		cfgs[i].Bootstrap = []ma.Multiaddr{bootstrapAddr}
	}

	// Start the rest
	var wg sync.WaitGroup
	for i := 1; i < nClusters; i++ {
		wg.Add(1)
		go func(i int) {
			clusters[i] = createCluster(t, cfgs[i], concfgs[i], apis[i], ipfss[i], states[i], trackers[i], mons[i], allocs[i], infs[i], sharders[i])
			wg.Done()
		}(i)
	}
	wg.Wait()

	// ---------------------------------------------

	// Yet an alternative way using PeerAdd
	// for i := 1; i < nClusters; i++ {
	// 	clusters[0].PeerAdd(clusterAddr(clusters[i]))
	// }
	delay()
	delay()
	return clusters, ipfsMocks
}

func shutdownClusters(t *testing.T, clusters []*Cluster, m []*test.IpfsMock) {
	for i, c := range clusters {
		err := c.Shutdown()
		if err != nil {
			t.Error(err)
		}
		m[i].Close()
	}
	os.RemoveAll("./e2eTestRaft")
}

func runF(t *testing.T, clusters []*Cluster, f func(*testing.T, *Cluster)) {
	var wg sync.WaitGroup
	for _, c := range clusters {
		wg.Add(1)
		go func(c *Cluster) {
			defer wg.Done()
			f(t, c)
		}(c)

	}
	wg.Wait()
}

func delay() {
	var d int
	if nClusters > 10 {
		d = 8

	} else if nClusters > 5 {
		d = 5
	} else {
		d = nClusters
	}
	time.Sleep(time.Duration(d) * time.Second)
}

func waitForLeader(t *testing.T, clusters []*Cluster) {
	timer := time.NewTimer(time.Minute)
	ticker := time.NewTicker(time.Second)
	// Wait for consensus to pick a new leader in case we shut it down

	// Make sure we don't check on a shutdown cluster
	j := rand.Intn(len(clusters))
	for clusters[j].shutdownB {
		j = rand.Intn(len(clusters))
	}

loop:
	for {
		select {
		case <-timer.C:
			t.Fatal("timed out waiting for a leader")
		case <-ticker.C:
			_, err := clusters[j].consensus.Leader()
			if err == nil {
				break loop
			}
		}
	}
}

func TestClustersVersion(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	f := func(t *testing.T, c *Cluster) {
		v := c.Version()
		if v != Version {
			t.Error("Bad version")
		}
	}
	runF(t, clusters, f)
}

func TestClustersPeers(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	delay()

	j := rand.Intn(nClusters) // choose a random cluster peer
	peers := clusters[j].Peers()

	if len(peers) != nClusters {
		t.Fatal("expected as many peers as clusters")
	}

	clusterIDMap := make(map[peer.ID]api.ID)
	peerIDMap := make(map[peer.ID]api.ID)

	for _, c := range clusters {
		id := c.ID()
		clusterIDMap[id.ID] = id
	}

	for _, p := range peers {
		peerIDMap[p.ID] = p
	}

	for k, id := range clusterIDMap {
		id2, ok := peerIDMap[k]
		if !ok {
			t.Fatal("expected id in both maps")
		}
		//if !crypto.KeyEqual(id.PublicKey, id2.PublicKey) {
		//	t.Error("expected same public key")
		//}
		if id.IPFS.ID != id2.IPFS.ID {
			t.Error("expected same ipfs daemon ID")
		}
	}
}

func TestClustersPin(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	exampleCid, _ := cid.Decode(test.TestCid1)
	prefix := exampleCid.Prefix()
	for i := 0; i < nPins; i++ {
		j := rand.Intn(nClusters)           // choose a random cluster peer
		h, err := prefix.Sum(randomBytes()) // create random cid
		checkErr(t, err)
		err = clusters[j].Pin(api.PinCid(h))
		if err != nil {
			t.Errorf("error pinning %s: %s", h, err)
		}
		// Test re-pin
		err = clusters[j].Pin(api.PinCid(h))
		if err != nil {
			t.Errorf("error repinning %s: %s", h, err)
		}
	}
	delay()
	fpinned := func(t *testing.T, c *Cluster) {
		status := c.tracker.StatusAll()
		for _, v := range status {
			if v.Status != api.TrackerStatusPinned {
				t.Errorf("%s should have been pinned but it is %s",
					v.Cid,
					v.Status.String())
			}
		}
		if l := len(status); l != nPins {
			t.Errorf("Pinned %d out of %d requests", l, nPins)
		}
	}
	runF(t, clusters, fpinned)

	// Unpin everything
	pinList := clusters[0].Pins()

	for i := 0; i < nPins; i++ {
		j := rand.Intn(nClusters) // choose a random cluster peer
		err := clusters[j].Unpin(pinList[i].Cid)
		if err != nil {
			t.Errorf("error unpinning %s: %s", pinList[i].Cid, err)
		}
		// test re-unpin
		err = clusters[j].Unpin(pinList[i].Cid)
		if err != nil {
			t.Errorf("error re-unpinning %s: %s", pinList[i].Cid, err)
		}

	}
	delay()

	funpinned := func(t *testing.T, c *Cluster) {
		status := c.tracker.StatusAll()
		if l := len(status); l != 0 {
			t.Errorf("Nothing should be pinned")
			//t.Errorf("%+v", status)
		}
	}
	runF(t, clusters, funpinned)
}

func TestClustersStatusAll(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h, _ := cid.Decode(test.TestCid1)
	clusters[0].Pin(api.PinCid(h))
	delay()
	// Global status
	f := func(t *testing.T, c *Cluster) {
		statuses, err := c.StatusAll()
		if err != nil {
			t.Error(err)
		}
		if len(statuses) != 1 {
			t.Fatal("bad status. Expected one item")
		}
		if statuses[0].Cid.String() != test.TestCid1 {
			t.Error("bad cid in status")
		}
		info := statuses[0].PeerMap
		if len(info) != nClusters {
			t.Error("bad info in status")
		}

		if info[c.host.ID()].Status != api.TrackerStatusPinned {
			t.Error("the hash should have been pinned")
		}

		status, err := c.Status(h)
		if err != nil {
			t.Error(err)
		}

		pinfo, ok := status.PeerMap[c.host.ID()]
		if !ok {
			t.Fatal("Host not in status")
		}

		if pinfo.Status != api.TrackerStatusPinned {
			t.Error("the status should show the hash as pinned")
		}
	}
	runF(t, clusters, f)
}

func TestClustersStatusAllWithErrors(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h, _ := cid.Decode(test.TestCid1)
	clusters[0].Pin(api.PinCid(h))
	delay()

	// shutdown 1 cluster peer
	clusters[1].Shutdown()

	f := func(t *testing.T, c *Cluster) {
		// skip if it's the shutdown peer
		if c.ID().ID == clusters[1].ID().ID {
			return
		}

		statuses, err := c.StatusAll()
		if err != nil {
			t.Error(err)
		}
		if len(statuses) != 1 {
			t.Fatal("bad status. Expected one item")
		}

		stts := statuses[0]
		if len(stts.PeerMap) != nClusters {
			t.Error("bad number of peers in status")
		}

		errst := stts.PeerMap[clusters[1].ID().ID]

		if errst.Cid.String() != test.TestCid1 {
			t.Error("errored pinInfo should have a good cid")
		}

		if errst.Status != api.TrackerStatusClusterError {
			t.Error("erroring status should be set to ClusterError")
		}

		// now check with Cid status
		status, err := c.Status(h)
		if err != nil {
			t.Error(err)
		}

		pinfo := status.PeerMap[clusters[1].ID().ID]

		if pinfo.Status != api.TrackerStatusClusterError {
			t.Error("erroring status should be ClusterError")
		}

		if pinfo.Cid.String() != test.TestCid1 {
			t.Error("errored status should have a good cid")
		}

	}
	runF(t, clusters, f)
}

func TestClustersSyncAllLocal(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h, _ := cid.Decode(test.ErrorCid) // This cid always fails
	h2, _ := cid.Decode(test.TestCid2)
	clusters[0].Pin(api.PinCid(h))
	clusters[0].Pin(api.PinCid(h2))
	delay()
	f := func(t *testing.T, c *Cluster) {
		// Sync bad ID
		infos, err := c.SyncAllLocal()
		if err != nil {
			// LocalSync() is asynchronous and should not show an
			// error even if Recover() fails.
			t.Error(err)
		}
		if len(infos) != 1 {
			t.Fatal("expected 1 elem slice")
		}
		// Last-known state may still be pinning
		if infos[0].Status != api.TrackerStatusPinError && infos[0].Status != api.TrackerStatusPinning {
			t.Error("element should be in Pinning or PinError state")
		}
	}
	// Test Local syncs
	runF(t, clusters, f)
}

func TestClustersSyncLocal(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h, _ := cid.Decode(test.ErrorCid) // This cid always fails
	h2, _ := cid.Decode(test.TestCid2)
	clusters[0].Pin(api.PinCid(h))
	clusters[0].Pin(api.PinCid(h2))
	delay()

	f := func(t *testing.T, c *Cluster) {
		info, err := c.SyncLocal(h)
		if err != nil {
			t.Error(err)
		}
		if info.Status != api.TrackerStatusPinError && info.Status != api.TrackerStatusPinning {
			t.Errorf("element is %s and not PinError", info.Status)
		}

		// Sync good ID
		info, err = c.SyncLocal(h2)
		if err != nil {
			t.Error(err)
		}
		if info.Status != api.TrackerStatusPinned {
			t.Error("element should be in Pinned state")
		}
	}
	// Test Local syncs
	runF(t, clusters, f)
}

func TestClustersSyncAll(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h, _ := cid.Decode(test.ErrorCid) // This cid always fails
	h2, _ := cid.Decode(test.TestCid2)
	clusters[0].Pin(api.PinCid(h))
	clusters[0].Pin(api.PinCid(h2))
	delay()

	j := rand.Intn(nClusters) // choose a random cluster peer
	ginfos, err := clusters[j].SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(ginfos) != 1 {
		t.Fatal("expected globalsync to have 1 elements")
	}
	if ginfos[0].Cid.String() != test.ErrorCid {
		t.Error("expected globalsync to have problems with test.ErrorCid")
	}
	for _, c := range clusters {
		inf, ok := ginfos[0].PeerMap[c.host.ID()]
		if !ok {
			t.Fatal("GlobalPinInfo should have this cluster")
		}
		if inf.Status != api.TrackerStatusPinError && inf.Status != api.TrackerStatusPinning {
			t.Error("should be PinError in all peers")
		}
	}
}

func TestClustersSync(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h, _ := cid.Decode(test.ErrorCid) // This cid always fails
	h2, _ := cid.Decode(test.TestCid2)
	clusters[0].Pin(api.PinCid(h))
	clusters[0].Pin(api.PinCid(h2))
	delay()

	j := rand.Intn(nClusters)
	ginfo, err := clusters[j].Sync(h)
	if err != nil {
		// we always attempt to return a valid response
		// with errors contained in GlobalPinInfo
		t.Fatal("did not expect an error")
	}
	pinfo, ok := ginfo.PeerMap[clusters[j].host.ID()]
	if !ok {
		t.Fatal("should have info for this host")
	}
	if pinfo.Error == "" {
		t.Error("pinInfo error should not be empty")
	}

	if ginfo.Cid.String() != test.ErrorCid {
		t.Error("GlobalPinInfo should be for test.ErrorCid")
	}

	for _, c := range clusters {
		inf, ok := ginfo.PeerMap[c.host.ID()]
		if !ok {
			t.Logf("%+v", ginfo)
			t.Fatal("GlobalPinInfo should not be empty for this host")
		}

		if inf.Status != api.TrackerStatusPinError && inf.Status != api.TrackerStatusPinning {
			t.Error("should be PinError or Pinning in all peers")
		}
	}

	// Test with a good Cid
	j = rand.Intn(nClusters)
	ginfo, err = clusters[j].Sync(h2)
	if err != nil {
		t.Fatal(err)
	}
	if ginfo.Cid.String() != test.TestCid2 {
		t.Error("GlobalPinInfo should be for testrCid2")
	}

	for _, c := range clusters {
		inf, ok := ginfo.PeerMap[c.host.ID()]
		if !ok {
			t.Fatal("GlobalPinInfo should have this cluster")
		}
		if inf.Status != api.TrackerStatusPinned {
			t.Error("the GlobalPinInfo should show Pinned in all peers")
		}
	}
}

func TestClustersRecoverLocal(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h, _ := cid.Decode(test.ErrorCid) // This cid always fails
	h2, _ := cid.Decode(test.TestCid2)
	clusters[0].Pin(api.PinCid(h))
	clusters[0].Pin(api.PinCid(h2))

	delay()

	f := func(t *testing.T, c *Cluster) {
		info, err := c.RecoverLocal(h)
		if err == nil {
			t.Error("expected an error recovering")
		}
		if info.Status != api.TrackerStatusPinError {
			t.Errorf("element is %s and not PinError", info.Status)
		}

		// Recover good ID
		info, err = c.SyncLocal(h2)
		if err != nil {
			t.Error(err)
		}
		if info.Status != api.TrackerStatusPinned {
			t.Error("element should be in Pinned state")
		}
	}
	// Test Local syncs
	runF(t, clusters, f)
}

func TestClustersRecover(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h, _ := cid.Decode(test.ErrorCid) // This cid always fails
	h2, _ := cid.Decode(test.TestCid2)
	clusters[0].Pin(api.PinCid(h))
	clusters[0].Pin(api.PinCid(h2))

	delay()

	j := rand.Intn(nClusters)
	ginfo, err := clusters[j].Recover(h)
	if err != nil {
		// we always attempt to return a valid response
		// with errors contained in GlobalPinInfo
		t.Fatal("did not expect an error")
	}
	pinfo, ok := ginfo.PeerMap[clusters[j].host.ID()]
	if !ok {
		t.Fatal("should have info for this host")
	}
	if pinfo.Error == "" {
		t.Error("pinInfo error should not be empty")
	}

	for _, c := range clusters {
		inf, ok := ginfo.PeerMap[c.host.ID()]
		if !ok {
			t.Fatal("GlobalPinInfo should not be empty for this host")
		}

		if inf.Status != api.TrackerStatusPinError {
			t.Logf("%+v", inf)
			t.Error("should be PinError in all peers")
		}
	}

	// Test with a good Cid
	j = rand.Intn(nClusters)
	ginfo, err = clusters[j].Recover(h2)
	if err != nil {
		t.Fatal(err)
	}
	if ginfo.Cid.String() != test.TestCid2 {
		t.Error("GlobalPinInfo should be for testrCid2")
	}

	for _, c := range clusters {
		inf, ok := ginfo.PeerMap[c.host.ID()]
		if !ok {
			t.Fatal("GlobalPinInfo should have this cluster")
		}
		if inf.Status != api.TrackerStatusPinned {
			t.Error("the GlobalPinInfo should show Pinned in all peers")
		}
	}
}

func TestClustersShutdown(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)

	f := func(t *testing.T, c *Cluster) {
		err := c.Shutdown()
		if err != nil {
			t.Error("should be able to shutdown cleanly")
		}
	}
	// Shutdown 3 times
	runF(t, clusters, f)
	runF(t, clusters, f)
	runF(t, clusters, f)
}

func TestClustersReplication(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = nClusters - 1
		c.config.ReplicationFactorMax = nClusters - 1
	}

	// Why is replication factor nClusters - 1?
	// Because that way we know that pinning nCluster
	// pins with an strategy like numpins/disk
	// will result in each peer holding locally exactly
	// nCluster pins.

	tmpCid, _ := cid.Decode(test.TestCid1)
	prefix := tmpCid.Prefix()

	for i := 0; i < nClusters; i++ {
		// Pick a random cluster and hash
		j := rand.Intn(nClusters)           // choose a random cluster peer
		h, err := prefix.Sum(randomBytes()) // create random cid
		checkErr(t, err)
		err = clusters[j].Pin(api.PinCid(h))
		if err != nil {
			t.Error(err)
		}
		time.Sleep(time.Second)

		// check that it is held by exactly nClusters -1 peers
		gpi, err := clusters[j].Status(h)
		if err != nil {
			t.Fatal(err)
		}

		numLocal := 0
		numRemote := 0
		for _, v := range gpi.PeerMap {
			if v.Status == api.TrackerStatusPinned {
				numLocal++
			} else if v.Status == api.TrackerStatusRemote {
				numRemote++
			}
		}
		if numLocal != nClusters-1 {
			t.Errorf("We wanted replication %d but it's only %d",
				nClusters-1, numLocal)
		}

		if numRemote != 1 {
			t.Errorf("We wanted 1 peer track as remote but %d do", numRemote)
		}
		time.Sleep(time.Second) // this is for metric to be up to date
	}

	f := func(t *testing.T, c *Cluster) {
		pinfos := c.tracker.StatusAll()
		if len(pinfos) != nClusters {
			t.Error("Pinfos does not have the expected pins")
		}
		numRemote := 0
		numLocal := 0
		for _, pi := range pinfos {
			switch pi.Status {
			case api.TrackerStatusPinned:
				numLocal++

			case api.TrackerStatusRemote:
				numRemote++
			}
		}
		if numLocal != nClusters-1 {
			t.Errorf("Expected %d local pins but got %d", nClusters-1, numLocal)
		}

		if numRemote != 1 {
			t.Errorf("Expected 1 remote pin but got %d", numRemote)
		}

		pins := c.Pins()
		for _, pin := range pins {
			allocs := pin.Allocations
			if len(allocs) != nClusters-1 {
				t.Errorf("Allocations are [%s]", allocs)
			}
			for _, a := range allocs {
				if a == c.id {
					pinfo := c.tracker.Status(pin.Cid)
					if pinfo.Status != api.TrackerStatusPinned {
						t.Errorf("Peer %s was allocated but it is not pinning cid", c.id)
					}
				}
			}
		}
	}

	runF(t, clusters, f)
}

// This test checks that we pin with ReplicationFactorMax when
// we can
func TestClustersReplicationFactorMax(t *testing.T) {
	if nClusters < 3 {
		t.Skip("Need at least 3 peers")
	}

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = 1
		c.config.ReplicationFactorMax = nClusters - 1
	}

	h, _ := cid.Decode(test.TestCid1)
	err := clusters[0].Pin(api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	delay()

	f := func(t *testing.T, c *Cluster) {
		p, err := c.PinGet(h)
		if err != nil {
			t.Fatal(err)
		}

		if len(p.Allocations) != nClusters-1 {
			t.Error("should have pinned nClusters - 1 allocations")
		}

		if p.ReplicationFactorMin != 1 {
			t.Error("rplMin should be 1")
		}

		if p.ReplicationFactorMax != nClusters-1 {
			t.Error("rplMax should be nClusters-1")
		}
	}
	runF(t, clusters, f)
}

// This tests checks that repinning something that is overpinned
// removes some allocations
func TestClustersReplicationFactorMaxLower(t *testing.T) {
	if nClusters < 5 {
		t.Skip("Need at least 5 peers")
	}

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = 1
		c.config.ReplicationFactorMax = nClusters
	}

	h, _ := cid.Decode(test.TestCid1)
	err := clusters[0].Pin(api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	delay()

	p1, err := clusters[0].PinGet(h)
	if err != nil {
		t.Fatal(err)
	}

	if len(p1.Allocations) != nClusters {
		t.Fatal("allocations should be nClusters")
	}

	err = clusters[0].Pin(api.Pin{
		Cid:                  h,
		ReplicationFactorMax: 2,
		ReplicationFactorMin: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	delay()

	p2, err := clusters[0].PinGet(h)
	if err != nil {
		t.Fatal(err)
	}

	if len(p2.Allocations) != 2 {
		t.Fatal("allocations should have been reduced to 2")
	}
}

// This test checks that when not all nodes are available,
// we pin in as many as we can aiming for ReplicationFactorMax
func TestClustersReplicationFactorInBetween(t *testing.T) {
	if nClusters < 5 {
		t.Skip("Need at least 5 peers")
	}

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = 1
		c.config.ReplicationFactorMax = nClusters
	}

	// Shutdown two peers
	clusters[nClusters-1].Shutdown()
	clusters[nClusters-2].Shutdown()

	time.Sleep(time.Second) // let metric expire

	waitForLeader(t, clusters)

	// allow metrics to arrive to new leader
	delay()

	h, _ := cid.Decode(test.TestCid1)
	err := clusters[0].Pin(api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	delay()

	f := func(t *testing.T, c *Cluster) {
		if c == clusters[nClusters-1] || c == clusters[nClusters-2] {
			return
		}
		p, err := c.PinGet(h)
		if err != nil {
			t.Fatal(err)
		}

		if len(p.Allocations) != nClusters-2 {
			t.Error("should have pinned nClusters-2 allocations")
		}

		if p.ReplicationFactorMin != 1 {
			t.Error("rplMin should be 1")
		}

		if p.ReplicationFactorMax != nClusters {
			t.Error("rplMax should be nClusters")
		}
	}
	runF(t, clusters, f)
}

// This test checks that we do not pin something for which
// we cannot reach ReplicationFactorMin
func TestClustersReplicationFactorMin(t *testing.T) {
	if nClusters < 5 {
		t.Skip("Need at least 5 peers")
	}

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = nClusters - 1
		c.config.ReplicationFactorMax = nClusters
	}

	// Shutdown two peers
	clusters[nClusters-1].Shutdown()
	clusters[nClusters-2].Shutdown()

	time.Sleep(time.Second) // let metric expire

	waitForLeader(t, clusters)

	// allow metrics to arrive to new leader
	delay()

	h, _ := cid.Decode(test.TestCid1)
	err := clusters[0].Pin(api.PinCid(h))
	if err == nil {
		t.Error("Pin should have failed as rplMin cannot be satisfied")
	}
	t.Log(err)
	if !strings.Contains(err.Error(), fmt.Sprintf("not enough peers to allocate CID")) {
		t.Fatal(err)
	}
}

// This tests checks that repinning something that has becomed
// underpinned actually changes nothing if it's sufficiently pinned
func TestClustersReplicationMinMaxNoRealloc(t *testing.T) {
	if nClusters < 5 {
		t.Skip("Need at least 5 peers")
	}

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = 1
		c.config.ReplicationFactorMax = nClusters
	}

	h, _ := cid.Decode(test.TestCid1)
	err := clusters[0].Pin(api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	// Shutdown two peers
	clusters[nClusters-1].Shutdown()
	clusters[nClusters-2].Shutdown()

	time.Sleep(time.Second) // let metric expire

	waitForLeader(t, clusters)

	// allow metrics to arrive to new leader
	delay()

	err = clusters[0].Pin(api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	p, err := clusters[0].PinGet(h)
	if err != nil {
		t.Fatal(err)
	}

	if len(p.Allocations) != nClusters {
		t.Error("allocations should still be nCluster even if not all available")
	}

	if p.ReplicationFactorMax != nClusters {
		t.Error("rplMax should have not changed")
	}
}

// This test checks that repinning something that has becomed
// underpinned does re-allocations when it's not sufficiently
// pinned anymore
func TestClustersReplicationMinMaxRealloc(t *testing.T) {
	if nClusters < 5 {
		t.Skip("Need at least 5 peers")
	}

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = 3
		c.config.ReplicationFactorMax = 4
	}

	h, _ := cid.Decode(test.TestCid1)
	err := clusters[0].Pin(api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	delay()

	p, err := clusters[0].PinGet(h)
	if err != nil {
		t.Fatal(err)
	}

	firstAllocations := p.Allocations

	peerIDMap := make(map[peer.ID]*Cluster)
	for _, a := range clusters {
		peerIDMap[a.id] = a
	}

	// kill two of the allocations
	alloc1 := peerIDMap[firstAllocations[0]]
	alloc2 := peerIDMap[firstAllocations[1]]
	safePeer := peerIDMap[firstAllocations[2]]

	alloc1.Shutdown()
	alloc2.Shutdown()

	time.Sleep(time.Second) // let metric expire

	waitForLeader(t, clusters)

	// allow metrics to arrive to new leader
	delay()

	// Repin - (although this might have been taken of if there was an alert
	err = safePeer.Pin(api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	p, err = safePeer.PinGet(h)
	if err != nil {
		t.Fatal(err)
	}

	secondAllocations := p.Allocations

	strings1 := api.PeersToStrings(firstAllocations)
	strings2 := api.PeersToStrings(secondAllocations)
	sort.Strings(strings1)
	sort.Strings(strings2)
	t.Logf("Allocs1: %s", strings1)
	t.Logf("Allocs2: %s", strings2)

	if fmt.Sprintf("%s", strings1) == fmt.Sprintf("%s", strings2) {
		t.Error("allocations should have changed")
	}

	lenSA := len(secondAllocations)
	expected := minInt(nClusters-2, 4)
	if lenSA != expected {
		t.Errorf("Inssufficient reallocation, could have allocated to %d peers but instead only allocated to %d peers", expected, lenSA)
	}

	if lenSA < 3 {
		t.Error("allocations should be more than rplMin")
	}
}

// In this test we check that repinning something
// when a node has gone down will re-assign the pin
func TestClustersReplicationRealloc(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = nClusters - 1
		c.config.ReplicationFactorMax = nClusters - 1
	}

	j := rand.Intn(nClusters)
	h, _ := cid.Decode(test.TestCid1)
	err := clusters[j].Pin(api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	// Let the pin arrive
	time.Sleep(time.Second / 2)

	pin := clusters[j].Pins()[0]
	pinSerial := pin.ToSerial()
	allocs := sort.StringSlice(pinSerial.Allocations)
	allocs.Sort()
	allocsStr := fmt.Sprintf("%s", allocs)

	// Re-pin should work and be allocated to the same
	// nodes
	err = clusters[j].Pin(api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	pin2 := clusters[j].Pins()[0]
	pinSerial2 := pin2.ToSerial()
	allocs2 := sort.StringSlice(pinSerial2.Allocations)
	allocs2.Sort()
	allocsStr2 := fmt.Sprintf("%s", allocs2)
	if allocsStr != allocsStr2 {
		t.Fatal("allocations changed without reason")
	}
	//t.Log(allocsStr)
	//t.Log(allocsStr2)

	var killedClusterIndex int
	// find someone that pinned it and kill that cluster
	for i, c := range clusters {
		pinfo := c.tracker.Status(h)
		if pinfo.Status == api.TrackerStatusPinned {
			//t.Logf("Killing %s", c.id.Pretty())
			killedClusterIndex = i
			t.Logf("Shutting down %s", c.ID().ID)
			c.Shutdown()
			break
		}
	}

	// let metrics expire and give time for the cluster to
	// see if they have lost the leader
	time.Sleep(4 * time.Second)
	waitForLeader(t, clusters)
	// wait for new metrics to arrive
	time.Sleep(2 * time.Second)

	// Make sure we haven't killed our randomly
	// selected cluster
	for j == killedClusterIndex {
		j = rand.Intn(nClusters)
	}

	// now pin should succeed
	err = clusters[j].Pin(api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	numPinned := 0
	for i, c := range clusters {
		if i == killedClusterIndex {
			continue
		}
		pinfo := c.tracker.Status(h)
		if pinfo.Status == api.TrackerStatusPinned {
			//t.Log(pinfo.Peer.Pretty())
			numPinned++
		}
	}

	if numPinned != nClusters-1 {
		t.Error("pin should have been correctly re-assigned")
	}
}

// In this test we try to pin something when there are not
// as many available peers a we need. It's like before, except
// more peers are killed.
func TestClustersReplicationNotEnoughPeers(t *testing.T) {
	if nClusters < 5 {
		t.Skip("Need at least 5 peers")
	}
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = nClusters - 1
		c.config.ReplicationFactorMax = nClusters - 1
	}

	j := rand.Intn(nClusters)
	h, _ := cid.Decode(test.TestCid1)
	err := clusters[j].Pin(api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	// Let the pin arrive
	time.Sleep(time.Second / 2)

	clusters[0].Shutdown()
	clusters[1].Shutdown()

	delay()
	waitForLeader(t, clusters)

	err = clusters[2].Pin(api.PinCid(h))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "not enough peers to allocate") {
		t.Error("different error than expected")
		t.Error(err)
	}
	//t.Log(err)
}

func TestClustersRebalanceOnPeerDown(t *testing.T) {
	if nClusters < 5 {
		t.Skip("Need at least 5 peers")
	}

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = nClusters - 1
		c.config.ReplicationFactorMax = nClusters - 1
	}

	// pin something
	h, _ := cid.Decode(test.TestCid1)
	clusters[0].Pin(api.PinCid(h))
	time.Sleep(time.Second * 2) // let the pin arrive
	pinLocal := 0
	pinRemote := 0
	var localPinner peer.ID
	var remotePinner peer.ID
	var remotePinnerCluster *Cluster

	status, _ := clusters[0].Status(h)

	// check it was correctly pinned
	for p, pinfo := range status.PeerMap {
		if pinfo.Status == api.TrackerStatusPinned {
			pinLocal++
			localPinner = p
		} else if pinfo.Status == api.TrackerStatusRemote {
			pinRemote++
			remotePinner = p
		}
	}

	if pinLocal != nClusters-1 || pinRemote != 1 {
		t.Fatal("Not pinned as expected")
	}

	// find a kill the local pinner
	for _, c := range clusters {
		if c.id == localPinner {
			c.Shutdown()
		} else if c.id == remotePinner {
			remotePinnerCluster = c
		}
	}

	// Sleep a monitoring interval
	time.Sleep(6 * time.Second)

	// It should be now pinned in the remote pinner
	if s := remotePinnerCluster.tracker.Status(h).Status; s != api.TrackerStatusPinned {
		t.Errorf("it should be pinned and is %s", s)
	}
}

// Helper function for verifying cluster graph.  Will only pass if exactly the
// peers in clusterIDs are fully connected to each other and the expected ipfs
// mock connectivity exists.  Cluster peers not in clusterIDs are assumed to
// be disconnected and the graph should reflect this
func validateClusterGraph(t *testing.T, graph api.ConnectGraph, clusterIDs map[peer.ID]struct{}) {
	// Check that all cluster peers see each other as peers
	for id1, peers := range graph.ClusterLinks {
		if _, ok := clusterIDs[id1]; !ok {
			if len(peers) != 0 {
				t.Errorf("disconnected peer %s is still connected in graph", id1)
			}
			continue
		}
		fmt.Printf("id: %s, peers: %v\n", id1, peers)
		if len(peers) > len(clusterIDs)-1 {
			t.Errorf("More peers recorded in graph than expected")
		}
		// Make lookup index for peers connected to id1
		peerIndex := make(map[peer.ID]struct{})
		for _, peer := range peers {
			peerIndex[peer] = struct{}{}
		}
		for id2 := range clusterIDs {
			if _, ok := peerIndex[id2]; id1 != id2 && !ok {
				t.Errorf("Expected graph to see peer %s connected to peer %s", id1, id2)
			}
		}
	}
	if len(graph.ClusterLinks) != nClusters {
		t.Errorf("Unexpected number of cluster nodes in graph")
	}

	// Check that all cluster peers are recorded as nodes in the graph
	for id := range clusterIDs {
		if _, ok := graph.ClusterLinks[id]; !ok {
			t.Errorf("Expected graph to record peer %s as a node", id)
		}
	}

	// Check that the mocked ipfs swarm is recorded
	if len(graph.IPFSLinks) != 1 {
		t.Error("Expected exactly one ipfs peer for all cluster nodes, the mocked peer")
	}
	links, ok := graph.IPFSLinks[test.TestPeerID1]
	if !ok {
		t.Error("Expected the mocked ipfs peer to be a node in the graph")
	} else {
		if len(links) != 2 || links[0] != test.TestPeerID4 ||
			links[1] != test.TestPeerID5 {
			t.Error("Swarm peers of mocked ipfs are not those expected")
		}
	}

	// Check that the cluster to ipfs connections are all recorded
	for id := range clusterIDs {
		if ipfsID, ok := graph.ClustertoIPFS[id]; !ok {
			t.Errorf("Expected graph to record peer %s's ipfs connection", id)
		} else {
			if ipfsID != test.TestPeerID1 {
				t.Errorf("Unexpected error %s", ipfsID)
			}
		}
	}
	if len(graph.ClustertoIPFS) > len(clusterIDs) {
		t.Error("More cluster to ipfs links recorded in graph than expected")
	}
}

// In this test we get a cluster graph report from a random peer in a healthy
// fully connected cluster and verify that it is formed as expected.
func TestClustersGraphConnected(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	delay()
	delay()

	j := rand.Intn(nClusters) // choose a random cluster peer to query
	graph, err := clusters[j].ConnectGraph()
	if err != nil {
		t.Fatal(err)
	}

	clusterIDs := make(map[peer.ID]struct{})
	for _, c := range clusters {
		id := c.ID().ID
		clusterIDs[id] = struct{}{}
	}
	validateClusterGraph(t, graph, clusterIDs)
}

// Similar to the previous test we get a cluster graph report from a peer.
// However now 2 peers have been shutdown and so we do not expect to see
// them in the graph
func TestClustersGraphUnhealthy(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	if nClusters < 5 {
		t.Skip("Need at least 5 peers")
	}

	j := rand.Intn(nClusters) // choose a random cluster peer to query
	// chose the clusters to shutdown
	discon1 := -1
	discon2 := -1
	for i := range clusters {
		if i != j {
			if discon1 == -1 {
				discon1 = i
			} else {
				discon2 = i
				break
			}
		}
	}

	clusters[discon1].Shutdown()
	clusters[discon2].Shutdown()
	delay()
	waitForLeader(t, clusters)
	delay()

	graph, err := clusters[j].ConnectGraph()
	if err != nil {
		t.Fatal(err)
	}

	clusterIDs := make(map[peer.ID]struct{})
	for i, c := range clusters {
		if i == discon1 || i == discon2 {
			continue
		}
		id := c.ID().ID
		clusterIDs[id] = struct{}{}
	}
	validateClusterGraph(t, graph, clusterIDs)
}
