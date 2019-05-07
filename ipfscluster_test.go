package ipfscluster

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	crypto "github.com/libp2p/go-libp2p-crypto"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	dht "github.com/libp2p/go-libp2p-kad-dht"

	"github.com/ipfs/ipfs-cluster/allocator/descendalloc"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/consensus/crdt"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/datastore/badger"
	"github.com/ipfs/ipfs-cluster/datastore/inmem"
	"github.com/ipfs/ipfs-cluster/informer/disk"
	"github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs/ipfs-cluster/monitor/pubsubmon"
	"github.com/ipfs/ipfs-cluster/observations"
	"github.com/ipfs/ipfs-cluster/pintracker/maptracker"
	"github.com/ipfs/ipfs-cluster/pintracker/stateless"
	"github.com/ipfs/ipfs-cluster/test"
	"github.com/ipfs/ipfs-cluster/version"

	ds "github.com/ipfs/go-datastore"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
)

var (
	// number of clusters to create
	nClusters = 5

	// number of pins to pin/unpin/check
	nPins = 100

	logLevel               = "CRITICAL"
	customLogLvlFacilities = logFacilities{}

	ptracker  = "map"
	consensus = "raft"

	testsFolder = "clusterTestsFolder"

	// When testing with fixed ports...
	// clusterPort   = 10000
	// apiPort       = 10100
	// ipfsProxyPort = 10200
)

type logFacilities []string

// String is the method to format the flag's value, part of the flag.Value interface.
func (lg *logFacilities) String() string {
	return fmt.Sprint(*lg)
}

// Set is the method to set the flag value, part of the flag.Value interface.
func (lg *logFacilities) Set(value string) error {
	if len(*lg) > 0 {
		return errors.New("logFacilities flag already set")
	}
	for _, lf := range strings.Split(value, ",") {
		*lg = append(*lg, lf)
	}
	return nil
}

func init() {
	flag.Var(&customLogLvlFacilities, "logfacs", "use -logLevel for only the following log facilities; comma-separated")
	flag.StringVar(&logLevel, "loglevel", logLevel, "default log level for tests")
	flag.IntVar(&nClusters, "nclusters", nClusters, "number of clusters to use")
	flag.IntVar(&nPins, "npins", nPins, "number of pins to pin/unpin/check")
	flag.StringVar(&ptracker, "tracker", ptracker, "tracker implementation")
	flag.StringVar(&consensus, "consensus", consensus, "consensus implementation")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	if len(customLogLvlFacilities) <= 0 {
		for f := range LoggingFacilities {
			SetFacilityLogLevel(f, logLevel)
		}

		for f := range LoggingFacilitiesExtra {
			SetFacilityLogLevel(f, logLevel)
		}
	}

	for _, f := range customLogLvlFacilities {
		if _, ok := LoggingFacilities[f]; ok {
			SetFacilityLogLevel(f, logLevel)
			continue
		}
		if _, ok := LoggingFacilitiesExtra[f]; ok {
			SetFacilityLogLevel(f, logLevel)
			continue
		}
	}
	ReadyTimeout = 11 * time.Second

	// GossipSub needs to heartbeat to discover newly connected hosts
	// This speeds things up a little.
	pubsub.GossipSubHeartbeatInterval = 50 * time.Millisecond
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

func createComponents(
	t *testing.T,
	host host.Host,
	pubsub *pubsub.PubSub,
	dht *dht.IpfsDHT,
	i int,
	staging bool,
) (
	*Config,
	ds.Datastore,
	Consensus,
	[]API,
	IPFSConnector,
	PinTracker,
	PeerMonitor,
	PinAllocator,
	Informer,
	Tracer,
	*test.IpfsMock,
) {
	ctx := context.Background()
	mock := test.NewIpfsMock(t)

	//apiAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", apiPort+i))
	// Bind on port 0
	apiAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	// Bind on Port 0
	// proxyAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", ipfsProxyPort+i))
	proxyAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	nodeAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", mock.Addr, mock.Port))

	peername := fmt.Sprintf("peer_%d", i)

	clusterCfg, apiCfg, ipfsproxyCfg, ipfshttpCfg, badgerCfg, raftCfg, crdtCfg, maptrackerCfg, statelesstrackerCfg, psmonCfg, diskInfCfg, tracingCfg := testingConfigs()

	clusterCfg.ID = host.ID()
	clusterCfg.PrivateKey = host.Peerstore().PrivKey(host.ID())
	clusterCfg.Peername = peername
	clusterCfg.LeaveOnShutdown = false
	clusterCfg.SetBaseDir(filepath.Join(testsFolder, host.ID().Pretty()))

	apiCfg.HTTPListenAddr = apiAddr

	ipfsproxyCfg.ListenAddr = proxyAddr
	ipfsproxyCfg.NodeAddr = nodeAddr

	ipfshttpCfg.NodeAddr = nodeAddr

	raftCfg.DataFolder = filepath.Join(testsFolder, host.ID().Pretty())

	badgerCfg.Folder = filepath.Join(testsFolder, host.ID().Pretty(), "badger")

	api, err := rest.NewAPI(ctx, apiCfg)
	checkErr(t, err)
	ipfsProxy, err := rest.NewAPI(ctx, apiCfg)
	checkErr(t, err)

	ipfs, err := ipfshttp.NewConnector(ipfshttpCfg)
	checkErr(t, err)
	tracker := makePinTracker(t, clusterCfg.ID, maptrackerCfg, statelesstrackerCfg, clusterCfg.Peername)

	alloc := descendalloc.NewAllocator()
	inf, err := disk.NewInformer(diskInfCfg)
	checkErr(t, err)

	store := makeStore(t, badgerCfg)
	cons := makeConsensus(t, store, host, pubsub, dht, raftCfg, staging, crdtCfg)

	var peersF func(context.Context) ([]peer.ID, error)
	if consensus == "raft" {
		peersF = cons.Peers
	}
	mon, err := pubsubmon.New(ctx, psmonCfg, pubsub, peersF)
	checkErr(t, err)

	tracingCfg.ServiceName = peername
	tracer, err := observations.SetupTracing(tracingCfg)
	checkErr(t, err)

	return clusterCfg, store, cons, []API{api, ipfsProxy}, ipfs, tracker, mon, alloc, inf, tracer, mock
}

func makeStore(t *testing.T, badgerCfg *badger.Config) ds.Datastore {
	switch consensus {
	case "crdt":
		dstr, err := badger.New(badgerCfg)
		checkErr(t, err)
		return dstr
	default:
		return inmem.New()
	}
}

func makeConsensus(t *testing.T, store ds.Datastore, h host.Host, psub *pubsub.PubSub, dht *dht.IpfsDHT, raftCfg *raft.Config, staging bool, crdtCfg *crdt.Config) Consensus {
	switch consensus {
	case "raft":
		raftCon, err := raft.NewConsensus(h, raftCfg, store, staging)
		checkErr(t, err)
		return raftCon
	case "crdt":
		crdtCon, err := crdt.New(h, dht, psub, crdtCfg, store)
		checkErr(t, err)
		return crdtCon
	default:
		panic("bad consensus")
	}
}

func makePinTracker(t *testing.T, pid peer.ID, mptCfg *maptracker.Config, sptCfg *stateless.Config, peerName string) PinTracker {
	var ptrkr PinTracker
	switch ptracker {
	case "map":
		ptrkr = maptracker.NewMapPinTracker(mptCfg, pid, peerName)
	case "stateless":
		ptrkr = stateless.New(sptCfg, pid, peerName)
	default:
		panic("bad pintracker")
	}
	return ptrkr
}

func createCluster(t *testing.T, host host.Host, dht *dht.IpfsDHT, clusterCfg *Config, store ds.Datastore, consensus Consensus, apis []API, ipfs IPFSConnector, tracker PinTracker, mon PeerMonitor, alloc PinAllocator, inf Informer, tracer Tracer) *Cluster {
	cl, err := NewCluster(context.Background(), host, dht, clusterCfg, store, consensus, apis, ipfs, tracker, mon, alloc, inf, tracer)
	checkErr(t, err)
	return cl
}

func createOnePeerCluster(t *testing.T, nth int, clusterSecret []byte) (*Cluster, *test.IpfsMock) {
	hosts, pubsubs, dhts := createHosts(t, clusterSecret, 1)
	clusterCfg, store, consensus, api, ipfs, tracker, mon, alloc, inf, tracer, mock := createComponents(t, hosts[0], pubsubs[0], dhts[0], nth, false)
	cl := createCluster(t, hosts[0], dhts[0], clusterCfg, store, consensus, api, ipfs, tracker, mon, alloc, inf, tracer)
	<-cl.Ready()
	return cl, mock
}

func createHosts(t *testing.T, clusterSecret []byte, nClusters int) ([]host.Host, []*pubsub.PubSub, []*dht.IpfsDHT) {
	hosts := make([]host.Host, nClusters, nClusters)
	pubsubs := make([]*pubsub.PubSub, nClusters, nClusters)
	dhts := make([]*dht.IpfsDHT, nClusters, nClusters)

	ctx := context.Background()

	clusterAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	for i := range hosts {
		priv, _, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
		checkErr(t, err)
		h, err := newHost(ctx, clusterSecret, priv, []ma.Multiaddr{clusterAddr})
		checkErr(t, err)

		// DHT needs to be created BEFORE connecting the peers, but
		// bootstrapped AFTER
		d, err := newDHT(ctx, h)
		checkErr(t, err)
		dhts[i] = d

		hosts[i] = routedHost(h, d)

		// Pubsub needs to be created BEFORE connecting the peers,
		// otherwise they are not picked up.
		psub, err := newPubSub(ctx, hosts[i])
		checkErr(t, err)
		pubsubs[i] = psub
	}

	return hosts, pubsubs, dhts
}

func createClusters(t *testing.T) ([]*Cluster, []*test.IpfsMock) {
	ctx := context.Background()
	os.RemoveAll(testsFolder)
	cfgs := make([]*Config, nClusters, nClusters)
	stores := make([]ds.Datastore, nClusters, nClusters)
	cons := make([]Consensus, nClusters, nClusters)
	apis := make([][]API, nClusters, nClusters)
	ipfss := make([]IPFSConnector, nClusters, nClusters)
	trackers := make([]PinTracker, nClusters, nClusters)
	mons := make([]PeerMonitor, nClusters, nClusters)
	allocs := make([]PinAllocator, nClusters, nClusters)
	infs := make([]Informer, nClusters, nClusters)
	tracers := make([]Tracer, nClusters, nClusters)
	ipfsMocks := make([]*test.IpfsMock, nClusters, nClusters)

	clusters := make([]*Cluster, nClusters, nClusters)

	// Uncomment when testing with fixed ports
	// clusterPeers := make([]ma.Multiaddr, nClusters, nClusters)

	hosts, pubsubs, dhts := createHosts(t, testingClusterSecret, nClusters)

	for i := 0; i < nClusters; i++ {
		// staging = true for all except first (i==0)
		cfgs[i], stores[i], cons[i], apis[i], ipfss[i], trackers[i], mons[i], allocs[i], infs[i], tracers[i], ipfsMocks[i] = createComponents(t, hosts[i], pubsubs[i], dhts[i], i, i != 0)
	}

	// Start first node
	clusters[0] = createCluster(t, hosts[0], dhts[0], cfgs[0], stores[0], cons[0], apis[0], ipfss[0], trackers[0], mons[0], allocs[0], infs[0], tracers[0])
	<-clusters[0].Ready()
	bootstrapAddr := clusterAddr(clusters[0])

	// Start the rest and join
	for i := 1; i < nClusters; i++ {
		clusters[i] = createCluster(t, hosts[i], dhts[i], cfgs[i], stores[i], cons[i], apis[i], ipfss[i], trackers[i], mons[i], allocs[i], infs[i], tracers[i])
		err := clusters[i].Join(ctx, bootstrapAddr)
		if err != nil {
			logger.Error(err)
			t.Fatal(err)
		}
		<-clusters[i].Ready()
	}

	// connect all hosts
	for _, h := range hosts {
		for _, h2 := range hosts {
			if h.ID() != h2.ID() {
				h.Peerstore().AddAddrs(h2.ID(), h2.Addrs(), peerstore.PermanentAddrTTL)
				_, err := h.Network().DialPeer(ctx, h2.ID())
				if err != nil {
					t.Log(err)
				}
			}

		}
	}

	// // Bootstrap the DHTs
	dhtCfg := dht.BootstrapConfig{
		Queries: 1,
		Period:  600 * time.Millisecond,
		Timeout: 300 * time.Millisecond,
	}

	for _, d := range dhts {
		d.BootstrapWithConfig(ctx, dhtCfg)
	}

	waitForLeader(t, clusters)

	return clusters, ipfsMocks
}

func clustersHealthy(t *testing.T, clusters []*Cluster) bool {
	t.Helper()
	if len(clusters) == 0 {
		return false
	}
	metrics := clusters[0].monitor.LatestMetrics(context.Background(), clusters[0].informer.Name())
	for _, m := range metrics {
		if m.Expired() {
			return false
		}
	}
	return len(clusters) == len(metrics)
}

func shutdownClusters(t *testing.T, clusters []*Cluster, m []*test.IpfsMock) {
	ctx := context.Background()
	for i, c := range clusters {
		err := c.Shutdown(ctx)
		if err != nil {
			t.Error(err)
		}
		m[i].Close()
	}
	os.RemoveAll(testsFolder)
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

//////////////////////////////////////
// Delay and wait functions
//
// Delays are used in tests to wait for certain events to happen:
//   * ttlDelay() waits for metrics to arrive. If you pin something
//     and your next operation depends on updated metrics, you need to wait
//   * pinDelay() accounts for the time necessary to pin something and for the new
//     log entry to be visible in all cluster peers
//   * delay just sleeps a second or two.
//   * waitForLeader functions make sure there is a raft leader, for example,
//     after killing the leader.
//
// The values for delays are a result of testing and adjusting so tests pass
// in travis, jenkins etc., taking into account the values used in the
// testing configuration (config_test.go).
func delay() {
	var d int
	if nClusters > 10 {
		d = 3000
	} else {
		d = 2000
	}
	time.Sleep(time.Duration(d) * time.Millisecond)
}

func pinDelay() {
	time.Sleep(500 * time.Millisecond)
}

func ttlDelay() {
	diskInfCfg := &disk.Config{}
	diskInfCfg.LoadJSON(testingDiskInfCfg)
	time.Sleep(diskInfCfg.MetricTTL * 3)
}

// Like waitForLeader but letting metrics expire before waiting, and
// waiting for new metrics to arrive afterwards.
func waitForLeaderAndMetrics(t *testing.T, clusters []*Cluster) {
	ttlDelay()
	waitForLeader(t, clusters)
	ttlDelay()
}

// Makes sure there is a leader and everyone knows about it.
func waitForLeader(t *testing.T, clusters []*Cluster) {
	if consensus == "crdt" {
		return // yai
	}
	ctx := context.Background()
	timer := time.NewTimer(time.Minute)
	ticker := time.NewTicker(100 * time.Millisecond)

loop:
	for {
		select {
		case <-timer.C:
			t.Fatal("timed out waiting for a leader")
		case <-ticker.C:
			for _, cl := range clusters {
				if cl.shutdownB {
					continue // skip shutdown clusters
				}
				_, err := cl.consensus.Leader(ctx)
				if err != nil {
					continue loop
				}
			}
			break loop
		}
	}
}

/////////////////////////////////////////

func TestClustersVersion(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	f := func(t *testing.T, c *Cluster) {
		v := c.Version()
		if v != version.Version.String() {
			t.Error("Bad version")
		}
	}
	runF(t, clusters, f)
}

func TestClustersPeers(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)

	delay()

	j := rand.Intn(nClusters) // choose a random cluster peer
	peers := clusters[j].Peers(ctx)

	if len(peers) != nClusters {
		t.Fatal("expected as many peers as clusters")
	}

	clusterIDMap := make(map[peer.ID]*api.ID)
	peerIDMap := make(map[peer.ID]*api.ID)

	for _, c := range clusters {
		id := c.ID(ctx)
		clusterIDMap[id.ID] = id
	}

	for _, p := range peers {
		if p.Error != "" {
			t.Error(p.ID, p.Error)
			continue
		}
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
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	prefix := test.Cid1.Prefix()

	ttlDelay()

	for i := 0; i < nPins; i++ {
		j := rand.Intn(nClusters)           // choose a random cluster peer
		h, err := prefix.Sum(randomBytes()) // create random cid
		checkErr(t, err)
		err = clusters[j].Pin(ctx, api.PinCid(h))
		if err != nil {
			t.Errorf("error pinning %s: %s", h, err)
		}
		// // Test re-pin
		// err = clusters[j].Pin(ctx, api.PinCid(h))
		// if err != nil {
		// 	t.Errorf("error repinning %s: %s", h, err)
		// }
	}
	switch consensus {
	case "crdt":
		time.Sleep(20 * time.Second)
	default:
		delay()
	}
	fpinned := func(t *testing.T, c *Cluster) {
		status := c.tracker.StatusAll(ctx)
		for _, v := range status {
			if v.Status != api.TrackerStatusPinned {
				t.Errorf("%s should have been pinned but it is %s", v.Cid, v.Status)
			}
		}
		if l := len(status); l != nPins {
			t.Errorf("Pinned %d out of %d requests", l, nPins)
		}
	}
	runF(t, clusters, fpinned)

	// Unpin everything
	pinList, err := clusters[0].Pins(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(pinList) != nPins {
		t.Fatalf("pin list has %d but pinned %d", len(pinList), nPins)
	}

	for i := 0; i < len(pinList); i++ {
		// test re-unpin fails
		j := rand.Intn(nClusters) // choose a random cluster peer
		err := clusters[j].Unpin(ctx, pinList[i].Cid)
		if err != nil {
			t.Errorf("error unpinning %s: %s", pinList[i].Cid, err)
		}
	}
	delay()
	for i := 0; i < len(pinList); i++ {
		j := rand.Intn(nClusters) // choose a random cluster peer
		err := clusters[j].Unpin(ctx, pinList[i].Cid)
		if err == nil {
			t.Errorf("expected error re-unpinning %s", pinList[i].Cid)
		}
	}

	delay()
	funpinned := func(t *testing.T, c *Cluster) {
		status := c.tracker.StatusAll(ctx)
		for _, v := range status {
			t.Errorf("%s should have been unpinned but it is %s", v.Cid, v.Status)
		}
	}
	runF(t, clusters, funpinned)
}

func TestClustersStatusAll(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h := test.Cid1
	clusters[0].Pin(ctx, api.PinCid(h))
	pinDelay()
	// Global status
	f := func(t *testing.T, c *Cluster) {
		statuses, err := c.StatusAll(ctx)
		if err != nil {
			t.Error(err)
		}
		if len(statuses) != 1 {
			t.Fatal("bad status. Expected one item")
		}
		if !statuses[0].Cid.Equals(h) {
			t.Error("bad cid in status")
		}
		info := statuses[0].PeerMap
		if len(info) != nClusters {
			t.Error("bad info in status")
		}

		pid := peer.IDB58Encode(c.host.ID())
		if info[pid].Status != api.TrackerStatusPinned {
			t.Error("the hash should have been pinned")
		}

		status, err := c.Status(ctx, h)
		if err != nil {
			t.Error(err)
		}

		pinfo, ok := status.PeerMap[pid]
		if !ok {
			t.Fatal("Host not in status")
		}

		if pinfo.Status != api.TrackerStatusPinned {
			t.Error(pinfo.Error)
			t.Error("the status should show the hash as pinned")
		}
	}
	runF(t, clusters, f)
}

func TestClustersStatusAllWithErrors(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h := test.Cid1
	clusters[0].Pin(ctx, api.PinCid(h))
	pinDelay()

	// shutdown 1 cluster peer
	clusters[1].Shutdown(ctx)
	delay()

	f := func(t *testing.T, c *Cluster) {
		// skip if it's the shutdown peer
		if c.ID(ctx).ID == clusters[1].ID(ctx).ID {
			return
		}

		statuses, err := c.StatusAll(ctx)
		if err != nil {
			t.Error(err)
		}
		if len(statuses) != 1 {
			t.Fatal("bad status. Expected one item")
		}

		// Raft and CRDT behave differently here
		switch consensus {
		case "raft":
			// Raft will have all statuses with one of them
			// being in ERROR because the peer is off

			stts := statuses[0]
			if len(stts.PeerMap) != nClusters {
				t.Error("bad number of peers in status")
			}

			pid := peer.IDB58Encode(clusters[1].id)
			errst := stts.PeerMap[pid]

			if !errst.Cid.Equals(h) {
				t.Error("errored pinInfo should have a good cid")
			}

			if errst.Status != api.TrackerStatusClusterError {
				t.Error("erroring status should be set to ClusterError")
			}

			// now check with Cid status
			status, err := c.Status(ctx, h)
			if err != nil {
				t.Error(err)
			}

			pinfo := status.PeerMap[pid]

			if pinfo.Status != api.TrackerStatusClusterError {
				t.Error("erroring status should be ClusterError")
			}

			if !pinfo.Cid.Equals(h) {
				t.Error("errored status should have a good cid")
			}
		case "crdt":
			// CRDT will not have contacted the offline peer because
			// its metric expired and therefore is not in the
			// peerset.
			if len(statuses[0].PeerMap) != nClusters-1 {
				t.Error("expected a different number of statuses")
			}
		default:
			t.Fatal("bad consensus")

		}

	}
	runF(t, clusters, f)
}

func TestClustersSyncAllLocal(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	clusters[0].Pin(ctx, api.PinCid(test.ErrorCid)) // This cid always fails
	clusters[0].Pin(ctx, api.PinCid(test.Cid2))
	pinDelay()
	pinDelay()

	f := func(t *testing.T, c *Cluster) {
		// Sync bad ID
		infos, err := c.SyncAllLocal(ctx)
		if err != nil {
			// LocalSync() is asynchronous and should not show an
			// error even if Recover() fails.
			t.Error(err)
		}
		if len(infos) != 1 {
			t.Fatalf("expected 1 elem slice, got = %d", len(infos))
		}
		// Last-known state may still be pinning
		if infos[0].Status != api.TrackerStatusPinError && infos[0].Status != api.TrackerStatusPinning {
			t.Errorf("element should be in Pinning or PinError state, got = %v", infos[0].Status)
		}
	}
	// Test Local syncs
	runF(t, clusters, f)
}

func TestClustersSyncLocal(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h := test.ErrorCid
	h2 := test.Cid2
	clusters[0].Pin(ctx, api.PinCid(h))
	clusters[0].Pin(ctx, api.PinCid(h2))
	pinDelay()
	pinDelay()

	f := func(t *testing.T, c *Cluster) {
		info, err := c.SyncLocal(ctx, h)
		if err != nil {
			t.Error(err)
		}
		if info.Status != api.TrackerStatusPinError && info.Status != api.TrackerStatusPinning {
			t.Errorf("element is %s and not PinError", info.Status)
		}

		// Sync good ID
		info, err = c.SyncLocal(ctx, h2)
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
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	clusters[0].Pin(ctx, api.PinCid(test.ErrorCid))
	clusters[0].Pin(ctx, api.PinCid(test.Cid2))
	pinDelay()
	pinDelay()

	j := rand.Intn(nClusters) // choose a random cluster peer
	ginfos, err := clusters[j].SyncAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ginfos) != 1 {
		t.Fatalf("expected globalsync to have 1 elements, got = %d", len(ginfos))
	}
	if !ginfos[0].Cid.Equals(test.ErrorCid) {
		t.Error("expected globalsync to have problems with test.ErrorCid")
	}
	for _, c := range clusters {
		inf, ok := ginfos[0].PeerMap[peer.IDB58Encode(c.host.ID())]
		if !ok {
			t.Fatal("GlobalPinInfo should have this cluster")
		}
		if inf.Status != api.TrackerStatusPinError && inf.Status != api.TrackerStatusPinning {
			t.Error("should be PinError in all peers")
		}
	}
}

func TestClustersSync(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h := test.ErrorCid // This cid always fails
	h2 := test.Cid2
	clusters[0].Pin(ctx, api.PinCid(h))
	clusters[0].Pin(ctx, api.PinCid(h2))
	pinDelay()
	pinDelay()

	j := rand.Intn(nClusters)
	ginfo, err := clusters[j].Sync(ctx, h)
	if err != nil {
		// we always attempt to return a valid response
		// with errors contained in GlobalPinInfo
		t.Fatal("did not expect an error")
	}
	pinfo, ok := ginfo.PeerMap[peer.IDB58Encode(clusters[j].host.ID())]
	if !ok {
		t.Fatal("should have info for this host")
	}
	if pinfo.Error == "" {
		t.Error("pinInfo error should not be empty")
	}

	if !ginfo.Cid.Equals(h) {
		t.Error("GlobalPinInfo should be for test.ErrorCid")
	}

	for _, c := range clusters {
		inf, ok := ginfo.PeerMap[peer.IDB58Encode(c.host.ID())]
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
	ginfo, err = clusters[j].Sync(ctx, h2)
	if err != nil {
		t.Fatal(err)
	}
	if !ginfo.Cid.Equals(h2) {
		t.Error("GlobalPinInfo should be for testrCid2")
	}

	for _, c := range clusters {
		inf, ok := ginfo.PeerMap[peer.IDB58Encode(c.host.ID())]
		if !ok {
			t.Fatal("GlobalPinInfo should have this cluster")
		}
		if inf.Status != api.TrackerStatusPinned {
			t.Error("the GlobalPinInfo should show Pinned in all peers")
		}
	}
}

func TestClustersRecoverLocal(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h := test.ErrorCid // This cid always fails
	h2 := test.Cid2

	ttlDelay()

	clusters[0].Pin(ctx, api.PinCid(h))
	clusters[0].Pin(ctx, api.PinCid(h2))
	pinDelay()
	pinDelay()

	f := func(t *testing.T, c *Cluster) {
		info, err := c.RecoverLocal(ctx, h)
		if err != nil {
			t.Fatal(err)
		}
		// Wait for queue to be processed
		delay()

		info = c.StatusLocal(ctx, h)
		if info.Status != api.TrackerStatusPinError {
			t.Errorf("element is %s and not PinError", info.Status)
		}

		// Recover good ID
		info, err = c.SyncLocal(ctx, h2)
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
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h := test.ErrorCid // This cid always fails
	h2 := test.Cid2

	ttlDelay()

	clusters[0].Pin(ctx, api.PinCid(h))
	clusters[0].Pin(ctx, api.PinCid(h2))

	pinDelay()
	pinDelay()

	j := rand.Intn(nClusters)
	_, err := clusters[j].Recover(ctx, h)
	if err != nil {
		// we always attempt to return a valid response
		// with errors contained in GlobalPinInfo
		t.Fatal("did not expect an error")
	}

	// Wait for queue to be processed
	delay()

	ginfo, err := clusters[j].Status(ctx, h)
	if err != nil {
		t.Fatal(err)
	}

	pinfo, ok := ginfo.PeerMap[peer.IDB58Encode(clusters[j].host.ID())]
	if !ok {
		t.Fatal("should have info for this host")
	}
	if pinfo.Error == "" {
		t.Error("pinInfo error should not be empty")
	}

	for _, c := range clusters {
		inf, ok := ginfo.PeerMap[peer.IDB58Encode(c.host.ID())]
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
	ginfo, err = clusters[j].Recover(ctx, h2)
	if err != nil {
		t.Fatal(err)
	}
	if !ginfo.Cid.Equals(h2) {
		t.Error("GlobalPinInfo should be for testrCid2")
	}

	for _, c := range clusters {
		inf, ok := ginfo.PeerMap[peer.IDB58Encode(c.host.ID())]
		if !ok {
			t.Fatal("GlobalPinInfo should have this cluster")
		}
		if inf.Status != api.TrackerStatusPinned {
			t.Error("the GlobalPinInfo should show Pinned in all peers")
		}
	}
}

func TestClustersShutdown(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)

	f := func(t *testing.T, c *Cluster) {
		err := c.Shutdown(ctx)
		if err != nil {
			t.Error("should be able to shutdown cleanly")
		}
	}
	// Shutdown 3 times
	runF(t, clusters, f)
	runF(t, clusters, f)
	runF(t, clusters, f)
}

func TestClustersReplicationOverall(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = nClusters - 1
		c.config.ReplicationFactorMax = nClusters - 1
	}

	ttlDelay()

	// wait for clusters to stablise
	for !clustersHealthy(t, clusters) {
	}

	// Why is replication factor nClusters - 1?
	// Because that way we know that pinning nCluster
	// pins with an strategy like numpins/disk
	// will result in each peer holding locally exactly
	// nCluster pins.

	prefix := test.Cid1.Prefix()

	for i := 0; i < nClusters; i++ {
		// Pick a random cluster and hash
		j := rand.Intn(nClusters)           // choose a random cluster peer
		h, err := prefix.Sum(randomBytes()) // create random cid
		checkErr(t, err)
		err = clusters[j].Pin(ctx, api.PinCid(h))
		if err != nil {
			t.Error(err)
		}
		pinDelay()

		// check that it is held by exactly nClusters - 1 peers
		gpi, err := clusters[j].Status(ctx, h)
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
			t.Errorf(
				"We wanted replication %d but it's only %d",
				nClusters-1,
				numLocal,
			)
		}

		if numRemote != 1 {
			t.Errorf("We wanted 1 peer track as remote but %d do", numRemote)
		}
		ttlDelay()
	}

	f := func(t *testing.T, c *Cluster) {
		// confirm that the pintracker state matches the current global state
		pinfos := c.tracker.StatusAll(ctx)
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
			t.Errorf("%s: Expected %d local pins but got %d", c.id.String(), nClusters-1, numLocal)
		}

		if numRemote != 1 {
			t.Errorf("%s: Expected 1 remote pin but got %d", c.id.String(), numRemote)
		}

		pins, err := c.Pins(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, pin := range pins {
			allocs := pin.Allocations
			if len(allocs) != nClusters-1 {
				t.Errorf("Allocations are [%s]", allocs)
			}
			for _, a := range allocs {
				if a == c.id {
					pinfo := c.tracker.Status(ctx, pin.Cid)
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
	ctx := context.Background()
	if nClusters < 3 {
		t.Skip("Need at least 3 peers")
	}

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = 1
		c.config.ReplicationFactorMax = nClusters - 1
	}

	ttlDelay()

	h := test.Cid1
	err := clusters[0].Pin(ctx, api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	f := func(t *testing.T, c *Cluster) {
		p, err := c.PinGet(ctx, h)
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
	ctx := context.Background()
	if nClusters < 5 {
		t.Skip("Need at least 5 peers")
	}

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = 1
		c.config.ReplicationFactorMax = nClusters
	}

	ttlDelay() // make sure we have places to pin

	h := test.Cid1
	err := clusters[0].Pin(ctx, api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	p1, err := clusters[0].PinGet(ctx, h)
	if err != nil {
		t.Fatal(err)
	}

	if len(p1.Allocations) != nClusters {
		t.Fatal("allocations should be nClusters")
	}

	pin := api.PinCid(h)
	pin.ReplicationFactorMin = 1
	pin.ReplicationFactorMax = 2
	err = clusters[0].Pin(ctx, pin)
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	p2, err := clusters[0].PinGet(ctx, h)
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
	ctx := context.Background()
	if nClusters < 5 {
		t.Skip("Need at least 5 peers")
	}

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = 1
		c.config.ReplicationFactorMax = nClusters
	}

	ttlDelay()

	// Shutdown two peers
	clusters[nClusters-1].Shutdown(ctx)
	clusters[nClusters-2].Shutdown(ctx)

	waitForLeaderAndMetrics(t, clusters)

	h := test.Cid1
	err := clusters[0].Pin(ctx, api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	f := func(t *testing.T, c *Cluster) {
		if c == clusters[nClusters-1] || c == clusters[nClusters-2] {
			return
		}
		p, err := c.PinGet(ctx, h)
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
	ctx := context.Background()
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
	clusters[nClusters-1].Shutdown(ctx)
	waitForLeaderAndMetrics(t, clusters)
	clusters[nClusters-2].Shutdown(ctx)
	waitForLeaderAndMetrics(t, clusters)

	h := test.Cid1
	err := clusters[0].Pin(ctx, api.PinCid(h))
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
	ctx := context.Background()
	if nClusters < 5 {
		t.Skip("Need at least 5 peers")
	}

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = 1
		c.config.ReplicationFactorMax = nClusters
	}

	ttlDelay()

	h := test.Cid1
	err := clusters[0].Pin(ctx, api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	// Shutdown two peers
	clusters[nClusters-1].Shutdown(ctx)
	waitForLeaderAndMetrics(t, clusters)
	clusters[nClusters-2].Shutdown(ctx)
	waitForLeaderAndMetrics(t, clusters)

	err = clusters[0].Pin(ctx, api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	p, err := clusters[0].PinGet(ctx, h)
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
	ctx := context.Background()
	if nClusters < 5 {
		t.Skip("Need at least 5 peers")
	}

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = 3
		c.config.ReplicationFactorMax = 4
	}

	ttlDelay() // make sure metrics are in

	h := test.Cid1
	err := clusters[0].Pin(ctx, api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	p, err := clusters[0].PinGet(ctx, h)
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

	alloc1.Shutdown(ctx)
	alloc2.Shutdown(ctx)

	waitForLeaderAndMetrics(t, clusters)

	// Repin - (although this might have been taken of if there was an alert
	err = safePeer.Pin(ctx, api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	p, err = safePeer.PinGet(ctx, h)
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
		t.Errorf("Insufficient reallocation, could have allocated to %d peers but instead only allocated to %d peers", expected, lenSA)
	}

	if lenSA < 3 {
		t.Error("allocations should be more than rplMin")
	}
}

// In this test we check that repinning something
// when a node has gone down will re-assign the pin
func TestClustersReplicationRealloc(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = nClusters - 1
		c.config.ReplicationFactorMax = nClusters - 1
	}

	ttlDelay()

	j := rand.Intn(nClusters)
	h := test.Cid1
	err := clusters[j].Pin(ctx, api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	// Let the pin arrive
	pinDelay()

	pinList, err := clusters[j].Pins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pin := pinList[0]
	allocs := sort.StringSlice(api.PeersToStrings(pin.Allocations))
	allocs.Sort()
	allocsStr := fmt.Sprintf("%s", allocs)

	// Re-pin should work and be allocated to the same
	// nodes
	err = clusters[j].Pin(ctx, api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	pinList2, err := clusters[j].Pins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pin2 := pinList2[0]
	allocs2 := sort.StringSlice(api.PeersToStrings(pin2.Allocations))
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
		pinfo := c.tracker.Status(ctx, h)
		if pinfo.Status == api.TrackerStatusPinned {
			//t.Logf("Killing %s", c.id.Pretty())
			killedClusterIndex = i
			t.Logf("Shutting down %s", c.ID(ctx).ID)
			c.Shutdown(ctx)
			break
		}
	}

	// let metrics expire and give time for the cluster to
	// see if they have lost the leader
	waitForLeaderAndMetrics(t, clusters)

	// Make sure we haven't killed our randomly
	// selected cluster
	for j == killedClusterIndex {
		j = rand.Intn(nClusters)
	}

	// now pin should succeed
	err = clusters[j].Pin(ctx, api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	numPinned := 0
	for i, c := range clusters {
		if i == killedClusterIndex {
			continue
		}
		pinfo := c.tracker.Status(ctx, h)
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
	ctx := context.Background()
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
	h := test.Cid1
	err := clusters[j].Pin(ctx, api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	// Let the pin arrive
	pinDelay()

	clusters[0].Shutdown(ctx)
	clusters[1].Shutdown(ctx)

	waitForLeaderAndMetrics(t, clusters)

	err = clusters[2].Pin(ctx, api.PinCid(h))
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
	ctx := context.Background()
	if nClusters < 5 {
		t.Skip("Need at least 5 peers")
	}

	if consensus == "crdt" {
		t.Log("FIXME when re-alloc changes come through")
		return
	}

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = nClusters - 1
		c.config.ReplicationFactorMax = nClusters - 1
	}

	// pin something
	h := test.Cid1
	clusters[0].Pin(ctx, api.PinCid(h))
	pinDelay()
	pinLocal := 0
	pinRemote := 0
	var localPinner string
	var remotePinner string
	var remotePinnerCluster *Cluster

	status, _ := clusters[0].Status(ctx, h)

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

	// kill the local pinner
	for _, c := range clusters {
		clid := peer.IDB58Encode(c.id)
		if clid == localPinner {
			c.Shutdown(ctx)
		} else if clid == remotePinner {
			remotePinnerCluster = c
		}
	}

	delay()
	waitForLeaderAndMetrics(t, clusters) // in case we killed the leader

	// It should be now pinned in the remote pinner
	if s := remotePinnerCluster.tracker.Status(ctx, h).Status; s != api.TrackerStatusPinned {
		t.Errorf("it should be pinned and is %s", s)
	}
}

// Helper function for verifying cluster graph.  Will only pass if exactly the
// peers in clusterIDs are fully connected to each other and the expected ipfs
// mock connectivity exists.  Cluster peers not in clusterIDs are assumed to
// be disconnected and the graph should reflect this
func validateClusterGraph(t *testing.T, graph api.ConnectGraph, clusterIDs map[string]struct{}, peerNum int) {
	// Check that all cluster peers see each other as peers
	for id1, peers := range graph.ClusterLinks {
		if _, ok := clusterIDs[id1]; !ok {
			if len(peers) != 0 {
				t.Errorf("disconnected peer %s is still connected in graph", id1)
			}
			continue
		}
		t.Logf("id: %s, peers: %v\n", id1, peers)
		if len(peers) > len(clusterIDs)-1 {
			t.Errorf("More peers recorded in graph than expected")
		}
		// Make lookup index for peers connected to id1
		peerIndex := make(map[string]struct{})
		for _, p := range peers {
			peerIndex[peer.IDB58Encode(p)] = struct{}{}
		}
		for id2 := range clusterIDs {
			if _, ok := peerIndex[id2]; id1 != id2 && !ok {
				t.Errorf("Expected graph to see peer %s connected to peer %s", id1, id2)
			}
		}
	}
	if len(graph.ClusterLinks) != peerNum {
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
	links, ok := graph.IPFSLinks[peer.IDB58Encode(test.PeerID1)]
	if !ok {
		t.Error("Expected the mocked ipfs peer to be a node in the graph")
	} else {
		if len(links) != 2 || links[0] != test.PeerID4 ||
			links[1] != test.PeerID5 {
			t.Error("Swarm peers of mocked ipfs are not those expected")
		}
	}

	// Check that the cluster to ipfs connections are all recorded
	for id := range clusterIDs {
		if ipfsID, ok := graph.ClustertoIPFS[id]; !ok {
			t.Errorf("Expected graph to record peer %s's ipfs connection", id)
		} else {
			if ipfsID != test.PeerID1 {
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
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)

	j := rand.Intn(nClusters) // choose a random cluster peer to query
	graph, err := clusters[j].ConnectGraph()
	if err != nil {
		t.Fatal(err)
	}

	clusterIDs := make(map[string]struct{})
	for _, c := range clusters {
		id := peer.IDB58Encode(c.ID(ctx).ID)
		clusterIDs[id] = struct{}{}
	}
	validateClusterGraph(t, graph, clusterIDs, nClusters)
}

// Similar to the previous test we get a cluster graph report from a peer.
// However now 2 peers have been shutdown and so we do not expect to see
// them in the graph
func TestClustersGraphUnhealthy(t *testing.T) {
	ctx := context.Background()
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

	clusters[discon1].Shutdown(ctx)
	clusters[discon2].Shutdown(ctx)

	waitForLeaderAndMetrics(t, clusters)

	graph, err := clusters[j].ConnectGraph()
	if err != nil {
		t.Fatal(err)
	}

	clusterIDs := make(map[string]struct{})
	for i, c := range clusters {
		if i == discon1 || i == discon2 {
			continue
		}
		id := peer.IDB58Encode(c.ID(ctx).ID)
		clusterIDs[id] = struct{}{}
	}
	peerNum := nClusters
	switch consensus {
	case "crdt":
		peerNum = nClusters - 2
	}

	validateClusterGraph(t, graph, clusterIDs, peerNum)
}

// Check that the pin is not re-assigned when a node
// that has disabled repinning goes down.
func TestClustersDisabledRepinning(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = nClusters - 1
		c.config.ReplicationFactorMax = nClusters - 1
		c.config.DisableRepinning = true
	}

	ttlDelay()

	j := rand.Intn(nClusters)
	h := test.Cid1
	err := clusters[j].Pin(ctx, api.PinCid(h))
	if err != nil {
		t.Fatal(err)
	}

	// Let the pin arrive
	pinDelay()

	var killedClusterIndex int
	// find someone that pinned it and kill that cluster
	for i, c := range clusters {
		pinfo := c.tracker.Status(ctx, h)
		if pinfo.Status == api.TrackerStatusPinned {
			killedClusterIndex = i
			t.Logf("Shutting down %s", c.ID(ctx).ID)
			c.Shutdown(ctx)
			break
		}
	}

	// let metrics expire and give time for the cluster to
	// see if they have lost the leader
	waitForLeaderAndMetrics(t, clusters)

	// Make sure we haven't killed our randomly
	// selected cluster
	for j == killedClusterIndex {
		j = rand.Intn(nClusters)
	}

	numPinned := 0
	for i, c := range clusters {
		if i == killedClusterIndex {
			continue
		}
		pinfo := c.tracker.Status(ctx, h)
		if pinfo.Status == api.TrackerStatusPinned {
			//t.Log(pinfo.Peer.Pretty())
			numPinned++
		}
	}

	if numPinned != nClusters-2 {
		t.Errorf("expected %d replicas for pin, got %d", nClusters-2, numPinned)
	}
}
