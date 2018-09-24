package main

import (
	"fmt"
	"os"
	"path/filepath"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/config"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/informer/disk"
	"github.com/ipfs/ipfs-cluster/informer/numpin"
	"github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs/ipfs-cluster/monitor/basic"
	"github.com/ipfs/ipfs-cluster/monitor/pubsubmon"
	"github.com/ipfs/ipfs-cluster/pintracker/maptracker"
	"github.com/ipfs/ipfs-cluster/pintracker/stateless"
)

type cfgs struct {
	clusterCfg          *ipfscluster.Config
	apiCfg              *rest.Config
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

func saveConfig(cfg *config.Manager, force bool) {
	if _, err := os.Stat(configPath); err == nil && !force {
		err := fmt.Errorf("%s exists. Try running: %s -f init", configPath, programName)
		checkErr("", err)
	}

	err := os.MkdirAll(filepath.Dir(configPath), 0700)
	err = cfg.SaveJSON(configPath)
	checkErr("saving new configuration", err)
	out("%s configuration written to %s\n", programName, configPath)
}
