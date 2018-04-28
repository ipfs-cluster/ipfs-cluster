package ipfscluster

import (
	"encoding/json"
	"testing"
)

var ccfgTestJSON = []byte(`
{
        "id": "QmUfSFm12eYCaRdypg48m8RqkXfLW7A2ZeGZb2skeHHDGA",
        "peername": "testpeer",
        "private_key": "CAASqAkwggSkAgEAAoIBAQDpT16IRF6bb9tHsCbQ7M+nb2aI8sz8xyt8PoAWM42ki+SNoESIxKb4UhFxixKvtEdGxNE6aUUVc8kFk6wTStJ/X3IGiMetwkXiFiUxabUF/8A6SyvnSVDm+wFuavugpVrZikjLcfrf2xOVgnG3deQQvd/qbAv14jTwMFl+T+8d/cXBo8Mn/leLZCQun/EJEnkXP5MjgNI8XcWUE4NnH3E0ESSm6Pkm8MhMDZ2fmzNgqEyJ0GVinNgSml3Pyha3PBSj5LRczLip/ie4QkKx5OHvX2L3sNv/JIUHse5HSbjZ1c/4oGCYMVTYCykWiczrxBUOlcr8RwnZLOm4n2bCt5ZhAgMBAAECggEAVkePwfzmr7zR7tTpxeGNeXHtDUAdJm3RWwUSASPXgb5qKyXVsm5nAPX4lXDE3E1i/nzSkzNS5PgIoxNVU10cMxZs6JW0okFx7oYaAwgAddN6lxQtjD7EuGaixN6zZ1k/G6vT98iS6i3uNCAlRZ9HVBmjsOF8GtYolZqLvfZ5izEVFlLVq/BCs7Y5OrDrbGmn3XupfitVWYExV0BrHpobDjsx2fYdTZkmPpSSvXNcm4Iq2AXVQzoqAfGo7+qsuLCZtVlyTfVKQjMvE2ffzN1dQunxixOvev/fz4WSjGnRpC6QLn6Oqps9+VxQKqKuXXqUJC+U45DuvA94Of9MvZfAAQKBgQD7xmXueXRBMr2+0WftybAV024ap0cXFrCAu+KWC1SUddCfkiV7e5w+kRJx6RH1cg4cyyCL8yhHZ99Z5V0Mxa/b/usuHMadXPyX5szVI7dOGgIC9q8IijN7B7GMFAXc8+qC7kivehJzjQghpRRAqvRzjDls4gmbNPhbH1jUiU124QKBgQDtOaW5/fOEtOq0yWbDLkLdjImct6oKMLhENL6yeIKjMYgifzHb2adk7rWG3qcMrdgaFtDVfqv8UmMEkzk7bSkovMVj3SkLzMz84ii1SkSfyaCXgt/UOzDkqAUYB0cXMppYA7jxHa2OY8oEHdBgmyJXdLdzJxCp851AoTlRUSePgQKBgQCQgKgUHOUaXnMEx88sbOuBO14gMg3dNIqM+Ejt8QbURmI8k3arzqA4UK8Tbb9+7b0nzXWanS5q/TT1tWyYXgW28DIuvxlHTA01aaP6WItmagrphIelERzG6f1+9ib/T4czKmvROvDIHROjq8lZ7ERs5Pg4g+sbh2VbdzxWj49EQQKBgFEna36ZVfmMOs7mJ3WWGeHY9ira2hzqVd9fe+1qNKbHhx7mDJR9fTqWPxuIh/Vac5dZPtAKqaOEO8OQ6f9edLou+ggT3LrgsS/B3tNGOPvA6mNqrk/Yf/15TWTO+I8DDLIXc+lokbsogC+wU1z5NWJd13RZZOX/JUi63vTmonYBAoGBAIpglLCH2sPXfmguO6p8QcQcv4RjAU1c0GP4P5PNN3Wzo0ItydVd2LHJb6MdmL6ypeiwNklzPFwTeRlKTPmVxJ+QPg1ct/3tAURN/D40GYw9ojDhqmdSl4HW4d6gHS2lYzSFeU5jkG49y5nirOOoEgHy95wghkh6BfpwHujYJGw4",
        "secret": "2588b80d5cb05374fa142aed6cbb047d1f4ef8ef15e37eba68c65b9d30df67ed",
        "leave_on_shutdown": true,
        "listen_multiaddress": "/ip4/127.0.0.1/tcp/10000",
        "state_sync_interval": "1m0s",
        "ipfs_sync_interval": "2m10s",
        "replication_factor_min": 5,
        "replication_factor_max": 5,
        "monitor_ping_interval": "2s",
        "disable_repinning": true
}
`)

func TestLoadJSON(t *testing.T) {
	cfg := &Config{}
	err := cfg.LoadJSON(ccfgTestJSON)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Peername != "testpeer" {
		t.Error("expected peername 'testpeer'")
	}

	if cfg.ReplicationFactorMin != 5 {
		t.Error("expected replication factor min == 5")
	}

	if !cfg.DisableRepinning {
		t.Error("expected disable_repinning to be true")
	}

	j := &configJSON{}

	json.Unmarshal(ccfgTestJSON, j)
	j.ID = "abc"
	tst, _ := json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error decoding ID")
	}

	j = &configJSON{}
	json.Unmarshal(ccfgTestJSON, j)
	j.Peername = ""
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if cfg.Peername == "" {
		t.Error("expected default peername")
	}

	j = &configJSON{}
	json.Unmarshal(ccfgTestJSON, j)
	j.PrivateKey = "abc"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error parsing private key")
	}

	j = &configJSON{}
	json.Unmarshal(ccfgTestJSON, j)
	j.ListenMultiaddress = "abc"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error parsing listen_multiaddress")
	}

	j = &configJSON{}
	json.Unmarshal(ccfgTestJSON, j)
	j.Secret = "abc"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error decoding secret")
	}

	j = &configJSON{}
	json.Unmarshal(ccfgTestJSON, j)
	j.ReplicationFactor = 0
	j.ReplicationFactorMin = 0
	j.ReplicationFactorMax = 0
	tst, _ = json.Marshal(j)
	cfg.LoadJSON(tst)
	if cfg.ReplicationFactorMin != -1 || cfg.ReplicationFactorMax != -1 {
		t.Error("expected default replication factor")
	}

	j = &configJSON{}
	json.Unmarshal(ccfgTestJSON, j)
	j.ReplicationFactor = 3
	tst, _ = json.Marshal(j)
	cfg.LoadJSON(tst)
	if cfg.ReplicationFactorMin != 3 || cfg.ReplicationFactorMax != 3 {
		t.Error("expected replicationFactor Min/Max override")
	}

	j = &configJSON{}
	json.Unmarshal(ccfgTestJSON, j)
	j.ReplicationFactorMin = -1
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error when only one replication factor is -1")
	}

	j = &configJSON{}
	json.Unmarshal(ccfgTestJSON, j)
	j.ReplicationFactorMin = 5
	j.ReplicationFactorMax = 4
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error when only rplMin > rplMax")
	}

	j = &configJSON{}
	json.Unmarshal(ccfgTestJSON, j)
	j.ReplicationFactorMin = 0
	j.ReplicationFactorMax = 0
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if cfg.ReplicationFactorMin != -1 || cfg.ReplicationFactorMax != -1 {
		t.Error("expected default replication factors")
	}
}

func TestToJSON(t *testing.T) {
	cfg := &Config{}
	cfg.LoadJSON(ccfgTestJSON)
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
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidate(t *testing.T) {
	cfg := &Config{}
	cfg.Default()
	cfg.ID = ""
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.MonitorPingInterval = 0
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.ReplicationFactorMin = 10
	cfg.ReplicationFactorMax = 5
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.ReplicationFactorMin = 0
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}
