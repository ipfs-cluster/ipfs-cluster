package ipfscluster

import (
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/informer/disk"
	"github.com/ipfs/ipfs-cluster/ipfsconn/ipfshttp"
	"github.com/ipfs/ipfs-cluster/monitor/basic"
	"github.com/ipfs/ipfs-cluster/pintracker/maptracker"
	"github.com/ipfs/ipfs-cluster/sharder"
)

var testingClusterSecret, _ = DecodeClusterSecret("2588b80d5cb05374fa142aed6cbb047d1f4ef8ef15e37eba68c65b9d30df67ed")

var testingClusterCfg = []byte(`{
    "id": "QmUfSFm12eYCaRdypg48m8RqkXfLW7A2ZeGZb2skeHHDGA",
    "private_key": "CAASqAkwggSkAgEAAoIBAQDpT16IRF6bb9tHsCbQ7M+nb2aI8sz8xyt8PoAWM42ki+SNoESIxKb4UhFxixKvtEdGxNE6aUUVc8kFk6wTStJ/X3IGiMetwkXiFiUxabUF/8A6SyvnSVDm+wFuavugpVrZikjLcfrf2xOVgnG3deQQvd/qbAv14jTwMFl+T+8d/cXBo8Mn/leLZCQun/EJEnkXP5MjgNI8XcWUE4NnH3E0ESSm6Pkm8MhMDZ2fmzNgqEyJ0GVinNgSml3Pyha3PBSj5LRczLip/ie4QkKx5OHvX2L3sNv/JIUHse5HSbjZ1c/4oGCYMVTYCykWiczrxBUOlcr8RwnZLOm4n2bCt5ZhAgMBAAECggEAVkePwfzmr7zR7tTpxeGNeXHtDUAdJm3RWwUSASPXgb5qKyXVsm5nAPX4lXDE3E1i/nzSkzNS5PgIoxNVU10cMxZs6JW0okFx7oYaAwgAddN6lxQtjD7EuGaixN6zZ1k/G6vT98iS6i3uNCAlRZ9HVBmjsOF8GtYolZqLvfZ5izEVFlLVq/BCs7Y5OrDrbGmn3XupfitVWYExV0BrHpobDjsx2fYdTZkmPpSSvXNcm4Iq2AXVQzoqAfGo7+qsuLCZtVlyTfVKQjMvE2ffzN1dQunxixOvev/fz4WSjGnRpC6QLn6Oqps9+VxQKqKuXXqUJC+U45DuvA94Of9MvZfAAQKBgQD7xmXueXRBMr2+0WftybAV024ap0cXFrCAu+KWC1SUddCfkiV7e5w+kRJx6RH1cg4cyyCL8yhHZ99Z5V0Mxa/b/usuHMadXPyX5szVI7dOGgIC9q8IijN7B7GMFAXc8+qC7kivehJzjQghpRRAqvRzjDls4gmbNPhbH1jUiU124QKBgQDtOaW5/fOEtOq0yWbDLkLdjImct6oKMLhENL6yeIKjMYgifzHb2adk7rWG3qcMrdgaFtDVfqv8UmMEkzk7bSkovMVj3SkLzMz84ii1SkSfyaCXgt/UOzDkqAUYB0cXMppYA7jxHa2OY8oEHdBgmyJXdLdzJxCp851AoTlRUSePgQKBgQCQgKgUHOUaXnMEx88sbOuBO14gMg3dNIqM+Ejt8QbURmI8k3arzqA4UK8Tbb9+7b0nzXWanS5q/TT1tWyYXgW28DIuvxlHTA01aaP6WItmagrphIelERzG6f1+9ib/T4czKmvROvDIHROjq8lZ7ERs5Pg4g+sbh2VbdzxWj49EQQKBgFEna36ZVfmMOs7mJ3WWGeHY9ira2hzqVd9fe+1qNKbHhx7mDJR9fTqWPxuIh/Vac5dZPtAKqaOEO8OQ6f9edLou+ggT3LrgsS/B3tNGOPvA6mNqrk/Yf/15TWTO+I8DDLIXc+lokbsogC+wU1z5NWJd13RZZOX/JUi63vTmonYBAoGBAIpglLCH2sPXfmguO6p8QcQcv4RjAU1c0GP4P5PNN3Wzo0ItydVd2LHJb6MdmL6ypeiwNklzPFwTeRlKTPmVxJ+QPg1ct/3tAURN/D40GYw9ojDhqmdSl4HW4d6gHS2lYzSFeU5jkG49y5nirOOoEgHy95wghkh6BfpwHujYJGw4",
    "secret": "2588b80d5cb05374fa142aed6cbb047d1f4ef8ef15e37eba68c65b9d30df67ed",
    "peers": [],
    "bootstrap": [],
    "leave_on_shutdown": true,
    "listen_multiaddress": "/ip4/127.0.0.1/tcp/10000",
    "state_sync_interval": "1m0s",
    "ipfs_sync_interval": "2m10s",
    "replication_factor": -1,
    "monitor_ping_interval": "1s"
}
`)

var testingRaftCfg = []byte(`{
    "data_folder": "raftFolderFromTests",
    "wait_for_leader_timeout": "30s",
    "commit_retries": 1,
    "commit_retry_delay": "1s",
    "network_timeout": "20s",
    "heartbeat_timeout": "1s",
    "election_timeout": "1s",
    "commit_timeout": "50ms",
    "max_append_entries": 64,
    "trailing_logs": 10240,
    "snapshot_interval": "2m0s",
    "snapshot_threshold": 8192,
    "leader_lease_timeout": "500ms"
}`)

var testingAPICfg = []byte(`{
    "listen_multiaddress": "/ip4/127.0.0.1/tcp/10002",
    "read_timeout": "30s",
    "read_header_timeout": "5s",
    "write_timeout": "1m0s",
    "idle_timeout": "2m0s"
}`)

var testingIpfsCfg = []byte(`{
    "proxy_listen_multiaddress": "/ip4/127.0.0.1/tcp/10001",
    "node_multiaddress": "/ip4/127.0.0.1/tcp/5001",
    "connect_swarms_delay": "7s",
    "proxy_read_timeout": "10m0s",
    "proxy_read_header_timeout": "10m0s",
    "proxy_write_timeout": "10m0s",
    "proxy_idle_timeout": "1m0s"
}`)

var testingTrackerCfg = []byte(`
{
      "pinning_timeout": "30s",
      "unpinning_timeout": "15s",
      "max_pin_queue_size": 4092
}
`)

var testingMonCfg = []byte(`{
    "check_interval": "1s"
}`)

var testingDiskInfCfg = []byte(`{
    "metric_ttl": "1s",
    "metric_type": "freespace"
}`)

var testingSharderCfg = []byte(`{
    "alloc_size": 5000000
}`)

func testingConfigs() (*Config, *rest.Config, *ipfshttp.Config, *raft.Config, *maptracker.Config, *basic.Config, *disk.Config, *sharder.Config) {
	clusterCfg, apiCfg, ipfsCfg, consensusCfg, trackerCfg, monCfg, diskInfCfg, sharderCfg := testingEmptyConfigs()
	clusterCfg.LoadJSON(testingClusterCfg)
	apiCfg.LoadJSON(testingAPICfg)
	ipfsCfg.LoadJSON(testingIpfsCfg)
	consensusCfg.LoadJSON(testingRaftCfg)
	trackerCfg.LoadJSON(testingTrackerCfg)
	monCfg.LoadJSON(testingMonCfg)
	diskInfCfg.LoadJSON(testingDiskInfCfg)
	sharderCfg.LoadJSON(testingSharderCfg)

	return clusterCfg, apiCfg, ipfsCfg, consensusCfg, trackerCfg, monCfg, diskInfCfg, sharderCfg
}

func testingEmptyConfigs() (*Config, *rest.Config, *ipfshttp.Config, *raft.Config, *maptracker.Config, *basic.Config, *disk.Config, *sharder.Config) {
	clusterCfg := &Config{}
	apiCfg := &rest.Config{}
	ipfshttpCfg := &ipfshttp.Config{}
	consensusCfg := &raft.Config{}
	trackerCfg := &maptracker.Config{}
	monCfg := &basic.Config{}
	diskInfCfg := &disk.Config{}
	sharderCfg := &sharder.Config{}
	return clusterCfg, apiCfg, ipfshttpCfg, consensusCfg, trackerCfg, monCfg, diskInfCfg, sharderCfg
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
