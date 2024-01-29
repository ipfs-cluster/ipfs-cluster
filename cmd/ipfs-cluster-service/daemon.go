package main

import (
	"context"
	"strings"
	"time"

	ipfscluster "github.com/ipfs-cluster/ipfs-cluster"
	"github.com/ipfs-cluster/ipfs-cluster/allocator/balanced"
	"github.com/ipfs-cluster/ipfs-cluster/api/ipfsproxy"
	"github.com/ipfs-cluster/ipfs-cluster/api/pinsvcapi"
	"github.com/ipfs-cluster/ipfs-cluster/api/rest"
	"github.com/ipfs-cluster/ipfs-cluster/cmdutils"
	"github.com/ipfs-cluster/ipfs-cluster/config"
	"github.com/ipfs-cluster/ipfs-cluster/consensus/crdt"
	"github.com/ipfs-cluster/ipfs-cluster/consensus/raft"
	"github.com/ipfs-cluster/ipfs-cluster/informer/disk"
	"github.com/ipfs-cluster/ipfs-cluster/informer/pinqueue"
	"github.com/ipfs-cluster/ipfs-cluster/informer/tags"
	"github.com/ipfs-cluster/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs-cluster/ipfs-cluster/monitor/pubsubmon"
	"github.com/ipfs-cluster/ipfs-cluster/observations"
	"github.com/ipfs-cluster/ipfs-cluster/pintracker/stateless"
	"go.opencensus.io/tag"

	ds "github.com/ipfs/go-datastore"
	dual "github.com/libp2p/go-libp2p-kad-dht/dual"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	host "github.com/libp2p/go-libp2p/core/host"
	peer "github.com/libp2p/go-libp2p/core/peer"

	ma "github.com/multiformats/go-multiaddr"

	sddaemon "github.com/coreos/go-systemd/v22/daemon"
	errors "github.com/pkg/errors"
	cli "github.com/urfave/cli"
)

func parseBootstraps(flagVal []string) (bootstraps []ma.Multiaddr) {
	for _, a := range flagVal {
		bAddr, err := ma.NewMultiaddr(strings.TrimSpace(a))
		checkErr("error parsing bootstrap multiaddress (%s)", err, a)
		bootstraps = append(bootstraps, bAddr)
	}
	return
}

// Runs the cluster peer
func daemon(c *cli.Context) error {
	logger.Info("Initializing. For verbose output run with \"-l debug\". Please wait...")

	ctx, cancel := context.WithCancel(context.Background())
	var bootstraps []ma.Multiaddr
	if bootStr := c.String("bootstrap"); bootStr != "" {
		bootstraps = parseBootstraps(strings.Split(bootStr, ","))
	}

	// Execution lock
	locker.lock()
	defer locker.tryUnlock()

	// Load all the configurations and identity
	cfgHelper, err := cmdutils.NewLoadedConfigHelper(configPath, identityPath)
	checkErr("loading configurations", err)
	defer cfgHelper.Manager().Shutdown()

	cfgs := cfgHelper.Configs()

	if c.Bool("stats") {
		cfgs.Metrics.EnableStats = true
	}
	cfgHelper.SetupTracing(c.Bool("tracing"))

	// Setup bootstrapping
	raftStaging := false
	switch cfgHelper.GetConsensus() {
	case cfgs.Raft.ConfigKey():
		if len(bootstraps) > 0 {
			// Cleanup state if bootstrapping
			raft.CleanupRaft(cfgs.Raft)
			raftStaging = true
		}
	case cfgs.Crdt.ConfigKey():
		if !c.Bool("no-trust") {
			crdtCfg := cfgs.Crdt
			crdtCfg.TrustedPeers = append(crdtCfg.TrustedPeers, ipfscluster.PeersFromMultiaddrs(bootstraps)...)
		}
	}

	if c.Bool("leave") {
		cfgs.Cluster.LeaveOnShutdown = true
	}

	store := setupDatastore(cfgHelper)

	host, pubsub, dht, err := ipfscluster.NewClusterHost(ctx, cfgHelper.Identity(), cfgs.Cluster, store)
	checkErr("creating libp2p host", err)

	cluster, err := createCluster(ctx, c, cfgHelper, host, pubsub, dht, store, raftStaging)
	checkErr("starting cluster", err)

	// noop if no bootstraps
	// if bootstrapping fails, consensus will never be ready
	// and timeout. So this can happen in background and we
	// avoid worrying about error handling here (since Cluster
	// will realize).
	go bootstrap(ctx, cluster, bootstraps)

	// send readiness notification to systemd
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-cluster.Ready():
			sddaemon.SdNotify(false, sddaemon.SdNotifyReady)
		}
	}()

	return cmdutils.HandleSignals(ctx, cancel, cluster, host, dht, store)
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
	dht *dual.DHT,
	store ds.Datastore,
	raftStaging bool,
) (*ipfscluster.Cluster, error) {

	cfgs := cfgHelper.Configs()
	cfgMgr := cfgHelper.Manager()
	cfgBytes, err := cfgMgr.ToDisplayJSON()
	checkErr("getting configuration string", err)
	logger.Debugf("Configuration:\n%s\n", cfgBytes)

	ctx, err = tag.New(ctx, tag.Upsert(observations.HostKey, host.ID().String()))
	checkErr("tag context with host id", err)

	err = observations.SetupMetrics(cfgs.Metrics)
	checkErr("setting up Metrics", err)

	tracer, err := observations.SetupTracing(cfgs.Tracing)
	checkErr("setting up Tracing", err)

	var apis []ipfscluster.API
	if cfgMgr.IsLoadedFromJSON(config.API, cfgs.Restapi.ConfigKey()) {
		var api *rest.API
		// Do NOT enable default Libp2p API endpoint on CRDT
		// clusters. Collaborative clusters are likely to share the
		// secret with untrusted peers, thus the API would be open for
		// anyone.
		if cfgHelper.GetConsensus() == cfgs.Raft.ConfigKey() {
			api, err = rest.NewAPIWithHost(ctx, cfgs.Restapi, host)
		} else {
			api, err = rest.NewAPI(ctx, cfgs.Restapi)
		}
		checkErr("creating REST API component", err)
		apis = append(apis, api)

	}

	if cfgMgr.IsLoadedFromJSON(config.API, cfgs.Pinsvcapi.ConfigKey()) {
		pinsvcapi, err := pinsvcapi.NewAPI(ctx, cfgs.Pinsvcapi)
		checkErr("creating Pinning Service API component", err)

		apis = append(apis, pinsvcapi)
	}

	if cfgMgr.IsLoadedFromJSON(config.API, cfgs.Ipfsproxy.ConfigKey()) {
		proxy, err := ipfsproxy.New(cfgs.Ipfsproxy)
		checkErr("creating IPFS Proxy component", err)

		apis = append(apis, proxy)
	}

	connector, err := ipfshttp.NewConnector(cfgs.Ipfshttp)
	checkErr("creating IPFS Connector component", err)

	var informers []ipfscluster.Informer
	if cfgMgr.IsLoadedFromJSON(config.Informer, cfgs.DiskInf.ConfigKey()) {
		diskInf, err := disk.NewInformer(cfgs.DiskInf)
		checkErr("creating disk informer", err)
		informers = append(informers, diskInf)
	}
	if cfgMgr.IsLoadedFromJSON(config.Informer, cfgs.TagsInf.ConfigKey()) {
		tagsInf, err := tags.New(cfgs.TagsInf)
		checkErr("creating numpin informer", err)
		informers = append(informers, tagsInf)
	}

	if cfgMgr.IsLoadedFromJSON(config.Informer, cfgs.PinQueueInf.ConfigKey()) {
		pinQueueInf, err := pinqueue.New(cfgs.PinQueueInf)
		checkErr("creating pinqueue informer", err)
		informers = append(informers, pinQueueInf)
	}

	// For legacy compatibility we need to make the allocator
	// automatically compatible with informers that have been loaded. For
	// simplicity we assume that anyone that does not specify an allocator
	// configuration (legacy configs), will be using "freespace"
	if !cfgMgr.IsLoadedFromJSON(config.Allocator, cfgs.BalancedAlloc.ConfigKey()) {
		cfgs.BalancedAlloc.AllocateBy = []string{"freespace"}
	}
	alloc, err := balanced.New(cfgs.BalancedAlloc)
	checkErr("creating allocator", err)

	cons, err := setupConsensus(
		cfgHelper,
		host,
		dht,
		pubsub,
		store,
		raftStaging,
	)
	if err != nil {
		store.Close()
		checkErr("setting up Consensus", err)
	}

	var peersF func(context.Context) ([]peer.ID, error)
	if cfgHelper.GetConsensus() == cfgs.Raft.ConfigKey() {
		peersF = cons.Peers
	}

	tracker := stateless.New(cfgs.Statelesstracker, host.ID(), cfgs.Cluster.Peername, cons.State)
	logger.Debug("stateless pintracker loaded")

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
		informers,
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

func setupDatastore(cfgHelper *cmdutils.ConfigHelper) ds.Datastore {
	dsName := cfgHelper.GetDatastore()
	stmgr, err := cmdutils.NewStateManager(cfgHelper.GetConsensus(), dsName, cfgHelper.Identity(), cfgHelper.Configs())
	checkErr("creating state manager", err)
	store, err := stmgr.GetStore()
	checkErr("creating datastore", err)
	if dsName != "" {
		logger.Infof("Datastore backend: %s", dsName)
	}
	return store
}

func setupConsensus(
	cfgHelper *cmdutils.ConfigHelper,
	h host.Host,
	dht *dual.DHT,
	pubsub *pubsub.PubSub,
	store ds.Datastore,
	raftStaging bool,
) (ipfscluster.Consensus, error) {

	cfgs := cfgHelper.Configs()
	switch cfgHelper.GetConsensus() {
	case cfgs.Raft.ConfigKey():
		rft, err := raft.NewConsensus(
			h,
			cfgHelper.Configs().Raft,
			store,
			raftStaging,
		)
		if err != nil {
			return nil, errors.Wrap(err, "creating Raft component")
		}
		ipfscluster.ReadyTimeout = cfgs.Raft.WaitForLeaderTimeout + 5*time.Second
		return rft, nil
	case cfgs.Crdt.ConfigKey():
		convrdt, err := crdt.New(
			h,
			dht,
			pubsub,
			cfgHelper.Configs().Crdt,
			store,
		)
		if err != nil {
			return nil, errors.Wrap(err, "creating CRDT component")
		}
		// go-ds-crdt migrations are the main cause that may need
		// additional time for this consensus layer to be ready.
		ipfscluster.ReadyTimeout = 356 * 24 * time.Hour
		return convrdt, nil
	default:
		return nil, errors.New("unknown consensus component")
	}
}
