//go:build !arm && !386 && !(openbsd && amd64)

package pebble

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
	"github.com/imdario/mergo"
	"github.com/kelseyhightower/envconfig"

	"github.com/ipfs-cluster/ipfs-cluster/config"
)

const configKey = "pebble"
const envConfigKey = "cluster_pebble"

// Default values for Pebble Config
const (
	DefaultSubFolder = "pebble"
)

var (
	// DefaultPebbleOptions for convenience.
	DefaultPebbleOptions pebble.Options
)

func init() {
	DefaultPebbleOptions = *DefaultPebbleOptions.EnsureDefaults()
	DefaultPebbleOptions.Levels[0].Compression = pebble.NoCompression
	DefaultPebbleOptions.Levels[0].FilterPolicy = bloom.FilterPolicy(10)
}

// Config is used to initialize a Pebble datastore. It implements the
// ComponentConfig interface.
type Config struct {
	config.Saver

	// The folder for this datastore. Non-absolute paths are relative to
	// the base configuration folder.
	Folder string

	PebbleOptions pebble.Options
}

// pebbleOptions is a subset of pebble.Options so it can be marshaled by us in
// the cluster configuration.
type pebbleOptions struct {
	BytesPerSync                int                       `json:"bytes_per_sync"`
	DisableWAL                  bool                      `json:"disable_wal"`
	FlushDelayDeleteRange       time.Duration             `json:"flush_delay_delete_range"`
	FlushDelayRangeKey          time.Duration             `json:"flush_delay_range_key"`
	FlushSplitBytes             int64                     `json:"flush_split_bytes"`
	FormatMajorVersion          pebble.FormatMajorVersion `json:"format_major_version"`
	L0CompactionFileThreshold   int                       `json:"l0_compaction_file_threshold"`
	L0CompactionThreshold       int                       `json:"l0_compaction_threshold"`
	L0StopWritesThreshold       int                       `json:"l0_stop_writes_threshold"`
	LBaseMaxBytes               int64                     `json:"l_base_max_bytes"`
	Levels                      []levelOptions            `json:"levels"`
	MaxOpenFiles                int                       `json:"max_open_files"`
	MemTableSize                int                       `json:"mem_table_size"`
	MemTableStopWritesThreshold int                       `json:"mem_table_stop_writes_threshold"`
	ReadOnly                    bool                      `json:"read_only"`
	WALBytesPerSync             int                       `json:"wal_bytes_per_sync"`
}

func (po *pebbleOptions) Unmarshal() *pebble.Options {
	pebbleOpts := &pebble.Options{}
	pebbleOpts.BytesPerSync = po.BytesPerSync
	pebbleOpts.DisableWAL = po.DisableWAL
	pebbleOpts.FlushDelayDeleteRange = po.FlushDelayDeleteRange
	pebbleOpts.FlushSplitBytes = po.FlushSplitBytes
	pebbleOpts.FormatMajorVersion = po.FormatMajorVersion
	pebbleOpts.L0CompactionFileThreshold = po.L0CompactionFileThreshold
	pebbleOpts.L0CompactionThreshold = po.L0CompactionThreshold
	pebbleOpts.L0StopWritesThreshold = po.L0StopWritesThreshold
	pebbleOpts.LBaseMaxBytes = po.LBaseMaxBytes
	pebbleOpts.Levels = make([]pebble.LevelOptions, len(po.Levels))
	for i := range po.Levels {
		pebbleOpts.Levels[i] = *po.Levels[i].Unmarshal()
	}
	pebbleOpts.MaxOpenFiles = po.MaxOpenFiles
	pebbleOpts.MemTableSize = po.MemTableSize
	pebbleOpts.MemTableStopWritesThreshold = po.MemTableStopWritesThreshold
	pebbleOpts.ReadOnly = po.ReadOnly
	pebbleOpts.WALBytesPerSync = po.WALBytesPerSync
	return pebbleOpts
}

func (po *pebbleOptions) Marshal(pebbleOpts *pebble.Options) {
	po.BytesPerSync = pebbleOpts.BytesPerSync
	po.DisableWAL = pebbleOpts.DisableWAL
	po.FlushDelayDeleteRange = pebbleOpts.FlushDelayDeleteRange
	po.FlushSplitBytes = pebbleOpts.FlushSplitBytes
	po.FormatMajorVersion = pebbleOpts.FormatMajorVersion
	po.L0CompactionFileThreshold = pebbleOpts.L0CompactionFileThreshold
	po.L0CompactionThreshold = pebbleOpts.L0CompactionThreshold
	po.L0StopWritesThreshold = pebbleOpts.L0StopWritesThreshold
	po.LBaseMaxBytes = pebbleOpts.LBaseMaxBytes
	po.Levels = make([]levelOptions, len(pebbleOpts.Levels))
	for i := range pebbleOpts.Levels {
		po.Levels[i].Marshal(&pebbleOpts.Levels[i])
	}
	po.MaxOpenFiles = pebbleOpts.MaxOpenFiles
	po.MemTableSize = pebbleOpts.MemTableSize
	po.MemTableStopWritesThreshold = pebbleOpts.MemTableStopWritesThreshold
	po.ReadOnly = pebbleOpts.ReadOnly
	po.WALBytesPerSync = pebbleOpts.WALBytesPerSync
}

// levelOptions carries options for pebble's per-level parameters.
type levelOptions struct {
	BlockRestartInterval int                `json:"block_restart_interval"`
	BlockSize            int                `json:"block_size"`
	BlockSizeThreshold   int                `json:"block_size_threshold"`
	Compression          pebble.Compression `json:"compression"`
	FilterType           pebble.FilterType  `json:"filter_type"`
	IndexBlockSize       int                `json:"index_block_size"`
	TargetFileSize       int64              `json:"target_file_size"`
}

func (lo *levelOptions) Unmarshal() *pebble.LevelOptions {
	levelOpts := &pebble.LevelOptions{}
	levelOpts.BlockRestartInterval = lo.BlockRestartInterval
	levelOpts.BlockSize = lo.BlockSize
	levelOpts.BlockSizeThreshold = lo.BlockSizeThreshold
	levelOpts.Compression = lo.Compression
	levelOpts.FilterType = lo.FilterType
	levelOpts.IndexBlockSize = lo.IndexBlockSize
	levelOpts.TargetFileSize = lo.TargetFileSize
	return levelOpts
}

func (lo *levelOptions) Marshal(levelOpts *pebble.LevelOptions) {
	lo.BlockRestartInterval = levelOpts.BlockRestartInterval
	lo.BlockSize = levelOpts.BlockSize
	lo.BlockSizeThreshold = levelOpts.BlockSizeThreshold
	lo.Compression = levelOpts.Compression
	lo.FilterType = levelOpts.FilterType
	lo.IndexBlockSize = levelOpts.IndexBlockSize
	lo.TargetFileSize = levelOpts.TargetFileSize
}

type jsonConfig struct {
	Folder        string        `json:"folder,omitempty"`
	PebbleOptions pebbleOptions `json:"pebble_options,omitempty"`
}

// ConfigKey returns a human-friendly identifier for this type of Datastore.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Default initializes this Config with sensible values.
func (cfg *Config) Default() error {
	cfg.Folder = DefaultSubFolder
	cfg.PebbleOptions = DefaultPebbleOptions
	cfg.PebbleOptions.Logger = logger

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

	if err := cfg.PebbleOptions.Validate(); err != nil {
		return err
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

	pebbleOpts := jcfg.PebbleOptions.Unmarshal()

	if err := mergo.Merge(&cfg.PebbleOptions, pebbleOpts, mergo.WithOverride); err != nil {
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

	po := &pebbleOptions{}
	po.Marshal(&cfg.PebbleOptions)
	jCfg.PebbleOptions = *po

	return jCfg
}

// GetFolder returns the Pebble folder.
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
