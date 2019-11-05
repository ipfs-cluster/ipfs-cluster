package ipfscluster

import (
	"github.com/ipfs/ipfs-cluster/api/ipfsproxy"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/config"
	"github.com/ipfs/ipfs-cluster/consensus/crdt"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/datastore/badger"
	"github.com/ipfs/ipfs-cluster/informer/disk"
	"github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs/ipfs-cluster/monitor/pubsubmon"
	"github.com/ipfs/ipfs-cluster/observations"
	"github.com/ipfs/ipfs-cluster/pintracker/stateless"
)

var testingClusterSecret, _ = DecodeClusterSecret("2588b80d5cb05374fa142aed6cbb047d1f4ef8ef15e37eba68c65b9d30df67ed")

var testingIdentity = []byte(`{
  "id": "12D3KooWQiK1sYbGNnD9XtWF1sP95cawwwNy3d2WUwtP71McwUfZ",
  "private_key": "CAESQJZ0wHQyoWGizG7eSATrDtTVlyyr99O8726jIu1lf2D+3VJBBAu6HXPRkbdNINBWlPMn+PK3bO6EgGGuaou8bKg="
}`)

var testingClusterCfg = []byte(`{
    "secret": "2588b80d5cb05374fa142aed6cbb047d1f4ef8ef15e37eba68c65b9d30df67ed",
    "leave_on_shutdown": false,
    "listen_multiaddress": "/ip4/127.0.0.1/tcp/10000",
    "connection_manager": {
         "high_water": 400,
         "low_water": 200,
         "grace_period": "2m0s"
    },
    "state_sync_interval": "1m0s",
    "replication_factor": -1,
    "monitor_ping_interval": "350ms",
    "peer_watch_interval": "200ms",
    "disable_repinning": false,
    "mdns_interval": "0s"
}`)

var testingRaftCfg = []byte(`{
    "data_folder": "raftFolderFromTests",
    "wait_for_leader_timeout": "10s",
    "commit_retries": 2,
    "commit_retry_delay": "50ms",
    "backups_rotate": 2,
    "network_timeout": "5s",
    "heartbeat_timeout": "200ms",
    "election_timeout": "200ms",
    "commit_timeout": "150ms",
    "max_append_entries": 256,
    "trailing_logs": 10240,
    "snapshot_interval": "2m0s",
    "snapshot_threshold": 8192,
    "leader_lease_timeout": "200ms"
}`)

var testingCrdtCfg = []byte(`{
    "cluster_name": "crdt-test",
    "trusted_peers": ["*"],
    "rebroadcast_interval": "150ms"
}`)

var testingBadgerCfg = []byte(`{
    "folder": "badgerFromTests",
    "badger_options": {
        "max_table_size": 1048576
    }
}`)

var testingAPICfg = []byte(`{
    "http_listen_multiaddress": "/ip4/127.0.0.1/tcp/10002",
    "read_timeout": "0",
    "read_header_timeout": "5s",
    "write_timeout": "0",
    "idle_timeout": "2m0s",
    "headers": {
      "Access-Control-Allow-Headers": [
        "X-Requested-With",
        "Range"
      ],
      "Access-Control-Allow-Methods": [
        "GET"
      ],
      "Access-Control-Allow-Origin": [
        "*"
      ]
    }
}`)

var testingProxyCfg = []byte(`{
    "listen_multiaddress": "/ip4/127.0.0.1/tcp/10001",
    "node_multiaddress": "/ip4/127.0.0.1/tcp/5001",
    "read_timeout": "0",
    "read_header_timeout": "10m0s",
    "write_timeout": "0",
    "idle_timeout": "1m0s"
}`)

var testingIpfsCfg = []byte(`{
    "node_multiaddress": "/ip4/127.0.0.1/tcp/5001",
    "connect_swarms_delay": "7s",
    "pin_timeout": "30s",
    "unpin_timeout": "15s"
}`)

var testingTrackerCfg = []byte(`
{
    "max_pin_queue_size": 4092,
    "concurrent_pins": 1
}
`)

var testingMonCfg = []byte(`{
    "check_interval": "550ms",
    "failure_threshold": 6
}`)

var testingDiskInfCfg = []byte(`{
    "metric_ttl": "350ms",
    "metric_type": "freespace"
}`)

var testingTracerCfg = []byte(`{
    "enable_tracing": false,
    "jaeger_agent_endpoint": "/ip4/0.0.0.0/udp/6831",
    "sampling_prob": 1,
    "service_name": "cluster-daemon"
}`)

func testingConfigs() (*config.Identity, *Config, *rest.Config, *ipfsproxy.Config, *ipfshttp.Config, *badger.Config, *raft.Config, *crdt.Config, *stateless.Config, *pubsubmon.Config, *disk.Config, *observations.TracingConfig) {
	identity, clusterCfg, apiCfg, proxyCfg, ipfsCfg, badgerCfg, raftCfg, crdtCfg, statelesstrkrCfg, pubsubmonCfg, diskInfCfg, tracingCfg := testingEmptyConfigs()
	identity.LoadJSON(testingIdentity)
	clusterCfg.LoadJSON(testingClusterCfg)
	apiCfg.LoadJSON(testingAPICfg)
	proxyCfg.LoadJSON(testingProxyCfg)
	ipfsCfg.LoadJSON(testingIpfsCfg)
	badgerCfg.LoadJSON(testingBadgerCfg)
	raftCfg.LoadJSON(testingRaftCfg)
	crdtCfg.LoadJSON(testingCrdtCfg)
	statelesstrkrCfg.LoadJSON(testingTrackerCfg)
	pubsubmonCfg.LoadJSON(testingMonCfg)
	diskInfCfg.LoadJSON(testingDiskInfCfg)
	tracingCfg.LoadJSON(testingTracerCfg)

	return identity, clusterCfg, apiCfg, proxyCfg, ipfsCfg, badgerCfg, raftCfg, crdtCfg, statelesstrkrCfg, pubsubmonCfg, diskInfCfg, tracingCfg
}

func testingEmptyConfigs() (*config.Identity, *Config, *rest.Config, *ipfsproxy.Config, *ipfshttp.Config, *badger.Config, *raft.Config, *crdt.Config, *stateless.Config, *pubsubmon.Config, *disk.Config, *observations.TracingConfig) {
	identity := &config.Identity{}
	clusterCfg := &Config{}
	apiCfg := &rest.Config{}
	proxyCfg := &ipfsproxy.Config{}
	ipfshttpCfg := &ipfshttp.Config{}
	badgerCfg := &badger.Config{}
	raftCfg := &raft.Config{}
	crdtCfg := &crdt.Config{}
	statelessCfg := &stateless.Config{}
	pubsubmonCfg := &pubsubmon.Config{}
	diskInfCfg := &disk.Config{}
	tracingCfg := &observations.TracingConfig{}
	return identity, clusterCfg, apiCfg, proxyCfg, ipfshttpCfg, badgerCfg, raftCfg, crdtCfg, statelessCfg, pubsubmonCfg, diskInfCfg, tracingCfg
}

// func TestConfigDefault(t *testing.T) {
// 	cfg := testingEmptyConfig()
// 	cfg.Default()
// 	err := cfg.Validate()
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// }

// func TestConfigToJSON(t *testing.T) {
// 	cfg := testingConfig()
// 	_, err := cfg.ToJSON()
// 	if err != nil {
// 		t.Error(err)
// 	}
// }

// func TestConfigToConfig(t *testing.T) {
// 	cfg := testingConfig()
// 	j, _ := cfg.ToJSON()
// 	cfg2 := testingEmptyConfig()
// 	err := cfg2.LoadJSON(j)
// 	if err != nil {
// 		t.Error(err)
// 	}
// }
