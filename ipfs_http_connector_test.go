package ipfscluster

import (
	"fmt"
	"net/http"
	"testing"

	cid "github.com/ipfs/go-cid"
	ma "github.com/multiformats/go-multiaddr"
)

func testIPFSConnectorConfig(mock *ipfsMock) *Config {
	cfg := testingConfig()
	addr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", mock.addr, mock.port))
	cfg.IPFSNodeAddr = addr
	return cfg
}

func testIPFSConnector(t *testing.T) (*IPFSHTTPConnector, *ipfsMock) {
	mock := newIpfsMock()
	cfg := testIPFSConnectorConfig(mock)

	ipfs, err := NewIPFSHTTPConnector(cfg)
	if err != nil {
		t.Fatal("creating an IPFSConnector should work: ", err)
	}
	ipfs.SetClient(mockRPCClient(t))
	return ipfs, mock
}

func TestNewIPFSHTTPConnector(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()
}

func TestIPFSID(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer ipfs.Shutdown()
	id, err := ipfs.ID()
	if err != nil {
		t.Fatal(err)
	}
	if id.ID != testPeerID {
		t.Error("expected testPeerID")
	}
	if len(id.Addresses) != 1 {
		t.Error("expected 1 address")
	}
	if id.Error != "" {
		t.Error("expected no error")
	}
	mock.Close()
	id, err = ipfs.ID()
	if err == nil {
		t.Error("expected an error")
	}
	if id.Error != err.Error() {
		t.Error("error messages should match")
	}
}

func TestIPFSPin(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()
	c, _ := cid.Decode(testCid)
	err := ipfs.Pin(c)
	if err != nil {
		t.Error("expected success pinning cid")
	}
	pinSt, err := ipfs.PinLsCid(c)
	if err != nil {
		t.Fatal("expected success doing ls")
	}
	if !pinSt.IsPinned() {
		t.Error("cid should have been pinned")
	}

	c2, _ := cid.Decode(errorCid)
	err = ipfs.Pin(c2)
	if err == nil {
		t.Error("expected error pinning cid")
	}
}

func TestIPFSUnpin(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
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

func TestIPFSPinLsCid(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()
	c, _ := cid.Decode(testCid)
	c2, _ := cid.Decode(testCid2)

	ipfs.Pin(c)
	ips, err := ipfs.PinLsCid(c)
	if err != nil || !ips.IsPinned() {
		t.Error("c should appear pinned")
	}

	ips, err = ipfs.PinLsCid(c2)
	if err != nil || ips != IPFSPinStatusUnpinned {
		t.Error("c2 should appear unpinned")
	}
}

func TestIPFSPinLs(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()
	c, _ := cid.Decode(testCid)
	c2, _ := cid.Decode(testCid2)

	ipfs.Pin(c)
	ipfs.Pin(c2)
	ipsMap, err := ipfs.PinLs()
	if err != nil {
		t.Error("should not error")
	}

	if len(ipsMap) != 2 {
		t.Fatal("the map does not contain expected keys")
	}

	if !ipsMap[testCid].IsPinned() || !ipsMap[testCid2].IsPinned() {
		t.Error("c1 and c2 should appear pinned")
	}
}

func TestIPFSProxy(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	cfg := testingConfig()
	host, _ := cfg.IPFSProxyAddr.ValueForProtocol(ma.P_IP4)
	port, _ := cfg.IPFSProxyAddr.ValueForProtocol(ma.P_TCP)
	res, err := http.Get(fmt.Sprintf("http://%s:%s/api/v0/add?arg=%s",
		host,
		port,
		testCid))
	if err != nil {
		t.Fatal("should forward requests to ipfs host: ", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Error("the request should have succeeded")
	}
}

func TestIPFSShutdown(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	if err := ipfs.Shutdown(); err != nil {
		t.Error("expected a clean shutdown")
	}
	if err := ipfs.Shutdown(); err != nil {
		t.Error("expected a second clean shutdown")
	}
}
