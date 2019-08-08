package cmdutils

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

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

// Configs carries config types used by a Cluster Peer.
type Configs struct {
	Cluster          *ipfscluster.Config
	Restapi          *rest.Config
	Ipfsproxy        *ipfsproxy.Config
	Ipfshttp         *ipfshttp.Config
	Raft             *raft.Config
	Crdt             *crdt.Config
	Maptracker       *maptracker.Config
	Statelesstracker *stateless.Config
	Pubsubmon        *pubsubmon.Config
	Diskinf          *disk.Config
	Numpininf        *numpin.Config
	Metrics          *observations.MetricsConfig
	Tracing          *observations.TracingConfig
	Badger           *badger.Config
}

// ConfigHelper helps managing the configuration and identity files with the
// standard set of cluster components.
type ConfigHelper struct {
	identity *config.Identity
	manager  *config.Manager
	configs  *Configs

	configPath   string
	identityPath string
}

// NewConfigHelper creates a config helper given the paths to the
// configuration and identity files.
func NewConfigHelper(configPath, identityPath string) *ConfigHelper {
	ch := &ConfigHelper{
		configPath:   configPath,
		identityPath: identityPath,
	}
	ch.init()
	return ch
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

// register all current cluster components
func (ch *ConfigHelper) init() {
	man := config.NewManager()
	cfgs := &Configs{
		Cluster:          &ipfscluster.Config{},
		Restapi:          &rest.Config{},
		Ipfsproxy:        &ipfsproxy.Config{},
		Ipfshttp:         &ipfshttp.Config{},
		Raft:             &raft.Config{},
		Crdt:             &crdt.Config{},
		Maptracker:       &maptracker.Config{},
		Statelesstracker: &stateless.Config{},
		Pubsubmon:        &pubsubmon.Config{},
		Diskinf:          &disk.Config{},
		Metrics:          &observations.MetricsConfig{},
		Tracing:          &observations.TracingConfig{},
		Badger:           &badger.Config{},
	}
	man.RegisterComponent(config.Cluster, cfgs.Cluster)
	man.RegisterComponent(config.API, cfgs.Restapi)
	man.RegisterComponent(config.API, cfgs.Ipfsproxy)
	man.RegisterComponent(config.IPFSConn, cfgs.Ipfshttp)
	man.RegisterComponent(config.Consensus, cfgs.Raft)
	man.RegisterComponent(config.Consensus, cfgs.Crdt)
	man.RegisterComponent(config.PinTracker, cfgs.Maptracker)
	man.RegisterComponent(config.PinTracker, cfgs.Statelesstracker)
	man.RegisterComponent(config.Monitor, cfgs.Pubsubmon)
	man.RegisterComponent(config.Informer, cfgs.Diskinf)
	man.RegisterComponent(config.Observations, cfgs.Metrics)
	man.RegisterComponent(config.Observations, cfgs.Tracing)
	man.RegisterComponent(config.Datastore, cfgs.Badger)

	ch.identity = nil // explicitly not set until loaded.
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

	ch.configs.Tracing.ClusterID = ch.Identity().ID.Pretty()
	ch.configs.Tracing.ClusterPeername = ch.configs.Cluster.Peername
	ch.configs.Tracing.EnableTracing = enabled
	ch.configs.Cluster.Tracing = enabled
	ch.configs.Raft.Tracing = enabled
	ch.configs.Crdt.Tracing = enabled
	ch.configs.Restapi.Tracing = enabled
	ch.configs.Ipfshttp.Tracing = enabled
	ch.configs.Ipfsproxy.Tracing = enabled
}
