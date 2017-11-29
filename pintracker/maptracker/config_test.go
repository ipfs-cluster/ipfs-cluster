package maptracker

import (
	"encoding/json"
	"testing"
)

var cfgJSON = []byte(`
{
      "pinning_timeout": "30s",
      "unpinning_timeout": "15s",
      "max_pin_queue_size": 4092
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
	j.PinningTimeout = "-10"
	tst, _ := json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err != nil {
		t.Error("did not expect an error")
	}
	if cfg.PinningTimeout != DefaultPinningTimeout {
		t.Error("expected default pinning_timeout")
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

	cfg.UnpinningTimeout = 0
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}
