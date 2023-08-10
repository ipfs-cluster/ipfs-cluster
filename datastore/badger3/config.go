package badger3

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"dario.cat/mergo"
	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/badger/v3/options"
	"github.com/kelseyhightower/envconfig"

	"github.com/ipfs-cluster/ipfs-cluster/config"
)

const configKey = "badger3"
const envConfigKey = "cluster_badger3"

// Default values for badger Config
const (
	DefaultSubFolder = "badger3"
)

var (
	// DefaultBadgerOptions has to be a var because badger.DefaultOptions
	// is. Values are customized during Init().
	DefaultBadgerOptions badger.Options

	// DefaultGCDiscardRatio for GC operations. See Badger docs.
	DefaultGCDiscardRatio float64 = 0.2
	// DefaultGCInterval specifies interval between GC cycles.
	DefaultGCInterval time.Duration = 15 * time.Minute
	// DefaultGCSleep specifies sleep time between GC rounds.
	DefaultGCSleep time.Duration = 10 * time.Second
)

func init() {
	DefaultBadgerOptions = badger.DefaultOptions("")
	// Better to slow down starts than shutdowns.
	DefaultBadgerOptions.CompactL0OnClose = false
	// Defaults to 1MB! For us that means everything goes into the LSM
	// tree and the LSM tree is supposed to be loaded into memory in full.
	// We only put very small things on the LSM tree by default (i.e. a
	// single CID).
	DefaultBadgerOptions.ValueThreshold = 100
	// Disable Block Cache: the cluster read-pattern at scale requires
	// looping regularly all keys. The CRDT read-patterm avoids reading
	// something twice. In general, it probably does not add much, and it
	// is recommended to be disabled when not using compression.
	DefaultBadgerOptions.BlockCacheSize = 0
	// Let's disable compression for values, better perf when reading and
	// usually the ratio between data stored by badger and the cluster
	// should be small. Users can always enable.
	DefaultBadgerOptions.Compression = options.None
	// There is a write lock in go-ds-crdt that writes batches one by one.
	// Also NewWriteBatch says that there can never be transaction
	// conflicts when doing batches. And IPFS will only write a block
	// once, or do it with the same values. In general, we probably don't
	// care about conflicts much (rows updated while a commit transaction
	// was open). Increases perf too.
	DefaultBadgerOptions.DetectConflicts = false
	// TODO: Increase memtable size. This will use some more memory, but any
	// normal system should be able to deal with using 256MiB for the
	// memtable. Badger puts a lot of things in memory anyways,
	// i.e. IndexCacheSize is set to 0. Note NumMemTables is 5.
	// DefaultBadgerOptions.MemTableSize = 268435456 // 256MiB

}

// Config is used to initialize a BadgerDB datastore. It implements the
// ComponentConfig interface.
type Config struct {
	config.Saver

	// The folder for this datastore. Non-absolute paths are relative to
	// the base configuration folder.
	Folder string

	// For GC operation. See Badger documentation.
	GCDiscardRatio float64

	// Interval between GC cycles. Each GC cycle runs one or more
	// rounds separated by GCSleep.
	GCInterval time.Duration

	// Time between rounds in a GC cycle
	GCSleep time.Duration

	BadgerOptions badger.Options
}

// badgerOptions is a copy of badger.Options so it can be marshaled by us.
type badgerOptions struct {
	Dir               string `json:"dir"`
	ValueDir          string `json:"value_dir"`
	SyncWrites        bool   `json:"sync_writes"`
	NumVersionsToKeep int    `json:"num_versions_to_keep"`
	ReadOnly          bool   `json:"read_only"`
	// Logger
	Compression    options.CompressionType `json:"compression"`
	InMemory       bool                    `json:"in_memory"`
	MetricsEnabled bool                    `json:"metrics_enabled"`
	NumGoroutines  int                     `json:"num_goroutines"`

	MemTableSize        int64 `json:"mem_table_size"`
	BaseTableSize       int64 `json:"base_table_size"`
	BaseLevelSize       int64 `json:"base_level_size"`
	LevelSizeMultiplier int   `json:"level_size_multiplier"`
	TableSizeMultiplier int   `json:"table_size_multiplier"`
	MaxLevels           int   `json:"max_levels"`

	VLogPercentile     float64 `json:"v_log_percentile"`
	ValueThreshold     int64   `json:"value_threshold"`
	NumMemtables       int     `json:"num_memtables"`
	BlockSize          int     `json:"block_size"`
	BloomFalsePositive float64 `json:"bloom_false_positive"`
	BlockCacheSize     int64   `json:"block_cache_size"`
	IndexCacheSize     int64   `json:"index_cache_size"`

	NumLevelZeroTables      int `json:"num_level_zero_tables"`
	NumLevelZeroTablesStall int `json:"num_level_zero_tables_stall"`

	ValueLogFileSize   int64  `json:"value_log_file_size"`
	ValueLogMaxEntries uint32 `json:"value_log_max_entries"`

	NumCompactors        int  `json:"num_compactors"`
	CompactL0OnClose     bool `json:"compact_l_0_on_close"`
	LmaxCompaction       bool `json:"lmax_compaction"`
	ZSTDCompressionLevel int  `json:"zstd_compression_level"`

	VerifyValueChecksum bool `json:"verify_value_checksum"`

	ChecksumVerificationMode options.ChecksumVerificationMode `json:"checksum_verification_mode"`
	DetectConflicts          bool                             `json:"detect_conflicts"`

	NamespaceOffset int `json:"namespace_offset"`
}

func (bo *badgerOptions) Unmarshal() *badger.Options {
	badgerOpts := &badger.Options{}
	badgerOpts.Dir = bo.Dir
	badgerOpts.ValueDir = bo.ValueDir
	badgerOpts.SyncWrites = bo.SyncWrites
	badgerOpts.NumVersionsToKeep = bo.NumVersionsToKeep
	badgerOpts.ReadOnly = bo.ReadOnly
	badgerOpts.Compression = bo.Compression
	badgerOpts.InMemory = bo.InMemory
	badgerOpts.MetricsEnabled = bo.MetricsEnabled
	badgerOpts.NumGoroutines = bo.NumGoroutines

	badgerOpts.MemTableSize = bo.MemTableSize
	badgerOpts.BaseTableSize = bo.BaseTableSize
	badgerOpts.BaseLevelSize = bo.BaseLevelSize
	badgerOpts.LevelSizeMultiplier = bo.LevelSizeMultiplier
	badgerOpts.TableSizeMultiplier = bo.TableSizeMultiplier
	badgerOpts.MaxLevels = bo.MaxLevels

	badgerOpts.VLogPercentile = bo.VLogPercentile
	badgerOpts.ValueThreshold = bo.ValueThreshold
	badgerOpts.NumMemtables = bo.NumMemtables
	badgerOpts.BlockSize = bo.BlockSize
	badgerOpts.BloomFalsePositive = bo.BloomFalsePositive
	badgerOpts.BlockCacheSize = bo.BlockCacheSize
	badgerOpts.IndexCacheSize = bo.IndexCacheSize

	badgerOpts.NumLevelZeroTables = bo.NumLevelZeroTables
	badgerOpts.NumLevelZeroTablesStall = bo.NumLevelZeroTablesStall

	badgerOpts.ValueLogFileSize = bo.ValueLogFileSize
	badgerOpts.ValueLogMaxEntries = bo.ValueLogMaxEntries

	badgerOpts.NumCompactors = bo.NumCompactors
	badgerOpts.CompactL0OnClose = bo.CompactL0OnClose
	badgerOpts.LmaxCompaction = bo.LmaxCompaction
	badgerOpts.ZSTDCompressionLevel = bo.ZSTDCompressionLevel

	badgerOpts.VerifyValueChecksum = bo.VerifyValueChecksum

	badgerOpts.ChecksumVerificationMode = bo.ChecksumVerificationMode
	badgerOpts.DetectConflicts = bo.DetectConflicts

	badgerOpts.NamespaceOffset = bo.NamespaceOffset

	return badgerOpts
}

func (bo *badgerOptions) Marshal(badgerOpts *badger.Options) {
	bo.Dir = badgerOpts.Dir
	bo.ValueDir = badgerOpts.ValueDir
	bo.SyncWrites = badgerOpts.SyncWrites
	bo.NumVersionsToKeep = badgerOpts.NumVersionsToKeep
	bo.ReadOnly = badgerOpts.ReadOnly
	bo.Compression = badgerOpts.Compression
	bo.InMemory = badgerOpts.InMemory
	bo.MetricsEnabled = badgerOpts.MetricsEnabled
	bo.NumGoroutines = badgerOpts.NumGoroutines

	bo.MemTableSize = badgerOpts.MemTableSize
	bo.BaseTableSize = badgerOpts.BaseTableSize
	bo.BaseLevelSize = badgerOpts.BaseLevelSize
	bo.LevelSizeMultiplier = badgerOpts.LevelSizeMultiplier
	bo.TableSizeMultiplier = badgerOpts.TableSizeMultiplier
	bo.MaxLevels = badgerOpts.MaxLevels

	bo.VLogPercentile = badgerOpts.VLogPercentile
	bo.ValueThreshold = badgerOpts.ValueThreshold
	bo.NumMemtables = badgerOpts.NumMemtables
	bo.BlockSize = badgerOpts.BlockSize
	bo.BloomFalsePositive = badgerOpts.BloomFalsePositive
	bo.BlockCacheSize = badgerOpts.BlockCacheSize
	bo.IndexCacheSize = badgerOpts.IndexCacheSize

	bo.NumLevelZeroTables = badgerOpts.NumLevelZeroTables
	bo.NumLevelZeroTablesStall = badgerOpts.NumLevelZeroTablesStall

	bo.ValueLogFileSize = badgerOpts.ValueLogFileSize
	bo.ValueLogMaxEntries = badgerOpts.ValueLogMaxEntries

	bo.NumCompactors = badgerOpts.NumCompactors
	bo.CompactL0OnClose = badgerOpts.CompactL0OnClose
	bo.LmaxCompaction = badgerOpts.LmaxCompaction
	bo.ZSTDCompressionLevel = badgerOpts.ZSTDCompressionLevel

	bo.VerifyValueChecksum = badgerOpts.VerifyValueChecksum

	bo.ChecksumVerificationMode = badgerOpts.ChecksumVerificationMode
	bo.DetectConflicts = badgerOpts.DetectConflicts

	bo.NamespaceOffset = badgerOpts.NamespaceOffset
}

type jsonConfig struct {
	Folder         string        `json:"folder,omitempty"`
	GCDiscardRatio float64       `json:"gc_discard_ratio"`
	GCInterval     string        `json:"gc_interval"`
	GCSleep        string        `json:"gc_sleep"`
	BadgerOptions  badgerOptions `json:"badger_options,omitempty"`
}

// ConfigKey returns a human-friendly identifier for this type of Datastore.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Default initializes this Config with sensible values.
func (cfg *Config) Default() error {
	cfg.Folder = DefaultSubFolder
	cfg.GCDiscardRatio = DefaultGCDiscardRatio
	cfg.GCInterval = DefaultGCInterval
	cfg.GCSleep = DefaultGCSleep
	cfg.BadgerOptions = DefaultBadgerOptions
	cfg.BadgerOptions.Logger = logger
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

	if cfg.GCDiscardRatio <= 0 || cfg.GCDiscardRatio >= 1 {
		return errors.New("gc_discard_ratio must be more than 0 and less than 1")
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

	// 0 is an invalid option anyways. In that case, set default (0.2)
	config.SetIfNotDefault(jcfg.GCDiscardRatio, &cfg.GCDiscardRatio)

	// If these durations are set, GC is enabled by default with default
	// values.
	err := config.ParseDurations("badger",
		&config.DurationOpt{Duration: jcfg.GCInterval, Dst: &cfg.GCInterval, Name: "gc_interval"},
		&config.DurationOpt{Duration: jcfg.GCSleep, Dst: &cfg.GCSleep, Name: "gc_sleep"},
	)
	if err != nil {
		return err
	}

	badgerOpts := jcfg.BadgerOptions.Unmarshal()

	if err := mergo.Merge(&cfg.BadgerOptions, badgerOpts, mergo.WithOverride); err != nil {
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

	jCfg.GCDiscardRatio = cfg.GCDiscardRatio
	jCfg.GCInterval = cfg.GCInterval.String()
	jCfg.GCSleep = cfg.GCSleep.String()

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
