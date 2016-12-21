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

// Name of this application
const name = `ipfscluster-server`

// Description provides a short summary of the functionality of this tool
var Description = fmt.Sprintf(`
%s runs an IPFS Cluster member.

A member is a server node which participates in the cluster consensus, follows
a distributed log of pinning and unpinning operations and manages pinning
operations to a configured IPFS daemon.

This server also provides an API for cluster management, an IPFS Proxy API which
forwards requests to IPFS and a number of components for internal communication
using LibP2P.

%s needs a valid configuration to run. This configuration is 
independent from IPFS and includes its own LibP2P key-pair. It can be 
initialized with -init and its default location is
 ~/%s/%s.

For any additional information, visit https://github.com/ipfs/ipfs-cluster.

`,
	name,
	name,
	DefaultPath,
	DefaultConfigFile)

// Default location for the configurations and data
var (
	// DefaultPath is initialized to something like ~/.ipfs-cluster/server.json
	// and holds all the ipfs-cluster data
	DefaultPath = ".ipfs-cluster"
	// The name of the configuration file inside DefaultPath
	DefaultConfigFile = "server.json"
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
	debugFlag    bool
	logLevelFlag string
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
	configPath = filepath.Join(DefaultPath, DefaultConfigFile)
	dataPath = filepath.Join(DefaultPath, DefaultDataFolder)

	flag.Usage = func() {
		out("Usage: %s [options]\n", name)
		out(Description)
		out("Options:\n")
		flag.PrintDefaults()
		out("\n")
	}
	flag.BoolVar(&initFlag, "init", false,
		"create a default configuration and exit")
	flag.StringVar(&configFlag, "config", configPath,
		"path to the ipfscluster-server configuration file")
	flag.BoolVar(&debugFlag, "debug", false,
		"enable full debug logs of ipfs cluster and consensus layers")
	flag.StringVar(&logLevelFlag, "loglevel", "info",
		"set the loglevel [critical, error, warning, notice, info, debug]")
	flag.Parse()
}

func out(m string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, m, a...)
}

func main() {
	// Catch SIGINT as a way to exit
	signalChan := make(chan os.Signal)
	signal.Notify(signalChan, os.Interrupt)

	setupLogging()
	setupDebug()
	if initFlag {
		err := initConfig()
		checkErr("creating configuration", err)
		os.Exit(0)
	}

	cfg, err := loadConfig()
	checkErr("loading configuration", err)
	api, err := ipfscluster.NewRESTAPI(cfg)
	checkErr("creating REST API component", err)
	proxy, err := ipfscluster.NewIPFSHTTPConnector(cfg)
	checkErr("creating IPFS Connector component", err)
	state := ipfscluster.NewMapState()
	tracker := ipfscluster.NewMapPinTracker(cfg)
	remote := ipfscluster.NewLibp2pRemote()
	cluster, err := ipfscluster.NewCluster(cfg,
		api, proxy, state, tracker, remote)
	checkErr("creating IPFS Cluster", err)

	// Wait until we are told to exit by a signal
	<-signalChan
	fmt.Println("aa")
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
	logging.SetLogLevel("ipfscluster", logLevelFlag)
}

func setupDebug() {
	if debugFlag {
		logging.SetLogLevel("ipfscluster", "debug")
		logging.SetLogLevel("libp2p-raft", "debug")
		ipfscluster.SilentRaft = false
	}
}

func initConfig() error {
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("%s exists. Try deleting it first", configPath)
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
	out("%s configuration written to %s",
		name, configPath)
	return nil
}

func loadConfig() (*ipfscluster.Config, error) {
	return ipfscluster.LoadConfig(configPath)
}
