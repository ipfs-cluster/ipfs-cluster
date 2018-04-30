package ipfshttp

import (
	"encoding/json"
	"testing"
)

var cfgJSON = []byte(`
{
      "proxy_listen_multiaddress": "/ip4/127.0.0.1/tcp/9095",
      "node_multiaddress": "/ip4/127.0.0.1/tcp/5001",
      "connect_swarms_delay": "7s",
      "proxy_read_timeout": "10m0s",
      "proxy_read_header_timeout": "5s",
      "proxy_write_timeout": "10m0s",
      "proxy_idle_timeout": "1m0s",
      "pin_method": "pin",
      "ipfs_request_timeout": "5m0s",
	  "pin_timeout": "24h",
	  "unpin_timeout": "3h"
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
	cfg.ProxyAddr = nil
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.ProxyReadTimeout = -1
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.ProxyReadHeaderTimeout = -2
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.ProxyIdleTimeout = -1
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}

	cfg.Default()
	cfg.ProxyWriteTimeout = -3
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}
