package ipfscluster

import (
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	cid "github.com/ipfs/go-cid"
	crypto "github.com/libp2p/go-libp2p-crypto"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
)

var (
	testCid1      = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq"
	testCid       = testCid1
	testCid2      = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmma"
	testCid3      = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmb"
	errorCid      = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmc"
	testPeerID, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")
)

//TestClusters*
var (
	// number of clusters to create
	nClusters = 5

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

func createComponents(t *testing.T, i int) (*Config, *RESTAPI, *IPFSHTTPConnector, *MapState, *MapPinTracker, *ipfsMock) {
	mock := newIpfsMock()
	clusterAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", clusterPort+i))
	apiAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", apiPort+i))
	proxyAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", ipfsProxyPort+i))
	nodeAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", mock.addr, mock.port))
	priv, pub, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
	checkErr(t, err)
	pid, err := peer.IDFromPublicKey(pub)
	checkErr(t, err)

	cfg, _ := NewDefaultConfig()
	cfg.ID = pid
	cfg.PrivateKey = priv
	cfg.ClusterPeers = []ma.Multiaddr{}
	cfg.ClusterAddr = clusterAddr
	cfg.APIAddr = apiAddr
	cfg.IPFSProxyAddr = proxyAddr
	cfg.IPFSNodeAddr = nodeAddr
	cfg.ConsensusDataFolder = "./e2eTestRaft/" + pid.Pretty()

	api, err := NewRESTAPI(cfg)
	checkErr(t, err)
	ipfs, err := NewIPFSHTTPConnector(cfg)
	checkErr(t, err)
	state := NewMapState()
	tracker := NewMapPinTracker(cfg)

	return cfg, api, ipfs, state, tracker, mock
}

func createCluster(t *testing.T, cfg *Config, api *RESTAPI, ipfs *IPFSHTTPConnector, state *MapState, tracker *MapPinTracker) *Cluster {
	cl, err := NewCluster(cfg, api, ipfs, state, tracker)
	checkErr(t, err)
	<-cl.Ready()
	return cl
}

func createOnePeerCluster(t *testing.T, nth int) (*Cluster, *ipfsMock) {
	cfg, api, ipfs, state, tracker, mock := createComponents(t, nth)
	cl := createCluster(t, cfg, api, ipfs, state, tracker)
	return cl, mock
}

func createClusters(t *testing.T) ([]*Cluster, []*ipfsMock) {
	os.RemoveAll("./e2eTestRaft")
	cfgs := make([]*Config, nClusters, nClusters)
	apis := make([]*RESTAPI, nClusters, nClusters)
	ipfss := make([]*IPFSHTTPConnector, nClusters, nClusters)
	states := make([]*MapState, nClusters, nClusters)
	trackers := make([]*MapPinTracker, nClusters, nClusters)
	ipfsMocks := make([]*ipfsMock, nClusters, nClusters)
	clusters := make([]*Cluster, nClusters, nClusters)

	clusterPeers := make([]ma.Multiaddr, nClusters, nClusters)
	for i := 0; i < nClusters; i++ {
		cfg, api, ipfs, state, tracker, mock := createComponents(t, i)
		cfgs[i] = cfg
		apis[i] = api
		ipfss[i] = ipfs
		states[i] = state
		trackers[i] = tracker
		ipfsMocks[i] = mock
		addr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s",
			clusterPort+i,
			cfg.ID.Pretty()))
		clusterPeers[i] = addr
	}
	for i := 0; i < nClusters; i++ {
		cfgs[i].ClusterPeers = make([]ma.Multiaddr, nClusters, nClusters)
		for j := 0; j < nClusters; j++ {
			cfgs[i].ClusterPeers[j] = clusterPeers[j]
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < nClusters; i++ {
		wg.Add(1)
		go func(i int) {
			clusters[i] = createCluster(t, cfgs[i], apis[i], ipfss[i], states[i], trackers[i])
			wg.Done()
		}(i)
	}
	wg.Wait()
	return clusters, ipfsMocks
}

func shutdownClusters(t *testing.T, clusters []*Cluster, m []*ipfsMock) {
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
	if nClusters > 5 {
		d = 5
	} else {
		d = nClusters
	}
	time.Sleep(time.Duration(d) * time.Second)
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

	clusterIDMap := make(map[peer.ID]ID)
	peerIDMap := make(map[peer.ID]ID)

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
		if !crypto.KeyEqual(id.PublicKey, id2.PublicKey) {
			t.Error("expected same public key")
		}
		if id.IPFS.ID != id2.IPFS.ID {
			t.Error("expected same ipfs daemon ID")
		}
	}
}

func TestClustersPin(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	exampleCid, _ := cid.Decode(testCid)
	prefix := exampleCid.Prefix()
	for i := 0; i < nPins; i++ {
		j := rand.Intn(nClusters)           // choose a random cluster peer
		h, err := prefix.Sum(randomBytes()) // create random cid
		checkErr(t, err)
		err = clusters[j].Pin(h)
		if err != nil {
			t.Errorf("error pinning %s: %s", h, err)
		}
		// Test re-pin
		err = clusters[j].Pin(h)
		if err != nil {
			t.Errorf("error repinning %s: %s", h, err)
		}
	}
	delay()
	fpinned := func(t *testing.T, c *Cluster) {
		status := c.tracker.StatusAll()
		for _, v := range status {
			if v.Status != TrackerStatusPinned {
				t.Errorf("%s should have been pinned but it is %s",
					v.CidStr,
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
		err := clusters[j].Unpin(pinList[i])
		if err != nil {
			t.Errorf("error unpinning %s: %s", pinList[i], err)
		}
		// test re-unpin
		err = clusters[j].Unpin(pinList[i])
		if err != nil {
			t.Errorf("error re-unpinning %s: %s", pinList[i], err)
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
	h, _ := cid.Decode(testCid)
	clusters[0].Pin(h)
	delay()
	// Global status
	f := func(t *testing.T, c *Cluster) {
		statuses, err := c.StatusAll()
		if err != nil {
			t.Error(err)
		}
		if len(statuses) == 0 {
			t.Fatal("bad status. Expected one item")
		}
		if statuses[0].Cid.String() != testCid {
			t.Error("bad cid in status")
		}
		info := statuses[0].PeerMap
		if len(info) != nClusters {
			t.Error("bad info in status")
		}

		if info[c.host.ID()].Status != TrackerStatusPinned {
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

		if pinfo.Status != TrackerStatusPinned {
			t.Error("the status should show the hash as pinned")
		}
	}
	runF(t, clusters, f)
}

func TestClustersSyncAllLocal(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h, _ := cid.Decode(errorCid) // This cid always fails
	h2, _ := cid.Decode(testCid2)
	clusters[0].Pin(h)
	clusters[0].Pin(h2)
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
		if infos[0].Status != TrackerStatusPinError && infos[0].Status != TrackerStatusPinning {
			t.Error("element should be in Pinning or PinError state")
		}
	}
	// Test Local syncs
	runF(t, clusters, f)
}

func TestClustersSyncLocal(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h, _ := cid.Decode(errorCid) // This cid always fails
	h2, _ := cid.Decode(testCid2)
	clusters[0].Pin(h)
	clusters[0].Pin(h2)
	delay()

	f := func(t *testing.T, c *Cluster) {
		info, err := c.SyncLocal(h)
		if err != nil {
			t.Error(err)
		}
		if info.Status != TrackerStatusPinError && info.Status != TrackerStatusPinning {
			t.Errorf("element is %s and not PinError", info.Status)
		}

		// Sync good ID
		info, err = c.SyncLocal(h2)
		if err != nil {
			t.Error(err)
		}
		if info.Status != TrackerStatusPinned {
			t.Error("element should be in Pinned state")
		}
	}
	// Test Local syncs
	runF(t, clusters, f)
}

func TestClustersSyncAll(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h, _ := cid.Decode(errorCid) // This cid always fails
	h2, _ := cid.Decode(testCid2)
	clusters[0].Pin(h)
	clusters[0].Pin(h2)
	delay()

	j := rand.Intn(nClusters) // choose a random cluster peer
	ginfos, err := clusters[j].SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(ginfos) != 1 {
		t.Fatal("expected globalsync to have 1 elements")
	}
	if ginfos[0].Cid.String() != errorCid {
		t.Error("expected globalsync to have problems with errorCid")
	}
	for _, c := range clusters {
		inf, ok := ginfos[0].PeerMap[c.host.ID()]
		if !ok {
			t.Fatal("GlobalPinInfo should have this cluster")
		}
		if inf.Status != TrackerStatusPinError && inf.Status != TrackerStatusPinning {
			t.Error("should be PinError in all peers")
		}
	}
}

func TestClustersSync(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h, _ := cid.Decode(errorCid) // This cid always fails
	h2, _ := cid.Decode(testCid2)
	clusters[0].Pin(h)
	clusters[0].Pin(h2)
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

	if ginfo.Cid.String() != errorCid {
		t.Error("GlobalPinInfo should be for errorCid")
	}

	for _, c := range clusters {
		inf, ok := ginfo.PeerMap[c.host.ID()]
		if !ok {
			t.Logf("%+v", ginfo)
			t.Fatal("GlobalPinInfo should not be empty for this host")
		}

		if inf.Status != TrackerStatusPinError && inf.Status != TrackerStatusPinning {
			t.Error("should be PinError or Pinning in all peers")
		}
	}

	// Test with a good Cid
	j = rand.Intn(nClusters)
	ginfo, err = clusters[j].Sync(h2)
	if err != nil {
		t.Fatal(err)
	}
	if ginfo.Cid.String() != testCid2 {
		t.Error("GlobalPinInfo should be for testrCid2")
	}

	for _, c := range clusters {
		inf, ok := ginfo.PeerMap[c.host.ID()]
		if !ok {
			t.Fatal("GlobalPinInfo should have this cluster")
		}
		if inf.Status != TrackerStatusPinned {
			t.Error("the GlobalPinInfo should show Pinned in all peers")
		}
	}
}

func TestClustersRecoverLocal(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h, _ := cid.Decode(errorCid) // This cid always fails
	h2, _ := cid.Decode(testCid2)
	clusters[0].Pin(h)
	clusters[0].Pin(h2)

	delay()

	f := func(t *testing.T, c *Cluster) {
		info, err := c.RecoverLocal(h)
		if err == nil {
			t.Error("expected an error recovering")
		}
		if info.Status != TrackerStatusPinError {
			t.Errorf("element is %s and not PinError", info.Status)
		}

		// Recover good ID
		info, err = c.SyncLocal(h2)
		if err != nil {
			t.Error(err)
		}
		if info.Status != TrackerStatusPinned {
			t.Error("element should be in Pinned state")
		}
	}
	// Test Local syncs
	runF(t, clusters, f)
}

func TestClustersRecover(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h, _ := cid.Decode(errorCid) // This cid always fails
	h2, _ := cid.Decode(testCid2)
	clusters[0].Pin(h)
	clusters[0].Pin(h2)

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
			t.Logf("%+v", ginfo)
			t.Fatal("GlobalPinInfo should not be empty for this host")
		}

		if inf.Status != TrackerStatusPinError {
			t.Error("should be PinError in all peers")
		}
	}

	// Test with a good Cid
	j = rand.Intn(nClusters)
	ginfo, err = clusters[j].Recover(h2)
	if err != nil {
		t.Fatal(err)
	}
	if ginfo.Cid.String() != testCid2 {
		t.Error("GlobalPinInfo should be for testrCid2")
	}

	for _, c := range clusters {
		inf, ok := ginfo.PeerMap[c.host.ID()]
		if !ok {
			t.Fatal("GlobalPinInfo should have this cluster")
		}
		if inf.Status != TrackerStatusPinned {
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
