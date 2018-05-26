package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	host "github.com/libp2p/go-libp2p-host"
	"github.com/urfave/cli"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/allocator/ascendalloc"
	"github.com/ipfs/ipfs-cluster/allocator/descendalloc"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/informer/disk"
	"github.com/ipfs/ipfs-cluster/informer/numpin"
	"github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs/ipfs-cluster/monitor/basic"
	"github.com/ipfs/ipfs-cluster/monitor/pubsubmon"
	"github.com/ipfs/ipfs-cluster/pintracker/maptracker"
	"github.com/ipfs/ipfs-cluster/pstoremgr"
	"github.com/ipfs/ipfs-cluster/state/mapstate"

	ma "github.com/multiformats/go-multiaddr"
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
	logger.Info("Initializing. For verbose output run with \"-l debug\". Please wait...")

	// Load all the configurations
	cfgMgr, cfgs := makeConfigs()

	// Run any migrations
	if c.Bool("upgrade") {
		err := upgrade()
		if err != errNoSnapshot {
			checkErr("upgrading state", err)
		} // otherwise continue
	}

	bootstraps := parseBootstraps(c.StringSlice("bootstrap"))

	// Execution lock
	err := locker.lock()
	checkErr("acquiring execution lock", err)
	defer locker.tryUnlock()

	// Load all the configurations
	// always wait for configuration to be saved
	defer cfgMgr.Shutdown()

	err = cfgMgr.LoadJSONFromFile(configPath)
	checkErr("loading configuration", err)

	// Cleanup state if bootstrapping
	raftStaging := false
	if len(bootstraps) > 0 {
		cleanupState(cfgs.consensusCfg)
		raftStaging = true
	}

	if c.Bool("leave") {
		cfgs.clusterCfg.LeaveOnShutdown = true
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cluster, err := createCluster(ctx, c, cfgs, raftStaging)
	checkErr("starting cluster", err)

	// noop if no bootstraps
	// if bootstrapping fails, consensus will never be ready
	// and timeout. So this can happen in background and we
	// avoid worrying about error handling here (since Cluster
	// will realize).
	go bootstrap(cluster, bootstraps)

	return handleSignals(cluster)
}

func createCluster(
	ctx context.Context,
	c *cli.Context,
	cfgs *cfgs,
	raftStaging bool,
) (*ipfscluster.Cluster, error) {

	host, err := ipfscluster.NewClusterHost(ctx, cfgs.clusterCfg)
	checkErr("creating libP2P Host", err)

	peerstoreMgr := pstoremgr.New(host, cfgs.clusterCfg.GetPeerstorePath())
	peerstoreMgr.ImportPeersFromPeerstore(false)

	api, err := rest.NewAPIWithHost(cfgs.apiCfg, host)
	checkErr("creating REST API component", err)

	proxy, err := ipfshttp.NewConnector(cfgs.ipfshttpCfg)
	checkErr("creating IPFS Connector component", err)

	state := mapstate.NewMapState()

	err = validateVersion(cfgs.clusterCfg, cfgs.consensusCfg)
	checkErr("validating version", err)

	raftcon, err := raft.NewConsensus(
		host,
		cfgs.consensusCfg,
		state,
		raftStaging,
	)
	checkErr("creating consensus component", err)

	tracker := maptracker.NewMapPinTracker(cfgs.trackerCfg, host.ID())
	mon := setupMonitor(c.String("monitor"), host, cfgs.monCfg, cfgs.pubsubmonCfg)
	informer, alloc := setupAllocation(c.String("alloc"), cfgs.diskInfCfg, cfgs.numpinInfCfg)

	ipfscluster.ReadyTimeout = cfgs.consensusCfg.WaitForLeaderTimeout + 5*time.Second

	return ipfscluster.NewCluster(
		host,
		cfgs.clusterCfg,
		raftcon,
		api,
		proxy,
		state,
		tracker,
		mon,
		alloc,
		informer,
	)
}

// bootstrap will bootstrap this peer to one of the bootstrap addresses
// if there are any.
func bootstrap(cluster *ipfscluster.Cluster, bootstraps []ma.Multiaddr) {
	for _, bstrap := range bootstraps {
		logger.Infof("Bootstrapping to %s", bstrap)
		err := cluster.Join(bstrap)
		if err != nil {
			logger.Errorf("bootstrap to %s failed: %s", bstrap, err)
		}
	}
}

func handleSignals(cluster *ipfscluster.Cluster) error {
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
			handleCtrlC(cluster, ctrlcCount)
		case <-cluster.Done():
			return nil
		}
	}
}

func handleCtrlC(cluster *ipfscluster.Cluster, ctrlcCount int) {
	switch ctrlcCount {
	case 1:
		go func() {
			err := cluster.Shutdown()
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
		os.Exit(-1)
	}
}

func setupAllocation(
	name string,
	diskInfCfg *disk.Config,
	numpinInfCfg *numpin.Config,
) (ipfscluster.Informer, ipfscluster.PinAllocator) {
	switch name {
	case "disk", "disk-freespace":
		informer, err := disk.NewInformer(diskInfCfg)
		checkErr("creating informer", err)
		return informer, descendalloc.NewAllocator()
	case "disk-reposize":
		informer, err := disk.NewInformer(diskInfCfg)
		checkErr("creating informer", err)
		return informer, ascendalloc.NewAllocator()
	case "numpin", "pincount":
		informer, err := numpin.NewInformer(numpinInfCfg)
		checkErr("creating informer", err)
		return informer, ascendalloc.NewAllocator()
	default:
		err := errors.New("unknown allocation strategy")
		checkErr("", err)
		return nil, nil
	}
}

func setupMonitor(
	name string,
	h host.Host,
	basicCfg *basic.Config,
	pubsubCfg *pubsubmon.Config,
) ipfscluster.PeerMonitor {
	switch name {
	case "basic":
		mon, err := basic.NewMonitor(basicCfg)
		checkErr("creating monitor", err)
		return mon
	case "pubsub":
		mon, err := pubsubmon.New(h, pubsubCfg)
		checkErr("creating monitor", err)
		return mon
	default:
		err := errors.New("unknown monitor type")
		checkErr("", err)
		return nil
	}

}
