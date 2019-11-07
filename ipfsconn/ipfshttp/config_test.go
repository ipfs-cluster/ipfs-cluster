package ipfshttp

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

var cfgJSON = []byte(`
{
	"node_multiaddress": "/ip4/127.0.0.1/tcp/5001",
	"connect_swarms_delay": "7s",
	"ipfs_request_timeout": "5m0s",
	"pin_timeout": "24h",
	"unpin_timeout": "3h",
	"repogc_timeout": "24h"
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
	j.NodeMultiaddress = "abc"
	tst, _ := json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error in node_multiaddress")
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

	cfg.NodeAddr = nil
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}

func TestApplyEnvVar(t *testing.T) {
	os.Setenv("CLUSTER_IPFSHTTP_PINTIMEOUT", "22m")
	cfg := &Config{}
	cfg.Default()
	cfg.ApplyEnvVars()

	if cfg.PinTimeout != 22*time.Minute {
		t.Fatal("failed to override pin_timeout with env var")
	}
}
