package crdt

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/config"
)

var configKey = "crdt"
var envConfigKey = "cluster_crdt"

// Default configuration values
var (
	DefaultClusterName         = "ipfs-cluster"
	DefaultPeersetMetric       = "ping"
	DefaultDatastoreNamespace  = "/c" // from "/crdt"
	DefaultRebroadcastInterval = time.Minute
	DefaultTrustedPeers        = []peer.ID{}
)

// Config is the configuration object for Consensus.
type Config struct {
	config.Saver

	hostShutdown bool

	// The topic we wish to subscribe to
	ClusterName string

	// Any update received from a peer outside this set is ignored and not
	// forwarded. Trusted peers can also access additional RPC endpoints
	// for this peer that are forbidden for other peers.
	TrustedPeers []peer.ID

	// The interval before re-announcing the current state
	// to the network when no activity is observed.
	RebroadcastInterval time.Duration

	// The name of the metric we use to obtain the peerset (every peer
	// with valid metric of this type is part of it).
	PeersetMetric string

	// All keys written to the datastore will be namespaced with this prefix
	DatastoreNamespace string

	// Tracing enables propagation of contexts across binary boundaries.
	Tracing bool
}

type jsonConfig struct {
	ClusterName         string   `json:"cluster_name"`
	TrustedPeers        []string `json:"trusted_peers"`
	RebroadcastInterval string   `json:"rebroadcast_interval,omitempty"`

	PeersetMetric      string `json:"peerset_metric,omitempty"`
	DatastoreNamespace string `json:"datastore_namespace,omitempty"`
}

// ConfigKey returns the section name for this type of configuration.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Validate returns an error if the configuration has invalid values.
func (cfg *Config) Validate() error {
	if cfg.ClusterName == "" {
		return errors.New("crdt.cluster_name cannot be empty")
	}

	if cfg.PeersetMetric == "" {
		return errors.New("crdt.peerset_metric needs a name")
	}

	if cfg.RebroadcastInterval <= 0 {
		return errors.New("crdt.rebroadcast_interval is invalid")
	}
	return nil
}

// LoadJSON takes a raw JSON slice and sets all the configuration fields.
func (cfg *Config) LoadJSON(raw []byte) error {
	jcfg := &jsonConfig{}
	err := json.Unmarshal(raw, jcfg)
	if err != nil {
		return fmt.Errorf("error unmarshaling %s config", configKey)
	}

	cfg.Default()

	return cfg.applyJSONConfig(jcfg)
}

func (cfg *Config) applyJSONConfig(jcfg *jsonConfig) error {
	cfg.ClusterName = jcfg.ClusterName

	cfg.TrustedPeers = api.StringsToPeers(jcfg.TrustedPeers)
	if len(cfg.TrustedPeers) != len(jcfg.TrustedPeers) {
		return errors.New("error parsing some peer IDs crdt.trusted_peers")
	}

	config.SetIfNotDefault(jcfg.PeersetMetric, &cfg.PeersetMetric)
	config.SetIfNotDefault(jcfg.DatastoreNamespace, &cfg.DatastoreNamespace)
	config.ParseDurations(
		"crdt",
		&config.DurationOpt{Duration: jcfg.RebroadcastInterval, Dst: &cfg.RebroadcastInterval, Name: "rebroadcast_interval"},
	)
	return cfg.Validate()
}

// ToJSON returns the JSON representation of this configuration.
func (cfg *Config) ToJSON() ([]byte, error) {
	jcfg := cfg.toJSONConfig()

	return config.DefaultJSONMarshal(jcfg)
}

func (cfg *Config) toJSONConfig() *jsonConfig {
	jcfg := &jsonConfig{
		ClusterName:         cfg.ClusterName,
		TrustedPeers:        api.PeersToStrings(cfg.TrustedPeers),
		PeersetMetric:       "",
		RebroadcastInterval: "",
	}

	if cfg.PeersetMetric != DefaultPeersetMetric {
		jcfg.PeersetMetric = cfg.PeersetMetric
		// otherwise leave empty/hidden
	}

	if cfg.DatastoreNamespace != DefaultDatastoreNamespace {
		jcfg.DatastoreNamespace = cfg.DatastoreNamespace
		// otherwise leave empty/hidden
	}

	if cfg.RebroadcastInterval != DefaultRebroadcastInterval {
		jcfg.RebroadcastInterval = cfg.RebroadcastInterval.String()
	}

	return jcfg
}

// Default sets the configuration fields to their default values.
func (cfg *Config) Default() error {
	cfg.ClusterName = DefaultClusterName
	cfg.RebroadcastInterval = DefaultRebroadcastInterval
	cfg.PeersetMetric = DefaultPeersetMetric
	cfg.DatastoreNamespace = DefaultDatastoreNamespace
	cfg.TrustedPeers = DefaultTrustedPeers
	return nil
}

// ApplyEnvVars fills in any Config fields found
// as environment variables.
func (cfg *Config) ApplyEnvVars() error {
	jcfg := cfg.toJSONConfig()

	err := envconfig.Process(envConfigKey, jcfg)
	if err != nil {
		return err
	}

	return cfg.applyJSONConfig(jcfg)
}
