package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"

	logging "github.com/ipfs/go-log"

	ipfscluster "github.com/ipfs/ipfs-cluster"
)

// ProgramName of this application
const programName = `ipfs-cluster-service`

// Description provides a short summary of the functionality of this tool
var Description = fmt.Sprintf(`
%s runs an IPFS Cluster member (version %s).

A member is a node which participates in the cluster consensus, follows
a distributed log of pinning and unpinning operations and manages pinning
operations to a configured IPFS daemon.

This node also provides an API for cluster management, an IPFS Proxy API which
forwards requests to IPFS and a number of components for internal communication
using LibP2P.

%s needs a valid configuration to run. This configuration is 
independent from IPFS and includes its own LibP2P key-pair. It can be 
initialized with -init and its default location is
 ~/%s/%s.

For feedback, bug reports or any additional information, visit
https://github.com/ipfs/ipfs-cluster.

`,
	programName,
	ipfscluster.Version,
	programName,
	DefaultPath,
	DefaultConfigFile)

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

// Command line flags
var (
	initFlag     bool
	configFlag   string
	forceFlag    bool
	debugFlag    bool
	logLevelFlag string
	versionFlag  bool
)

func init() {
	if path := os.Getenv("IPFSCLUSTER_PATH"); path != "" {
		DefaultPath = path
	} else {
		usr, err := user.Current()
		if err != nil {
			panic("cannot guess the current user")
		}
		DefaultPath = filepath.Join(
			usr.HomeDir,
			".ipfs-cluster")
	}

	flag.Usage = func() {
		out("Usage: %s [options]\n", programName)
		out(Description)
		out("Options:\n")
		flag.PrintDefaults()
		out("\n")
	}
	flag.BoolVar(&initFlag, "init", false,
		"create a default configuration and exit")
	flag.StringVar(&configFlag, "config", DefaultPath,
		"path to the ipfs-cluster-service configuration and data folder")
	flag.BoolVar(&forceFlag, "f", false,
		"force configuration overwrite when running -init")
	flag.BoolVar(&debugFlag, "debug", false,
		"enable full debug logs of ipfs cluster and consensus layers")
	flag.StringVar(&logLevelFlag, "loglevel", "info",
		"set the loglevel [critical, error, warning, notice, info, debug]")
	flag.BoolVar(&versionFlag, "version", false,
		fmt.Sprintf("display %s version", programName))
	flag.Parse()

	absPath, err := filepath.Abs(configFlag)
	if err != nil {
		panic("error expading " + configFlag)
	}
	configPath = filepath.Join(absPath, DefaultConfigFile)
	dataPath = filepath.Join(absPath, DefaultDataFolder)

	setupLogging()
	setupDebug()
	if versionFlag {
		fmt.Println(ipfscluster.Version)
	}
	if initFlag || flag.Arg(0) == "init" {
		err := initConfig()
		checkErr("creating configuration", err)
		os.Exit(0)
	}
}

func out(m string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, m, a...)
}

func main() {
	// Catch SIGINT as a way to exit
	signalChan := make(chan os.Signal)
	signal.Notify(signalChan, os.Interrupt)

	cfg, err := loadConfig()
	checkErr("error loading configuration", err)
	api, err := ipfscluster.NewRESTAPI(cfg)
	checkErr("creating REST API component", err)
	proxy, err := ipfscluster.NewIPFSHTTPConnector(cfg)
	checkErr("creating IPFS Connector component", err)
	state := ipfscluster.NewMapState()
	tracker := ipfscluster.NewMapPinTracker(cfg)
	cluster, err := ipfscluster.NewCluster(cfg,
		api, proxy, state, tracker)
	checkErr("creating IPFS Cluster", err)

	// Wait until we are told to exit by a signal
	<-signalChan
	err = cluster.Shutdown()
	checkErr("shutting down IPFS Cluster", err)
	os.Exit(0)
}

func checkErr(doing string, err error) {
	if err != nil {
		out("error %s: %s\n", doing, err)
		os.Exit(1)
	}
}

func setupLogging() {
	logging.SetLogLevel("cluster", logLevelFlag)
	logging.SetLogLevel("libp2p-rpc", logLevelFlag)
}

func setupDebug() {
	if debugFlag {
		logging.SetLogLevel("cluster", "debug")
		logging.SetLogLevel("libp2p-raft", "debug")
		logging.SetLogLevel("libp2p-rpc", "debug")
		ipfscluster.SilentRaft = false
	}
}

func initConfig() error {
	if _, err := os.Stat(configPath); err == nil && !forceFlag {
		return fmt.Errorf("%s exists. Try running with -f", configPath)
	}
	cfg, err := ipfscluster.NewDefaultConfig()
	if err != nil {
		return err
	}
	cfg.ConsensusDataFolder = dataPath
	err = os.MkdirAll(DefaultPath, 0700)
	err = cfg.Save(configPath)
	if err != nil {
		return err
	}
	out("%s configuration written to %s\n",
		programName, configPath)
	return nil
}

func loadConfig() (*ipfscluster.Config, error) {
	return ipfscluster.LoadConfig(configPath)
}
