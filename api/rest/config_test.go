package rest

import (
	"encoding/json"
	"testing"
)

var cfgJSON = []byte(`
{
      "listen_multiaddress": "/ip4/127.0.0.1/tcp/9094",
      "ssl_cert_file": "test/server.crt",
      "ssl_key_file": "test/server.key",
      "read_timeout": "30s",
      "read_header_timeout": "5s",
      "write_timeout": "1m0s",
      "idle_timeout": "2m0s",
      "basic_auth_credentials": null
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
	j.ListenMultiaddress = "abc"
	tst, _ := json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error decoding listen multiaddress")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.ReadTimeout = "0"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error in read_timeout")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.BasicAuthCreds = make(map[string]string)
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error with empty basic auth map")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.SSLCertFile = "abc"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error with TLS configuration")
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

	cfg.ListenAddr = nil
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.IdleTimeout = 0
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}
