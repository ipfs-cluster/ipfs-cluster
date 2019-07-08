package main

import (
	"errors"
	"os"
	"path/filepath"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/api/ipfsproxy"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/config"
	"github.com/ipfs/ipfs-cluster/consensus/crdt"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/datastore/badger"
	"github.com/ipfs/ipfs-cluster/informer/disk"
	"github.com/ipfs/ipfs-cluster/informer/numpin"
	"github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs/ipfs-cluster/monitor/pubsubmon"
	"github.com/ipfs/ipfs-cluster/observations"
	"github.com/ipfs/ipfs-cluster/pintracker/maptracker"
	"github.com/ipfs/ipfs-cluster/pintracker/stateless"
)

type cfgs struct {
	clusterCfg          *ipfscluster.Config
	apiCfg              *rest.Config
	ipfsproxyCfg        *ipfsproxy.Config
	ipfshttpCfg         *ipfshttp.Config
	raftCfg             *raft.Config
	crdtCfg             *crdt.Config
	maptrackerCfg       *maptracker.Config
	statelessTrackerCfg *stateless.Config
	pubsubmonCfg        *pubsubmon.Config
	diskInfCfg          *disk.Config
	numpinInfCfg        *numpin.Config
	metricsCfg          *observations.MetricsConfig
	tracingCfg          *observations.TracingConfig
	badgerCfg           *badger.Config
}

func makeConfigs() (*config.Manager, *cfgs) {
	cfg := config.NewManager()
	clusterCfg := &ipfscluster.Config{}
	apiCfg := &rest.Config{}
	ipfsproxyCfg := &ipfsproxy.Config{}
	ipfshttpCfg := &ipfshttp.Config{}
	raftCfg := &raft.Config{}
	crdtCfg := &crdt.Config{}
	maptrackerCfg := &maptracker.Config{}
	statelessCfg := &stateless.Config{}
	pubsubmonCfg := &pubsubmon.Config{}
	diskInfCfg := &disk.Config{}
	numpinInfCfg := &numpin.Config{}
	metricsCfg := &observations.MetricsConfig{}
	tracingCfg := &observations.TracingConfig{}
	badgerCfg := &badger.Config{}
	cfg.RegisterComponent(config.Cluster, clusterCfg)
	cfg.RegisterComponent(config.API, apiCfg)
	cfg.RegisterComponent(config.API, ipfsproxyCfg)
	cfg.RegisterComponent(config.IPFSConn, ipfshttpCfg)
	cfg.RegisterComponent(config.Consensus, raftCfg)
	cfg.RegisterComponent(config.Consensus, crdtCfg)
	cfg.RegisterComponent(config.PinTracker, maptrackerCfg)
	cfg.RegisterComponent(config.PinTracker, statelessCfg)
	cfg.RegisterComponent(config.Monitor, pubsubmonCfg)
	cfg.RegisterComponent(config.Informer, diskInfCfg)
	cfg.RegisterComponent(config.Informer, numpinInfCfg)
	cfg.RegisterComponent(config.Observations, metricsCfg)
	cfg.RegisterComponent(config.Observations, tracingCfg)
	cfg.RegisterComponent(config.Datastore, badgerCfg)
	return cfg, &cfgs{
		clusterCfg,
		apiCfg,
		ipfsproxyCfg,
		ipfshttpCfg,
		raftCfg,
		crdtCfg,
		maptrackerCfg,
		statelessCfg,
		pubsubmonCfg,
		diskInfCfg,
		numpinInfCfg,
		metricsCfg,
		tracingCfg,
		badgerCfg,
	}
}

func makeAndLoadConfigs() (*config.Manager, *config.Identity, *cfgs) {
	ident := loadIdentity()
	cfgMgr, cfgs := makeConfigs()
	checkErr("reading configuration", cfgMgr.LoadJSONFileAndEnv(configPath))
	return cfgMgr, ident, cfgs
}

func loadIdentity() *config.Identity {
	_, err := os.Stat(identityPath)

	ident := &config.Identity{}
	// temporary hack to convert identity
	if os.IsNotExist(err) {
		clusterConfig, err := config.GetClusterConfig(configPath)
		checkErr("loading configuration", err)
		err = ident.LoadJSON(clusterConfig)
		if err != nil {
			checkErr("", errors.New("error loading identity"))
		}

		err = ident.SaveJSON(identityPath)
		checkErr("saving identity.json ", err)

		err = ident.ApplyEnvVars()
		checkErr("applying environment variables to the identity", err)

		out("\nNOTICE: identity information extracted from %s and saved as %s.\n\n", DefaultConfigFile, DefaultIdentityFile)
		return ident
	}

	err = ident.LoadJSONFromFile(identityPath)
	checkErr("loading identity from %s", err, DefaultIdentityFile)

	err = ident.ApplyEnvVars()
	checkErr("applying environment variables to the identity", err)

	return ident
}

func makeConfigFolder() {
	f := filepath.Dir(configPath)
	if _, err := os.Stat(f); os.IsNotExist(err) {
		err := os.MkdirAll(f, 0700)
		checkErr("creating configuration folder (%s)", err, f)
	}
}

func saveConfig(cfg *config.Manager) {
	makeConfigFolder()
	err := cfg.SaveJSON(configPath)
	checkErr("saving new configuration", err)
	out("configuration written to %s\n", configPath)
}

func propagateTracingConfig(ident *config.Identity, cfgs *cfgs, tracingFlag bool) *cfgs {
	// tracingFlag represents the cli flag passed to ipfs-cluster-service daemon.
	// It takes priority. If false, fallback to config file value.
	tracingValue := tracingFlag
	if !tracingFlag {
		tracingValue = cfgs.tracingCfg.EnableTracing
	}
	// propagate to any other interested configuration
	cfgs.tracingCfg.ClusterID = ident.ID.Pretty()
	cfgs.tracingCfg.ClusterPeername = cfgs.clusterCfg.Peername
	cfgs.tracingCfg.EnableTracing = tracingValue
	cfgs.clusterCfg.Tracing = tracingValue
	cfgs.raftCfg.Tracing = tracingValue
	cfgs.crdtCfg.Tracing = tracingValue
	cfgs.apiCfg.Tracing = tracingValue
	cfgs.ipfshttpCfg.Tracing = tracingValue
	cfgs.ipfsproxyCfg.Tracing = tracingValue

	return cfgs
}
