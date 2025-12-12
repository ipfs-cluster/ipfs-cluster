package pebble

import (
	"testing"
)

var cfgJSON = []byte(`
{
    "folder": "test",
     "pebble_options": {
        "bytes_per_sync": 524288,
        "disable_wal": true,
        "flush_delay_delete_range": 0,
        "flush_delay_range_key": 0,
        "flush_split_bytes": 4194304,
        "format_major_version": 16,
        "l0_compaction_file_threshold": 500,
        "l0_compaction_threshold": 2,
        "l0_stop_writes_threshold": 12,
        "l_base_max_bytes": 67108864,
        "levels": [
          {
            "block_restart_interval": 16,
            "block_size": 4096,
            "block_size_threshold": 90,
            "compression": 0,
            "filter_type": 0,
            "index_block_size": 8000,
            "target_file_size": 2097152
          },
          {
            "block_restart_interval": 16,
            "block_size": 4096,
            "block_size_threshold": 90,
            "compression": 1,
            "filter_type": 0,
            "index_block_size": 8000,
            "target_file_size": 2097152
          }
        ],
        "max_open_files": 1000,
        "mem_table_size": 4194304,
        "mem_table_stop_writes_threshold": 2,
        "read_only": false,
        "wal_bytes_per_sync": 0,
        "max_concurrent_compactions": 2
    }
}
`)

func TestLoadJSON(t *testing.T) {
	cfg := &Config{}
	err := cfg.LoadJSON(cfgJSON)
	if err != nil {
		t.Fatal(err)
	}
}

func TestToJSON(t *testing.T) {
	cfg := &Config{}
	cfg.LoadJSON(cfgJSON)

	if cfg.PebbleOptions.L0CompactionThreshold != 2 {
		t.Fatalf("got: %d, want: %d", cfg.PebbleOptions.L0CompactionThreshold, 2)
	}

	if !cfg.PebbleOptions.DisableWAL {
		t.Fatal("Disable WAL should be true")
	}

	// level0 compression set to 0, default is snappy
	if comp := cfg.PebbleOptions.Levels[0].Compression().Name; comp != "Snappy" {
		t.Fatal("compression is ", comp)
	}

	// inherits level 1 config - no compression
	if comp := cfg.PebbleOptions.Levels[5].Compression().Name; comp != "NoCompression" {
		t.Fatal("compression is ", comp)
	}

	// hardcoded
	if tfs := cfg.PebbleOptions.TargetFileSizes[0]; tfs != 2097152 {
		t.Fatal("TargetFileSize[0] is ", tfs)
	}

	// hardcoded
	if tfs := cfg.PebbleOptions.TargetFileSizes[1]; tfs != 2097152 {
		t.Fatal("TargetFileSize[1] is ", tfs)
	}

	// prev * 2
	if tfs := cfg.PebbleOptions.TargetFileSizes[2]; tfs != 2097152*2 {
		t.Fatal("TargetFileSize[2] is ", tfs)
	}

	newjson, err := cfg.ToJSON()
	if err != nil {
		t.Fatal(err)
	}

	cfg = &Config{}
	err = cfg.LoadJSON(newjson)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDefault(t *testing.T) {
	cfg := &Config{}
	cfg.Default()
	if cfg.Validate() != nil {
		t.Fatal("error validating")
	}

	cfg.PebbleOptions.MemTableStopWritesThreshold = 0
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}
