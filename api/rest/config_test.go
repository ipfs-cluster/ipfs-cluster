package rest

import (
	"encoding/json"
	"testing"
	"time"

	crypto "github.com/libp2p/go-libp2p-crypto"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
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

	if cfg.ReadTimeout != 30*time.Second ||
		cfg.WriteTimeout != time.Minute ||
		cfg.ReadHeaderTimeout != 5*time.Second ||
		cfg.IdleTimeout != 2*time.Minute {
		t.Error("error parsing timeouts")
	}

	j := &jsonConfig{}

	json.Unmarshal(cfgJSON, j)
	j.HTTPListenMultiaddress = "abc"
	tst, _ := json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error decoding listen multiaddress")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.ReadTimeout = "-1"
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

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.ID = "abc"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error with ID")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.Libp2pListenMultiaddress = "abc"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error with libp2p address")
	}

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.PrivateKey = "abc"
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error with private key")
	}
}

func TestLibp2pConfig(t *testing.T) {
	cfg := &Config{}
	err := cfg.Default()
	if err != nil {
		t.Fatal(err)
	}

	priv, pub, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ID = pid
	cfg.PrivateKey = priv
	addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	cfg.Libp2pListenAddr = addr

	err = cfg.Validate()
	if err != nil {
		t.Error(err)
	}

	cfgJSON, err := cfg.ToJSON()
	if err != nil {
		t.Fatal(err)
	}

	err = cfg.LoadJSON(cfgJSON)
	if err != nil {
		t.Fatal(err)
	}

	// Test creating a new API with a libp2p config
	rest, err := NewAPI(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rest.Shutdown()

	badPid, _ := peer.IDB58Decode("QmTQ6oKHDwFjzr4ihirVCLJe8CxanxD3ZjGRYzubFuNDjE")
	cfg.ID = badPid
	err = cfg.Validate()
	if err == nil {
		t.Error("expected id-privkey mismatch")
	}
	cfg.ID = pid

	cfg.PrivateKey = nil
	err = cfg.Validate()
	if err == nil {
		t.Error("expected missing private key error")
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

	cfg.Default()
	cfg.IdleTimeout = -1
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}
