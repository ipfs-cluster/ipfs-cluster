package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"syscall"

	//	_ "net/http/pprof"

	logging "github.com/ipfs/go-log"
	ma "github.com/multiformats/go-multiaddr"
	cli "github.com/urfave/cli"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/allocator/ascendalloc"
	"github.com/ipfs/ipfs-cluster/allocator/descendalloc"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/config"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/informer/disk"
	"github.com/ipfs/ipfs-cluster/informer/numpin"
	"github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs/ipfs-cluster/monitor/basic"
	"github.com/ipfs/ipfs-cluster/pintracker/maptracker"
	"github.com/ipfs/ipfs-cluster/state/mapstate"
)

// ProgramName of this application
const programName = `ipfs-cluster-service`

// We store a commit id here
var commit string

// Description provides a short summary of the functionality of this tool
var Description = fmt.Sprintf(`
%s runs an IPFS Cluster node.

A node participates in the cluster consensus, follows a distributed log
of pinning and unpinning requests and manages pinning operations to a
configured IPFS daemon.

This node also provides an API for cluster management, an IPFS Proxy API which
forwards requests to IPFS and a number of components for internal communication
using LibP2P. This is a simplified view of the components:

             +------------------+
             | ipfs-cluster-ctl |
             +---------+--------+
                       |
                       | HTTP
ipfs-cluster-service   |                           HTTP
+----------+--------+--v--+----------------------+      +-------------+
| RPC/Raft | Peer 1 | API | IPFS Connector/Proxy +------> IPFS daemon |
+----^-----+--------+-----+----------------------+      +-------------+
     | libp2p
     |
+----v-----+--------+-----+----------------------+      +-------------+
| RPC/Raft | Peer 2 | API | IPFS Connector/Proxy +------> IPFS daemon |
+----^-----+--------+-----+----------------------+      +-------------+
     |
     |
+----v-----+--------+-----+----------------------+      +-------------+
| RPC/Raft | Peer 3 | API | IPFS Connector/Proxy +------> IPFS daemon |
+----------+--------+-----+----------------------+      +-------------+


%s needs a valid configuration to run. This configuration is
independent from IPFS and includes its own LibP2P key-pair. It can be
initialized with "init" and its default location is
 ~/%s/%s.

For feedback, bug reports or any additional information, visit
https://github.com/ipfs/ipfs-cluster.


EXAMPLES

Initial configuration:

$ ipfs-cluster-service init

Launch a cluster:

$ ipfs-cluster-service

Launch a peer and join existing cluster:

$ ipfs-cluster-service --bootstrap /ip4/192.168.1.2/tcp/9096/ipfs/QmPSoSaPXpyunaBwHs1rZBKYSqRV4bLRk32VGYLuvdrypL
`,
	programName,
	programName,
	DefaultPath,
	DefaultConfigFile)

var logger = logging.Logger("service")

// Default location for the configurations and data
var (
	// DefaultPath is initialized to $HOME/.ipfs-cluster
	// and holds all the ipfs-cluster data
	DefaultPath string
	// The name of the configuration file inside DefaultPath
	DefaultConfigFile = "service.json"
)

var (
	configPath string
)

func init() {
	// Set the right commit. The only way I could make this work
	ipfscluster.Commit = commit

	// We try guessing user's home from the HOME variable. This
	// allows HOME hacks for things like Snapcraft builds. HOME
	// should be set in all UNIX by the OS. Alternatively, we fall back to
	// usr.HomeDir (which should work on Windows etc.).
	home := os.Getenv("HOME")
	if home == "" {
		usr, err := user.Current()
		if err != nil {
			panic(fmt.Sprintf("cannot get current user: %s", err))
		}
		home = usr.HomeDir
	}

	DefaultPath = filepath.Join(home, ".ipfs-cluster")
}

func out(m string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, m, a...)
}

func checkErr(doing string, err error) {
	if err != nil {
		out("error %s: %s\n", doing, err)
		err = locker.tryUnlock()
		if err != nil {
			out("error releasing execution lock: %s\n", err)
		}
		os.Exit(1)
	}
}

func main() {
	// go func() {
	//	log.Println(http.ListenAndServe("localhost:6060", nil))
	// }()

	app := cli.NewApp()
	app.Name = programName
	app.Usage = "IPFS Cluster node"
	app.Description = Description
	//app.Copyright = "© Protocol Labs, Inc."
	app.Version = ipfscluster.Version
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "config, c",
			Value:  DefaultPath,
			Usage:  "path to the configuration and data `FOLDER`",
			EnvVar: "IPFS_CLUSTER_PATH",
		},
		cli.BoolFlag{
			Name:  "force, f",
			Usage: "forcefully proceed with some actions. i.e. overwriting configuration",
		},
		cli.StringFlag{
			Name:  "bootstrap, j",
			Usage: "join a cluster providing an existing peer's `multiaddress`. Overrides the \"bootstrap\" values from the configuration",
		},
		cli.BoolFlag{
			Name:   "leave, x",
			Usage:  "remove peer from cluster on exit. Overrides \"leave_on_shutdown\"",
			Hidden: true,
		},
		cli.BoolFlag{
			Name:  "debug, d",
			Usage: "enable full debug logging (very verbose)",
		},
		cli.StringFlag{
			Name:  "loglevel, l",
			Value: "info",
			Usage: "set the loglevel for cluster only [critical, error, warning, info, debug]",
		},
		cli.StringFlag{
			Name:  "alloc, a",
			Value: "disk-freespace",
			Usage: "allocation strategy to use [disk-freespace,disk-reposize,numpin].",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:  "init",
			Usage: "create a default configuration and exit",
			Description: fmt.Sprintf(`
This command will initialize a new service.json configuration file
for %s.

By default, %s requires a cluster secret. This secret will be
automatically generated, but can be manually provided with --custom-secret
(in which case it will be prompted), or by setting the CLUSTER_SECRET
environment variable.

The private key for the libp2p node is randomly generated in all cases.

Note that the --force first-level-flag allows to overwrite an existing
configuration.
`, programName, programName),
			ArgsUsage: " ",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "custom-secret, s",
					Usage: "prompt for the cluster secret",
				},
			},
			Action: func(c *cli.Context) error {
				userSecret, userSecretDefined := userProvidedSecret(c.Bool("custom-secret"))
				cfg, clustercfg, _, _, _, _, _, _, _ := makeConfigs()
				defer cfg.Shutdown() // wait for saves

				// Generate defaults for all registered components
				err := cfg.Default()
				checkErr("generating default configuration", err)

				// Set user secret
				if userSecretDefined {
					clustercfg.Secret = userSecret
				}

				// Save
				saveConfig(cfg, c.GlobalBool("force"))
				return nil
			},
		},
		{
			Name:   "daemon",
			Usage:  "run the IPFS Cluster peer (default)",
			Action: daemon,
		},
		{
			Name:  "state",
			Usage: "Manage ipfs-cluster-state",
			Subcommands: []cli.Command{
				{
					Name:  "upgrade",
					Usage: "upgrade the IPFS Cluster state to the current version",
					Description: `

This command upgrades the internal state of the ipfs-cluster node 
specified in the latest raft snapshot. The state format is migrated from the 
version of the snapshot to the version supported by the current cluster version. 
To successfully run an upgrade of an entire cluster, shut down each peer without
removal, upgrade state using this command, and restart every peer.
`,
					Action: func(c *cli.Context) error {
						err := upgrade()
						checkErr("upgrading state", err)
						return err
					},
				},
			},
		},
	}

	app.Before = func(c *cli.Context) error {
		absPath, err := filepath.Abs(c.String("config"))
		if err != nil {
			return err
		}

		configPath = filepath.Join(absPath, DefaultConfigFile)

		setupLogging(c.String("loglevel"))
		if c.Bool("debug") {
			setupDebug()
		}

		locker = &lock{path: absPath}

		return nil
	}

	app.Action = run

	app.Run(os.Args)
}

// run daemon() by default, or error.
func run(c *cli.Context) error {
	if len(c.Args()) > 0 {
		return fmt.Errorf("unknown subcommand. Run \"%s help\" for more info", programName)
	}
	return daemon(c)
}

func daemon(c *cli.Context) error {
	// Load all the configurations
	cfg, clusterCfg, apiCfg, ipfshttpCfg, consensusCfg, trackerCfg, monCfg, diskInfCfg, numpinInfCfg := makeConfigs()
	// Execution lock
	err := locker.lock()
	checkErr("acquiring execution lock", err)
	defer locker.tryUnlock()

	// Load all the configurations
	// always wait for configuration to be saved
	defer cfg.Shutdown()

	err = cfg.LoadJSONFromFile(configPath)
	checkErr("loading configuration", err)

	if a := c.String("bootstrap"); a != "" {
		if len(clusterCfg.Peers) > 0 && !c.Bool("force") {
			return errors.New("the configuration provides cluster.Peers. Use -f to ignore and proceed bootstrapping")
		}
		joinAddr, err := ma.NewMultiaddr(a)
		if err != nil {
			return fmt.Errorf("error parsing multiaddress: %s", err)
		}
		clusterCfg.Bootstrap = []ma.Multiaddr{joinAddr}
		clusterCfg.Peers = []ma.Multiaddr{}
	}

	if c.Bool("leave") {
		clusterCfg.LeaveOnShutdown = true
	}

	api, err := rest.NewAPI(apiCfg)
	checkErr("creating REST API component", err)

	proxy, err := ipfshttp.NewConnector(ipfshttpCfg)
	checkErr("creating IPFS Connector component", err)

	state := mapstate.NewMapState()

	err = validateVersion(clusterCfg, consensusCfg)
	checkErr("validating version", err)

	tracker := maptracker.NewMapPinTracker(trackerCfg, clusterCfg.ID)
	mon, err := basic.NewMonitor(monCfg)
	checkErr("creating Monitor component", err)
	informer, alloc := setupAllocation(c.String("alloc"), diskInfCfg, numpinInfCfg)

	cluster, err := ipfscluster.NewCluster(
		clusterCfg,
		consensusCfg,
		api,
		proxy,
		state,
		tracker,
		mon,
		alloc,
		informer)
	checkErr("starting cluster", err)

	signalChan := make(chan os.Signal, 20)
	signal.Notify(signalChan,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP)
	for {
		select {
		case <-signalChan:
			err = cluster.Shutdown()
			checkErr("shutting down cluster", err)
		case <-cluster.Done():
			return nil

			//case <-cluster.Ready():
		}
	}
}

var facilities = []string{
	"service",
	"cluster",
	"restapi",
	"ipfshttp",
	"mapstate",
	"monitor",
	"consensus",
	"pintracker",
	"ascendalloc",
	"descendalloc",
	"diskinfo",
	"config",
}

func setupLogging(lvl string) {
	for _, f := range facilities {
		ipfscluster.SetFacilityLogLevel(f, lvl)
	}
}

func setupAllocation(name string, diskInfCfg *disk.Config, numpinInfCfg *numpin.Config) (ipfscluster.Informer, ipfscluster.PinAllocator) {
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

func setupDebug() {
	l := "DEBUG"
	for _, f := range facilities {
		ipfscluster.SetFacilityLogLevel(f, l)
	}

	ipfscluster.SetFacilityLogLevel("p2p-gorpc", l)
	ipfscluster.SetFacilityLogLevel("raft", l)
	//SetFacilityLogLevel("swarm2", l)
	//SetFacilityLogLevel("libp2p-raft", l)
}

func saveConfig(cfg *config.Manager, force bool) {
	if _, err := os.Stat(configPath); err == nil && !force {
		err := fmt.Errorf("%s exists. Try running: %s -f init", configPath, programName)
		checkErr("", err)
	}

	err := os.MkdirAll(filepath.Dir(configPath), 0700)
	err = cfg.SaveJSON(configPath)
	checkErr("saving new configuration", err)
	out("%s configuration written to %s\n",
		programName, configPath)
}

func userProvidedSecret(enterSecret bool) ([]byte, bool) {
	var secret string
	if enterSecret {
		secret = promptUser("Enter cluster secret (32-byte hex string): ")
	} else if envSecret, envSecretDefined := os.LookupEnv("CLUSTER_SECRET"); envSecretDefined {
		secret = envSecret
	} else {
		return nil, false
	}

	decodedSecret, err := ipfscluster.DecodeClusterSecret(secret)
	checkErr("parsing user-provided secret", err)
	return decodedSecret, true
}

func promptUser(msg string) string {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print(msg)
	scanner.Scan()
	return scanner.Text()
}

func makeConfigs() (*config.Manager, *ipfscluster.Config, *rest.Config, *ipfshttp.Config, *raft.Config, *maptracker.Config, *basic.Config, *disk.Config, *numpin.Config) {
	cfg := config.NewManager()
	clusterCfg := &ipfscluster.Config{}
	apiCfg := &rest.Config{}
	ipfshttpCfg := &ipfshttp.Config{}
	consensusCfg := &raft.Config{}
	trackerCfg := &maptracker.Config{}
	monCfg := &basic.Config{}
	diskInfCfg := &disk.Config{}
	numpinInfCfg := &numpin.Config{}
	cfg.RegisterComponent(config.Cluster, clusterCfg)
	cfg.RegisterComponent(config.API, apiCfg)
	cfg.RegisterComponent(config.IPFSConn, ipfshttpCfg)
	cfg.RegisterComponent(config.Consensus, consensusCfg)
	cfg.RegisterComponent(config.PinTracker, trackerCfg)
	cfg.RegisterComponent(config.Monitor, monCfg)
	cfg.RegisterComponent(config.Informer, diskInfCfg)
	cfg.RegisterComponent(config.Informer, numpinInfCfg)
	return cfg, clusterCfg, apiCfg, ipfshttpCfg, consensusCfg, trackerCfg, monCfg, diskInfCfg, numpinInfCfg
}
