package leveldb

import (
	"testing"
)

var cfgJSON = []byte(`
{
    "folder": "test",
    "leveldb_options": {
         "no_sync": true,
         "compaction_total_size_multiplier": 1.5
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

	if !cfg.LevelDBOptions.NoSync {
		t.Fatalf("NoSync should be true")
	}

	if cfg.LevelDBOptions.CompactionTotalSizeMultiplier != 1.5 {
		t.Fatal("TotalSizeMultiplier should be 1.5")
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
