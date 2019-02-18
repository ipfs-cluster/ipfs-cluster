package disk

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

var cfgJSON = []byte(`
{
    "metric_ttl": "1s",
    "metric_type": "freespace"  
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
	j.MetricTTL = "-10"
	tst, _ := json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error decoding metric_ttl")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.Type = "abc"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error decoding check_interval")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.Type = "reposize"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err != nil {
		t.Error("reposize should be a valid type")
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

	cfg.MetricTTL = 0
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.Type = MetricRepoSize
	if cfg.Validate() != nil {
		t.Fatal("MetricRepoSize is a valid type")
	}
}

func TestApplyEnvVars(t *testing.T) {
	os.Setenv("CLUSTER_DISK_METRICTTL", "22s")
	cfg := &Config{}
	cfg.ApplyEnvVars()

	if cfg.MetricTTL != 22*time.Second {
		t.Fatal("failed to override metric_ttl with env var")
	}
}
