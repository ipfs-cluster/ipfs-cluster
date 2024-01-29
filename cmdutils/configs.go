package cmdutils

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	ipfscluster "github.com/ipfs-cluster/ipfs-cluster"
	"github.com/ipfs-cluster/ipfs-cluster/allocator/balanced"
	"github.com/ipfs-cluster/ipfs-cluster/api/ipfsproxy"
	"github.com/ipfs-cluster/ipfs-cluster/api/pinsvcapi"
	"github.com/ipfs-cluster/ipfs-cluster/api/rest"
	"github.com/ipfs-cluster/ipfs-cluster/config"
	"github.com/ipfs-cluster/ipfs-cluster/consensus/crdt"
	"github.com/ipfs-cluster/ipfs-cluster/consensus/raft"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/badger"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/badger3"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/leveldb"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/pebble"
	"github.com/ipfs-cluster/ipfs-cluster/informer/disk"
	"github.com/ipfs-cluster/ipfs-cluster/informer/numpin"
	"github.com/ipfs-cluster/ipfs-cluster/informer/pinqueue"
	"github.com/ipfs-cluster/ipfs-cluster/informer/tags"
	"github.com/ipfs-cluster/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs-cluster/ipfs-cluster/monitor/pubsubmon"
	"github.com/ipfs-cluster/ipfs-cluster/observations"
	"github.com/ipfs-cluster/ipfs-cluster/pintracker/stateless"
)

// Configs carries config types used by a Cluster Peer.
type Configs struct {
	Cluster          *ipfscluster.Config
	Restapi          *rest.Config
	Pinsvcapi        *pinsvcapi.Config
	Ipfsproxy        *ipfsproxy.Config
	Ipfshttp         *ipfshttp.Config
	Raft             *raft.Config
	Crdt             *crdt.Config
	Statelesstracker *stateless.Config
	Pubsubmon        *pubsubmon.Config
	BalancedAlloc    *balanced.Config
	DiskInf          *disk.Config
	NumpinInf        *numpin.Config
	TagsInf          *tags.Config
	PinQueueInf      *pinqueue.Config
	Metrics          *observations.MetricsConfig
	Tracing          *observations.TracingConfig
	Badger           *badger.Config
	Badger3          *badger3.Config
	LevelDB          *leveldb.Config
	Pebble           *pebble.Config
}

// ConfigHelper helps managing the configuration and identity files with the
// standard set of cluster components.
type ConfigHelper struct {
	identity *config.Identity
	manager  *config.Manager
	configs  *Configs

	configPath   string
	identityPath string
	consensus    string
	datastore    string
}

// NewConfigHelper creates a config helper given the paths to the
// configuration and identity files.
// Remember to Shutdown() the ConfigHelper.Manager() after use.
func NewConfigHelper(configPath, identityPath, consensus, datastore string) *ConfigHelper {
	ch := &ConfigHelper{
		configPath:   configPath,
		identityPath: identityPath,
		consensus:    consensus,
		datastore:    datastore,
	}
	ch.init()
	return ch
}

// NewLoadedConfigHelper creates a config helper given the paths to the
// configuration and identity files and loads the configurations from disk.
// Remember to Shutdown() the ConfigHelper.Manager() after use.
func NewLoadedConfigHelper(configPath, identityPath string) (*ConfigHelper, error) {
	cfgHelper := NewConfigHelper(configPath, identityPath, "", "")
	err := cfgHelper.LoadFromDisk()
	return cfgHelper, err
}

// LoadConfigFromDisk parses the configuration from disk.
func (ch *ConfigHelper) LoadConfigFromDisk() error {
	return ch.manager.LoadJSONFileAndEnv(ch.configPath)
}

// LoadIdentityFromDisk parses the identity from disk.
func (ch *ConfigHelper) LoadIdentityFromDisk() error {
	// load identity with hack for 0.11.0 - identity separation.
	_, err := os.Stat(ch.identityPath)
	ident := &config.Identity{}
	// temporary hack to convert identity
	if os.IsNotExist(err) {
		clusterConfig, err := config.GetClusterConfig(ch.configPath)
		if err != nil {
			return err
		}
		err = ident.LoadJSON(clusterConfig)
		if err != nil {
			return errors.Wrap(err, "error loading identity")
		}

		err = ident.SaveJSON(ch.identityPath)
		if err != nil {
			return errors.Wrap(err, "error saving identity")
		}

		fmt.Fprintf(
			os.Stderr,
			"\nNOTICE: identity information extracted from %s and saved as %s.\n\n",
			ch.configPath,
			ch.identityPath,
		)
	} else { // leave this part when the hack is removed.
		err = ident.LoadJSONFromFile(ch.identityPath)
		if err != nil {
			return fmt.Errorf("error loading identity from %s: %s", ch.identityPath, err)
		}
	}

	err = ident.ApplyEnvVars()
	if err != nil {
		return errors.Wrap(err, "error applying environment variables to the identity")
	}
	ch.identity = ident
	return nil
}

// LoadFromDisk loads both configuration and identity from disk.
func (ch *ConfigHelper) LoadFromDisk() error {
	err := ch.LoadConfigFromDisk()
	if err != nil {
		return err
	}
	return ch.LoadIdentityFromDisk()
}

// Identity returns the Identity object. It returns an empty identity
// if not loaded yet.
func (ch *ConfigHelper) Identity() *config.Identity {
	return ch.identity
}

// Manager returns the config manager with all the
// cluster configurations registered.
func (ch *ConfigHelper) Manager() *config.Manager {
	return ch.manager
}

// Configs returns the Configs object which holds all the cluster
// configurations. Configurations are empty if they have not been loaded from
// disk.
func (ch *ConfigHelper) Configs() *Configs {
	return ch.configs
}

// GetConsensus attempts to return the configured consensus.
// If the ConfigHelper was initialized with a consensus string
// then it returns that.
//
// Otherwise it checks whether one of the consensus configurations
// has been loaded. If both or none have been loaded, it returns
// an empty string.
func (ch *ConfigHelper) GetConsensus() string {
	if ch.consensus != "" {
		return ch.consensus
	}
	crdtLoaded := ch.manager.IsLoadedFromJSON(config.Consensus, ch.configs.Crdt.ConfigKey())
	raftLoaded := ch.manager.IsLoadedFromJSON(config.Consensus, ch.configs.Raft.ConfigKey())
	if crdtLoaded == raftLoaded { //both loaded or none
		return ""
	}

	if crdtLoaded {
		return ch.configs.Crdt.ConfigKey()
	}
	return ch.configs.Raft.ConfigKey()
}

// GetDatastore attempts to return the configured datastore.  If the
// ConfigHelper was initialized with a datastore string, then it returns that.
//
// Otherwise it checks whether one of the datastore configurations has been
// loaded. If none or more than one have been loaded, it returns an empty
// string. Otherwise it returns the key of the loaded configuration.
func (ch *ConfigHelper) GetDatastore() string {
	if ch.datastore != "" {
		return ch.datastore
	}

	badgerLoaded := ch.manager.IsLoadedFromJSON(config.Datastore, ch.configs.Badger.ConfigKey())
	badger3Loaded := ch.manager.IsLoadedFromJSON(config.Datastore, ch.configs.Badger3.ConfigKey())
	levelDBLoaded := ch.manager.IsLoadedFromJSON(config.Datastore, ch.configs.LevelDB.ConfigKey())
	pebbleLoaded := ch.manager.IsLoadedFromJSON(config.Datastore, ch.configs.Pebble.ConfigKey())

	nLoaded := 0
	for _, v := range []bool{badgerLoaded, badger3Loaded, levelDBLoaded, pebbleLoaded} {
		if v {
			nLoaded++
		}
	}
	if nLoaded == 0 || nLoaded > 1 {
		return ""
	}
	switch {
	case badgerLoaded:
		return ch.configs.Badger.ConfigKey()
	case badger3Loaded:
		return ch.configs.Badger3.ConfigKey()
	case levelDBLoaded:
		return ch.configs.LevelDB.ConfigKey()
	case pebbleLoaded:
		return ch.configs.Pebble.ConfigKey()
	default:
		return ""
	}
}

// register all current cluster components
func (ch *ConfigHelper) init() {
	man := config.NewManager()
	cfgs := &Configs{
		Cluster:          &ipfscluster.Config{},
		Restapi:          rest.NewConfig(),
		Pinsvcapi:        pinsvcapi.NewConfig(),
		Ipfsproxy:        &ipfsproxy.Config{},
		Ipfshttp:         &ipfshttp.Config{},
		Raft:             &raft.Config{},
		Crdt:             &crdt.Config{},
		Statelesstracker: &stateless.Config{},
		Pubsubmon:        &pubsubmon.Config{},
		BalancedAlloc:    &balanced.Config{},
		DiskInf:          &disk.Config{},
		NumpinInf:        &numpin.Config{},
		TagsInf:          &tags.Config{},
		PinQueueInf:      &pinqueue.Config{},
		Metrics:          &observations.MetricsConfig{},
		Tracing:          &observations.TracingConfig{},
		Badger:           &badger.Config{},
		Badger3:          &badger3.Config{},
		LevelDB:          &leveldb.Config{},
		Pebble:           &pebble.Config{},
	}
	man.RegisterComponent(config.Cluster, cfgs.Cluster)
	man.RegisterComponent(config.API, cfgs.Restapi)
	man.RegisterComponent(config.API, cfgs.Pinsvcapi)
	man.RegisterComponent(config.API, cfgs.Ipfsproxy)
	man.RegisterComponent(config.IPFSConn, cfgs.Ipfshttp)
	man.RegisterComponent(config.PinTracker, cfgs.Statelesstracker)
	man.RegisterComponent(config.Monitor, cfgs.Pubsubmon)
	man.RegisterComponent(config.Allocator, cfgs.BalancedAlloc)
	man.RegisterComponent(config.Informer, cfgs.DiskInf)
	// man.RegisterComponent(config.Informer, cfgs.Numpininf)
	man.RegisterComponent(config.Informer, cfgs.TagsInf)
	man.RegisterComponent(config.Informer, cfgs.PinQueueInf)
	man.RegisterComponent(config.Observations, cfgs.Metrics)
	man.RegisterComponent(config.Observations, cfgs.Tracing)

	registerDatastores := false

	switch ch.consensus {
	case cfgs.Raft.ConfigKey():
		man.RegisterComponent(config.Consensus, cfgs.Raft)
	case cfgs.Crdt.ConfigKey():
		man.RegisterComponent(config.Consensus, cfgs.Crdt)
		registerDatastores = true
	default:
		man.RegisterComponent(config.Consensus, cfgs.Raft)
		man.RegisterComponent(config.Consensus, cfgs.Crdt)
		registerDatastores = true
	}

	if registerDatastores {
		switch ch.datastore {
		case cfgs.Badger.ConfigKey():
			man.RegisterComponent(config.Datastore, cfgs.Badger)
		case cfgs.Badger3.ConfigKey():
			man.RegisterComponent(config.Datastore, cfgs.Badger3)
		case cfgs.LevelDB.ConfigKey():
			man.RegisterComponent(config.Datastore, cfgs.LevelDB)
		case cfgs.Pebble.ConfigKey():
			man.RegisterComponent(config.Datastore, cfgs.Pebble)

		default:
			man.RegisterComponent(config.Datastore, cfgs.Badger)
			man.RegisterComponent(config.Datastore, cfgs.Badger3)
			man.RegisterComponent(config.Datastore, cfgs.LevelDB)
			man.RegisterComponent(config.Datastore, cfgs.Pebble)

		}
	}

	ch.identity = &config.Identity{}
	ch.manager = man
	ch.configs = cfgs
}

// MakeConfigFolder creates the folder to hold
// configuration and identity files.
func (ch *ConfigHelper) MakeConfigFolder() error {
	f := filepath.Dir(ch.configPath)
	if _, err := os.Stat(f); os.IsNotExist(err) {
		err := os.MkdirAll(f, 0700)
		if err != nil {
			return err
		}
	}
	return nil
}

// SaveConfigToDisk saves the configuration file to disk.
func (ch *ConfigHelper) SaveConfigToDisk() error {
	err := ch.MakeConfigFolder()
	if err != nil {
		return err
	}
	return ch.manager.SaveJSON(ch.configPath)
}

// SaveIdentityToDisk saves the identity file to disk.
func (ch *ConfigHelper) SaveIdentityToDisk() error {
	err := ch.MakeConfigFolder()
	if err != nil {
		return err
	}
	return ch.Identity().SaveJSON(ch.identityPath)
}

// SetupTracing propagates tracingCfg.EnableTracing to all other
// configurations. Use only when identity has been loaded or generated.  The
// forceEnabled parameter allows to override the EnableTracing value.
func (ch *ConfigHelper) SetupTracing(forceEnabled bool) {
	enabled := forceEnabled || ch.configs.Tracing.EnableTracing

	ch.configs.Tracing.ClusterID = ch.Identity().ID.String()
	ch.configs.Tracing.ClusterPeername = ch.configs.Cluster.Peername
	ch.configs.Tracing.EnableTracing = enabled
	ch.configs.Cluster.Tracing = enabled
	ch.configs.Raft.Tracing = enabled
	ch.configs.Crdt.Tracing = enabled
	ch.configs.Restapi.Tracing = enabled
	ch.configs.Pinsvcapi.Tracing = enabled
	ch.configs.Ipfshttp.Tracing = enabled
	ch.configs.Ipfsproxy.Tracing = enabled
}
