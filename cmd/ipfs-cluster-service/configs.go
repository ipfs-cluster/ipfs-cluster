package main

import (
	"os"
	"path/filepath"

	ipfscluster "github.com/elastos/Elastos.NET.Hive.Cluster"
	"github.com/elastos/Elastos.NET.Hive.Cluster/api/ipfsproxy"
	"github.com/elastos/Elastos.NET.Hive.Cluster/api/rest"
	"github.com/elastos/Elastos.NET.Hive.Cluster/config"
	"github.com/elastos/Elastos.NET.Hive.Cluster/consensus/raft"
	"github.com/elastos/Elastos.NET.Hive.Cluster/informer/disk"
	"github.com/elastos/Elastos.NET.Hive.Cluster/informer/numpin"
	"github.com/elastos/Elastos.NET.Hive.Cluster/ipfsconn/ipfshttp"
	"github.com/elastos/Elastos.NET.Hive.Cluster/monitor/basic"
	"github.com/elastos/Elastos.NET.Hive.Cluster/monitor/pubsubmon"
	"github.com/elastos/Elastos.NET.Hive.Cluster/pintracker/maptracker"
	"github.com/elastos/Elastos.NET.Hive.Cluster/pintracker/stateless"
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
	}
}

func saveConfig(cfg *config.Manager) {
	err := os.MkdirAll(filepath.Dir(configPath), 0700)
	err = cfg.SaveJSON(configPath)
	checkErr("saving new configuration", err)
	out("%s configuration written to %s\n", programName, configPath)
}
