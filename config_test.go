package ipfscluster

import (
	"github.com/ipfs/ipfs-cluster/api/ipfsproxy"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/informer/disk"
	"github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs/ipfs-cluster/monitor/basic"
	"github.com/ipfs/ipfs-cluster/monitor/pubsubmon"
	"github.com/ipfs/ipfs-cluster/pintracker/maptracker"
	"github.com/ipfs/ipfs-cluster/pintracker/stateless"
)

var testingClusterSecret, _ = DecodeClusterSecret("2588b80d5cb05374fa142aed6cbb047d1f4ef8ef15e37eba68c65b9d30df67ed")

var testingClusterCfg = []byte(`{
    "id": "QmUfSFm12eYCaRdypg48m8RqkXfLW7A2ZeGZb2skeHHDGA",
    "private_key": "CAASqAkwggSkAgEAAoIBAQDpT16IRF6bb9tHsCbQ7M+nb2aI8sz8xyt8PoAWM42ki+SNoESIxKb4UhFxixKvtEdGxNE6aUUVc8kFk6wTStJ/X3IGiMetwkXiFiUxabUF/8A6SyvnSVDm+wFuavugpVrZikjLcfrf2xOVgnG3deQQvd/qbAv14jTwMFl+T+8d/cXBo8Mn/leLZCQun/EJEnkXP5MjgNI8XcWUE4NnH3E0ESSm6Pkm8MhMDZ2fmzNgqEyJ0GVinNgSml3Pyha3PBSj5LRczLip/ie4QkKx5OHvX2L3sNv/JIUHse5HSbjZ1c/4oGCYMVTYCykWiczrxBUOlcr8RwnZLOm4n2bCt5ZhAgMBAAECggEAVkePwfzmr7zR7tTpxeGNeXHtDUAdJm3RWwUSASPXgb5qKyXVsm5nAPX4lXDE3E1i/nzSkzNS5PgIoxNVU10cMxZs6JW0okFx7oYaAwgAddN6lxQtjD7EuGaixN6zZ1k/G6vT98iS6i3uNCAlRZ9HVBmjsOF8GtYolZqLvfZ5izEVFlLVq/BCs7Y5OrDrbGmn3XupfitVWYExV0BrHpobDjsx2fYdTZkmPpSSvXNcm4Iq2AXVQzoqAfGo7+qsuLCZtVlyTfVKQjMvE2ffzN1dQunxixOvev/fz4WSjGnRpC6QLn6Oqps9+VxQKqKuXXqUJC+U45DuvA94Of9MvZfAAQKBgQD7xmXueXRBMr2+0WftybAV024ap0cXFrCAu+KWC1SUddCfkiV7e5w+kRJx6RH1cg4cyyCL8yhHZ99Z5V0Mxa/b/usuHMadXPyX5szVI7dOGgIC9q8IijN7B7GMFAXc8+qC7kivehJzjQghpRRAqvRzjDls4gmbNPhbH1jUiU124QKBgQDtOaW5/fOEtOq0yWbDLkLdjImct6oKMLhENL6yeIKjMYgifzHb2adk7rWG3qcMrdgaFtDVfqv8UmMEkzk7bSkovMVj3SkLzMz84ii1SkSfyaCXgt/UOzDkqAUYB0cXMppYA7jxHa2OY8oEHdBgmyJXdLdzJxCp851AoTlRUSePgQKBgQCQgKgUHOUaXnMEx88sbOuBO14gMg3dNIqM+Ejt8QbURmI8k3arzqA4UK8Tbb9+7b0nzXWanS5q/TT1tWyYXgW28DIuvxlHTA01aaP6WItmagrphIelERzG6f1+9ib/T4czKmvROvDIHROjq8lZ7ERs5Pg4g+sbh2VbdzxWj49EQQKBgFEna36ZVfmMOs7mJ3WWGeHY9ira2hzqVd9fe+1qNKbHhx7mDJR9fTqWPxuIh/Vac5dZPtAKqaOEO8OQ6f9edLou+ggT3LrgsS/B3tNGOPvA6mNqrk/Yf/15TWTO+I8DDLIXc+lokbsogC+wU1z5NWJd13RZZOX/JUi63vTmonYBAoGBAIpglLCH2sPXfmguO6p8QcQcv4RjAU1c0GP4P5PNN3Wzo0ItydVd2LHJb6MdmL6ypeiwNklzPFwTeRlKTPmVxJ+QPg1ct/3tAURN/D40GYw9ojDhqmdSl4HW4d6gHS2lYzSFeU5jkG49y5nirOOoEgHy95wghkh6BfpwHujYJGw4",
    "secret": "2588b80d5cb05374fa142aed6cbb047d1f4ef8ef15e37eba68c65b9d30df67ed",
    "leave_on_shutdown": false,
    "listen_multiaddress": "/ip4/127.0.0.1/tcp/10000",
    "state_sync_interval": "1m0s",
    "ipfs_sync_interval": "2m10s",
    "replication_factor": -1,
    "monitor_ping_interval": "150ms",
    "peer_watch_interval": "100ms",
    "disable_repinning": false
}
`)

var testingRaftCfg = []byte(`{
    "data_folder": "raftFolderFromTests",
    "wait_for_leader_timeout": "10s",
    "commit_retries": 2,
    "commit_retry_delay": "50ms",
    "backups_rotate": 2,
    "network_timeout": "5s",
    "heartbeat_timeout": "100ms",
    "election_timeout": "100ms",
    "commit_timeout": "50ms",
    "max_append_entries": 256,
    "trailing_logs": 10240,
    "snapshot_interval": "2m0s",
    "snapshot_threshold": 8192,
    "leader_lease_timeout": "80ms"
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
    "pin_method": "pin",
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
    "check_interval": "300ms"
}`)

var testingDiskInfCfg = []byte(`{
    "metric_ttl": "150ms",
    "metric_type": "freespace"
}`)

func testingConfigs() (*Config, *rest.Config, *ipfsproxy.Config, *ipfshttp.Config, *raft.Config, *maptracker.Config, *stateless.Config, *basic.Config, *pubsubmon.Config, *disk.Config) {
	clusterCfg, apiCfg, proxyCfg, ipfsCfg, consensusCfg, maptrackerCfg, statelesstrkrCfg, basicmonCfg, pubsubmonCfg, diskInfCfg := testingEmptyConfigs()
	clusterCfg.LoadJSON(testingClusterCfg)
	apiCfg.LoadJSON(testingAPICfg)
	proxyCfg.LoadJSON(testingProxyCfg)
	ipfsCfg.LoadJSON(testingIpfsCfg)
	consensusCfg.LoadJSON(testingRaftCfg)
	maptrackerCfg.LoadJSON(testingTrackerCfg)
	statelesstrkrCfg.LoadJSON(testingTrackerCfg)
	basicmonCfg.LoadJSON(testingMonCfg)
	pubsubmonCfg.LoadJSON(testingMonCfg)
	diskInfCfg.LoadJSON(testingDiskInfCfg)

	return clusterCfg, apiCfg, proxyCfg, ipfsCfg, consensusCfg, maptrackerCfg, statelesstrkrCfg, basicmonCfg, pubsubmonCfg, diskInfCfg
}

func testingEmptyConfigs() (*Config, *rest.Config, *ipfsproxy.Config, *ipfshttp.Config, *raft.Config, *maptracker.Config, *stateless.Config, *basic.Config, *pubsubmon.Config, *disk.Config) {
	clusterCfg := &Config{}
	apiCfg := &rest.Config{}
	proxyCfg := &ipfsproxy.Config{}
	ipfshttpCfg := &ipfshttp.Config{}
	consensusCfg := &raft.Config{}
	maptrackerCfg := &maptracker.Config{}
	statelessCfg := &stateless.Config{}
	basicmonCfg := &basic.Config{}
	pubsubmonCfg := &pubsubmon.Config{}
	diskInfCfg := &disk.Config{}
	return clusterCfg, apiCfg, proxyCfg, ipfshttpCfg, consensusCfg, maptrackerCfg, statelessCfg, basicmonCfg, pubsubmonCfg, diskInfCfg
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
