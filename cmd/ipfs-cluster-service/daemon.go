package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/allocator/descendalloc"
	"github.com/ipfs/ipfs-cluster/api/ipfsproxy"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/cmdutils"
	"github.com/ipfs/ipfs-cluster/config"
	"github.com/ipfs/ipfs-cluster/consensus/crdt"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/informer/disk"
	"github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs/ipfs-cluster/monitor/pubsubmon"
	"github.com/ipfs/ipfs-cluster/observations"
	"github.com/ipfs/ipfs-cluster/pintracker/maptracker"
	"github.com/ipfs/ipfs-cluster/pintracker/stateless"
	"go.opencensus.io/tag"

	ds "github.com/ipfs/go-datastore"
	host "github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-core/peer"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	ma "github.com/multiformats/go-multiaddr"

	errors "github.com/pkg/errors"
	cli "github.com/urfave/cli"
)

func parseBootstraps(flagVal []string) (bootstraps []ma.Multiaddr) {
	for _, a := range flagVal {
		bAddr, err := ma.NewMultiaddr(a)
		checkErr("error parsing bootstrap multiaddress (%s)", err, a)
		bootstraps = append(bootstraps, bAddr)
	}
	return
}

// Runs the cluster peer
func daemon(c *cli.Context) error {
	if c.String("consensus") == "" {
		checkErr("starting daemon", errors.New("--consensus flag must be set to \"raft\" or \"crdt\""))
	}

	logger.Info("Initializing. For verbose output run with \"-l debug\". Please wait...")

	ma.SwapToP2pMultiaddrs()

	ctx, cancel := context.WithCancel(context.Background())

	bootstraps := parseBootstraps(c.StringSlice("bootstrap"))

	// Execution lock
	locker.lock()
	defer locker.tryUnlock()

	// Load all the configurations and identity
	cfgHelper := loadConfigHelper()
	defer cfgHelper.Manager().Shutdown()

	cfgs := cfgHelper.Configs()

	if !c.Bool("no-trust") {
		crdtCfg := cfgs.Crdt
		crdtCfg.TrustedPeers = append(crdtCfg.TrustedPeers, ipfscluster.PeersFromMultiaddrs(bootstraps)...)
	}

	if c.Bool("stats") {
		cfgs.Metrics.EnableStats = true
	}
	cfgHelper.SetupTracing(c.Bool("tracing"))

	// Cleanup state if bootstrapping
	raftStaging := false
	if len(bootstraps) > 0 && c.String("consensus") == "raft" {
		raft.CleanupRaft(cfgs.Raft)
		raftStaging = true
	}

	if c.Bool("leave") {
		cfgs.Cluster.LeaveOnShutdown = true
	}

	host, pubsub, dht, err := ipfscluster.NewClusterHost(ctx, cfgHelper.Identity(), cfgs.Cluster)
	checkErr("creating libp2p host", err)

	cluster, err := createCluster(ctx, c, cfgHelper, host, pubsub, dht, raftStaging)
	checkErr("starting cluster", err)

	// noop if no bootstraps
	// if bootstrapping fails, consensus will never be ready
	// and timeout. So this can happen in background and we
	// avoid worrying about error handling here (since Cluster
	// will realize).
	go bootstrap(ctx, cluster, bootstraps)

	return handleSignals(ctx, cancel, cluster, host, dht)
}

// createCluster creates all the necessary things to produce the cluster
// object and returns it along the datastore so the lifecycle can be handled
// (the datastore needs to be Closed after shutting down the Cluster).
func createCluster(
	ctx context.Context,
	c *cli.Context,
	cfgHelper *cmdutils.ConfigHelper,
	host host.Host,
	pubsub *pubsub.PubSub,
	dht *dht.IpfsDHT,
	raftStaging bool,
) (*ipfscluster.Cluster, error) {

	cfgs := cfgHelper.Configs()
	cfgMgr := cfgHelper.Manager()

	ctx, err := tag.New(ctx, tag.Upsert(observations.HostKey, host.ID().Pretty()))
	checkErr("tag context with host id", err)

	var apis []ipfscluster.API
	if cfgMgr.IsLoadedFromJSON(config.API, cfgs.Restapi.ConfigKey()) {
		rest, err := rest.NewAPIWithHost(ctx, cfgs.Restapi, host)
		checkErr("creating REST API component", err)
		apis = append(apis, rest)
	}

	if cfgMgr.IsLoadedFromJSON(config.API, cfgs.Ipfsproxy.ConfigKey()) {
		proxy, err := ipfsproxy.New(cfgs.Ipfsproxy)
		checkErr("creating IPFS Proxy component", err)

		apis = append(apis, proxy)
	}

	connector, err := ipfshttp.NewConnector(cfgs.Ipfshttp)
	checkErr("creating IPFS Connector component", err)

	tracker := setupPinTracker(
		c.String("pintracker"),
		host,
		cfgs.Maptracker,
		cfgs.Statelesstracker,
		cfgs.Cluster.Peername,
	)

	informer, err := disk.NewInformer(cfgs.Diskinf)
	checkErr("creating disk informer", err)
	alloc := descendalloc.NewAllocator()

	ipfscluster.ReadyTimeout = cfgs.Raft.WaitForLeaderTimeout + 5*time.Second

	err = observations.SetupMetrics(cfgs.Metrics)
	checkErr("setting up Metrics", err)

	tracer, err := observations.SetupTracing(cfgs.Tracing)
	checkErr("setting up Tracing", err)

	store := setupDatastore(c.String("consensus"), cfgHelper.Identity(), cfgs)

	cons, err := setupConsensus(
		c.String("consensus"),
		host,
		dht,
		pubsub,
		cfgs,
		store,
		raftStaging,
	)
	if err != nil {
		store.Close()
		checkErr("setting up Consensus", err)
	}

	var peersF func(context.Context) ([]peer.ID, error)
	if c.String("consensus") == "raft" {
		peersF = cons.Peers
	}

	mon, err := pubsubmon.New(ctx, cfgs.Pubsubmon, pubsub, peersF)
	if err != nil {
		store.Close()
		checkErr("setting up PeerMonitor", err)
	}

	return ipfscluster.NewCluster(
		ctx,
		host,
		dht,
		cfgs.Cluster,
		store,
		cons,
		apis,
		connector,
		tracker,
		mon,
		alloc,
		informer,
		tracer,
	)
}

// bootstrap will bootstrap this peer to one of the bootstrap addresses
// if there are any.
func bootstrap(ctx context.Context, cluster *ipfscluster.Cluster, bootstraps []ma.Multiaddr) {
	for _, bstrap := range bootstraps {
		logger.Infof("Bootstrapping to %s", bstrap)
		err := cluster.Join(ctx, bstrap)
		if err != nil {
			logger.Errorf("bootstrap to %s failed: %s", bstrap, err)
		}
	}
}

func handleSignals(
	ctx context.Context,
	cancel context.CancelFunc,
	cluster *ipfscluster.Cluster,
	host host.Host,
	dht *dht.IpfsDHT,
) error {
	signalChan := make(chan os.Signal, 20)
	signal.Notify(
		signalChan,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
	)

	var ctrlcCount int
	for {
		select {
		case <-signalChan:
			ctrlcCount++
			handleCtrlC(ctx, cluster, ctrlcCount)
		case <-cluster.Done():
			cancel()
			dht.Close()
			host.Close()
			return nil
		}
	}
}

func handleCtrlC(ctx context.Context, cluster *ipfscluster.Cluster, ctrlcCount int) {
	switch ctrlcCount {
	case 1:
		go func() {
			err := cluster.Shutdown(ctx)
			checkErr("shutting down cluster", err)
		}()
	case 2:
		out(`


!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
Shutdown is taking too long! Press Ctrl-c again to manually kill cluster.
Note that this may corrupt the local cluster state.
!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!


`)
	case 3:
		out("exiting cluster NOW")
		locker.tryUnlock()
		os.Exit(-1)
	}
}

func setupPinTracker(
	name string,
	h host.Host,
	mapCfg *maptracker.Config,
	statelessCfg *stateless.Config,
	peerName string,
) ipfscluster.PinTracker {
	switch name {
	case "map":
		ptrk := maptracker.NewMapPinTracker(mapCfg, h.ID(), peerName)
		logger.Debug("map pintracker loaded")
		return ptrk
	case "stateless":
		ptrk := stateless.New(statelessCfg, h.ID(), peerName)
		logger.Debug("stateless pintracker loaded")
		return ptrk
	default:
		err := errors.New("unknown pintracker type")
		checkErr("", err)
		return nil
	}
}

func setupDatastore(
	consensus string,
	ident *config.Identity,
	cfgs *cmdutils.Configs,
) ds.Datastore {
	stmgr, err := cmdutils.NewStateManager(consensus, ident, cfgs)
	checkErr("creating state manager", err)
	store, err := stmgr.GetStore()
	checkErr("creating datastore", err)
	return store
}

func setupConsensus(
	name string,
	h host.Host,
	dht *dht.IpfsDHT,
	pubsub *pubsub.PubSub,
	cfgs *cmdutils.Configs,
	store ds.Datastore,
	raftStaging bool,
) (ipfscluster.Consensus, error) {
	switch name {
	case "raft":
		rft, err := raft.NewConsensus(
			h,
			cfgs.Raft,
			store,
			raftStaging,
		)
		if err != nil {
			return nil, errors.Wrap(err, "creating Raft component")
		}
		return rft, nil
	case "crdt":
		convrdt, err := crdt.New(
			h,
			dht,
			pubsub,
			cfgs.Crdt,
			store,
		)
		if err != nil {
			return nil, errors.Wrap(err, "creating CRDT component")
		}
		return convrdt, nil
	default:
		return nil, errors.New("unknown consensus component")
	}
}
