package ipfscluster

import "testing"

func testingConfig() *Config {
	jcfg := &JSONConfig{
		ID:         "QmUfSFm12eYCaRdypg48m8RqkXfLW7A2ZeGZb2skeHHDGA",
		PrivateKey: "CAASqAkwggSkAgEAAoIBAQDpT16IRF6bb9tHsCbQ7M+nb2aI8sz8xyt8PoAWM42ki+SNoESIxKb4UhFxixKvtEdGxNE6aUUVc8kFk6wTStJ/X3IGiMetwkXiFiUxabUF/8A6SyvnSVDm+wFuavugpVrZikjLcfrf2xOVgnG3deQQvd/qbAv14jTwMFl+T+8d/cXBo8Mn/leLZCQun/EJEnkXP5MjgNI8XcWUE4NnH3E0ESSm6Pkm8MhMDZ2fmzNgqEyJ0GVinNgSml3Pyha3PBSj5LRczLip/ie4QkKx5OHvX2L3sNv/JIUHse5HSbjZ1c/4oGCYMVTYCykWiczrxBUOlcr8RwnZLOm4n2bCt5ZhAgMBAAECggEAVkePwfzmr7zR7tTpxeGNeXHtDUAdJm3RWwUSASPXgb5qKyXVsm5nAPX4lXDE3E1i/nzSkzNS5PgIoxNVU10cMxZs6JW0okFx7oYaAwgAddN6lxQtjD7EuGaixN6zZ1k/G6vT98iS6i3uNCAlRZ9HVBmjsOF8GtYolZqLvfZ5izEVFlLVq/BCs7Y5OrDrbGmn3XupfitVWYExV0BrHpobDjsx2fYdTZkmPpSSvXNcm4Iq2AXVQzoqAfGo7+qsuLCZtVlyTfVKQjMvE2ffzN1dQunxixOvev/fz4WSjGnRpC6QLn6Oqps9+VxQKqKuXXqUJC+U45DuvA94Of9MvZfAAQKBgQD7xmXueXRBMr2+0WftybAV024ap0cXFrCAu+KWC1SUddCfkiV7e5w+kRJx6RH1cg4cyyCL8yhHZ99Z5V0Mxa/b/usuHMadXPyX5szVI7dOGgIC9q8IijN7B7GMFAXc8+qC7kivehJzjQghpRRAqvRzjDls4gmbNPhbH1jUiU124QKBgQDtOaW5/fOEtOq0yWbDLkLdjImct6oKMLhENL6yeIKjMYgifzHb2adk7rWG3qcMrdgaFtDVfqv8UmMEkzk7bSkovMVj3SkLzMz84ii1SkSfyaCXgt/UOzDkqAUYB0cXMppYA7jxHa2OY8oEHdBgmyJXdLdzJxCp851AoTlRUSePgQKBgQCQgKgUHOUaXnMEx88sbOuBO14gMg3dNIqM+Ejt8QbURmI8k3arzqA4UK8Tbb9+7b0nzXWanS5q/TT1tWyYXgW28DIuvxlHTA01aaP6WItmagrphIelERzG6f1+9ib/T4czKmvROvDIHROjq8lZ7ERs5Pg4g+sbh2VbdzxWj49EQQKBgFEna36ZVfmMOs7mJ3WWGeHY9ira2hzqVd9fe+1qNKbHhx7mDJR9fTqWPxuIh/Vac5dZPtAKqaOEO8OQ6f9edLou+ggT3LrgsS/B3tNGOPvA6mNqrk/Yf/15TWTO+I8DDLIXc+lokbsogC+wU1z5NWJd13RZZOX/JUi63vTmonYBAoGBAIpglLCH2sPXfmguO6p8QcQcv4RjAU1c0GP4P5PNN3Wzo0ItydVd2LHJb6MdmL6ypeiwNklzPFwTeRlKTPmVxJ+QPg1ct/3tAURN/D40GYw9ojDhqmdSl4HW4d6gHS2lYzSFeU5jkG49y5nirOOoEgHy95wghkh6BfpwHujYJGw4",

		ClusterListenMultiaddress:   "/ip4/127.0.0.1/tcp/10000",
		APIListenMultiaddress:       "/ip4/127.0.0.1/tcp/10002",
		IPFSProxyListenMultiaddress: "/ip4/127.0.0.1/tcp/10001",
		ConsensusDataFolder:         "./raftFolderFromTests",
		RaftConfig: &RaftConfig{
			EnableSingleNode:        true,
			SnapshotIntervalSeconds: 120,
		},
	}

	cfg, _ := jcfg.ToConfig()
	return cfg
}

func TestDefaultConfig(t *testing.T) {
	_, err := NewDefaultConfig()
	if err != nil {
		t.Fatal(err)
	}
}

func TestConfigToJSON(t *testing.T) {
	cfg, err := NewDefaultConfig()
	if err != nil {
		t.Fatal(err)
	}
	_, err = cfg.ToJSONConfig()
	if err != nil {
		t.Error(err)
	}
}

func TestConfigToConfig(t *testing.T) {
	cfg, _ := NewDefaultConfig()
	j, _ := cfg.ToJSONConfig()
	_, err := j.ToConfig()
	if err != nil {
		t.Error(err)
	}

	j.ID = "abc"
	_, err = j.ToConfig()
	if err == nil {
		t.Error("expected error decoding ID")
	}

	j, _ = cfg.ToJSONConfig()
	j.PrivateKey = "abc"
	_, err = j.ToConfig()
	if err == nil {
		t.Error("expected error parsing private key")
	}

	j, _ = cfg.ToJSONConfig()
	j.ClusterListenMultiaddress = "abc"
	_, err = j.ToConfig()
	if err == nil {
		t.Error("expected error parsing cluster_listen_multiaddress")
	}

	j, _ = cfg.ToJSONConfig()
	j.APIListenMultiaddress = "abc"
	_, err = j.ToConfig()
	if err == nil {
		t.Error("expected error parsing api_listen_multiaddress")
	}

	j, _ = cfg.ToJSONConfig()
	j.IPFSProxyListenMultiaddress = "abc"
	_, err = j.ToConfig()
	if err == nil {
		t.Error("expected error parsing ipfs_proxy_listen_multiaddress")
	}

	j, _ = cfg.ToJSONConfig()
	j.IPFSNodeMultiaddress = "abc"
	_, err = j.ToConfig()
	if err == nil {
		t.Error("expected error parsing ipfs_node_multiaddress")
	}

	j, _ = cfg.ToJSONConfig()
	j.ClusterPeers = []string{"abc"}
	_, err = j.ToConfig()
	if err == nil {
		t.Error("expected error parsing cluster_peers")
	}
}
