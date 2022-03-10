package pinsvcapi

import (
	"context"
	"testing"
	"time"

	clustertest "github.com/ipfs/ipfs-cluster/test"

	libp2p "github.com/libp2p/go-libp2p"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	SSLCertFile         = "test/server.crt"
	SSLKeyFile          = "test/server.key"
	clientOrigin        = "myorigin"
	validUserName       = "validUserName"
	validUserPassword   = "validUserPassword"
	adminUserName       = "adminUserName"
	adminUserPassword   = "adminUserPassword"
	invalidUserName     = "invalidUserName"
	invalidUserPassword = "invalidUserPassword"
)

func testAPIwithConfig(t *testing.T, cfg *Config, name string) *API {
	ctx := context.Background()
	apiMAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	h, err := libp2p.New(libp2p.ListenAddrs(apiMAddr))
	if err != nil {
		t.Fatal(err)
	}

	cfg.HTTPListenAddr = []ma.Multiaddr{apiMAddr}

	rest, err := NewAPIWithHost(ctx, cfg, h)
	if err != nil {
		t.Fatalf("should be able to create a new %s API: %s", name, err)
	}

	// No keep alive for tests
	rest.SetKeepAlivesEnabled(false)
	rest.SetClient(clustertest.NewMockRPCClient(t))

	return rest
}

func testAPI(t *testing.T) *API {
	cfg := NewConfig()
	cfg.Default()
	cfg.CORSAllowedOrigins = []string{clientOrigin}
	cfg.CORSAllowedMethods = []string{"GET", "POST", "DELETE"}
	//cfg.CORSAllowedHeaders = []string{"Content-Type"}
	cfg.CORSMaxAge = 10 * time.Minute

	return testAPIwithConfig(t, cfg, "basic")
}
