package ipfsproxy

import (
	"encoding/json"
	"testing"
)

var cfgJSON = []byte(`
{
      "listen_multiaddress": "/ip4/127.0.0.1/tcp/9095",
      "node_multiaddress": "/ip4/127.0.0.1/tcp/5001",
      "read_timeout": "10m0s",
      "read_header_timeout": "5s",
      "write_timeout": "10m0s",
      "idle_timeout": "1m0s",
      "extract_headers_extra": [],
      "extract_headers_path": "/api/v0/version",
      "extract_headers_ttl": "5m"
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
	j.ProxyListenMultiaddress = "abc"
	tst, _ := json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error decoding proxy_listen_multiaddress")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.NodeMultiaddress = "abc"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error in node_multiaddress")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.ProxyReadTimeout = "-aber"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error in proxy_read_timeout")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.ExtractHeadersTTL = -10
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error in extract_headers_ttl")
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

	cfg.Default()
	cfg.ListenAddr = nil
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.ReadTimeout = -1
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.ReadHeaderTimeout = -2
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.IdleTimeout = -1
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.WriteTimeout = -3
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.ExtractHeadersPath = ""
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}
