package raft

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"time"

	"github.com/ipfs/ipfs-cluster/config"

	hraft "github.com/hashicorp/raft"
)

// ConfigKey is the default configuration key for holding this component's
// configuration section.
var configKey = "raft"

// Configuration defaults
var (
	DefaultDataSubFolder        = "ipfs-cluster-data"
	DefaultWaitForLeaderTimeout = 15 * time.Second
	DefaultCommitRetries        = 1
	DefaultNetworkTimeout       = 10 * time.Second
	DefaultCommitRetryDelay     = 200 * time.Millisecond
)

// Config allows to configure the Raft Consensus component for ipfs-cluster.
// The component's configuration section is represented by ConfigJSON.
// Config implements the ComponentConfig interface.
type Config struct {
	config.Saver

	// will shutdown libp2p host on shutdown. Useful for testing
	hostShutdown bool

	// A Hashicorp Raft's configuration object.
	RaftConfig *hraft.Config
	// A folder to store Raft's data.
	DataFolder string
	// LeaderTimeout specifies how long to wait for a leader before
	// failing an operation.
	WaitForLeaderTimeout time.Duration
	// NetworkTimeout specifies how long before a Raft network
	// operation is timed out
	NetworkTimeout time.Duration
	// CommitRetries specifies how many times we retry a failed commit until
	// we give up.
	CommitRetries int
	// How long to wait between retries
	CommitRetryDelay time.Duration
}

// ConfigJSON represents a human-friendly Config
// object which can be saved to JSON.  Most configuration keys are converted
// into simple types like strings, and key names aim to be self-explanatory
// for the user.
// Check https://godoc.org/github.com/hashicorp/raft#Config for extended
// description on all Raft-specific keys.
type jsonConfig struct {
	// Storage folder for snapshots, log store etc. Used by
	// the Raft.
	DataFolder string `json:"data_folder,omitempty"`

	// How long to wait for a leader before failing
	WaitForLeaderTimeout string `json:"wait_for_leader_timeout"`

	// How long to wait before timing out network operations
	NetworkTimeout string `json:"network_timeout"`

	// How many retries to make upon a failed commit
	CommitRetries int `json:"commit_retries"`

	// How long to wait between commit retries
	CommitRetryDelay string `json:"commit_retry_delay"`

	// HeartbeatTimeout specifies the time in follower state without
	// a leader before we attempt an election.
	HeartbeatTimeout string `json:"heartbeat_timeout,omitempty"`

	// ElectionTimeout specifies the time in candidate state without
	// a leader before we attempt an election.
	ElectionTimeout string `json:"election_timeout,omitempty"`

	// CommitTimeout controls the time without an Apply() operation
	// before we heartbeat to ensure a timely commit.
	CommitTimeout string `json:"commit_timeout,omitempty"`

	// MaxAppendEntries controls the maximum number of append entries
	// to send at once.
	MaxAppendEntries int `json:"max_append_entries,omitempty"`

	// TrailingLogs controls how many logs we leave after a snapshot.
	TrailingLogs uint64 `json:"trailing_logs,omitempty"`

	// SnapshotInterval controls how often we check if we should perform
	// a snapshot.
	SnapshotInterval string `json:"snapshot_interval,omitempty"`

	// SnapshotThreshold controls how many outstanding logs there must be
	// before we perform a snapshot.
	SnapshotThreshold uint64 `json:"snapshot_threshold,omitempty"`

	// LeaderLeaseTimeout is used to control how long the "lease" lasts
	// for being the leader without being able to contact a quorum
	// of nodes. If we reach this interval without contact, we will
	// step down as leader.
	LeaderLeaseTimeout string `json:"leader_lease_timeout,omitempty"`

	// StartAsLeader forces Raft to start in the leader state. This should
	// never be used except for testing purposes, as it can cause a split-brain.
	StartAsLeader bool `json:"start_as_leader,omitempty"`

	// The unique ID for this server across all time. When running with
	// ProtocolVersion < 3, you must set this to be the same as the network
	// address of your transport.
	// LocalID string `json:local_id`
}

// ConfigKey returns a human-friendly indentifier for this Config.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Validate checks that this configuration has working values,
// at least in appearance.
func (cfg *Config) Validate() error {
	if cfg.RaftConfig == nil {
		return errors.New("no hashicorp/raft.Config")
	}
	if cfg.WaitForLeaderTimeout <= 0 {
		return errors.New("wait_for_leader_timeout <= 0")
	}

	if cfg.NetworkTimeout <= 0 {
		return errors.New("network_timeout <= 0")
	}

	if cfg.CommitRetries < 0 {
		return errors.New("commit_retries is invalid")
	}

	if cfg.CommitRetryDelay <= 0 {
		return errors.New("commit_retry_delay is invalid")
	}

	return hraft.ValidateConfig(cfg.RaftConfig)
}

// LoadJSON parses a json-encoded configuration (see jsonConfig).
// The Config will have default values for all fields not explicited
// in the given json object.
func (cfg *Config) LoadJSON(raw []byte) error {
	jcfg := &jsonConfig{}
	err := json.Unmarshal(raw, jcfg)
	if err != nil {
		logger.Error("Error unmarshaling raft config")
		return err
	}

	cfg.Default()

	parseDuration := func(txt string) time.Duration {
		d, _ := time.ParseDuration(txt)
		if txt != "" && d == 0 {
			logger.Warningf("%s is not a valid duration. Default will be used", txt)
		}
		return d
	}

	// Parse durations. We ignore errors as 0 will take Default values.
	waitForLeaderTimeout := parseDuration(jcfg.WaitForLeaderTimeout)
	networkTimeout := parseDuration(jcfg.NetworkTimeout)
	commitRetryDelay := parseDuration(jcfg.CommitRetryDelay)
	heartbeatTimeout := parseDuration(jcfg.HeartbeatTimeout)
	electionTimeout := parseDuration(jcfg.ElectionTimeout)
	commitTimeout := parseDuration(jcfg.CommitTimeout)
	snapshotInterval := parseDuration(jcfg.SnapshotInterval)
	leaderLeaseTimeout := parseDuration(jcfg.LeaderLeaseTimeout)

	// Set all values in config. For some, take defaults if they are 0.
	// Set values from jcfg if they are not 0 values

	// Own values
	config.SetIfNotDefault(jcfg.DataFolder, &cfg.DataFolder)
	config.SetIfNotDefault(waitForLeaderTimeout, &cfg.WaitForLeaderTimeout)
	config.SetIfNotDefault(networkTimeout, &cfg.NetworkTimeout)
	cfg.CommitRetries = jcfg.CommitRetries
	config.SetIfNotDefault(commitRetryDelay, &cfg.CommitRetryDelay)

	// Raft values
	config.SetIfNotDefault(heartbeatTimeout, &cfg.RaftConfig.HeartbeatTimeout)
	config.SetIfNotDefault(electionTimeout, &cfg.RaftConfig.ElectionTimeout)
	config.SetIfNotDefault(commitTimeout, &cfg.RaftConfig.CommitTimeout)
	config.SetIfNotDefault(jcfg.MaxAppendEntries, &cfg.RaftConfig.MaxAppendEntries)
	config.SetIfNotDefault(jcfg.TrailingLogs, &cfg.RaftConfig.TrailingLogs)
	config.SetIfNotDefault(snapshotInterval, &cfg.RaftConfig.SnapshotInterval)
	config.SetIfNotDefault(jcfg.SnapshotThreshold, &cfg.RaftConfig.SnapshotThreshold)
	config.SetIfNotDefault(leaderLeaseTimeout, &cfg.RaftConfig.LeaderLeaseTimeout)

	return cfg.Validate()
}

// ToJSON returns the pretty JSON representation of a Config.
func (cfg *Config) ToJSON() ([]byte, error) {
	jcfg := &jsonConfig{}
	jcfg.DataFolder = cfg.DataFolder
	jcfg.WaitForLeaderTimeout = cfg.WaitForLeaderTimeout.String()
	jcfg.NetworkTimeout = cfg.NetworkTimeout.String()
	jcfg.CommitRetries = cfg.CommitRetries
	jcfg.CommitRetryDelay = cfg.CommitRetryDelay.String()
	jcfg.HeartbeatTimeout = cfg.RaftConfig.HeartbeatTimeout.String()
	jcfg.ElectionTimeout = cfg.RaftConfig.ElectionTimeout.String()
	jcfg.CommitTimeout = cfg.RaftConfig.CommitTimeout.String()
	jcfg.MaxAppendEntries = cfg.RaftConfig.MaxAppendEntries
	jcfg.TrailingLogs = cfg.RaftConfig.TrailingLogs
	jcfg.SnapshotInterval = cfg.RaftConfig.SnapshotInterval.String()
	jcfg.SnapshotThreshold = cfg.RaftConfig.SnapshotThreshold
	jcfg.LeaderLeaseTimeout = cfg.RaftConfig.LeaderLeaseTimeout.String()

	return config.DefaultJSONMarshal(jcfg)
}

// Default initializes this configuration with working defaults.
func (cfg *Config) Default() error {
	cfg.DataFolder = "" // empty so it gets omitted
	cfg.WaitForLeaderTimeout = DefaultWaitForLeaderTimeout
	cfg.NetworkTimeout = DefaultNetworkTimeout
	cfg.CommitRetries = DefaultCommitRetries
	cfg.CommitRetryDelay = DefaultCommitRetryDelay
	cfg.RaftConfig = hraft.DefaultConfig()

	// These options are imposed over any Default Raft Config.
	cfg.RaftConfig.ShutdownOnRemove = false
	cfg.RaftConfig.LocalID = "will_be_set_automatically"

	// Set up logging
	cfg.RaftConfig.LogOutput = ioutil.Discard
	cfg.RaftConfig.Logger = raftStdLogger // see logging.go
	return nil
}
