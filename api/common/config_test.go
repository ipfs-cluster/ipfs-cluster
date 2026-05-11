package common

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	types "github.com/ipfs-cluster/ipfs-cluster/api"
	logging "github.com/ipfs/go-log/v2"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	peer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// Default testing values
var (
	DefaultReadTimeout        = 0 * time.Second
	DefaultReadHeaderTimeout  = 5 * time.Second
	DefaultWriteTimeout       = 0 * time.Second
	DefaultIdleTimeout        = 120 * time.Second
	DefaultMaxHeaderBytes     = minMaxHeaderBytes
	DefaultHTTPListenAddrs    = []string{"/ip4/127.0.0.1/tcp/9094"}
	DefaultHeaders            = map[string][]string{}
	DefaultCORSAllowedOrigins = []string{"*"}
	DefaultCORSAllowedMethods = []string{}
	DefaultCORSAllowedHeaders = []string{}
	DefaultCORSExposedHeaders = []string{
		"Content-Type",
		"X-Stream-Output",
		"X-Chunked-Output",
		"X-Content-Length",
	}
	DefaultCORSAllowCredentials = true
	DefaultCORSMaxAge           time.Duration // 0. Means always.
)

func defaultFunc(cfg *Config) error {
	// http
	addrs := make([]ma.Multiaddr, 0, len(DefaultHTTPListenAddrs))
	for _, def := range DefaultHTTPListenAddrs {
		httpListen, err := ma.NewMultiaddr(def)
		if err != nil {
			return err
		}
		addrs = append(addrs, httpListen)
	}
	cfg.HTTPListenAddr = addrs
	cfg.PathSSLCertFile = ""
	cfg.PathSSLKeyFile = ""
	cfg.ReadTimeout = DefaultReadTimeout
	cfg.ReadHeaderTimeout = DefaultReadHeaderTimeout
	cfg.WriteTimeout = DefaultWriteTimeout
	cfg.IdleTimeout = DefaultIdleTimeout
	cfg.MaxHeaderBytes = DefaultMaxHeaderBytes

	// libp2p
	cfg.ID = ""
	cfg.PrivateKey = nil
	cfg.Libp2pListenAddr = nil

	// Auth
	cfg.BasicAuthCredentials = nil

	// Logs
	cfg.HTTPLogFile = ""

	// Headers
	cfg.Headers = DefaultHeaders

	cfg.CORSAllowedOrigins = DefaultCORSAllowedOrigins
	cfg.CORSAllowedMethods = DefaultCORSAllowedMethods
	cfg.CORSAllowedHeaders = DefaultCORSAllowedHeaders
	cfg.CORSExposedHeaders = DefaultCORSExposedHeaders
	cfg.CORSAllowCredentials = DefaultCORSAllowCredentials
	cfg.CORSMaxAge = DefaultCORSMaxAge

	return nil
}

var cfgJSON = []byte(`
{
	"listen_multiaddress": "/ip4/127.0.0.1/tcp/12122",
	"ssl_cert_file": "test/server.crt",
	"ssl_key_file": "test/server.key",
	"read_timeout": "30s",
	"read_header_timeout": "5s",
	"write_timeout": "1m0s",
	"idle_timeout": "2m0s",
	"max_header_bytes": 16384,
	"basic_auth_credentials": null,
	"http_log_file": "",
	"cors_allowed_origins": ["myorigin"],
	"cors_allowed_methods": ["GET"],
	"cors_allowed_headers": ["X-Custom"],
	"cors_exposed_headers": ["X-Chunked-Output"],
	"cors_allow_credentials": false,
	"cors_max_age": "1s"
}
`)

func newTestConfig() *Config {
	cfg := &Config{}
	cfg.ConfigKey = "testapi"
	cfg.EnvConfigKey = "cluster_testapi"
	cfg.Logger = logging.Logger("testapi")
	cfg.RequestLogger = logging.Logger("testapilog")
	cfg.DefaultFunc = defaultFunc
	cfg.APIErrorFunc = func(err error, status int) error {
		return types.Error{Code: status, Message: err.Error()}
	}
	return cfg
}

func newDefaultTestConfig(t *testing.T) *Config {
	t.Helper()
	cfg := newTestConfig()
	if err := defaultFunc(cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestLoadEmptyJSON(t *testing.T) {
	cfg := newTestConfig()
	err := cfg.LoadJSON([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadJSON(t *testing.T) {
	cfg := newTestConfig()
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
	j.HTTPListenMultiaddress = []string{"abc"}
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
	j.BasicAuthCredentials = make(map[string]string)
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
	j.Libp2pListenMultiaddress = []string{"abc"}
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

	j = &jsonConfig{}
	json.Unmarshal(cfgJSON, j)
	j.MaxHeaderBytes = minMaxHeaderBytes - 1
	tst, _ = json.Marshal(j)
	err = cfg.LoadJSON(tst)
	if err == nil {
		t.Error("expected error with MaxHeaderBytes")
	}
}

func TestApplyEnvVars(t *testing.T) {
	username := "admin"
	password := "thisaintmypassword"
	user1 := "user1"
	user1pass := "user1passwd"
	os.Setenv("CLUSTER_TESTAPI_BASICAUTHCREDENTIALS", username+":"+password+","+user1+":"+user1pass)
	cfg := newDefaultTestConfig(t)
	err := cfg.ApplyEnvVars()
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := cfg.BasicAuthCredentials[username]; !ok {
		t.Fatalf("username '%s' not set in BasicAuthCreds map: %v", username, cfg.BasicAuthCredentials)
	}

	if _, ok := cfg.BasicAuthCredentials[user1]; !ok {
		t.Fatalf("username '%s' not set in BasicAuthCreds map: %v", user1, cfg.BasicAuthCredentials)
	}

	if gotpasswd := cfg.BasicAuthCredentials[username]; gotpasswd != password {
		t.Errorf("password not what was set in env var, got: %s, want: %s", gotpasswd, password)
	}

	if gotpasswd := cfg.BasicAuthCredentials[user1]; gotpasswd != user1pass {
		t.Errorf("password not what was set in env var, got: %s, want: %s", gotpasswd, user1pass)
	}
}

func TestLibp2pConfig(t *testing.T) {
	ctx := context.Background()
	cfg := newDefaultTestConfig(t)

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
	cfg.HTTPListenAddr = []ma.Multiaddr{addr}
	cfg.Libp2pListenAddr = []ma.Multiaddr{addr}

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
	rest, err := NewAPI(ctx, cfg,
		func(c *rpc.Client) []Route { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer rest.Shutdown(ctx)

	badPid, _ := peer.Decode("QmTQ6oKHDwFjzr4ihirVCLJe8CxanxD3ZjGRYzubFuNDjE")
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
	cfg := newTestConfig()
	cfg.LoadJSON(cfgJSON)
	newjson, err := cfg.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	cfg = newTestConfig()
	err = cfg.LoadJSON(newjson)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDefault(t *testing.T) {
	cfg := newDefaultTestConfig(t)
	if cfg.Validate() != nil {
		t.Fatal("error validating")
	}

	err := defaultFunc(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.IdleTimeout = -1
	if cfg.Validate() == nil {
		t.Fatal("expected error validating")
	}
}
