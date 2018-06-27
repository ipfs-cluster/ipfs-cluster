package main

import (
	"os"
	"path/filepath"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/api/ipfsproxy"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/config"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/informer/disk"
	"github.com/ipfs/ipfs-cluster/informer/numpin"
	"github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs/ipfs-cluster/monitor/basic"
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
	consensusCfg        *raft.Config
	maptrackerCfg       *maptracker.Config
	statelessTrackerCfg *stateless.Config
	monCfg              *basic.Config
	pubsubmonCfg        *pubsubmon.Config
	diskInfCfg          *disk.Config
	numpinInfCfg        *numpin.Config
	metricsCfg          *observations.MetricsConfig
	tracingCfg          *observations.TracingConfig
}

func makeConfigs() (*config.Manager, *cfgs) {
	cfg := config.NewManager()
	clusterCfg := &ipfscluster.Config{}
	apiCfg := &rest.Config{}
	ipfsproxyCfg := &ipfsproxy.Config{}
	ipfshttpCfg := &ipfshttp.Config{}
	consensusCfg := &raft.Config{}
	maptrackerCfg := &maptracker.Config{}
	statelessCfg := &stateless.Config{}
	monCfg := &basic.Config{}
	pubsubmonCfg := &pubsubmon.Config{}
	diskInfCfg := &disk.Config{}
	numpinInfCfg := &numpin.Config{}
	metricsCfg := &observations.MetricsConfig{}
	tracingCfg := &observations.TracingConfig{}
	cfg.RegisterComponent(config.Cluster, clusterCfg)
	cfg.RegisterComponent(config.API, apiCfg)
	cfg.RegisterComponent(config.API, ipfsproxyCfg)
	cfg.RegisterComponent(config.IPFSConn, ipfshttpCfg)
	cfg.RegisterComponent(config.Consensus, consensusCfg)
	cfg.RegisterComponent(config.PinTracker, maptrackerCfg)
	cfg.RegisterComponent(config.PinTracker, statelessCfg)
	cfg.RegisterComponent(config.Monitor, monCfg)
	cfg.RegisterComponent(config.Monitor, pubsubmonCfg)
	cfg.RegisterComponent(config.Informer, diskInfCfg)
	cfg.RegisterComponent(config.Informer, numpinInfCfg)
	cfg.RegisterComponent(config.Observations, metricsCfg)
	cfg.RegisterComponent(config.Observations, tracingCfg)
	return cfg, &cfgs{
		clusterCfg,
		apiCfg,
		ipfsproxyCfg,
		ipfshttpCfg,
		consensusCfg,
		maptrackerCfg,
		statelessCfg,
		monCfg,
		pubsubmonCfg,
		diskInfCfg,
		numpinInfCfg,
		metricsCfg,
		tracingCfg,
	}
}

func saveConfig(cfg *config.Manager) {
	err := os.MkdirAll(filepath.Dir(configPath), 0700)
	err = cfg.SaveJSON(configPath)
	checkErr("saving new configuration", err)
	out("%s configuration written to %s\n", programName, configPath)
}

func propagateTracingConfig(cfgs *cfgs, tracingFlag bool) *cfgs {
	// tracingFlag represents the cli flag passed to ipfs-cluster-service daemon.
	// It takes priority. If false, fallback to config file value.
	tracingValue := tracingFlag
	if !tracingFlag {
		tracingValue = cfgs.tracingCfg.EnableTracing
	}
	// propagate to any other interested configuration
	cfgs.tracingCfg.EnableTracing = tracingValue
	cfgs.clusterCfg.Tracing = tracingValue
	cfgs.consensusCfg.Tracing = tracingValue
	cfgs.apiCfg.Tracing = tracingValue
	cfgs.ipfshttpCfg.Tracing = tracingValue
	cfgs.ipfsproxyCfg.Tracing = tracingValue

	return cfgs
}
