package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	ipfscluster "github.com/ipfs-cluster/ipfs-cluster"
	"github.com/ipfs-cluster/ipfs-cluster/allocator/balanced"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/api/rest"
	"github.com/ipfs-cluster/ipfs-cluster/cmdutils"
	"github.com/ipfs-cluster/ipfs-cluster/config"
	"github.com/ipfs-cluster/ipfs-cluster/consensus/crdt"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/badger"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/leveldb"
	"github.com/ipfs-cluster/ipfs-cluster/informer/disk"
	"github.com/ipfs-cluster/ipfs-cluster/informer/pinqueue"
	"github.com/ipfs-cluster/ipfs-cluster/informer/tags"
	"github.com/ipfs-cluster/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs-cluster/ipfs-cluster/monitor/pubsubmon"
	"github.com/ipfs-cluster/ipfs-cluster/observations"
	"github.com/ipfs-cluster/ipfs-cluster/pintracker/stateless"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	cli "github.com/urfave/cli/v2"
)

func printFirstStart() {
	fmt.Printf(`
No clusters configured yet!
 
If this is the first time you are running %s,
be sure to check out the usage documentation. Here are some
examples to get you going:

$ %s --help                     - general description and usage help
$ %s <clusterName> --help       - Help and subcommands for the <clusterName>'s follower peer
$ %s <clusterName> info --help  - Help for the "info" subcommand (same for others).
`, programName, programName, programName, programName)
}

func printNotInitialized(clusterName string) {
	fmt.Printf(`
This cluster peer has not been initialized.

Try running "%s %s init <config-url>" first.
`, programName, clusterName)
}

func setLogLevels(lvl string) {
	for f := range ipfscluster.LoggingFacilities {
		ipfscluster.SetFacilityLogLevel(f, lvl)
	}

	for f := range ipfscluster.LoggingFacilitiesExtra {
		ipfscluster.SetFacilityLogLevel(f, lvl)
	}
}

// returns whether the config folder exists
func isInitialized(absPath string) bool {
	_, err := os.Stat(absPath)
	return err == nil
}

func listClustersCmd(c *cli.Context) error {
	absPath, _, _ := buildPaths(c, "")
	f, err := os.Open(absPath)
	if os.IsNotExist(err) {
		printFirstStart()
		return nil
	}
	if err != nil {
		return cli.Exit(err, 1)
	}

	dirs, err := f.Readdir(-1)
	if err != nil {
		return cli.Exit(errors.Wrapf(err, "reading %s", absPath), 1)
	}

	var filteredDirs []string
	for _, d := range dirs {
		if d.IsDir() {
			configPath := filepath.Join(absPath, d.Name(), DefaultConfigFile)
			if _, err := os.Stat(configPath); err == nil {
				filteredDirs = append(filteredDirs, d.Name())
			}
		}
	}

	if len(filteredDirs) == 0 {
		printFirstStart()
		return nil
	}

	fmt.Printf("Configurations found for %d follower peers. For info and help, try running:\n\n", len(filteredDirs))
	for _, d := range filteredDirs {
		fmt.Printf("%s \"%s\"\n", programName, d)
	}
	fmt.Printf("\nTip: \"%s --help\" for help and examples.\n", programName)

	return nil
}

func infoCmd(c *cli.Context) error {
	clusterName := c.String("clusterName")

	// Avoid pollution of the screen
	setLogLevels("critical")

	absPath, configPath, identityPath := buildPaths(c, clusterName)

	if !isInitialized(absPath) {
		printNotInitialized(clusterName)
		return cli.Exit("", 1)
	}

	cfgHelper, err := cmdutils.NewLoadedConfigHelper(configPath, identityPath)
	var url string
	if err != nil {
		if config.IsErrFetchingSource(err) {
			url = fmt.Sprintf(
				"failed retrieving configuration source (%s)",
				cfgHelper.Manager().Source,
			)
			ipfsCfg := ipfshttp.Config{}
			ipfsCfg.Default()
			cfgHelper.Configs().Ipfshttp = &ipfsCfg
		} else {
			return cli.Exit(errors.Wrapf(err, "reading the configurations in %s", absPath), 1)
		}
	} else {
		url = fmt.Sprintf("Available (%s)", cfgHelper.Manager().Source)
	}
	cfgHelper.Manager().Shutdown()

	fmt.Printf("Information about follower peer for Cluster \"%s\":\n\n", clusterName)
	fmt.Printf("Config folder: %s\n", absPath)
	fmt.Printf("Config source URL: %s\n", url)

	ctx := context.Background()
	client, err := getClient(absPath, clusterName)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "error creating client"), 1)
	}
	_, err = client.Version(ctx)
	fmt.Printf("Cluster Peer online: %t\n", err == nil)

	// Either we loaded a valid config, or we are using a default. Worth
	// applying env vars in the second case.
	if err := cfgHelper.Configs().Ipfshttp.ApplyEnvVars(); err != nil {
		return cli.Exit(errors.Wrap(err, "applying environment variables to ipfshttp config"), 1)
	}

	cfgHelper.Configs().Ipfshttp.ConnectSwarmsDelay = 0
	connector, err := ipfshttp.NewConnector(cfgHelper.Configs().Ipfshttp)
	if err == nil {
		_, err = connector.ID(ctx)
	}
	fmt.Printf("IPFS peer online: %t\n", err == nil)

	if c.Command.Name == "" {
		fmt.Printf("Additional help:\n\n")
		fmt.Printf("-------------------------------------------------\n\n")
		return cli.ShowAppHelp(c)
	}
	return nil
}

func initCmd(c *cli.Context) error {
	if !c.Args().Present() {
		return cli.Exit("configuration URL not provided", 1)
	}
	cfgURL := c.Args().First()

	return initCluster(c, false, cfgURL)
}

func initCluster(c *cli.Context, ignoreReinit bool, cfgURL string) error {
	clusterName := c.String(clusterNameFlag)

	absPath, configPath, identityPath := buildPaths(c, clusterName)

	if isInitialized(absPath) {
		if ignoreReinit {
			fmt.Println("Configuration for this cluster already exists. Skipping initialization.")
			fmt.Printf("If you wish to re-initialize, simply delete %s\n\n", absPath)
			return nil
		}
		cmdutils.ErrorOut("Configuration for this cluster already exists.\n")
		cmdutils.ErrorOut("Please delete %s if you wish to re-initialize.", absPath)
		return cli.Exit("", 1)
	}

	gw := c.String("gateway")

	if !strings.HasPrefix(cfgURL, "http://") && !strings.HasPrefix(cfgURL, "https://") {
		fmt.Printf("%s will be assumed to be an DNSLink-powered address: /ipns/%s.\n", cfgURL, cfgURL)
		fmt.Printf("It will be resolved using the local IPFS daemon's gateway (%s).\n", gw)
		fmt.Println("If this is not the case, specify the full url starting with http:// or https://.")
		fmt.Println("(You can override the gateway URL by setting IPFS_GATEWAY)")
		fmt.Println()
		cfgURL = fmt.Sprintf("http://%s/ipns/%s", gw, cfgURL)
	}

	// Setting the datastore here is useless, as we initialize with remote
	// config and we will have an empty service.json with the source only.
	// That source will decide which datastore is actually used.
	cfgHelper := cmdutils.NewConfigHelper(configPath, identityPath, "crdt", "")
	cfgHelper.Manager().Shutdown()
	cfgHelper.Manager().Source = cfgURL
	err := cfgHelper.Manager().Default()
	if err != nil {
		return cli.Exit(errors.Wrap(err, "error generating default config"), 1)
	}

	ident := cfgHelper.Identity()
	err = ident.Default()
	if err != nil {
		return cli.Exit(errors.Wrap(err, "error generating identity"), 1)
	}

	err = ident.ApplyEnvVars()
	if err != nil {
		return cli.Exit(errors.Wrap(err, "error applying environment variables to the identity"), 1)
	}

	err = cfgHelper.SaveIdentityToDisk()
	if err != nil {
		return cli.Exit(errors.Wrapf(err, "error saving %s", identityPath), 1)
	}
	fmt.Printf("Identity written to %s.\n", identityPath)

	err = cfgHelper.SaveConfigToDisk()
	if err != nil {
		return cli.Exit(errors.Wrapf(err, "saving %s", configPath), 1)
	}

	fmt.Printf("Configuration written to %s.\n", configPath)
	fmt.Printf("Cluster \"%s\" follower peer initialized.\n\n", clusterName)
	fmt.Printf(
		"You can now use \"%s %s run\" to start a follower peer for this cluster.\n",
		programName,
		clusterName,
	)
	fmt.Println("(Remember to start your IPFS daemon before)")
	return nil
}

func runCmd(c *cli.Context) error {
	clusterName := c.String(clusterNameFlag)

	if cfgURL := c.String("init"); cfgURL != "" {
		err := initCluster(c, true, cfgURL)
		if err != nil {
			return err
		}
	}

	absPath, configPath, identityPath := buildPaths(c, clusterName)

	if !isInitialized(absPath) {
		printNotInitialized(clusterName)
		return cli.Exit("", 1)
	}

	fmt.Printf("Starting the IPFS Cluster follower peer for \"%s\".\nCTRL-C to stop it.\n", clusterName)
	fmt.Println("Checking if IPFS is online (will wait for 2 minutes)...")
	ctxIpfs, cancelIpfs := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelIpfs()
	err := cmdutils.WaitForIPFS(ctxIpfs)
	if err != nil {
		return cli.Exit("timed out waiting for IPFS to be available", 1)
	}

	setLogLevels(logLevel) // set to "info" by default.
	// Avoid API logs polluting the screen everytime we
	// run some "list" command.
	ipfscluster.SetFacilityLogLevel("restapilog", "error")

	cfgHelper, err := cmdutils.NewLoadedConfigHelper(configPath, identityPath)
	if err != nil {
		return cli.Exit(errors.Wrapf(err, "reading the configurations in %s", absPath), 1)
	}
	cfgHelper.Manager().Shutdown()
	cfgs := cfgHelper.Configs()

	stmgr, err := cmdutils.NewStateManager(cfgHelper.GetConsensus(), cfgHelper.GetDatastore(), cfgHelper.Identity(), cfgs)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "creating state manager"), 1)
	}

	store, err := stmgr.GetStore()
	if err != nil {
		return cli.Exit(errors.Wrap(err, "creating datastore"), 1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host, pubsub, dht, err := ipfscluster.NewClusterHost(ctx, cfgHelper.Identity(), cfgs.Cluster, store)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "error creating libp2p components"), 1)
	}

	// Always run followers in follower mode.
	cfgs.Cluster.FollowerMode = true
	// Do not let trusted peers GC this peer
	// Defaults to Trusted otherwise.
	cfgs.Cluster.RPCPolicy["Cluster.RepoGCLocal"] = ipfscluster.RPCClosed

	// Discard API configurations and create our own (unix socket)
	apiCfg := rest.NewConfig()
	cfgs.Restapi = apiCfg
	_ = apiCfg.Default()
	listenSocket, err := socketAddress(absPath, clusterName)
	if err != nil {
		return cli.Exit(err, 1)
	}
	apiCfg.HTTPListenAddr = []multiaddr.Multiaddr{listenSocket}
	// Allow customization via env vars
	err = apiCfg.ApplyEnvVars()
	if err != nil {
		return cli.Exit(errors.Wrap(err, "error applying environmental variables to restapi configuration"), 1)
	}

	rest, err := rest.NewAPI(ctx, apiCfg)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "creating REST API component"), 1)
	}

	connector, err := ipfshttp.NewConnector(cfgs.Ipfshttp)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "creating IPFS Connector component"), 1)
	}

	var informers []ipfscluster.Informer
	if cfgHelper.Manager().IsLoadedFromJSON(config.Informer, cfgs.DiskInf.ConfigKey()) {
		diskInf, err := disk.NewInformer(cfgs.DiskInf)
		if err != nil {
			return cli.Exit(errors.Wrap(err, "creating disk informer"), 1)
		}
		informers = append(informers, diskInf)
	}
	if cfgHelper.Manager().IsLoadedFromJSON(config.Informer, cfgs.TagsInf.ConfigKey()) {
		tagsInf, err := tags.New(cfgs.TagsInf)
		if err != nil {
			return cli.Exit(errors.Wrap(err, "creating numpin informer"), 1)
		}
		informers = append(informers, tagsInf)
	}

	if cfgHelper.Manager().IsLoadedFromJSON(config.Informer, cfgs.PinQueueInf.ConfigKey()) {
		pinQueueInf, err := pinqueue.New(cfgs.PinQueueInf)
		if err != nil {
			return cli.Exit(errors.Wrap(err, "creating pinqueue informer"), 1)
		}
		informers = append(informers, pinQueueInf)
	}

	alloc, err := balanced.New(cfgs.BalancedAlloc)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "creating metrics allocator"), 1)
	}

	crdtcons, err := crdt.New(
		host,
		dht,
		pubsub,
		cfgs.Crdt,
		store,
	)
	if err != nil {
		store.Close()
		return cli.Exit(errors.Wrap(err, "creating CRDT component"), 1)
	}

	tracker := stateless.New(cfgs.Statelesstracker, host.ID(), cfgs.Cluster.Peername, crdtcons.State)

	mon, err := pubsubmon.New(ctx, cfgs.Pubsubmon, pubsub, nil)
	if err != nil {
		store.Close()
		return cli.Exit(errors.Wrap(err, "setting up PeerMonitor"), 1)
	}

	// Hardcode disabled tracing and metrics to avoid mistakenly
	// exposing any user data.
	tracerCfg := observations.TracingConfig{}
	_ = tracerCfg.Default()
	tracerCfg.EnableTracing = false
	cfgs.Tracing = &tracerCfg
	tracer, err := observations.SetupTracing(&tracerCfg)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "error setting up tracer"), 1)
	}

	// This does nothing since we are not calling SetupMetrics anyways
	// But stays just to be explicit.
	metricsCfg := observations.MetricsConfig{}
	_ = metricsCfg.Default()
	metricsCfg.EnableStats = false
	cfgs.Metrics = &metricsCfg

	// We are going to run a cluster peer and should do an
	// oderly shutdown if we are interrupted: cancel default
	// signal handling and leave things to HandleSignals.
	signal.Stop(signalChan)
	close(signalChan)

	cluster, err := ipfscluster.NewCluster(
		ctx,
		host,
		dht,
		cfgs.Cluster,
		store,
		crdtcons,
		[]ipfscluster.API{rest},
		connector,
		tracker,
		mon,
		alloc,
		informers,
		tracer,
	)
	if err != nil {
		store.Close()
		return cli.Exit(errors.Wrap(err, "error creating cluster peer"), 1)
	}

	return cmdutils.HandleSignals(ctx, cancel, cluster, host, dht, store)
}

// List
func listCmd(c *cli.Context) error {
	clusterName := c.String("clusterName")

	absPath, configPath, identityPath := buildPaths(c, clusterName)
	if !isInitialized(absPath) {
		printNotInitialized(clusterName)
		return cli.Exit("", 1)
	}

	err := printStatusOnline(absPath, clusterName)
	if err == nil {
		return nil
	}

	// There was an error. Try offline status
	apiErr, ok := err.(*api.Error)
	if ok && apiErr.Code != 0 {
		return cli.Exit(
			errors.Wrapf(
				err,
				"The Peer API seems to be running but returned with code %d",
				apiErr.Code,
			), 1)
	}

	// We are on offline mode so we cannot rely on IPFS being
	// running and most probably our configuration is remote and
	// to be loaded from IPFS. Thus we need to find a different
	// way to decide whether to load badger/leveldb, and once we
	// know, do it with the default settings.
	hasLevelDB := false
	lDBCfg := &leveldb.Config{}
	lDBCfg.SetBaseDir(absPath)
	lDBCfg.Default()
	levelDBInfo, err := os.Stat(lDBCfg.GetFolder())
	if err == nil && levelDBInfo.IsDir() {
		hasLevelDB = true
	}

	hasBadger := false
	badgerCfg := &badger.Config{}
	badgerCfg.SetBaseDir(absPath)
	badgerCfg.Default()
	badgerInfo, err := os.Stat(badgerCfg.GetFolder())
	if err == nil && badgerInfo.IsDir() {
		hasBadger = true
	}

	if hasLevelDB && hasBadger {
		return cli.Exit(errors.Wrapf(err, "found both leveldb (%s) and badger (%s) folders: cannot determine which to use in offline mode", lDBCfg.GetFolder(), badgerCfg.GetFolder()), 1)
	}

	// Since things were initialized, assume there is one at least.
	dstoreType := "leveldb"
	if hasBadger {
		dstoreType = "badger"
	}
	cfgHelper := cmdutils.NewConfigHelper(configPath, identityPath, "crdt", dstoreType)
	cfgHelper.Manager().Shutdown() // not needed
	cfgHelper.Configs().Badger.SetBaseDir(absPath)
	cfgHelper.Configs().LevelDB.SetBaseDir(absPath)
	cfgHelper.Manager().Default() // we have a default crdt config with either leveldb or badger registered.
	cfgHelper.Manager().ApplyEnvVars()

	err = printStatusOffline(cfgHelper)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "error obtaining the pinset"), 1)
	}

	return nil
}

func printStatusOnline(absPath, clusterName string) error {
	ctx := context.Background()
	client, err := getClient(absPath, clusterName)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "error creating client"), 1)
	}

	out := make(chan api.GlobalPinInfo, 1024)
	errCh := make(chan error, 1)

	go func() {
		defer close(errCh)
		errCh <- client.StatusAll(ctx, 0, true, out)
	}()

	var pid string
	for gpi := range out {
		if pid == "" { // do this once
			// PeerMap will only have one key
			for k := range gpi.PeerMap {
				pid = k
				break
			}
		}
		pinInfo := gpi.PeerMap[pid]
		printPin(gpi.Cid, pinInfo.Status.String(), gpi.Name, pinInfo.Error)
	}
	err = <-errCh
	return err
}

func printStatusOffline(cfgHelper *cmdutils.ConfigHelper) error {
	mgr, err := cmdutils.NewStateManagerWithHelper(cfgHelper)
	if err != nil {
		return err
	}
	store, err := mgr.GetStore()
	if err != nil {
		return err
	}
	defer store.Close()
	st, err := mgr.GetOfflineState(store)
	if err != nil {
		return err
	}

	out := make(chan api.Pin, 1024)
	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		errCh <- st.List(context.Background(), out)
	}()

	for pin := range out {
		printPin(pin.Cid, "offline", pin.Name, "")
	}

	err = <-errCh
	return err
}

func printPin(c api.Cid, status, name, err string) {
	if err != "" {
		name = name + " (" + err + ")"
	}
	fmt.Printf("%-20s %s %s\n", status, c, name)
}
