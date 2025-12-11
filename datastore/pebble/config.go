package pebble

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"dario.cat/mergo"
	"github.com/cockroachdb/pebble/v2"
	"github.com/cockroachdb/pebble/v2/bloom"
	"github.com/cockroachdb/pebble/v2/sstable"
	"github.com/cockroachdb/pebble/v2/sstable/block"
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
	// DefaultCacheSize sets the maximum size of the block cache.
	DefaultCacheSize int64 = 1 << 30 // Pebble's default: 8MiB
	// DefaultMemTableSize sets the size of the memtables and affecst total
	// size of the WAL. It must be under 4GB.
	DefaultMemTableSize uint64 = 64 << 20 // Pebble's default: 4MiB
	// DefaultMemTableStopWritesThreshold defines how many memtables can
	// be queued for writing before stopping writes (memtable memory
	// consumption should approach
	// MemTableStopWritesThreshold*MemTableSize in that case).
	DefaultMemTableStopWritesThreshold = 20 // Pebble's default: 12
	// DefaultBytesPerSync controls how often to call the Filesystem
	// Sync.
	DefaultBytesPerSync = 1 << 20 // Pebble's default: 512KiB
	// DefaultMaxConcurrentCompactions controls how many compactions
	// happen at a single time.
	DefaultMaxConcurrentCompactions = 5 // Pebble's default: 1
	// DefaultMaxOpenFiles controls how many files can be kept open by
	// Pebble.
	DefaultMaxOpenFiles = 1000 // Pebble's default: 500
	// DefaultL0CompactionThreshold defines the read amplification on L0
	// that triggers compaction
	DefaultL0CompactionThreshold = 4 // Pebble's default: 4
	// DefaultL0CompactionFileThreshold defines the number of files that
	// trigger compactions of L0
	DefaultL0CompactionFileThreshold = 750 // Pebble's default: 500
	// DefaultL0StopWritesThreshold defines the critical threshold for
	// read amplification on L0, which stops writes until compaction
	// reduces it.
	DefaultL0StopWritesThreshold = 12 // Pebble's default : 4
	// DefaultLBaseMaxBytes defines maximum size of LBase, where memtables
	// are temporally written to
	DefaultLBaseMaxBytes int64 = 128 << 20 // Pebble's default: 64MiB
	// DefaultL0TargetFileSize defines the target filesize for L0. It is
	// multiplied by 2 for every subsequent level.
	DefaultL0TargetFileSize int64 = 4 << 20 // Pebble's default: 4M
	// DefaultBlockSize defines the target size for table blocks (used in
	// all levels).
	DefaultBlockSize int = 4 << 10 // Pebble's default: 4KiB
	// DefaultFilterPolicy defines the number of bits used per key for
	// bloom filters. 10 yields a 1% false positive rate.
	DefaultFilterPolicy bloom.FilterPolicy = 10 // Pebble's default: 10
	// DefaultFormatMajorVersion sets the format of Pebble on-disk files.
	DefaultFormatMajorVersion = pebble.FormatNewest
)

func init() {
	DefaultPebbleOptions.EnsureDefaults()
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
	EventListener               *pebble.EventListener     `json:"-"`
	CacheSizeBytes              int64                     `json:"cache_size_bytes"`
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
	MaxOpenFiles                int                       `json:"max_open_files"`
	MemTableSize                uint64                    `json:"mem_table_size"`
	MemTableStopWritesThreshold int                       `json:"mem_table_stop_writes_threshold"`
	ReadOnly                    bool                      `json:"read_only"`
	WALBytesPerSync             int                       `json:"wal_bytes_per_sync"`
	Levels                      []levelOptions            `json:"levels"`
}

func (po *pebbleOptions) Unmarshal() *pebble.Options {
	pebbleOpts := &pebble.Options{}
	if size := po.CacheSizeBytes; size > 0 {
		cache := pebble.NewCache(size)
		pebbleOpts.Cache = cache
	}
	pebbleOpts.BytesPerSync = po.BytesPerSync
	pebbleOpts.DisableWAL = po.DisableWAL
	pebbleOpts.FlushDelayDeleteRange = po.FlushDelayDeleteRange
	pebbleOpts.FlushSplitBytes = po.FlushSplitBytes
	pebbleOpts.FormatMajorVersion = po.FormatMajorVersion
	pebbleOpts.L0CompactionFileThreshold = po.L0CompactionFileThreshold
	pebbleOpts.L0CompactionThreshold = po.L0CompactionThreshold
	pebbleOpts.L0StopWritesThreshold = po.L0StopWritesThreshold
	pebbleOpts.LBaseMaxBytes = po.LBaseMaxBytes
	// pebbleOpts.Levels
	pebbleOpts.MaxOpenFiles = po.MaxOpenFiles
	pebbleOpts.MemTableSize = po.MemTableSize
	pebbleOpts.MemTableStopWritesThreshold = po.MemTableStopWritesThreshold
	pebbleOpts.ReadOnly = po.ReadOnly
	pebbleOpts.WALBytesPerSync = po.WALBytesPerSync
	for i := range po.Levels {
		lvlOpts, targetFileSize := po.Levels[i].Unmarshal()
		pebbleOpts.Levels[i] = *lvlOpts
		pebbleOpts.TargetFileSizes[i] = targetFileSize
	}
	return pebbleOpts
}

func (po *pebbleOptions) Marshal(pebbleOpts *pebble.Options) {
	if pebbleOpts.Cache != nil {
		po.CacheSizeBytes = pebbleOpts.Cache.MaxSize()
	}
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
		po.Levels[i].Marshal(&pebbleOpts.Levels[i], pebbleOpts.TargetFileSizes[i])
	}
	po.MaxOpenFiles = pebbleOpts.MaxOpenFiles
	po.MemTableSize = pebbleOpts.MemTableSize
	po.MemTableStopWritesThreshold = pebbleOpts.MemTableStopWritesThreshold
	po.ReadOnly = pebbleOpts.ReadOnly
	po.WALBytesPerSync = pebbleOpts.WALBytesPerSync
}

// levelOptions carries options for pebble's per-level parameters.
// Compression used to be:
//
// const (
//
//	DefaultCompression Compression = iota
//	NoCompression
//	SnappyCompression
//	ZstdCompression
//	NCompression -- which means NoCompression it seems
//
// )
type levelOptions struct {
	BlockRestartInterval int                `json:"block_restart_interval"`
	BlockSize            int                `json:"block_size"`
	BlockSizeThreshold   int                `json:"block_size_threshold"`
	Compression          int                `json:"compression"`
	FilterType           pebble.FilterType  `json:"filter_type"`
	FilterPolicy         bloom.FilterPolicy `json:"filter_policy"`
	IndexBlockSize       int                `json:"index_block_size"`
	TargetFileSize       int64              `json:"target_file_size"`
}

func (lo *levelOptions) Unmarshal() (*pebble.LevelOptions, int64) {
	levelOpts := &pebble.LevelOptions{}
	levelOpts.BlockRestartInterval = lo.BlockRestartInterval
	levelOpts.BlockSize = lo.BlockSize
	levelOpts.BlockSizeThreshold = lo.BlockSizeThreshold
	// On prevous relesases we could return the int. Now we need to provide a profile.
	levelOpts.Compression = func() *sstable.CompressionProfile {
		switch lo.Compression {
		case 0:
			return block.DefaultCompression
		case 1:
			return block.NoCompression
		case 2:
			return block.SnappyCompression
		case 3:
			return block.ZstdCompression
		default:
			return block.NoCompression
		}
	}
	levelOpts.FilterType = lo.FilterType
	levelOpts.FilterPolicy = bloom.FilterPolicy(lo.FilterPolicy)
	levelOpts.IndexBlockSize = lo.IndexBlockSize
	return levelOpts, lo.TargetFileSize
}

func (lo *levelOptions) Marshal(levelOpts *pebble.LevelOptions, targetFileSize int64) {
	lo.BlockRestartInterval = levelOpts.BlockRestartInterval
	lo.BlockSize = levelOpts.BlockSize
	lo.BlockSizeThreshold = levelOpts.BlockSizeThreshold
	switch levelOpts.Compression().Name {
	case "snappy":
		lo.Compression = 2
	case "zlib":
		lo.Compression = 3
	default:
		lo.Compression = 1 // NoCompression
	}
	lo.FilterType = levelOpts.FilterType

	if fp, ok := levelOpts.FilterPolicy.(bloom.FilterPolicy); ok {
		lo.FilterPolicy = fp
	}

	lo.IndexBlockSize = levelOpts.IndexBlockSize
	lo.TargetFileSize = targetFileSize
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
	eventListener := pebble.MakeLoggingEventListener(logger)
	cfg.PebbleOptions.EventListener = &eventListener

	cache := pebble.NewCache(DefaultCacheSize)
	cfg.PebbleOptions.Cache = cache
	cfg.PebbleOptions.FormatMajorVersion = DefaultFormatMajorVersion
	cfg.PebbleOptions.MemTableSize = DefaultMemTableSize
	cfg.PebbleOptions.MemTableStopWritesThreshold = DefaultMemTableStopWritesThreshold
	cfg.PebbleOptions.BytesPerSync = DefaultBytesPerSync
	cfg.PebbleOptions.MaxOpenFiles = DefaultMaxOpenFiles
	cfg.PebbleOptions.L0CompactionThreshold = DefaultL0CompactionThreshold
	cfg.PebbleOptions.L0CompactionFileThreshold = DefaultL0CompactionFileThreshold
	cfg.PebbleOptions.L0StopWritesThreshold = DefaultL0StopWritesThreshold
	cfg.PebbleOptions.LBaseMaxBytes = DefaultLBaseMaxBytes

	// cfg.PebbleOptions.Levels = make([]pebble.LevelOptions, 7) // fixed to [7]LevelOptions
	// Deprecated: cfg.PebbleOptions.Levels[0].TargetFileSize = DefaultL0TargetFileSize
	cfg.PebbleOptions.TargetFileSizes[0] = DefaultL0TargetFileSize //added
	for i := 0; i < len(cfg.PebbleOptions.Levels); i++ {
		l := &cfg.PebbleOptions.Levels[i]
		l.BlockSize = DefaultBlockSize
		l.FilterPolicy = DefaultFilterPolicy
		l.FilterType = pebble.TableFilter
		if i > 0 {
			cfg.PebbleOptions.TargetFileSizes[i] = cfg.PebbleOptions.TargetFileSizes[i-1] * 2
		}
		if i == 0 {
			l.EnsureL0Defaults() // does not overwite, only sets the rest.
		} else {
			l.EnsureL1PlusDefaults(&cfg.PebbleOptions.Levels[i-1])
		}
	}

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
