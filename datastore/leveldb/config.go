package leveldb

import (
	"encoding/json"
	"errors"
	"path/filepath"

	"dario.cat/mergo"
	"github.com/ipfs-cluster/ipfs-cluster/config"
	"github.com/kelseyhightower/envconfig"
	goleveldb "github.com/syndtr/goleveldb/leveldb/opt"
)

const configKey = "leveldb"
const envConfigKey = "cluster_leveldb"

// Default values for LevelDB Config
const (
	DefaultSubFolder = "leveldb"
)

var (
	// DefaultLevelDBOptions carries default options. Values are customized during Init().
	DefaultLevelDBOptions goleveldb.Options
)

func init() {
	// go-ipfs uses defaults and only allows to configure compression, but
	// otherwise stores a small amount of values in LevelDB.
	// We leave defaults.
	// Example:
	DefaultLevelDBOptions.NoSync = false
}

// Config is used to initialize a LevelDB datastore. It implements the
// ComponentConfig interface.
type Config struct {
	config.Saver

	// The folder for this datastore. Non-absolute paths are relative to
	// the base configuration folder.
	Folder string

	LevelDBOptions goleveldb.Options
}

// levelDBOptions allows json serialization in our configuration of the
// goleveldb Options.
type levelDBOptions struct {
	BlockCacheCapacity                    int       `json:"block_cache_capacity"`
	BlockCacheEvictRemoved                bool      `json:"block_cache_evict_removed"`
	BlockRestartInterval                  int       `json:"block_restart_interval"`
	BlockSize                             int       `json:"block_size"`
	CompactionExpandLimitFactor           int       `json:"compaction_expand_limit_factor"`
	CompactionGPOverlapsFactor            int       `json:"compaction_gp_overlaps_factor"`
	CompactionL0Trigger                   int       `json:"compaction_l0_trigger"`
	CompactionSourceLimitFactor           int       `json:"compaction_source_limit_factor"`
	CompactionTableSize                   int       `json:"compaction_table_size"`
	CompactionTableSizeMultiplier         float64   `json:"compaction_table_size_multiplier"`
	CompactionTableSizeMultiplierPerLevel []float64 `json:"compaction_table_size_multiplier_per_level"`
	CompactionTotalSize                   int       `json:"compaction_total_size"`
	CompactionTotalSizeMultiplier         float64   `json:"compaction_total_size_multiplier"`
	CompactionTotalSizeMultiplierPerLevel []float64 `json:"compaction_total_size_multiplier_per_level"`
	Compression                           uint      `json:"compression"`
	DisableBufferPool                     bool      `json:"disable_buffer_pool"`
	DisableBlockCache                     bool      `json:"disable_block_cache"`
	DisableCompactionBackoff              bool      `json:"disable_compaction_backoff"`
	DisableLargeBatchTransaction          bool      `json:"disable_large_batch_transaction"`
	IteratorSamplingRate                  int       `json:"iterator_sampling_rate"`
	NoSync                                bool      `json:"no_sync"`
	NoWriteMerge                          bool      `json:"no_write_merge"`
	OpenFilesCacheCapacity                int       `json:"open_files_cache_capacity"`
	ReadOnly                              bool      `json:"read_only"`
	Strict                                uint      `json:"strict"`
	WriteBuffer                           int       `json:"write_buffer"`
	WriteL0PauseTrigger                   int       `json:"write_l0_pause_trigger"`
	WriteL0SlowdownTrigger                int       `json:"write_l0_slowdown_trigger"`
}

func (ldbo *levelDBOptions) Unmarshal() *goleveldb.Options {
	goldbo := &goleveldb.Options{}
	goldbo.BlockCacheCapacity = ldbo.BlockCacheCapacity
	goldbo.BlockCacheEvictRemoved = ldbo.BlockCacheEvictRemoved
	goldbo.BlockRestartInterval = ldbo.BlockRestartInterval
	goldbo.BlockSize = ldbo.BlockSize
	goldbo.CompactionExpandLimitFactor = ldbo.CompactionExpandLimitFactor
	goldbo.CompactionGPOverlapsFactor = ldbo.CompactionGPOverlapsFactor
	goldbo.CompactionL0Trigger = ldbo.CompactionL0Trigger
	goldbo.CompactionSourceLimitFactor = ldbo.CompactionSourceLimitFactor
	goldbo.CompactionTableSize = ldbo.CompactionTableSize
	goldbo.CompactionTableSizeMultiplier = ldbo.CompactionTableSizeMultiplier
	goldbo.CompactionTableSizeMultiplierPerLevel = ldbo.CompactionTableSizeMultiplierPerLevel
	goldbo.CompactionTotalSize = ldbo.CompactionTotalSize
	goldbo.CompactionTotalSizeMultiplier = ldbo.CompactionTotalSizeMultiplier
	goldbo.CompactionTotalSizeMultiplierPerLevel = ldbo.CompactionTotalSizeMultiplierPerLevel
	goldbo.Compression = goleveldb.Compression(ldbo.Compression)
	goldbo.DisableBufferPool = ldbo.DisableBufferPool
	goldbo.DisableBlockCache = ldbo.DisableBlockCache
	goldbo.DisableCompactionBackoff = ldbo.DisableCompactionBackoff
	goldbo.DisableLargeBatchTransaction = ldbo.DisableLargeBatchTransaction
	goldbo.IteratorSamplingRate = ldbo.IteratorSamplingRate
	goldbo.NoSync = ldbo.NoSync
	goldbo.NoWriteMerge = ldbo.NoWriteMerge
	goldbo.OpenFilesCacheCapacity = ldbo.OpenFilesCacheCapacity
	goldbo.ReadOnly = ldbo.ReadOnly
	goldbo.Strict = goleveldb.Strict(ldbo.Strict)
	goldbo.WriteBuffer = ldbo.WriteBuffer
	goldbo.WriteL0PauseTrigger = ldbo.WriteL0PauseTrigger
	goldbo.WriteL0SlowdownTrigger = ldbo.WriteL0SlowdownTrigger
	return goldbo
}

func (ldbo *levelDBOptions) Marshal(goldbo *goleveldb.Options) {
	ldbo.BlockCacheCapacity = goldbo.BlockCacheCapacity
	ldbo.BlockCacheEvictRemoved = goldbo.BlockCacheEvictRemoved
	ldbo.BlockRestartInterval = goldbo.BlockRestartInterval
	ldbo.BlockSize = goldbo.BlockSize
	ldbo.CompactionExpandLimitFactor = goldbo.CompactionExpandLimitFactor
	ldbo.CompactionGPOverlapsFactor = goldbo.CompactionGPOverlapsFactor
	ldbo.CompactionL0Trigger = goldbo.CompactionL0Trigger
	ldbo.CompactionSourceLimitFactor = goldbo.CompactionSourceLimitFactor
	ldbo.CompactionTableSize = goldbo.CompactionTableSize
	ldbo.CompactionTableSizeMultiplier = goldbo.CompactionTableSizeMultiplier
	ldbo.CompactionTableSizeMultiplierPerLevel = goldbo.CompactionTableSizeMultiplierPerLevel
	ldbo.CompactionTotalSize = goldbo.CompactionTotalSize
	ldbo.CompactionTotalSizeMultiplier = goldbo.CompactionTotalSizeMultiplier
	ldbo.CompactionTotalSizeMultiplierPerLevel = goldbo.CompactionTotalSizeMultiplierPerLevel
	ldbo.Compression = uint(goldbo.Compression)
	ldbo.DisableBufferPool = goldbo.DisableBufferPool
	ldbo.DisableBlockCache = goldbo.DisableBlockCache
	ldbo.DisableCompactionBackoff = goldbo.DisableCompactionBackoff
	ldbo.DisableLargeBatchTransaction = goldbo.DisableLargeBatchTransaction
	ldbo.IteratorSamplingRate = goldbo.IteratorSamplingRate
	ldbo.NoSync = goldbo.NoSync
	ldbo.NoWriteMerge = goldbo.NoWriteMerge
	ldbo.OpenFilesCacheCapacity = goldbo.OpenFilesCacheCapacity
	ldbo.ReadOnly = goldbo.ReadOnly
	ldbo.Strict = uint(goldbo.Strict)
	ldbo.WriteBuffer = goldbo.WriteBuffer
	ldbo.WriteL0PauseTrigger = goldbo.WriteL0PauseTrigger
	ldbo.WriteL0SlowdownTrigger = goldbo.WriteL0SlowdownTrigger
}

type jsonConfig struct {
	Folder         string         `json:"folder,omitempty"`
	LevelDBOptions levelDBOptions `json:"leveldb_options,omitempty"`
}

// ConfigKey returns a human-friendly identifier for this type of Datastore.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Default initializes this Config with sensible values.
func (cfg *Config) Default() error {
	cfg.Folder = DefaultSubFolder
	cfg.LevelDBOptions = DefaultLevelDBOptions
	return nil
}

// ApplyEnvVars fills in any Config fields found as environment variables.
func (cfg *Config) ApplyEnvVars() error {
	jcfg := cfg.toJSONConfig()

	err := envconfig.Process(envConfigKey, jcfg)
	if err != nil {
		return err
	}

	return cfg.applyJSONConfig(jcfg)
}

// Validate checks that the fields of this Config have working values,
// at least in appearance.
func (cfg *Config) Validate() error {
	if cfg.Folder == "" {
		return errors.New("folder is unset")
	}

	return nil
}

// LoadJSON reads the fields of this Config from a JSON byteslice as
// generated by ToJSON.
func (cfg *Config) LoadJSON(raw []byte) error {
	jcfg := &jsonConfig{}
	err := json.Unmarshal(raw, jcfg)
	if err != nil {
		return err
	}
	cfg.Default()

	return cfg.applyJSONConfig(jcfg)
}

func (cfg *Config) applyJSONConfig(jcfg *jsonConfig) error {
	config.SetIfNotDefault(jcfg.Folder, &cfg.Folder)

	ldbOpts := jcfg.LevelDBOptions.Unmarshal()

	if err := mergo.Merge(&cfg.LevelDBOptions, ldbOpts, mergo.WithOverride); err != nil {
		return err
	}

	return cfg.Validate()
}

// ToJSON generates a JSON-formatted human-friendly representation of this
// Config.
func (cfg *Config) ToJSON() (raw []byte, err error) {
	jcfg := cfg.toJSONConfig()

	raw, err = config.DefaultJSONMarshal(jcfg)
	return
}

func (cfg *Config) toJSONConfig() *jsonConfig {
	jCfg := &jsonConfig{}

	if cfg.Folder != DefaultSubFolder {
		jCfg.Folder = cfg.Folder
	}

	bo := &levelDBOptions{}
	bo.Marshal(&cfg.LevelDBOptions)
	jCfg.LevelDBOptions = *bo

	return jCfg
}

// GetFolder returns the LevelDB folder.
func (cfg *Config) GetFolder() string {
	if filepath.IsAbs(cfg.Folder) {
		return cfg.Folder
	}

	return filepath.Join(cfg.BaseDir, cfg.Folder)
}

// ToDisplayJSON returns JSON config as a string.
func (cfg *Config) ToDisplayJSON() ([]byte, error) {
	return config.DisplayJSON(cfg.toJSONConfig())
}
