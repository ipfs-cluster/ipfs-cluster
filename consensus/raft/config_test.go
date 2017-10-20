package raft

import (
	"encoding/json"
	"testing"
)

var cfgJSON = []byte(`
{
    "heartbeat_timeout": "1s",
    "election_timeout": "1s",
    "commit_timeout": "50ms",
    "max_append_entries": 64,
    "trailing_logs": 10240,
    "snapshot_interval": "2m0s",
    "snapshot_threshold": 8192,
    "leader_lease_timeout": "500ms"
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
	j.HeartbeatTimeout = "-1"
	tst, _ := json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error decoding heartbeat_timeout")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.ElectionTimeout = "abc"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error in timeout")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.CommitTimeout = "abc"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error in timeout")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.LeaderLeaseTimeout = "abc"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error in timeout")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.SnapshotInterval = "abc"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error in snapshot_interval")
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

	cfg.HashiraftCfg.HeartbeatTimeout = 0
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.HashiraftCfg = nil
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}
