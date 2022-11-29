package badger3

import (
	"testing"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/badger/v3/options"
)

var cfgJSON = []byte(`
{
    "folder": "test",
    "gc_discard_ratio": 0.1,
    "gc_sleep": "2m",
    "badger_options": {
         "max_levels": 4,
	 "compression": 2
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

	if cfg.BadgerOptions.Compression != options.ZSTD {
		t.Fatalf("got: %d, want: %d", cfg.BadgerOptions.Compression, options.ZSTD)
	}

	if cfg.BadgerOptions.ValueLogFileSize != badger.DefaultOptions("").ValueLogFileSize {
		t.Fatalf(
			"got: %d, want: %d",
			cfg.BadgerOptions.ValueLogFileSize,
			badger.DefaultOptions("").ValueLogFileSize,
		)
	}

	if cfg.BadgerOptions.ChecksumVerificationMode != badger.DefaultOptions("").ChecksumVerificationMode {
		t.Fatalf("ChecksumVerificationMode is not nil: got: %v, want: %v", cfg.BadgerOptions.ChecksumVerificationMode, badger.DefaultOptions("").ChecksumVerificationMode)
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
