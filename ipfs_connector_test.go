package ipfscluster

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	cid "github.com/ipfs/go-cid"
)

func testIPFSConnectorConfig(mock *ipfsMock) *Config {
	url, _ := url.Parse(mock.server.URL)
	h := strings.Split(url.Host, ":")
	i, _ := strconv.Atoi(h[1])

	cfg := testingConfig()
	cfg.IPFSHost = h[0]
	cfg.IPFSPort = i
	return cfg
}

func ipfsConnector(t *testing.T) (*IPFSHTTPConnector, *ipfsMock) {
	mock := newIpfsMock()
	cfg := testIPFSConnectorConfig(mock)

	ipfs, err := NewIPFSHTTPConnector(cfg)
	if err != nil {
		t.Fatal("creating an IPFSConnector should work: ", err)
	}
	return ipfs, mock
}

func TestNewIPFSHTTPConnector(t *testing.T) {
	ipfs, mock := ipfsConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	ch := ipfs.RpcChan()
	if ch == nil {
		t.Error("RpcCh should be created")
	}
}

func TestIPFSPin(t *testing.T) {
	ipfs, mock := ipfsConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()
	c, _ := cid.Decode(testCid)
	err := ipfs.Pin(c)
	if err != nil {
		t.Error("expected success pinning cid")
	}
	yes, err := ipfs.IsPinned(c)
	if err != nil {
		t.Fatal("expected success doing ls")
	}
	if !yes {
		t.Error("cid should have been pinned")
	}

	c2, _ := cid.Decode(errorCid)
	err = ipfs.Pin(c2)
	if err == nil {
		t.Error("expected error pinning cid")
	}
}

func TestIPFSUnpin(t *testing.T) {
	ipfs, mock := ipfsConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()
	c, _ := cid.Decode(testCid)
	err := ipfs.Unpin(c)
	if err != nil {
		t.Error("expected success unpinning non-pinned cid")
	}
	ipfs.Pin(c)
	err = ipfs.Unpin(c)
	if err != nil {
		t.Error("expected success unpinning pinned cid")
	}
}

func TestIPFSIsPinned(t *testing.T) {
	ipfs, mock := ipfsConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()
	c, _ := cid.Decode(testCid)
	c2, _ := cid.Decode(testCid2)

	ipfs.Pin(c)
	isp, err := ipfs.IsPinned(c)
	if err != nil || !isp {
		t.Error("c should appear pinned")
	}

	isp, err = ipfs.IsPinned(c2)
	if err != nil || isp {
		t.Error("c2 should appear unpinned")
	}
}

func TestIPFSProxy(t *testing.T) {
	ipfs, mock := ipfsConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	cfg := testingConfig()
	res, err := http.Get(fmt.Sprintf("http://%s:%d/api/v0/add?arg=%s",
		cfg.IPFSAPIListenAddr,
		cfg.IPFSAPIListenPort,
		testCid))
	if err != nil {
		t.Fatal("should forward requests to ipfs host: ", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Error("the request should have succeeded")
	}
}

func TestIPFSShutdown(t *testing.T) {
	ipfs, mock := ipfsConnector(t)
	defer mock.Close()
	if err := ipfs.Shutdown(); err != nil {
		t.Error("expected a clean shutdown")
	}
	if err := ipfs.Shutdown(); err != nil {
		t.Error("expected a second clean shutdown")
	}
}
