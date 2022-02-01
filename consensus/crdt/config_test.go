package crdt

import (
	"os"
	"testing"
	"time"
)

var cfgJSON = []byte(`
{
    "cluster_name": "test",
    "trusted_peers": ["QmUZ13osndQ5uL4tPWHXe3iBgBgq9gfewcBMSCAuMBsDJ6"],
    "batching": {
        "max_batch_size": 30,
        "max_batch_age": "5s",
        "max_queue_size": 150
    },
    "repair_interval": "1m"
}
`)

func TestLoadJSON(t *testing.T) {
	cfg := &Config{}
	err := cfg.LoadJSON(cfgJSON)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TrustAll {
		t.Error("TrustAll should not be enabled when peers in trusted peers")
	}

	if cfg.Batching.MaxBatchSize != 30 ||
		cfg.Batching.MaxBatchAge != 5*time.Second ||
		cfg.Batching.MaxQueueSize != 150 {
		t.Error("Batching options were not parsed correctly")
	}
	if cfg.RepairInterval != time.Minute {
		t.Error("repair interval not set")
	}

	cfg = &Config{}
	err = cfg.LoadJSON([]byte(`
{
    "cluster_name": "test",
    "trusted_peers": ["abc"]
}`))

	if err == nil {
		t.Fatal("expected error parsing trusted_peers")
	}

	cfg = &Config{}
	err = cfg.LoadJSON([]byte(`
{
    "cluster_name": "test",
    "trusted_peers": []
}`))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.TrustAll {
		t.Error("TrustAll is only enabled with '*'")
	}

	cfg = &Config{}
	err = cfg.LoadJSON([]byte(`
{
    "cluster_name": "test",
    "trusted_peers": ["QmUZ13osndQ5uL4tPWHXe3iBgBgq9gfewcBMSCAuMBsDJ6", "*"]
}`))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.TrustAll {
		t.Error("expected TrustAll to be true")
	}

	if cfg.Batching.MaxQueueSize != DefaultBatchingMaxQueueSize {
		t.Error("MaxQueueSize should be default when unset")
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

	cfg.ClusterName = ""
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.PeersetMetric = ""
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.RebroadcastInterval = 0
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.Batching.MaxQueueSize = -3
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.RepairInterval = -3
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}

func TestApplyEnvVars(t *testing.T) {
	os.Setenv("CLUSTER_CRDT_CLUSTERNAME", "test2")
	os.Setenv("CLUSTER_CRDT_BATCHING_MAXBATCHSIZE", "5")
	os.Setenv("CLUSTER_CRDT_BATCHING_MAXBATCHAGE", "10s")

	cfg := &Config{}
	cfg.Default()
	cfg.ApplyEnvVars()

	if cfg.ClusterName != "test2" {
		t.Error("failed to override cluster_name with env var")
	}

	if cfg.Batching.MaxBatchSize != 5 {
		t.Error("MaxBatchSize as env var does not work")
	}

	if cfg.Batching.MaxBatchAge != 10*time.Second {
		t.Error("MaxBatchAge as env var does not work")
	}
}
