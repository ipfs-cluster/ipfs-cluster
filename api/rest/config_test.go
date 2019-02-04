package rest

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	crypto "github.com/libp2p/go-libp2p-crypto"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
)

var cfgJSON = []byte(`
{
      "listen_multiaddress": "/ip4/127.0.0.1/tcp/12122",
      "ssl_cert_file": "test/server.crt",
      "ssl_key_file": "test/server.key",
      "read_timeout": "30s",
      "read_header_timeout": "5s",
      "write_timeout": "1m0s",
      "idle_timeout": "2m0s",
      "basic_auth_credentials": null,
      "cors_allowed_origins": ["myorigin"],
      "cors_allowed_methods": ["GET"],
      "cors_allowed_headers": ["X-Custom"],
      "cors_exposed_headers": ["X-Chunked-Output"],
      "cors_allow_credentials": false,
      "cors_max_age": "1s"
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

func TestLoadJSONEnvConfig(t *testing.T) {
	username := "admin"
	password := "thisaintmypassword"
	user1 := "user1"
	user1pass := "user1passwd"
	os.Setenv("CLUSTER_RESTAPI_BASICAUTHCREDS", username+":"+password+","+user1+":"+user1pass)
	cfg := &Config{}
	err := cfg.LoadJSON(cfgJSON)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := cfg.BasicAuthCreds[username]; !ok {
		t.Fatalf("username '%s' not set in BasicAuthCreds map: %v", username, cfg.BasicAuthCreds)
	}

	if _, ok := cfg.BasicAuthCreds[user1]; !ok {
		t.Fatalf("username '%s' not set in BasicAuthCreds map: %v", user1, cfg.BasicAuthCreds)
	}

	if gotpasswd := cfg.BasicAuthCreds[username]; gotpasswd != password {
		t.Errorf("password not what was set in env var, got: %s, want: %s", gotpasswd, password)
	}

	if gotpasswd := cfg.BasicAuthCreds[user1]; gotpasswd != user1pass {
		t.Errorf("password not what was set in env var, got: %s, want: %s", gotpasswd, user1pass)
	}
}

func TestLibp2pConfig(t *testing.T) {
	ctx := context.Background()
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
	rest, err := NewAPI(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rest.Shutdown(ctx)

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
