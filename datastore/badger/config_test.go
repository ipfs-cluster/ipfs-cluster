package badger

import (
	"fmt"
	"testing"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
)

var cfgJSON = []byte(`
{
    "folder": "test",
	"value_log_loading_mode": 1
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
	if cfg.BadgerOptions.ValueLogFileSize != badger.DefaultOptions.ValueLogFileSize {
		t.Fatalf(
			"got: %d, want: %d",
			cfg.BadgerOptions.ValueLogFileSize,
			badger.DefaultOptions.ValueLogFileSize,
		)
	}

	fmt.Printf("%+v\n", cfg.BadgerOptions)

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
