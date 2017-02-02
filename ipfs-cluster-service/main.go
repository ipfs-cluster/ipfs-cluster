package main

import (
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"

	logging "github.com/ipfs/go-log"
	"github.com/urfave/cli"

	ipfscluster "github.com/ipfs/ipfs-cluster"
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
using LibP2P.

%s needs a valid configuration to run. This configuration is
independent from IPFS and includes its own LibP2P key-pair. It can be
initialized with "init" and its default location is
 ~/%s/%s.

For feedback, bug reports or any additional information, visit
https://github.com/ipfs/ipfs-cluster.
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
	app.UsageText = Description
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
			Usage: "force configuration overwrite when running 'init'",
		},
		cli.BoolFlag{
			Name:  "debug, d",
			Usage: "enable full debug logging",
		},
		cli.StringFlag{
			Name:  "loglevel, l",
			Value: "info",
			Usage: "set the loglevel [critical, error, warning, info, debug]",
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

	app.Action = func(c *cli.Context) error {
		if c.Bool("init") {
			initConfig(c.Bool("force"))
			return nil
		}

		cfg, err := loadConfig()
		checkErr("loading configuration", err)

		api, err := ipfscluster.NewRESTAPI(cfg)
		checkErr("creating REST API component", err)

		proxy, err := ipfscluster.NewIPFSHTTPConnector(cfg)
		checkErr("creating IPFS Connector component", err)

		state := ipfscluster.NewMapState()
		tracker := ipfscluster.NewMapPinTracker(cfg)
		cluster, err := ipfscluster.NewCluster(
			cfg,
			api,
			proxy,
			state,
			tracker)
		checkErr("starting cluster", err)

		signalChan := make(chan os.Signal)
		signal.Notify(signalChan, os.Interrupt)

		for {
			select {
			case <-signalChan:
				err = cluster.Shutdown()
				checkErr("shutting down cluster", err)
				return nil
			case <-cluster.Done():
				return nil
			case <-cluster.Ready():
				logger.Info("IPFS Cluster is ready")
			}
		}
	}

	app.Run(os.Args)
}

func setupLogging(lvl string) {
	logging.SetLogLevel("service", lvl)
	logging.SetLogLevel("cluster", lvl)
	//logging.SetLogLevel("raft", lvl)
}

func setupDebug() {
	logging.SetLogLevel("cluster", "debug")
	//logging.SetLogLevel("libp2p-raft", "debug")
	logging.SetLogLevel("p2p-gorpc", "debug")
	//logging.SetLogLevel("swarm2", "debug")
	logging.SetLogLevel("raft", "debug")
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
