package stateless

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

var cfgJSON = []byte(`
{
	"max_pin_queue_size": 4092,
	"concurrent_pins": 2,
	"priority_pin_max_age": "240h",
	"priority_pin_max_retries": 4
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
	j.PriorityPinMaxAge = "216h"
	j.PriorityPinMaxRetries = 2
	tst, _ := json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err != nil {
		t.Error("did not expect an error")
	}
	if cfg.ConcurrentPins != 10 {
		t.Error("expected 10 concurrent pins")
	}
	if cfg.PriorityPinMaxAge != 9*24*time.Hour {
		t.Error("expected 9 days max age")
	}
	if cfg.PriorityPinMaxRetries != 2 {
		t.Error("expected 2 max retries")
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
	cfg.ConcurrentPins = 3
	cfg.PriorityPinMaxRetries = -1
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}

func TestApplyEnvVars(t *testing.T) {
	os.Setenv("CLUSTER_STATELESS_CONCURRENTPINS", "22")
	os.Setenv("CLUSTER_STATELESS_PRIORITYPINMAXAGE", "72h")
	cfg := &Config{}
	cfg.ApplyEnvVars()

	if cfg.ConcurrentPins != 22 {
		t.Fatal("failed to override concurrent_pins with env var")
	}

	if cfg.PriorityPinMaxAge != 3*24*time.Hour {
		t.Fatal("failed to override priority_pin_max_age with env var")
	}
}
