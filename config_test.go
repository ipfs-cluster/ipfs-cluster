package ipfscluster

import (
	"github.com/ipfs-cluster/ipfs-cluster/allocator/balanced"
	"github.com/ipfs-cluster/ipfs-cluster/api/ipfsproxy"
	"github.com/ipfs-cluster/ipfs-cluster/api/rest"
	"github.com/ipfs-cluster/ipfs-cluster/config"
	"github.com/ipfs-cluster/ipfs-cluster/consensus/crdt"
	"github.com/ipfs-cluster/ipfs-cluster/consensus/raft"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/badger"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/badger3"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/leveldb"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/pebble"
	"github.com/ipfs-cluster/ipfs-cluster/informer/disk"
	"github.com/ipfs-cluster/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs-cluster/ipfs-cluster/monitor/pubsubmon"
	"github.com/ipfs-cluster/ipfs-cluster/observations"
	"github.com/ipfs-cluster/ipfs-cluster/pintracker/stateless"
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
    "pin_recover_interval": "1m0s",
    "replication_factor": -1,
    "monitor_ping_interval": "250ms",
    "peer_watch_interval": "1s",
    "pin_only_on_trusted_peers": true,
    "disable_repinning": false,
    "mdns_interval": "0s"
}`)

var testingRaftCfg = []byte(`{
    "data_folder": "raftFolderFromTests",
    "wait_for_leader_timeout": "5s",
    "commit_retries": 2,
    "commit_retry_delay": "50ms",
    "backups_rotate": 2,
    "network_timeout": "5s",
    "heartbeat_timeout": "700ms",
    "election_timeout": "1s",
    "commit_timeout": "250ms",
    "max_append_entries": 256,
    "trailing_logs": 10240,
    "snapshot_interval": "2m0s",
    "snapshot_threshold": 8192,
    "leader_lease_timeout": "500ms"
}`)

var testingCrdtCfg = []byte(`{
    "cluster_name": "crdt-test",
    "trusted_peers": ["*"],
    "rebroadcast_interval": "250ms"
}`)

var testingBadgerCfg = []byte(`{
    "folder": "badgerFromTests",
    "gc_interval": "0m",
    "gc_sleep": "0m",
    "badger_options": {
        "max_table_size": 1048576
    }
}`)

var testingBadger3Cfg = []byte(`
{
      "gc_discard_ratio": 0.2,
      "gc_interval": "0s",
      "gc_sleep": "0s",
      "badger_options": {
        "dir": "",
        "value_dir": "",
        "sync_writes": false,
        "num_versions_to_keep": 1,
        "read_only": false,
        "compression": 0,
        "in_memory": false,
        "metrics_enabled": true,
        "num_goroutines": 8,
        "mem_table_size": 1048576,
        "base_table_size": 2097152,
        "base_level_size": 10485760,
        "level_size_multiplier": 10,
        "table_size_multiplier": 2,
        "max_levels": 7,
        "v_log_percentile": 0,
        "value_threshold": 100,
        "num_memtables": 5,
        "block_size": 4096,
        "bloom_false_positive": 0.01,
        "block_cache_size": 0,
        "index_cache_size": 0,
        "num_level_zero_tables": 5,
        "num_level_zero_tables_stall": 15,
        "value_log_file_size": 1073741823,
        "value_log_max_entries": 1000000,
        "num_compactors": 4,
        "compact_l_0_on_close": false,
        "lmax_compaction": false,
        "zstd_compression_level": 1,
        "verify_value_checksum": false,
        "checksum_verification_mode": 0,
        "detect_conflicts": false,
        "namespace_offset": -1
      }
    }
`)

var testingLevelDBCfg = []byte(`{
    "folder": "leveldbFromTests",
    "leveldb_options": {
    }
}`)

var testingPebbleCfg = []byte(`
{
      "pebble_options": {
        "bytes_per_sync": 524288,
        "disable_wal": false,
        "flush_delay_delete_range": 0,
        "flush_delay_range_key": 0,
        "flush_split_bytes": 4194304,
        "format_major_version": 1,
        "l0_compaction_file_threshold": 500,
        "l0_compaction_threshold": 4,
        "l0_stop_writes_threshold": 12,
        "l_base_max_bytes": 67108864,
        "levels": [
          {
            "block_restart_interval": 16,
            "block_size": 4096,
            "block_size_threshold": 90,
            "Compression": 1,
            "filter_type": 0,
            "index_block_size": 8000,
            "target_file_size": 2097152
          }
        ],
        "max_open_files": 1000,
        "mem_table_size": 1048576,
        "mem_table_stop_writes_threshold": 2,
        "read_only": false,
        "wal_bytes_per_sync": 0
      }
    }
`)

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
    "unpin_timeout": "15s",
    "informer_trigger_interval": 10
}`)

var testingTrackerCfg = []byte(`
{
    "max_pin_queue_size": 4092,
    "concurrent_pins": 1
}
`)

var testingMonCfg = []byte(`{
    "check_interval": "800ms",
    "failure_threshold": 6
}`)

var testingAllocBalancedCfg = []byte(`{
    "allocate_by": ["freespace"]
}`)

var testingDiskInfCfg = []byte(`{
    "metric_ttl": "900ms"
}`)

var testingTracerCfg = []byte(`{
    "enable_tracing": false,
    "jaeger_agent_endpoint": "/ip4/0.0.0.0/udp/6831",
    "sampling_prob": 1,
    "service_name": "cluster-daemon"
}`)

func testingConfigs() (*config.Identity, *Config, *rest.Config, *ipfsproxy.Config, *ipfshttp.Config, *badger.Config, *badger3.Config, *leveldb.Config, *pebble.Config, *raft.Config, *crdt.Config, *stateless.Config, *pubsubmon.Config, *balanced.Config, *disk.Config, *observations.TracingConfig) {
	identity, clusterCfg, apiCfg, proxyCfg, ipfsCfg, badgerCfg, badger3Cfg, levelDBCfg, pebbleCfg, raftCfg, crdtCfg, statelesstrkrCfg, pubsubmonCfg, allocBalancedCfg, diskInfCfg, tracingCfg := testingEmptyConfigs()
	identity.LoadJSON(testingIdentity)
	clusterCfg.LoadJSON(testingClusterCfg)
	apiCfg.LoadJSON(testingAPICfg)
	proxyCfg.LoadJSON(testingProxyCfg)
	ipfsCfg.LoadJSON(testingIpfsCfg)
	badgerCfg.LoadJSON(testingBadgerCfg)
	badger3Cfg.LoadJSON(testingBadger3Cfg)
	levelDBCfg.LoadJSON(testingLevelDBCfg)
	pebbleCfg.LoadJSON(testingPebbleCfg)
	raftCfg.LoadJSON(testingRaftCfg)
	crdtCfg.LoadJSON(testingCrdtCfg)
	statelesstrkrCfg.LoadJSON(testingTrackerCfg)
	pubsubmonCfg.LoadJSON(testingMonCfg)
	allocBalancedCfg.LoadJSON(testingAllocBalancedCfg)
	diskInfCfg.LoadJSON(testingDiskInfCfg)
	tracingCfg.LoadJSON(testingTracerCfg)

	return identity, clusterCfg, apiCfg, proxyCfg, ipfsCfg, badgerCfg, badger3Cfg, levelDBCfg, pebbleCfg, raftCfg, crdtCfg, statelesstrkrCfg, pubsubmonCfg, allocBalancedCfg, diskInfCfg, tracingCfg
}

func testingEmptyConfigs() (*config.Identity, *Config, *rest.Config, *ipfsproxy.Config, *ipfshttp.Config, *badger.Config, *badger3.Config, *leveldb.Config, *pebble.Config, *raft.Config, *crdt.Config, *stateless.Config, *pubsubmon.Config, *balanced.Config, *disk.Config, *observations.TracingConfig) {
	identity := &config.Identity{}
	clusterCfg := &Config{}
	apiCfg := rest.NewConfig()
	proxyCfg := &ipfsproxy.Config{}
	ipfshttpCfg := &ipfshttp.Config{}
	badgerCfg := &badger.Config{}
	badger3Cfg := &badger3.Config{}
	levelDBCfg := &leveldb.Config{}
	pebbleCfg := &pebble.Config{}
	raftCfg := &raft.Config{}
	crdtCfg := &crdt.Config{}
	statelessCfg := &stateless.Config{}
	pubsubmonCfg := &pubsubmon.Config{}
	allocBalancedCfg := &balanced.Config{}
	diskInfCfg := &disk.Config{}
	tracingCfg := &observations.TracingConfig{}
	return identity, clusterCfg, apiCfg, proxyCfg, ipfshttpCfg, badgerCfg, badger3Cfg, levelDBCfg, pebbleCfg, raftCfg, crdtCfg, statelessCfg, pubsubmonCfg, allocBalancedCfg, diskInfCfg, tracingCfg
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
