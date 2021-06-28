package badger

import (
	"testing"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
)

var cfgJSON = []byte(`
{
    "folder": "test",
    "gc_discard_ratio": 0.1,
    "gc_sleep": "2m",
    "badger_options": {
         "max_levels": 4,
	 "value_log_loading_mode": 0
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

	if cfg.GCDiscardRatio != 0.1 {
		t.Fatal("GCDiscardRatio should be 0.1")
	}

	if cfg.GCInterval != DefaultGCInterval {
		t.Fatal("GCInterval should default as it is unset")
	}

	if cfg.GCSleep != 2*time.Minute {
		t.Fatal("GCSleep should be 2m")
	}

	if cfg.BadgerOptions.ValueLogLoadingMode != options.FileIO {
		t.Fatalf("got: %d, want: %d", cfg.BadgerOptions.ValueLogLoadingMode, options.FileIO)
	}

	if cfg.BadgerOptions.ValueLogFileSize != badger.DefaultOptions("").ValueLogFileSize {
		t.Fatalf(
			"got: %d, want: %d",
			cfg.BadgerOptions.ValueLogFileSize,
			badger.DefaultOptions("").ValueLogFileSize,
		)
	}

	if cfg.BadgerOptions.TableLoadingMode != badger.DefaultOptions("").TableLoadingMode {
		t.Fatalf("TableLoadingMode is not nil: got: %v, want: %v", cfg.BadgerOptions.TableLoadingMode, badger.DefaultOptions("").TableLoadingMode)
	}

	if cfg.BadgerOptions.MaxLevels != 4 {
		t.Fatalf("MaxLevels should be 4, got: %d", cfg.BadgerOptions.MaxLevels)
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

	cfg.GCDiscardRatio = 0
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}
