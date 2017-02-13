package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"

	logging "github.com/ipfs/go-log"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/allocator/numpinalloc"
	"github.com/ipfs/ipfs-cluster/informer/numpin"
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
	// DefaultPath is initialized to something like ~/.ipfs-cluster/service.json
	// and holds all the ipfs-cluster data
	DefaultPath = ".ipfs-cluster"
	// The name of the configuration file inside DefaultPath
	DefaultConfigFile = "service.json"
	// The name of the data folder inside DefaultPath
	DefaultDataFolder = "data"
)

var (
	configPath string
	dataPath   string
)

func init() {
	// The only way I could make this work
	ipfscluster.Commit = commit
	usr, err := user.Current()
	if err != nil {
		panic("cannot guess the current user")
	}
	DefaultPath = filepath.Join(
		usr.HomeDir,
		".ipfs-cluster")
}

func out(m string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, m, a...)
}

func checkErr(doing string, err error) {
	if err != nil {
		out("error %s: %s\n", doing, err)
		os.Exit(1)
	}
}

func main() {
	app := cli.NewApp()
	app.Name = programName
	app.Usage = "IPFS Cluster node"
	app.Description = Description
	//app.Copyright = "© Protocol Labs, Inc."
	app.Version = ipfscluster.Version
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:   "init",
			Usage:  "create a default configuration and exit",
			Hidden: true,
		},
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
	}

	app.Commands = []cli.Command{
		{
			Name:  "init",
			Usage: "create a default configuration and exit",
			Action: func(c *cli.Context) error {
				initConfig(c.GlobalBool("force"))
				return nil
			},
		},
		{
			Name:   "run",
			Usage:  "run the IPFS Cluster peer (default)",
			Action: run,
		},
	}

	app.Before = func(c *cli.Context) error {
		absPath, err := filepath.Abs(c.String("config"))
		if err != nil {
			return err
		}

		configPath = filepath.Join(absPath, DefaultConfigFile)
		dataPath = filepath.Join(absPath, DefaultDataFolder)

		setupLogging(c.String("loglevel"))
		if c.Bool("debug") {
			setupDebug()
		}
		return nil
	}

	app.Action = run

	app.Run(os.Args)
}

func run(c *cli.Context) error {
	if c.Bool("init") {
		initConfig(c.Bool("force"))
		return nil
	}

	cfg, err := loadConfig()
	checkErr("loading configuration", err)

	if a := c.String("bootstrap"); a != "" {
		if len(cfg.ClusterPeers) > 0 && !c.Bool("force") {
			return errors.New("the configuration provides ClusterPeers. Use -f to ignore and proceed bootstrapping")
		}
		joinAddr, err := ma.NewMultiaddr(a)
		if err != nil {
			return fmt.Errorf("error parsing multiaddress: %s", err)
		}
		cfg.Bootstrap = []ma.Multiaddr{joinAddr}
		cfg.ClusterPeers = []ma.Multiaddr{}
	}

	if c.Bool("leave") {
		cfg.LeaveOnShutdown = true
	}

	api, err := ipfscluster.NewRESTAPI(cfg)
	checkErr("creating REST API component", err)

	proxy, err := ipfscluster.NewIPFSHTTPConnector(cfg)
	checkErr("creating IPFS Connector component", err)

	state := mapstate.NewMapState()
	tracker := ipfscluster.NewMapPinTracker(cfg)
	mon := ipfscluster.NewStdPeerMonitor(5)
	informer := numpin.NewInformer()
	alloc := numpinalloc.NewAllocator()

	cluster, err := ipfscluster.NewCluster(
		cfg,
		api,
		proxy,
		state,
		tracker,
		mon,
		alloc,
		informer)
	checkErr("starting cluster", err)

	signalChan := make(chan os.Signal, 20)
	signal.Notify(signalChan, os.Interrupt)
	for {
		select {
		case <-signalChan:
			err = cluster.Shutdown()
			checkErr("shutting down cluster", err)
		case <-cluster.Done():
			return nil
		case <-cluster.Ready():
		}
	}
}

func setupLogging(lvl string) {
	ipfscluster.SetFacilityLogLevel("service", lvl)
	ipfscluster.SetFacilityLogLevel("cluster", lvl)
	//ipfscluster.SetFacilityLogLevel("raft", lvl)
}

func setupDebug() {
	l := "DEBUG"
	ipfscluster.SetFacilityLogLevel("cluster", l)
	ipfscluster.SetFacilityLogLevel("raft", l)
	ipfscluster.SetFacilityLogLevel("p2p-gorpc", l)
	//SetFacilityLogLevel("swarm2", l)
	//SetFacilityLogLevel("libp2p-raft", l)
}

func initConfig(force bool) {
	if _, err := os.Stat(configPath); err == nil && !force {
		err := fmt.Errorf("%s exists. Try running with -f", configPath)
		checkErr("", err)
	}
	cfg, err := ipfscluster.NewDefaultConfig()
	checkErr("creating default configuration", err)
	cfg.ConsensusDataFolder = dataPath
	err = os.MkdirAll(filepath.Dir(configPath), 0700)
	err = cfg.Save(configPath)
	checkErr("saving new configuration", err)
	out("%s configuration written to %s\n",
		programName, configPath)
}

func loadConfig() (*ipfscluster.Config, error) {
	return ipfscluster.LoadConfig(configPath)
}
