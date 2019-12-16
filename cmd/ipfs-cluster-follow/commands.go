package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/allocator/descendalloc"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/cmdutils"
	"github.com/ipfs/ipfs-cluster/config"
	"github.com/ipfs/ipfs-cluster/consensus/crdt"
	"github.com/ipfs/ipfs-cluster/informer/disk"
	"github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs/ipfs-cluster/monitor/pubsubmon"
	"github.com/ipfs/ipfs-cluster/observations"
	"github.com/ipfs/ipfs-cluster/pintracker/stateless"
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

func printNotInitialized(clusterName string, err error) {
	fmt.Printf(`
This cluster peer has not been initialized or configurations cannot be read.

In the former case, try running "%s %s init <config-url>" first.
In the latter case, find more information in the error message below.


(Error message was: %s)
`, programName, clusterName, err)
}

func setLogLevels(lvl string) {
	for f := range ipfscluster.LoggingFacilities {
		ipfscluster.SetFacilityLogLevel(f, lvl)
	}
}

func listCmd(c *cli.Context) error {
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
	fmt.Printf("\nTip: \"%s --help\" for help and examples.", programName)

	return nil
}

func infoCmd(c *cli.Context) error {
	clusterName := c.String("clusterName")

	// Avoid pollution of the screen
	setLogLevels("critical")

	absPath, configPath, identityPath := buildPaths(c, clusterName)
	cfgHelper, err := cmdutils.NewLoadedConfigHelper(configPath, identityPath)
	var url string
	if err != nil {
		if config.IsErrFetchingSource(err) {
			url = fmt.Sprintf(
				"failed retrieving configuration source: %s",
				cfgHelper.Manager().Source,
			)
		} else {
			printNotInitialized(clusterName, err)
			return cli.Exit("", 1)
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

	connector, err := ipfshttp.NewConnector(cfgHelper.Configs().Ipfshttp)
	if err == nil {
		_, err = connector.ID(ctx)
	}
	fmt.Printf("IPFS peer online: %t\n", err == nil)

	if c.Command.Name == "" {
		fmt.Printf("Additional help:\n\n")
		fmt.Printf("-------------------------------------------------\n\n")
		cli.ShowAppHelp(c)
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

	if _, err := os.Stat(absPath); !os.IsNotExist(err) {
		if ignoreReinit {
			fmt.Println("Configuration for this cluster already exists. Skipping initialization.")
			fmt.Printf("If you wish to re-initialize, simply delete %s\n\n", absPath)
			return nil
		}
		cmdutils.ErrorOut("Configuration for this cluster already exists.\n")
		cmdutils.ErrorOut("Please delete %s if you wish to re-initialize.", absPath)
		return cli.Exit("", 1)
	}

	if !strings.HasPrefix(cfgURL, "http://") && !strings.HasPrefix(cfgURL, "https://") {
		fmt.Printf("%s will be assumed to be an DNSLink-powered address: /ipns/%s\n", cfgURL, cfgURL)
		fmt.Println("It will be resolved using the local IPFS daemon's gateway (localhost:8080)")
		fmt.Println("if this is not the case, specify the full url starting with http:// or https://.")
		fmt.Println()
		cfgURL = fmt.Sprintf("http://127.0.0.1:8080/ipns/%s", cfgURL)
	}

	cfgHelper := cmdutils.NewConfigHelper(configPath, identityPath, "crdt")
	cfgHelper.Manager().Shutdown()
	cfgHelper.Manager().Source = cfgURL
	cfgHelper.Manager().Default()

	ident := cfgHelper.Identity()
	err := ident.Default()
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

	absPath, configPath, identityPath := buildPaths(c, clusterName)
	cfgHelper, err := cmdutils.NewLoadedConfigHelper(configPath, identityPath)
	if err != nil {
		printNotInitialized(clusterName, err)
		return cli.Exit("", 1)
	}
	cfgHelper.Manager().Shutdown()
	cfgs := cfgHelper.Configs()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host, pubsub, dht, err := ipfscluster.NewClusterHost(ctx, cfgHelper.Identity(), cfgs.Cluster)

	if err != nil {
		return cli.Exit(errors.Wrap(err, "error creating libp2p components"), 1)
	}

	// Always run followers in follower mode.
	cfgs.Cluster.FollowerMode = true

	// Discard API configurations and create our own
	apiCfg := rest.Config{}
	cfgs.Restapi = &apiCfg
	apiCfg.Default()
	listenSocket, err := socketAddress(absPath, clusterName)
	if err != nil {
		return cli.Exit(err, 1)
	}
	apiCfg.HTTPListenAddr = listenSocket
	// Allow customization via env vars
	err = apiCfg.ApplyEnvVars()
	if err != nil {
		return cli.Exit(errors.Wrap(err, "error applying enviromental variables to restapi configuration"), 1)
	}

	rest, err := rest.NewAPI(ctx, &apiCfg)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "creating REST API component"), 1)
	}

	connector, err := ipfshttp.NewConnector(cfgs.Ipfshttp)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "creating IPFS Connector component"), 1)
	}

	informer, err := disk.NewInformer(cfgs.Diskinf)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "creating disk informer"), 1)
	}
	alloc := descendalloc.NewAllocator()

	stmgr, err := cmdutils.NewStateManager(cfgHelper.GetConsensus(), cfgHelper.Identity(), cfgs)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "creating state manager"), 1)
	}

	store, err := stmgr.GetStore()
	if err != nil {
		return cli.Exit(errors.Wrap(err, "creating datastore"), 1)
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
	tracerCfg.Default()
	tracerCfg.EnableTracing = false
	cfgs.Tracing = &tracerCfg
	cfgHelper.SetupTracing(false)
	tracer, err := observations.SetupTracing(&tracerCfg)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "error setting up tracer"), 1)
	}

	metricsCfg := observations.MetricsConfig{}
	metricsCfg.Default()
	metricsCfg.EnableStats = false
	cfgs.Metrics = &metricsCfg
	err = observations.SetupMetrics(cfgs.Metrics)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "error setting up metrics"), 1)
	}

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
		[]ipfscluster.Informer{informer},
		tracer,
	)
	if err != nil {
		store.Close()
		return cli.Exit(errors.Wrap(err, "error creating cluster peer"), 1)
	}

	return cmdutils.HandleSignals(ctx, cancel, cluster, host, dht)
}

// List
func pinsetCmd(c *cli.Context) error {
	clusterName := c.String("clusterName")

	absPath, configPath, identityPath := buildPaths(c, clusterName)
	cfgHelper, err := cmdutils.NewLoadedConfigHelper(configPath, identityPath)
	if err != nil {
		fmt.Println("error loading configurations.")
		if config.IsErrFetchingSource(err) {
			fmt.Println("Make sure the source URL is reachable:")
		}
		return cli.Exit(err, 1)
	}
	cfgHelper.Manager().Shutdown()

	err = printStatusOnline(absPath, clusterName)
	if err != nil {
		apiErr, ok := err.(*api.Error)
		if ok && apiErr.Code != 0 {
			return cli.Exit(
				errors.Wrapf(
					err,
					"The Peer API seems to be running but returned with code %d",
					apiErr.Code,
				), 1)
		}

		err := printStatusOffline(cfgHelper)
		if err != nil {
			logger.Error(err)
			return cli.Exit("error obtaining the pinset", 1)
		}
	}
	return nil
}

func printStatusOnline(absPath, clusterName string) error {
	ctx := context.Background()
	client, err := getClient(absPath, clusterName)
	if err != nil {
		return cli.Exit(errors.Wrap(err, "error creating client"), 1)
	}
	gpis, err := client.StatusAll(ctx, 0, true)
	if err != nil {
		return err
	}
	// do not return errors after this.

	var pid string
	for _, gpi := range gpis {
		if pid == "" { // do this once
			// PeerMap will only have one key
			for k := range gpi.PeerMap {
				pid = k
				break
			}
		}
		pinInfo := gpi.PeerMap[pid]

		// Get pin name
		var name string
		pin, err := client.Allocation(ctx, gpi.Cid)
		if err != nil {
			name = "(" + err.Error() + ")"
		} else {
			name = pin.Name
		}

		printPin(gpi.Cid, pinInfo.Status.String(), name, pinInfo.Error)
	}
	return nil
}

func printStatusOffline(cfgHelper *cmdutils.ConfigHelper) error {
	// The blockstore module loaded from ipfs-lite tends to print
	// an error when the datastore is closed before the bloom
	// filter cached has finished building. Could not find a way
	// to avoid it other than disabling bloom chaching on offline
	// ipfs-lite peers which is overkill. So we just hide it.
	ipfscluster.SetFacilityLogLevel("blockstore", "critical")

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
	pins, err := st.List(context.Background())
	if err != nil {
		return err
	}
	for _, pin := range pins {
		printPin(pin.Cid, "offline", pin.Name, "")
	}
	return nil
}

func printPin(c cid.Cid, status, name, err string) {
	if err != "" {
		name = name + " (" + err + ")"
	}
	fmt.Printf("%-20s %s %s\n", status, c, name)
}
