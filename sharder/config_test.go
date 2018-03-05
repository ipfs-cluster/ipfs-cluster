package sharder

import (
	"encoding/json"
	"testing"
)

var cfgJSON = []byte(`
{
      "alloc_size": 6000000
}
`)

func TestLoadJSON(t *testing.T) {
	cfg := &Config{}
	err := cfg.LoadJSON(cfgJSON)
	if err != nil {
		t.Fatal(err)
	}

	j := &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.AllocSize = 0
	tst, _ := json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err != nil {
		t.Error("did not expect an error")
	}
	if cfg.AllocSize != DefaultAllocSize {
		t.Error("expected default alloc_size")
	}
}

func TestToJSON(t *testing.T) {
	cfg := &Config{}
	cfg.LoadJSON(cfgJSON)
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

	cfg.AllocSize = 100
	if cfg.Validate() == nil {
		t.Fatal("expecting error validating")
	}
}
