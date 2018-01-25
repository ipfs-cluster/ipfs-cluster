package client

import (
	"fmt"
	"strings"
	"testing"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/test"
)

func testAPI(t *testing.T) *rest.API {
	//logging.SetDebugLogging()
	apiMAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

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

func apiMAddr(a *rest.API) ma.Multiaddr {
	hostPort := strings.Split(a.HTTPAddress(), ":")

	addr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%s", hostPort[1]))
	return addr
}

func testClient(t *testing.T) (*Client, *rest.API) {
	api := testAPI(t)

	cfg := &Config{
		APIAddr:           apiMAddr(api),
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
	addr, _ := ma.NewMultiaddr("/ip4/1.2.3.4/tcp/1234")
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
	if c.urlPrefix != "http://1.2.3.4:1234" {
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
