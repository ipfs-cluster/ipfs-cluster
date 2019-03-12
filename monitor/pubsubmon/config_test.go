package pubsubmon

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

var cfgJSON = []byte(`
{
      "check_interval": "15s",
	  "failure_threshold": 3.0
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
	j.CheckInterval = "-10"
	tst, _ := json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error decoding check_interval")
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

	cfg.CheckInterval = 0
	cfg.FailureThreshold = -0.1
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}

func TestApplyEnvVars(t *testing.T) {
	os.Setenv("CLUSTER_PUBSUBMON_CHECKINTERVAL", "22s")
	os.Setenv("CLUSTER_PUBSUBMON_FAILURETHRESHOLD", "4.0")
	cfg := &Config{}
	cfg.ApplyEnvVars()

	if cfg.CheckInterval != 22*time.Second {
		t.Fatal("failed to override check_interval with env var")
	}
	if cfg.FailureThreshold != 4.0 {
		t.Fatal("failed to override failure_threshold with env var")
	}
}
