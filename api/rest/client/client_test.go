package client

import (
	"testing"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/test"
)

var apiAddr = "/ip4/127.0.0.1/tcp/10005"

func testAPI(t *testing.T) *rest.API {
	//logging.SetDebugLogging()
	apiMAddr, _ := ma.NewMultiaddr(apiAddr)

	cfg := &rest.Config{}
	cfg.Default()
	cfg.ListenAddr = apiMAddr

	rest, err := rest.NewAPI(cfg)
	if err != nil {
		t.Fatal("should be able to create a new Api: ", err)
	}

	rest.SetClient(test.NewMockRPCClient(t))
	return rest
}

func testClient(t *testing.T) (*Client, *rest.API) {
	api := testAPI(t)

	addr, _ := ma.NewMultiaddr(apiAddr)
	cfg := &Config{
		APIAddr:           addr,
		DisableKeepAlives: true,
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	return c, api
}

func TestNewClient(t *testing.T) {
	_, api := testClient(t)
	api.Shutdown()
}

func TestDefaultAddress(t *testing.T) {
	cfg := &Config{
		APIAddr:           nil,
		DisableKeepAlives: true,
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.urlPrefix != "http://127.0.0.1:9094" {
		t.Error("default should be used")
	}
}

func TestMultiaddressPreference(t *testing.T) {
	addr, _ := ma.NewMultiaddr(apiAddr)
	cfg := &Config{
		APIAddr:           addr,
		Host:              "localhost",
		Port:              "9094",
		DisableKeepAlives: true,
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.urlPrefix != "http://127.0.0.1:10005" {
		t.Error("APIAddr should be used")
	}
}

func TestHostPort(t *testing.T) {
	cfg := &Config{
		APIAddr:           nil,
		Host:              "localhost",
		Port:              "9094",
		DisableKeepAlives: true,
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.urlPrefix != "http://localhost:9094" {
		t.Error("Host Port should be used")
	}
}

func TestDNSMultiaddress(t *testing.T) {
	addr2, _ := ma.NewMultiaddr("/dns4/localhost/tcp/1234")
	cfg := &Config{
		APIAddr:           addr2,
		Host:              "localhost",
		Port:              "9094",
		DisableKeepAlives: true,
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.urlPrefix != "http://127.0.0.1:1234" {
		t.Error("bad resolved address")
	}
}
