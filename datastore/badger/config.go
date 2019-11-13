package badger

import (
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/imdario/mergo"
	"github.com/kelseyhightower/envconfig"

	"github.com/ipfs/ipfs-cluster/config"
)

const configKey = "badger"
const envConfigKey = "cluster_badger"

// Default values for badger Config
const (
	DefaultSubFolder = "badger"
)

var (
	// DefaultBadgerOptions has to be a var because badger.DefaultOptions
	// is. Values are customized during Init().
	DefaultBadgerOptions badger.Options
)

func init() {
	// Following go-ds-badger guidance
	DefaultBadgerOptions = badger.DefaultOptions("")
	DefaultBadgerOptions.CompactL0OnClose = false
	DefaultBadgerOptions.Truncate = true
	DefaultBadgerOptions.ValueLogLoadingMode = options.FileIO
	// Explicitly set this to mmap. This doesn't use much memory anyways.
	DefaultBadgerOptions.TableLoadingMode = options.MemoryMap
	// Reduce this from 64MiB to 16MiB. That means badger will hold on to
	// 20MiB by default instead of 80MiB.
	DefaultBadgerOptions.MaxTableSize = 16 << 20
}

// Config is used to initialize a BadgerDB datastore. It implements the
// ComponentConfig interface.
type Config struct {
	config.Saver

	// The folder for this datastore. Non-absolute paths are relative to
	// the base configuration folder.
	Folder string

	BadgerOptions badger.Options
}

// badgerOptions is a copy of options.BadgerOptions but
// without the Logger as it cannot be marshalled to/from
// JSON.
type badgerOptions struct {
	Dir                     string                   `json:"dir"`
	ValueDir                string                   `json:"value_dir"`
	SyncWrites              bool                     `json:"sync_writes"`
	TableLoadingMode        *options.FileLoadingMode `json:"table_loading_mode"`
	ValueLogLoadingMode     *options.FileLoadingMode `json:"value_log_loading_mode"`
	NumVersionsToKeep       int                      `json:"num_versions_to_keep"`
	MaxTableSize            int64                    `json:"max_table_size"`
	LevelSizeMultiplier     int                      `json:"level_size_multiplier"`
	MaxLevels               int                      `json:"max_levels"`
	ValueThreshold          int                      `json:"value_threshold"`
	NumMemtables            int                      `json:"num_memtables"`
	NumLevelZeroTables      int                      `json:"num_level_zero_tables"`
	NumLevelZeroTablesStall int                      `json:"num_level_zero_tables_stall"`
	LevelOneSize            int64                    `json:"level_one_size"`
	ValueLogFileSize        int64                    `json:"value_log_file_size"`
	ValueLogMaxEntries      uint32                   `json:"value_log_max_entries"`
	NumCompactors           int                      `json:"num_compactors"`
	CompactL0OnClose        bool                     `json:"compact_l_0_on_close"`
	ReadOnly                bool                     `json:"read_only"`
	Truncate                bool                     `json:"truncate"`
}

func (bo *badgerOptions) Unmarshal() *badger.Options {
	badgerOpts := &badger.Options{}
	badgerOpts.Dir = bo.Dir
	badgerOpts.ValueDir = bo.ValueDir
	badgerOpts.SyncWrites = bo.SyncWrites
	if tlm := bo.TableLoadingMode; tlm != nil {
		badgerOpts.TableLoadingMode = *tlm
	}
	if vlm := bo.ValueLogLoadingMode; vlm != nil {
		badgerOpts.ValueLogLoadingMode = *vlm
	}
	badgerOpts.NumVersionsToKeep = bo.NumVersionsToKeep
	badgerOpts.MaxTableSize = bo.MaxTableSize
	badgerOpts.LevelSizeMultiplier = bo.LevelSizeMultiplier
	badgerOpts.MaxLevels = bo.MaxLevels
	badgerOpts.ValueThreshold = bo.ValueThreshold
	badgerOpts.NumMemtables = bo.NumMemtables
	badgerOpts.NumLevelZeroTables = bo.NumLevelZeroTables
	badgerOpts.NumLevelZeroTablesStall = bo.NumLevelZeroTablesStall
	badgerOpts.LevelOneSize = bo.LevelOneSize
	badgerOpts.ValueLogFileSize = bo.ValueLogFileSize
	badgerOpts.ValueLogMaxEntries = bo.ValueLogMaxEntries
	badgerOpts.NumCompactors = bo.NumCompactors
	badgerOpts.CompactL0OnClose = bo.CompactL0OnClose
	badgerOpts.ReadOnly = bo.ReadOnly
	badgerOpts.Truncate = bo.Truncate

	return badgerOpts
}

func (bo *badgerOptions) Marshal(badgerOpts *badger.Options) {
	bo.Dir = badgerOpts.Dir
	bo.ValueDir = badgerOpts.ValueDir
	bo.SyncWrites = badgerOpts.SyncWrites
	bo.TableLoadingMode = &badgerOpts.TableLoadingMode
	bo.ValueLogLoadingMode = &badgerOpts.ValueLogLoadingMode
	bo.NumVersionsToKeep = badgerOpts.NumVersionsToKeep
	bo.MaxTableSize = badgerOpts.MaxTableSize
	bo.LevelSizeMultiplier = badgerOpts.LevelSizeMultiplier
	bo.MaxLevels = badgerOpts.MaxLevels
	bo.ValueThreshold = badgerOpts.ValueThreshold
	bo.NumMemtables = badgerOpts.NumMemtables
	bo.NumLevelZeroTables = badgerOpts.NumLevelZeroTables
	bo.NumLevelZeroTablesStall = badgerOpts.NumLevelZeroTablesStall
	bo.LevelOneSize = badgerOpts.LevelOneSize
	bo.ValueLogFileSize = badgerOpts.ValueLogFileSize
	bo.ValueLogMaxEntries = badgerOpts.ValueLogMaxEntries
	bo.NumCompactors = badgerOpts.NumCompactors
	bo.CompactL0OnClose = badgerOpts.CompactL0OnClose
	bo.ReadOnly = badgerOpts.ReadOnly
	bo.Truncate = badgerOpts.Truncate
}

type jsonConfig struct {
	Folder        string        `json:"folder,omitempty"`
	BadgerOptions badgerOptions `json:"badger_options,omitempty"`
}

// ConfigKey returns a human-friendly identifier for this type of Datastore.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Default initializes this Config with sensible values.
func (cfg *Config) Default() error {
	cfg.Folder = DefaultSubFolder
	cfg.BadgerOptions = DefaultBadgerOptions
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

	badgerOpts := jcfg.BadgerOptions.Unmarshal()

	if err := mergo.Merge(&cfg.BadgerOptions, badgerOpts, mergo.WithOverride); err != nil {
		return err
	}

	if jcfg.BadgerOptions.TableLoadingMode != nil {
		cfg.BadgerOptions.TableLoadingMode = *jcfg.BadgerOptions.TableLoadingMode
	}

	if jcfg.BadgerOptions.ValueLogLoadingMode != nil {
		cfg.BadgerOptions.ValueLogLoadingMode = *jcfg.BadgerOptions.ValueLogLoadingMode
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

	bo := &badgerOptions{}
	bo.Marshal(&cfg.BadgerOptions)
	jCfg.BadgerOptions = *bo

	return jCfg
}

// GetFolder returns the BadgerDB folder.
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
