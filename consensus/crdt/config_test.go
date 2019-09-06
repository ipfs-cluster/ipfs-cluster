package crdt

import (
	"os"
	"testing"
)

var cfgJSON = []byte(`
{
    "cluster_name": "test",
    "trusted_peers": ["QmUZ13osndQ5uL4tPWHXe3iBgBgq9gfewcBMSCAuMBsDJ6"]
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
}

func TestApplyEnvVars(t *testing.T) {
	os.Setenv("CLUSTER_CRDT_CLUSTERNAME", "test2")
	cfg := &Config{}
	cfg.Default()
	cfg.ApplyEnvVars()

	if cfg.ClusterName != "test2" {
		t.Fatal("failed to override cluster_name with env var")
	}
}
