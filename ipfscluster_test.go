package ipfscluster

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/allocator/balanced"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/api/rest"
	"github.com/ipfs-cluster/ipfs-cluster/consensus/crdt"
	"github.com/ipfs-cluster/ipfs-cluster/consensus/raft"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/badger"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/badger3"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/inmem"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/leveldb"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/pebble"
	"github.com/ipfs-cluster/ipfs-cluster/informer/disk"
	"github.com/ipfs-cluster/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs-cluster/ipfs-cluster/monitor/pubsubmon"
	"github.com/ipfs-cluster/ipfs-cluster/observations"
	"github.com/ipfs-cluster/ipfs-cluster/pintracker/stateless"
	"github.com/ipfs-cluster/ipfs-cluster/state"
	"github.com/ipfs-cluster/ipfs-cluster/test"
	"github.com/ipfs-cluster/ipfs-cluster/version"

	ds "github.com/ipfs/go-datastore"
	libp2p "github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	dual "github.com/libp2p/go-libp2p-kad-dht/dual"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	host "github.com/libp2p/go-libp2p/core/host"
	peer "github.com/libp2p/go-libp2p/core/peer"
	peerstore "github.com/libp2p/go-libp2p/core/peerstore"
	routedhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	ma "github.com/multiformats/go-multiaddr"
)

var (
	// number of clusters to create
	nClusters = 5

	// number of pins to pin/unpin/check
	nPins = 100

	logLevel               = "FATAL"
	customLogLvlFacilities = logFacilities{}

	consensus = "crdt"
	datastore = "pebble"

	ttlDelayTime = 2 * time.Second // set on Main to diskInf.MetricTTL
	testsFolder  = "clusterTestsFolder"

	// When testing with fixed ports...
	// clusterPort   = 10000
	// apiPort       = 10100
	// ipfsProxyPort = 10200

	mrand = rand.New(rand.NewSource(time.Now().UnixNano()))
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

// TestMain runs test initialization. Since Go1.13 we cannot run this on init()
// as flag.Parse() does not work well there
// (see https://golang.org/src/testing/testing.go#L211)
func TestMain(m *testing.M) {
	ReadyTimeout = 11 * time.Second

	// GossipSub needs to heartbeat to discover newly connected hosts
	// This speeds things up a little.
	pubsub.GossipSubHeartbeatInterval = 50 * time.Millisecond

	flag.Var(&customLogLvlFacilities, "logfacs", "use -logLevel for only the following log facilities; comma-separated")
	flag.StringVar(&logLevel, "loglevel", logLevel, "default log level for tests")
	flag.IntVar(&nClusters, "nclusters", nClusters, "number of clusters to use")
	flag.IntVar(&nPins, "npins", nPins, "number of pins to pin/unpin/check")
	flag.StringVar(&consensus, "consensus", consensus, "consensus implementation")
	flag.StringVar(&datastore, "datastore", datastore, "datastore backend")
	flag.Parse()

	if len(customLogLvlFacilities) <= 0 {
		for f := range LoggingFacilities {
			SetFacilityLogLevel(f, logLevel)
		}

		for f := range LoggingFacilitiesExtra {
			SetFacilityLogLevel(f, logLevel)
		}
	}

	for _, f := range customLogLvlFacilities {
		SetFacilityLogLevel(f, logLevel)
	}

	diskInfCfg := &disk.Config{}
	diskInfCfg.LoadJSON(testingDiskInfCfg)
	ttlDelayTime = diskInfCfg.MetricTTL * 2

	os.Exit(m.Run())
}

func randomBytes() []byte {
	bs := make([]byte, 64)
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
	dht *dual.DHT,
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

	ident, clusterCfg, apiCfg, ipfsproxyCfg, ipfshttpCfg, badgerCfg, badger3Cfg, levelDBCfg, pebbleCfg, raftCfg, crdtCfg, statelesstrackerCfg, psmonCfg, allocBalancedCfg, diskInfCfg, tracingCfg := testingConfigs()

	ident.ID = host.ID()
	ident.PrivateKey = host.Peerstore().PrivKey(host.ID())
	clusterCfg.Peername = peername
	clusterCfg.LeaveOnShutdown = false
	clusterCfg.SetBaseDir(filepath.Join(testsFolder, host.ID().String()))

	apiCfg.HTTPListenAddr = []ma.Multiaddr{apiAddr}

	ipfsproxyCfg.ListenAddr = []ma.Multiaddr{proxyAddr}
	ipfsproxyCfg.NodeAddr = nodeAddr

	ipfshttpCfg.NodeAddr = nodeAddr

	raftCfg.DataFolder = filepath.Join(testsFolder, host.ID().String())

	badgerCfg.Folder = filepath.Join(testsFolder, host.ID().String(), "badger")
	badger3Cfg.Folder = filepath.Join(testsFolder, host.ID().String(), "badger3")
	levelDBCfg.Folder = filepath.Join(testsFolder, host.ID().String(), "leveldb")
	pebbleCfg.Folder = filepath.Join(testsFolder, host.ID().String(), "pebble")

	api, err := rest.NewAPI(ctx, apiCfg)
	if err != nil {
		t.Fatal(err)
	}

	ipfsProxy, err := rest.NewAPI(ctx, apiCfg)
	if err != nil {
		t.Fatal(err)
	}

	ipfs, err := ipfshttp.NewConnector(ipfshttpCfg)
	if err != nil {
		t.Fatal(err)
	}

	alloc, err := balanced.New(allocBalancedCfg)
	if err != nil {
		t.Fatal(err)
	}
	inf, err := disk.NewInformer(diskInfCfg)
	if err != nil {
		t.Fatal(err)
	}

	store := makeStore(t, badgerCfg, badger3Cfg, levelDBCfg, pebbleCfg)
	cons := makeConsensus(t, store, host, pubsub, dht, raftCfg, staging, crdtCfg)
	tracker := stateless.New(statelesstrackerCfg, ident.ID, clusterCfg.Peername, cons.State)

	var peersF func(context.Context) ([]peer.ID, error)
	if consensus == "raft" {
		peersF = cons.Peers
	}
	mon, err := pubsubmon.New(ctx, psmonCfg, pubsub, peersF)
	if err != nil {
		t.Fatal(err)
	}
	tracingCfg.ServiceName = peername
	tracer, err := observations.SetupTracing(tracingCfg)
	if err != nil {
		t.Fatal(err)
	}

	return clusterCfg, store, cons, []API{api, ipfsProxy}, ipfs, tracker, mon, alloc, inf, tracer, mock
}

func makeStore(t *testing.T, badgerCfg *badger.Config, badger3Cfg *badger3.Config, levelDBCfg *leveldb.Config, pebbleCfg *pebble.Config) ds.Datastore {
	switch consensus {
	case "crdt":
		switch datastore {
		case "badger":
			dstr, err := badger.New(badgerCfg)
			if err != nil {
				t.Fatal(err)
			}
			return dstr
		case "badger3":
			dstr, err := badger3.New(badger3Cfg)
			if err != nil {
				t.Fatal(err)
			}
			return dstr
		case "leveldb":
			dstr, err := leveldb.New(levelDBCfg)
			if err != nil {
				t.Fatal(err)
			}
			return dstr
		case "pebble":
			dstr, err := pebble.New(pebbleCfg)
			if err != nil {
				t.Fatal(err)
			}
			return dstr
		default:
			t.Fatal("bad datastore")
			return nil
		}

	default:
		return inmem.New()
	}
}

func makeConsensus(t *testing.T, store ds.Datastore, h host.Host, psub *pubsub.PubSub, dht *dual.DHT, raftCfg *raft.Config, staging bool, crdtCfg *crdt.Config) Consensus {
	switch consensus {
	case "raft":
		raftCon, err := raft.NewConsensus(h, raftCfg, store, staging)
		if err != nil {
			t.Fatal(err)
		}
		return raftCon
	case "crdt":
		crdtCon, err := crdt.New(h, dht, psub, crdtCfg, store)
		if err != nil {
			t.Fatal(err)
		}
		return crdtCon
	default:
		panic("bad consensus")
	}
}

func createCluster(t *testing.T, host host.Host, dht *dual.DHT, clusterCfg *Config, store ds.Datastore, consensus Consensus, apis []API, ipfs IPFSConnector, tracker PinTracker, mon PeerMonitor, alloc PinAllocator, inf Informer, tracer Tracer) *Cluster {
	cl, err := NewCluster(context.Background(), host, nil, dht, clusterCfg, store, consensus, apis, ipfs, tracker, mon, alloc, []Informer{inf}, tracer)
	if err != nil {
		t.Fatal(err)
	}
	return cl
}

func createOnePeerCluster(t *testing.T, nth int, clusterSecret []byte) (*Cluster, *test.IpfsMock) {
	hosts, pubsubs, dhts := createHosts(t, clusterSecret, 1)
	clusterCfg, store, consensus, api, ipfs, tracker, mon, alloc, inf, tracer, mock := createComponents(t, hosts[0], pubsubs[0], dhts[0], nth, false)
	cl := createCluster(t, hosts[0], dhts[0], clusterCfg, store, consensus, api, ipfs, tracker, mon, alloc, inf, tracer)
	<-cl.Ready()
	return cl, mock
}

func createHosts(t *testing.T, clusterSecret []byte, nClusters int) ([]host.Host, []*pubsub.PubSub, []*dual.DHT) {
	hosts := make([]host.Host, nClusters)
	pubsubs := make([]*pubsub.PubSub, nClusters)
	dhts := make([]*dual.DHT, nClusters)

	tcpaddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	//quicAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic")
	for i := range hosts {
		priv, _, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
		if err != nil {
			t.Fatal(err)
		}

		h, p, d := createHost(t, priv, clusterSecret, []ma.Multiaddr{tcpaddr})
		hosts[i] = h
		dhts[i] = d
		pubsubs[i] = p
	}

	return hosts, pubsubs, dhts
}

func createHost(t *testing.T, priv crypto.PrivKey, clusterSecret []byte, listen []ma.Multiaddr) (host.Host, *pubsub.PubSub, *dual.DHT) {
	ctx := context.Background()

	h, err := newHost(ctx, clusterSecret, priv, libp2p.ListenAddrs(listen...))
	if err != nil {
		t.Fatal(err)
	}

	// DHT needs to be created BEFORE connecting the peers
	d, err := newTestDHT(ctx, h)
	if err != nil {
		t.Fatal(err)
	}

	// Pubsub needs to be created BEFORE connecting the peers,
	// otherwise they are not picked up.
	// Note: this is possibly a pubsub bug that was fixed.
	cfg := &Config{}
	cfg.Default()
	cfg.LoadJSON(testingClusterCfg)
	psub, err := newPubSub(ctx, cfg, h)
	if err != nil {
		t.Fatal(err)
	}

	return routedhost.Wrap(h, d), psub, d
}

func newTestDHT(ctx context.Context, h host.Host) (*dual.DHT, error) {
	return newDHT(ctx, h, nil,
		dual.DHTOption(dht.RoutingTableRefreshPeriod(600*time.Millisecond)),
		dual.DHTOption(dht.RoutingTableRefreshQueryTimeout(300*time.Millisecond)),
		dual.LanDHTOption(dht.AddressFilter(nil)),
	)
}

func createClusters(t *testing.T) ([]*Cluster, []*test.IpfsMock) {
	ctx := context.Background()
	os.RemoveAll(testsFolder)
	cfgs := make([]*Config, nClusters)
	stores := make([]ds.Datastore, nClusters)
	cons := make([]Consensus, nClusters)
	apis := make([][]API, nClusters)
	ipfss := make([]IPFSConnector, nClusters)
	trackers := make([]PinTracker, nClusters)
	mons := make([]PeerMonitor, nClusters)
	allocs := make([]PinAllocator, nClusters)
	infs := make([]Informer, nClusters)
	tracers := make([]Tracer, nClusters)
	ipfsMocks := make([]*test.IpfsMock, nClusters)

	clusters := make([]*Cluster, nClusters)

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

	waitForLeader(t, clusters)
	waitForClustersHealthy(t, clusters)

	return clusters, ipfsMocks
}

func shutdownClusters(t *testing.T, clusters []*Cluster, m []*test.IpfsMock) {
	for i, c := range clusters {
		shutdownCluster(t, c, m[i])
	}
	os.RemoveAll(testsFolder)
}

func shutdownCluster(t *testing.T, c *Cluster, m *test.IpfsMock) {
	err := c.Shutdown(context.Background())
	if err != nil {
		t.Error(err)
	}
	c.dht.Close()
	c.host.Close()
	c.datastore.Close()
	m.Close()
}

func collectGlobalPinInfos(t *testing.T, out <-chan api.GlobalPinInfo, timeout time.Duration) []api.GlobalPinInfo {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var gpis []api.GlobalPinInfo
	for {
		select {
		case <-ctx.Done():
			t.Error(ctx.Err())
			return gpis
		case gpi, ok := <-out:
			if !ok {
				return gpis
			}
			gpis = append(gpis, gpi)
		}
	}
}

func collectPinInfos(t *testing.T, out <-chan api.PinInfo) []api.PinInfo {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var pis []api.PinInfo
	for {
		select {
		case <-ctx.Done():
			t.Error(ctx.Err())
			return pis
		case pi, ok := <-out:
			if !ok {
				return pis
			}
			pis = append(pis, pi)
		}
	}
}

func runF(t *testing.T, clusters []*Cluster, f func(*testing.T, *Cluster)) {
	t.Helper()
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

// ////////////////////////////////////
// Delay and wait functions
//
// Delays are used in tests to wait for certain events to happen:
//   - ttlDelay() waits for metrics to arrive. If you pin something
//     and your next operation depends on updated metrics, you need to wait
//   - pinDelay() accounts for the time necessary to pin something and for the new
//     log entry to be visible in all cluster peers
//   - delay just sleeps a second or two.
//   - waitForLeader functions make sure there is a raft leader, for example,
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
	time.Sleep(800 * time.Millisecond)
}

func ttlDelay() {
	time.Sleep(ttlDelayTime)
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

func waitForClustersHealthy(t *testing.T, clusters []*Cluster) {
	t.Helper()
	if len(clusters) == 0 {
		return
	}

	timer := time.NewTimer(15 * time.Second)
	for {
		ttlDelay()
		metrics := clusters[0].monitor.LatestMetrics(context.Background(), clusters[0].informers[0].Name())
		healthy := 0
		for _, m := range metrics {
			if !m.Expired() {
				healthy++
			}
		}
		if len(clusters) == healthy {
			return
		}

		select {
		case <-timer.C:
			t.Fatal("timed out waiting for clusters to be healthy")
		default:
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

	j := mrand.Intn(nClusters) // choose a random cluster peer

	out := make(chan api.ID, len(clusters))
	clusters[j].Peers(ctx, out)

	if len(out) != nClusters {
		t.Fatal("expected as many peers as clusters")
	}

	clusterIDMap := make(map[peer.ID]api.ID)
	peerIDMap := make(map[peer.ID]api.ID)

	for _, c := range clusters {
		id := c.ID(ctx)
		clusterIDMap[id.ID] = id
	}

	for p := range out {
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
		j := mrand.Intn(nClusters)          // choose a random cluster peer
		h, err := prefix.Sum(randomBytes()) // create random cid
		if err != nil {
			t.Fatal(err)
		}
		_, err = clusters[j].Pin(ctx, api.NewCid(h), api.PinOptions{})
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
		time.Sleep(10 * time.Second)
	default:
		delay()
	}
	fpinned := func(t *testing.T, c *Cluster) {
		out := make(chan api.PinInfo, 10)

		go func() {
			err := c.tracker.StatusAll(ctx, api.TrackerStatusUndefined, out)
			if err != nil {
				t.Error(err)
			}
		}()

		status := collectPinInfos(t, out)

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
	pinList, err := clusters[0].pinsSlice(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(pinList) != nPins {
		t.Fatalf("pin list has %d but pinned %d", len(pinList), nPins)
	}

	for i := 0; i < len(pinList); i++ {
		// test re-unpin fails
		j := mrand.Intn(nClusters) // choose a random cluster peer
		_, err := clusters[j].Unpin(ctx, pinList[i].Cid)
		if err != nil {
			t.Errorf("error unpinning %s: %s", pinList[i].Cid, err)
		}
	}

	switch consensus {
	case "crdt":
		time.Sleep(10 * time.Second)
	default:
		delay()
	}

	for i := 0; i < len(pinList); i++ {
		j := mrand.Intn(nClusters) // choose a random cluster peer
		_, err := clusters[j].Unpin(ctx, pinList[i].Cid)
		if err == nil {
			t.Errorf("expected error re-unpinning %s", pinList[i].Cid)
		}
	}

	delay()

	funpinned := func(t *testing.T, c *Cluster) {
		out := make(chan api.PinInfo)
		go func() {
			err := c.tracker.StatusAll(ctx, api.TrackerStatusUndefined, out)
			if err != nil {
				t.Error(err)
			}
		}()

		status := collectPinInfos(t, out)
		for _, v := range status {
			t.Errorf("%s should have been unpinned but it is %s", v.Cid, v.Status)
		}
	}
	runF(t, clusters, funpinned)
}

func TestClustersPinUpdate(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	prefix := test.Cid1.Prefix()

	ttlDelay()

	h, _ := prefix.Sum(randomBytes())  // create random cid
	h2, _ := prefix.Sum(randomBytes()) // create random cid

	_, err := clusters[0].PinUpdate(ctx, api.NewCid(h), api.NewCid(h2), api.PinOptions{})
	if err == nil || err != state.ErrNotFound {
		t.Fatal("pin update should fail when from is not pinned")
	}

	_, err = clusters[0].Pin(ctx, api.NewCid(h), api.PinOptions{})
	if err != nil {
		t.Errorf("error pinning %s: %s", h, err)
	}

	pinDelay()
	expiry := time.Now().AddDate(1, 0, 0)
	opts2 := api.PinOptions{
		UserAllocations: []peer.ID{clusters[0].host.ID()}, // should not be used
		PinUpdate:       api.NewCid(h),
		Name:            "new name",
		ExpireAt:        expiry,
	}

	_, err = clusters[0].Pin(ctx, api.NewCid(h2), opts2) // should call PinUpdate
	if err != nil {
		t.Errorf("error pin-updating %s: %s", h2, err)
	}

	pinDelay()

	f := func(t *testing.T, c *Cluster) {
		pinget, err := c.PinGet(ctx, api.NewCid(h2))
		if err != nil {
			t.Fatal(err)
		}

		if len(pinget.Allocations) != 0 {
			t.Error("new pin should be allocated everywhere like pin1")
		}

		if pinget.MaxDepth != -1 {
			t.Error("updated pin should be recursive like pin1")
		}
		// We compare Unix seconds because our protobuf serde will have
		// lost any sub-second precision.
		if pinget.ExpireAt.Unix() != expiry.Unix() {
			t.Errorf("Expiry didn't match. Expected: %s. Got: %s", expiry, pinget.ExpireAt)
		}

		if pinget.Name != "new name" {
			t.Error("name should be kept")
		}
	}
	runF(t, clusters, f)
}

func TestClustersPinDirect(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	prefix := test.Cid1.Prefix()

	ttlDelay()

	h, _ := prefix.Sum(randomBytes()) // create random cid

	_, err := clusters[0].Pin(ctx, api.NewCid(h), api.PinOptions{Mode: api.PinModeDirect})
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	f := func(t *testing.T, c *Cluster, mode api.PinMode) {
		pinget, err := c.PinGet(ctx, api.NewCid(h))
		if err != nil {
			t.Fatal(err)
		}

		if pinget.Mode != mode {
			t.Error("pin should be pinned in direct mode")
		}

		if pinget.MaxDepth != mode.ToPinDepth() {
			t.Errorf("pin should have max-depth %d but has %d", mode.ToPinDepth(), pinget.MaxDepth)
		}

		pInfo := c.StatusLocal(ctx, api.NewCid(h))
		if pInfo.Error != "" {
			t.Error(pInfo.Error)
		}
		if pInfo.Status != api.TrackerStatusPinned {
			t.Error(pInfo.Error)
			t.Error("the status should show the hash as pinned")
		}
	}

	runF(t, clusters, func(t *testing.T, c *Cluster) {
		f(t, c, api.PinModeDirect)
	})

	// Convert into a recursive mode
	_, err = clusters[0].Pin(ctx, api.NewCid(h), api.PinOptions{Mode: api.PinModeRecursive})
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	runF(t, clusters, func(t *testing.T, c *Cluster) {
		f(t, c, api.PinModeRecursive)
	})

	// This should fail as we cannot convert back to direct
	_, err = clusters[0].Pin(ctx, api.NewCid(h), api.PinOptions{Mode: api.PinModeDirect})
	if err == nil {
		t.Error("a recursive pin cannot be converted back to direct pin")
	}
}

func TestClustersStatusAll(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h := test.Cid1
	clusters[0].Pin(ctx, h, api.PinOptions{Name: "test"})
	pinDelay()
	// Global status
	f := func(t *testing.T, c *Cluster) {
		out := make(chan api.GlobalPinInfo, 10)
		go func() {
			err := c.StatusAll(ctx, api.TrackerStatusUndefined, out)
			if err != nil {
				t.Error(err)
			}
		}()

		statuses := collectGlobalPinInfos(t, out, 5*time.Second)
		if len(statuses) != 1 {
			t.Fatal("bad status. Expected one item")
		}
		if !statuses[0].Cid.Equals(h) {
			t.Error("bad cid in status")
		}

		if statuses[0].Name != "test" {
			t.Error("globalPinInfo should have the name")
		}

		info := statuses[0].PeerMap
		if len(info) != nClusters {
			t.Error("bad info in status")
		}

		for _, pi := range info {
			if pi.IPFS != test.PeerID1 {
				t.Error("ipfs not set in pin status")
			}
		}

		pid := c.host.ID().String()
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
	clusters[0].Pin(ctx, h, api.PinOptions{Name: "test"})
	pinDelay()

	// shutdown 1 cluster peer
	clusters[1].Shutdown(ctx)
	clusters[1].host.Close()
	delay()

	f := func(t *testing.T, c *Cluster) {
		// skip if it's the shutdown peer
		if c.ID(ctx).ID == clusters[1].ID(ctx).ID {
			return
		}

		out := make(chan api.GlobalPinInfo, 10)
		go func() {
			err := c.StatusAll(ctx, api.TrackerStatusUndefined, out)
			if err != nil {
				t.Error(err)
			}
		}()

		statuses := collectGlobalPinInfos(t, out, 5*time.Second)

		if len(statuses) != 1 {
			t.Fatal("bad status. Expected one item")
		}

		if !statuses[0].Cid.Equals(h) {
			t.Error("wrong Cid in globalPinInfo")
		}

		if statuses[0].Name != "test" {
			t.Error("wrong Name in globalPinInfo")
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

			pid := clusters[1].id.String()
			errst := stts.PeerMap[pid]

			if errst.Status != api.TrackerStatusClusterError {
				t.Error("erroring status should be set to ClusterError:", errst.Status)
			}
			if errst.PeerName != "peer_1" {
				t.Error("peername should have been set in the erroring peer too from the cache")
			}

			if errst.IPFS != test.PeerID1 {
				t.Error("IPFS ID should have been set in the erroring peer too from the cache")
			}

			// now check with Cid status
			status, err := c.Status(ctx, h)
			if err != nil {
				t.Error(err)
			}

			pinfo := status.PeerMap[pid]

			if pinfo.Status != api.TrackerStatusClusterError {
				t.Error("erroring status should be ClusterError:", pinfo.Status)
			}

			if pinfo.PeerName != "peer_1" {
				t.Error("peername should have been set in the erroring peer too from the cache")
			}

			if pinfo.IPFS != test.PeerID1 {
				t.Error("IPFS ID should have been set in the erroring peer too from the cache")
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

func TestClustersRecoverLocal(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h := test.ErrorCid // This cid always fails
	h2 := test.Cid2

	ttlDelay()

	clusters[0].Pin(ctx, h, api.PinOptions{})
	clusters[0].Pin(ctx, h2, api.PinOptions{})
	pinDelay()
	pinDelay()

	f := func(t *testing.T, c *Cluster) {
		_, err := c.RecoverLocal(ctx, h)
		if err != nil {
			t.Fatal(err)
		}
		// Wait for queue to be processed
		delay()

		info := c.StatusLocal(ctx, h)
		if info.Status != api.TrackerStatusPinError {
			t.Errorf("element is %s and not PinError", info.Status)
		}

		// Recover good ID
		info, _ = c.RecoverLocal(ctx, h2)
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

	clusters[0].Pin(ctx, h, api.PinOptions{})
	clusters[0].Pin(ctx, h2, api.PinOptions{})

	pinDelay()
	pinDelay()

	j := mrand.Intn(nClusters)
	ginfo, err := clusters[j].Recover(ctx, h)
	if err != nil {
		// we always attempt to return a valid response
		// with errors contained in GlobalPinInfo
		t.Fatal("did not expect an error")
	}
	if len(ginfo.PeerMap) != nClusters {
		t.Error("number of peers do not match")
	}
	// Wait for queue to be processed
	delay()

	ginfo, err = clusters[j].Status(ctx, h)
	if err != nil {
		t.Fatal(err)
	}

	pinfo, ok := ginfo.PeerMap[clusters[j].host.ID().String()]
	if !ok {
		t.Fatal("should have info for this host")
	}
	if pinfo.Error == "" {
		t.Error("pinInfo error should not be empty")
	}

	for _, c := range clusters {
		inf, ok := ginfo.PeerMap[c.host.ID().String()]
		if !ok {
			t.Fatal("GlobalPinInfo should not be empty for this host")
		}

		if inf.Status != api.TrackerStatusPinError {
			t.Logf("%+v", inf)
			t.Error("should be PinError in all peers")
		}
	}

	// Test with a good Cid
	j = mrand.Intn(nClusters)
	ginfo, err = clusters[j].Recover(ctx, h2)
	if err != nil {
		t.Fatal(err)
	}
	if !ginfo.Cid.Equals(h2) {
		t.Error("GlobalPinInfo should be for testrCid2")
	}
	if len(ginfo.PeerMap) != nClusters {
		t.Error("number of peers do not match")
	}

	for _, c := range clusters {
		inf, ok := ginfo.PeerMap[c.host.ID().String()]
		if !ok {
			t.Fatal("GlobalPinInfo should have this cluster")
		}
		if inf.Status != api.TrackerStatusPinned {
			t.Error("the GlobalPinInfo should show Pinned in all peers")
		}
	}
}

func TestClustersRecoverAll(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	h1 := test.Cid1
	hError := test.ErrorCid

	ttlDelay()

	clusters[0].Pin(ctx, h1, api.PinOptions{})
	clusters[0].Pin(ctx, hError, api.PinOptions{})

	pinDelay()

	out := make(chan api.GlobalPinInfo)
	go func() {
		err := clusters[mrand.Intn(nClusters)].RecoverAll(ctx, out)
		if err != nil {
			t.Error(err)
		}
	}()

	gInfos := collectGlobalPinInfos(t, out, 5*time.Second)

	if len(gInfos) != 1 {
		t.Error("expected one items")
	}

	for _, gInfo := range gInfos {
		if len(gInfo.PeerMap) != nClusters {
			t.Error("number of peers do not match")
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

	// Why is replication factor nClusters - 1?
	// Because that way we know that pinning nCluster
	// pins with an strategy like numpins/disk
	// will result in each peer holding locally exactly
	// nCluster pins.

	prefix := test.Cid1.Prefix()

	for i := 0; i < nClusters; i++ {
		// Pick a random cluster and hash
		j := mrand.Intn(nClusters)          // choose a random cluster peer
		h, err := prefix.Sum(randomBytes()) // create random cid
		if err != nil {
			t.Fatal(err)
		}
		_, err = clusters[j].Pin(ctx, api.NewCid(h), api.PinOptions{})
		if err != nil {
			t.Error(err)
		}
		pinDelay()

		// check that it is held by exactly nClusters - 1 peers
		gpi, err := clusters[j].Status(ctx, api.NewCid(h))
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
		out := make(chan api.PinInfo, 100)

		go func() {
			err := c.tracker.StatusAll(ctx, api.TrackerStatusUndefined, out)
			if err != nil {
				t.Error(err)
			}
		}()
		pinfos := collectPinInfos(t, out)
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

		outPins := make(chan api.Pin)
		go func() {
			err := c.Pins(ctx, outPins)
			if err != nil {
				t.Error(err)
			}
		}()
		for pin := range outPins {
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
	_, err := clusters[0].Pin(ctx, h, api.PinOptions{})
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
		c.config.DisableRepinning = true
	}

	ttlDelay() // make sure we have places to pin

	h := test.Cid1
	_, err := clusters[0].Pin(ctx, h, api.PinOptions{})
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

	opts := api.PinOptions{
		ReplicationFactorMin: 1,
		ReplicationFactorMax: 2,
	}
	_, err = clusters[0].Pin(ctx, h, opts)
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
		c.config.DisableRepinning = true
	}

	ttlDelay()

	// Shutdown two peers
	clusters[nClusters-1].Shutdown(ctx)
	clusters[nClusters-2].Shutdown(ctx)

	waitForLeaderAndMetrics(t, clusters)

	h := test.Cid1
	_, err := clusters[0].Pin(ctx, h, api.PinOptions{})
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
		c.config.DisableRepinning = true
	}

	// Shutdown two peers
	clusters[nClusters-1].Shutdown(ctx)
	waitForLeaderAndMetrics(t, clusters)
	clusters[nClusters-2].Shutdown(ctx)
	waitForLeaderAndMetrics(t, clusters)

	h := test.Cid1
	_, err := clusters[0].Pin(ctx, h, api.PinOptions{})
	if err == nil {
		t.Error("Pin should have failed as rplMin cannot be satisfied")
	}
	t.Log(err)
	if !strings.Contains(err.Error(), "not enough peers to allocate CID") {
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
		c.config.DisableRepinning = true
	}

	ttlDelay()

	h := test.Cid1
	_, err := clusters[0].Pin(ctx, h, api.PinOptions{})
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	// Shutdown two peers
	clusters[nClusters-1].Shutdown(ctx)
	waitForLeaderAndMetrics(t, clusters)
	clusters[nClusters-2].Shutdown(ctx)
	waitForLeaderAndMetrics(t, clusters)

	_, err = clusters[0].Pin(ctx, h, api.PinOptions{})
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
// pinned anymore.
// FIXME: The manual repin only works if the pin options changed.
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
		c.config.DisableRepinning = true
	}

	ttlDelay() // make sure metrics are in

	h := test.Cid1
	_, err := clusters[0].Pin(ctx, h, api.PinOptions{
		Name: "a",
	})
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
	// Only the first allocated peer (or the second if the first is
	// alerting) will automatically repin.
	alloc1 := peerIDMap[firstAllocations[1]]
	alloc2 := peerIDMap[firstAllocations[2]]
	safePeer := peerIDMap[firstAllocations[0]]

	alloc1.Shutdown(ctx)
	alloc2.Shutdown(ctx)

	waitForLeaderAndMetrics(t, clusters)

	// Repin - (although this should have been taken of as alerts
	// happen for the shutdown nodes. We force re-allocation by
	// changing the name.
	_, err = safePeer.Pin(ctx, h, api.PinOptions{
		Name: "b",
	})
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
		c.config.DisableRepinning = true
	}

	ttlDelay()

	j := mrand.Intn(nClusters)
	h := test.Cid1
	_, err := clusters[j].Pin(ctx, h, api.PinOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Let the pin arrive
	pinDelay()

	pinList, err := clusters[j].pinsSlice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pin := pinList[0]
	allocs := sort.StringSlice(api.PeersToStrings(pin.Allocations))
	allocs.Sort()
	allocsStr := fmt.Sprintf("%s", allocs)

	// Re-pin should work and be allocated to the same
	// nodes
	_, err = clusters[j].Pin(ctx, h, api.PinOptions{})
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	pinList2, err := clusters[j].pinsSlice(ctx)
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
			//t.Logf("Killing %s", c.id)
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
		j = mrand.Intn(nClusters)
	}

	// now pin should succeed
	_, err = clusters[j].Pin(ctx, h, api.PinOptions{})
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
		t.Log(pinfo.Peer, pinfo.Status)
		if pinfo.Status == api.TrackerStatusPinned {
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
		c.config.DisableRepinning = true
	}

	ttlDelay()

	j := mrand.Intn(nClusters)
	_, err := clusters[j].Pin(ctx, test.Cid1, api.PinOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Let the pin arrive
	pinDelay()

	clusters[0].Shutdown(ctx)
	clusters[1].Shutdown(ctx)

	waitForLeaderAndMetrics(t, clusters)

	_, err = clusters[2].Pin(ctx, test.Cid2, api.PinOptions{})
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

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	for _, c := range clusters {
		c.config.ReplicationFactorMin = nClusters - 1
		c.config.ReplicationFactorMax = nClusters - 1
	}

	// pin something
	h := test.Cid1
	clusters[0].Pin(ctx, h, api.PinOptions{})
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
		clid := c.id.String()
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

// Helper function for verifying cluster graph. Will only pass if exactly the
// peers in clusterIDs are fully connected to each other and the expected ipfs
// mock connectivity exists. Cluster peers not in clusterIDs are assumed to
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
			peerIndex[p.String()] = struct{}{}
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

	if len(graph.ClusterTrustLinks) != peerNum {
		t.Errorf("Unexpected number of trust links in graph")
	}

	// Check that the mocked ipfs swarm is recorded
	if len(graph.IPFSLinks) != 1 {
		t.Error("Expected exactly one ipfs peer for all cluster nodes, the mocked peer")
	}
	links, ok := graph.IPFSLinks[test.PeerID1.String()]
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

	ttlDelay()

	j := mrand.Intn(nClusters) // choose a random cluster peer to query
	graph, err := clusters[j].ConnectGraph()
	if err != nil {
		t.Fatal(err)
	}

	clusterIDs := make(map[string]struct{})
	for _, c := range clusters {
		id := c.ID(ctx).ID.String()
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

	j := mrand.Intn(nClusters) // choose a random cluster peer to query
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
	clusters[discon1].host.Close()
	clusters[discon2].Shutdown(ctx)
	clusters[discon2].host.Close()

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
		id := c.ID(ctx).ID.String()
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

	j := mrand.Intn(nClusters)
	h := test.Cid1
	_, err := clusters[j].Pin(ctx, h, api.PinOptions{})
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
		j = mrand.Intn(nClusters)
	}

	numPinned := 0
	for i, c := range clusters {
		if i == killedClusterIndex {
			continue
		}
		pinfo := c.tracker.Status(ctx, h)
		if pinfo.Status == api.TrackerStatusPinned {
			//t.Log(pinfo.Peer)
			numPinned++
		}
	}

	if numPinned != nClusters-2 {
		t.Errorf("expected %d replicas for pin, got %d", nClusters-2, numPinned)
	}
}

func TestRepoGC(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	f := func(t *testing.T, c *Cluster) {
		gRepoGC, err := c.RepoGC(context.Background())
		if err != nil {
			t.Fatal("gc should have worked:", err)
		}

		if gRepoGC.PeerMap == nil {
			t.Fatal("expected a non-nil peer map")
		}

		if len(gRepoGC.PeerMap) != nClusters {
			t.Errorf("expected repo gc information for %d peer", nClusters)
		}
		for _, repoGC := range gRepoGC.PeerMap {
			testRepoGC(t, repoGC)
		}
	}

	runF(t, clusters, f)
}

func TestClustersFollowerMode(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)

	_, err := clusters[0].Pin(ctx, test.Cid1, api.PinOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = clusters[0].Pin(ctx, test.ErrorCid, api.PinOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Let the pins arrive
	pinDelay()

	// Set Cluster1 to follower mode
	clusters[1].config.FollowerMode = true

	t.Run("follower cannot pin", func(t *testing.T) {
		_, err := clusters[1].PinPath(ctx, "/ipfs/"+test.Cid2.String(), api.PinOptions{})
		if err != errFollowerMode {
			t.Error("expected follower mode error")
		}
		_, err = clusters[1].Pin(ctx, test.Cid2, api.PinOptions{})
		if err != errFollowerMode {
			t.Error("expected follower mode error")
		}
	})

	t.Run("follower cannot unpin", func(t *testing.T) {
		_, err := clusters[1].UnpinPath(ctx, "/ipfs/"+test.Cid1.String())
		if err != errFollowerMode {
			t.Error("expected follower mode error")
		}
		_, err = clusters[1].Unpin(ctx, test.Cid1)
		if err != errFollowerMode {
			t.Error("expected follower mode error")
		}
	})

	t.Run("follower cannot add", func(t *testing.T) {
		sth := test.NewShardingTestHelper()
		defer sth.Clean(t)
		params := api.DefaultAddParams()
		params.Shard = false
		params.Name = "testlocal"
		mfr, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		r := multipart.NewReader(mfr, mfr.Boundary())
		_, err = clusters[1].AddFile(ctx, r, params)
		if err != errFollowerMode {
			t.Error("expected follower mode error")
		}
	})

	t.Run("follower status itself only", func(t *testing.T) {
		gpi, err := clusters[1].Status(ctx, test.Cid1)
		if err != nil {
			t.Error("status should work")
		}
		if len(gpi.PeerMap) != 1 {
			t.Fatal("globalPinInfo should only have one peer")
		}
	})
}

func TestClusterPinsWithExpiration(t *testing.T) {
	ctx := context.Background()

	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)

	ttlDelay()

	cl := clusters[mrand.Intn(nClusters)] // choose a random cluster peer to query

	c := test.Cid1
	expireIn := 1 * time.Second
	opts := api.PinOptions{
		ExpireAt: time.Now().Add(expireIn),
	}
	_, err := cl.Pin(ctx, c, opts)
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	pinDelay()

	pins, err := cl.pinsSlice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 1 {
		t.Error("pin should be part of the state")
	}

	// wait till expiry time
	time.Sleep(expireIn)

	// manually call state sync on all peers, so we don't have to wait till
	// state sync interval
	for _, c := range clusters {
		err = c.StateSync(ctx)
		if err != nil {
			t.Error(err)
		}
	}

	pinDelay()

	// state sync should have unpinned expired pin
	pins, err = cl.pinsSlice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 0 {
		t.Error("pin should not be part of the state")
	}
}

func TestClusterAlerts(t *testing.T) {
	ctx := context.Background()
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)

	if len(clusters) < 2 {
		t.Skip("need at least 2 nodes for this test")
	}

	ttlDelay()

	for _, c := range clusters[1:] {
		c.Shutdown(ctx)
	}

	ttlDelay()

	alerts := clusters[0].Alerts()
	if len(alerts) == 0 {
		t.Error("expected at least one alert")
	}
}
