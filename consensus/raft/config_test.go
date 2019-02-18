package raft

import (
	"encoding/json"
	"os"
	"testing"

	hraft "github.com/hashicorp/raft"
)

var cfgJSON = []byte(`
{
    "init_peerset": [],
    "wait_for_leader_timeout": "15s",
    "network_timeout": "1s",
    "commit_retries": 1,
    "commit_retry_delay": "200ms",
    "backups_rotate": 5,
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
	j.HeartbeatTimeout = "1us"
	tst, _ := json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error decoding heartbeat_timeout")
	}

	json.Unmarshal(cfgJSON, j)
	j.LeaderLeaseTimeout = "abc"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err != nil {
		t.Fatal(err)
	}
	def := hraft.DefaultConfig()
	if cfg.RaftConfig.LeaderLeaseTimeout != def.LeaderLeaseTimeout {
		t.Error("expected default leader lease")
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

	cfg.RaftConfig.HeartbeatTimeout = 0
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.RaftConfig = nil
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.CommitRetries = -1
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.WaitForLeaderTimeout = 0
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.BackupsRotate = 0

	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}

func TestApplyEnvVars(t *testing.T) {
	os.Setenv("CLUSTER_RAFT_COMMITRETRIES", "300")
	cfg := &Config{}
	cfg.Default()
	cfg.ApplyEnvVars()

	if cfg.CommitRetries != 300 {
		t.Fatal("failed to override commit_retries with env var")
	}
}
