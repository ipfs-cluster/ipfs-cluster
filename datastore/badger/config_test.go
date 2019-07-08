package badger

import (
	"testing"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
)

var cfgJSON = []byte(`
{
    "folder": "test",
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
