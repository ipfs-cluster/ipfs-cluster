package ipfscluster

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/allocator/ascendalloc"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/api/restapi"
	"github.com/ipfs/ipfs-cluster/informer/disk"
	"github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs/ipfs-cluster/monitor/basic"
	"github.com/ipfs/ipfs-cluster/pintracker/maptracker"
	"github.com/ipfs/ipfs-cluster/state"
	"github.com/ipfs/ipfs-cluster/state/mapstate"
	"github.com/ipfs/ipfs-cluster/test"

	crypto "gx/ipfs/QmNiCwBNA8MWDADTFVq1BonUEJbS2SvjAoNkZZrhEwcuUi/go-libp2p-crypto"
	ma "gx/ipfs/QmSWLfmj5frN9xVLMMN846dMDriy5wN5jeghUm7aTW3DAG/go-multiaddr"
	peer "gx/ipfs/QmZcUPvPhD1Xvk6mwijYF8AfR3mG31S1YsEfHG4khrFPRr/go-libp2p-peer"
	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

//TestClusters*
var (
	// number of clusters to create
	nClusters = 6

	// number of pins to pin/unpin/check
	nPins = 500

	// ports
	clusterPort   = 20000
	apiPort       = 20500
	ipfsProxyPort = 21000
)

func init() {
	rand.Seed(time.Now().UnixNano())
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

func createComponents(t *testing.T, i int) (*Config, API, IPFSConnector, state.State, PinTracker, PeerMonitor, PinAllocator, Informer, *test.IpfsMock) {
	mock := test.NewIpfsMock()
	clusterAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", clusterPort+i))
	apiAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", apiPort+i))
	proxyAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", ipfsProxyPort+i))
	nodeAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", mock.Addr, mock.Port))
	priv, pub, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
	checkErr(t, err)
	pid, err := peer.IDFromPublicKey(pub)
	checkErr(t, err)

	cfg, _ := NewDefaultConfig()
	cfg.ID = pid
	cfg.PrivateKey = priv
	cfg.Bootstrap = []ma.Multiaddr{}
	cfg.ClusterAddr = clusterAddr
	cfg.APIAddr = apiAddr
	cfg.IPFSProxyAddr = proxyAddr
	cfg.IPFSNodeAddr = nodeAddr
	cfg.ConsensusDataFolder = "./e2eTestRaft/" + pid.Pretty()
	cfg.LeaveOnShutdown = false
	cfg.ReplicationFactor = -1
	cfg.MonitoringIntervalSeconds = 2

	api, err := restapi.NewRESTAPI(cfg.APIAddr)
	checkErr(t, err)
	ipfs, err := ipfshttp.NewConnector(
		cfg.IPFSNodeAddr,
		cfg.IPFSProxyAddr)
	checkErr(t, err)
	state := mapstate.NewMapState()
	tracker := maptracker.NewMapPinTracker(cfg.ID)
	mon := basic.NewStdPeerMonitor(cfg.MonitoringIntervalSeconds)
	alloc := ascendalloc.NewAllocator()
	disk.MetricTTL = 1 // second
	inf := disk.NewInformer()

	return cfg, api, ipfs, state, tracker, mon, alloc, inf, mock
}

func createCluster(t *testing.T, cfg *Config, api API, ipfs IPFSConnector, state state.State, tracker PinTracker, mon PeerMonitor, alloc PinAllocator, inf Informer) *Cluster {
	cl, err := NewCluster(cfg, api, ipfs, state, tracker, mon, alloc, inf)
	checkErr(t, err)
	<-cl.Ready()
	return cl
}

func createOnePeerCluster(t *testing.T, nth int) (*Cluster, *test.IpfsMock) {
	cfg, api, ipfs, state, tracker, mon, alloc, inf, mock := createComponents(t, nth)
	cl := createCluster(t, cfg, api, ipfs, state, tracker, mon, alloc, inf)
	return cl, mock
}

func createClusters(t *testing.T) ([]*Cluster, []*test.IpfsMock) {
	os.RemoveAll("./e2eTestRaft")
	cfgs := make([]*Config, nClusters, nClusters)
	apis := make([]API, nClusters, nClusters)
	ipfss := make([]IPFSConnector, nClusters, nClusters)
	states := make([]state.State, nClusters, nClusters)
	trackers := make([]PinTracker, nClusters, nClusters)
	mons := make([]PeerMonitor, nClusters, nClusters)
	allocs := make([]PinAllocator, nClusters, nClusters)
	infs := make([]Informer, nClusters, nClusters)
	ipfsMocks := make([]*test.IpfsMock, nClusters, nClusters)
	clusters := make([]*Cluster, nClusters, nClusters)

	clusterPeers := make([]ma.Multiaddr, nClusters, nClusters)
	for i := 0; i < nClusters; i++ {
		cfg, api, ipfs, state, tracker, mon, alloc, inf, mock := createComponents(t, i)
		cfgs[i] = cfg
		apis[i] = api
		ipfss[i] = ipfs
		states[i] = state
		trackers[i] = tracker
		mons[i] = mon
		allocs[i] = alloc
		infs[i] = inf
		ipfsMocks[i] = mock
		addr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s",
			clusterPort+i,
			cfg.ID.Pretty()))
		clusterPeers[i] = addr
	}

	// Set up the cluster using ClusterPeers
	for i := 0; i < nClusters; i++ {
		cfgs[i].ClusterPeers = make([]ma.Multiaddr, nClusters, nClusters)
		for j := 0; j < nClusters; j++ {
			cfgs[i].ClusterPeers[j] = clusterPeers[j]
		}
	}

	// Alternative way of starting using bootstrap
	// for i := 1; i < nClusters; i++ {
	// 	addr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s",
	// 		clusterPort,
	// 		cfgs[0].ID.Pretty()))

	// 	// Use previous cluster  for bootstrapping
	// 	cfgs[i].Bootstrap = []ma.Multiaddr{addr}
	// }

	var wg sync.WaitGroup
	for i := 0; i < nClusters; i++ {
		wg.Add(1)
		go func(i int) {
			clusters[i] = createCluster(t, cfgs[i], apis[i], ipfss[i], states[i], trackers[i], mons[i], allocs[i], infs[i])
			wg.Done()
		}(i)
	}
	wg.Wait()

	// Yet an alternative way using PeerAdd
	// for i := 1; i < nClusters; i++ {
	// 	clusters[0].PeerAdd(clusterAddr(clusters[i]))
	// }
	delay()
	return clusters, ipfsMocks
}

func shutdownClusters(t *testing.T, clusters []*Cluster, m []*test.IpfsMock) {
	for i, c := range clusters {
		m[i].Close()
		err := c.Shutdown()
		if err != nil {
			t.Error(err)
		}
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
	for clusters[j].shutdown {
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
		c.config.ReplicationFactor = nClusters - 1
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
		time.Sleep(time.Second / 2) // this is for metric to be up to date
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

// In this test we check that repinning something
// when a node has gone down will re-assign the pin
func TestClustersReplicationRealloc(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactor = nClusters - 1
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
			c.Shutdown()
			break
		}
	}

	// let metrics expire
	time.Sleep(2 * time.Second)

	waitForLeader(t, clusters)

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
		c.config.ReplicationFactor = nClusters - 1
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
	if !strings.Contains(err.Error(), "enough allocations") {
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
		c.config.ReplicationFactor = nClusters - 1
	}

	// pin something
	h, _ := cid.Decode(test.TestCid1)
	clusters[0].Pin(api.PinCid(h))
	time.Sleep(time.Second / 2) // let the pin arrive
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
