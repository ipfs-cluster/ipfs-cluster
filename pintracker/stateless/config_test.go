package stateless

import (
	"encoding/json"
	"os"
	"testing"
)

var cfgJSON = []byte(`
{
	"max_pin_queue_size": 4092,
	"concurrent_pins": 2
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
	j.ConcurrentPins = 10
	tst, _ := json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err != nil {
		t.Error("did not expect an error")
	}
	if cfg.ConcurrentPins != 10 {
		t.Error("expected 10 concurrent pins")
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

	cfg.ConcurrentPins = -2
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}

func TestApplyEnvVars(t *testing.T) {
	os.Setenv("CLUSTER_STATELESS_CONCURRENTPINS", "22")
	cfg := &Config{}
	cfg.ApplyEnvVars()

	if cfg.ConcurrentPins != 22 {
		t.Fatal("failed to override concurrent_pins with env var")
	}
}
