package ipfscluster

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"

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
	if err != nil || ips != api.IPFSPinStatusUnpinned {
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

func TestIPFSProxyVersion(t *testing.T) {
	// This makes sure default handler is used

	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	cfg := testingConfig()
	host, _ := cfg.IPFSProxyAddr.ValueForProtocol(ma.P_IP4)
	port, _ := cfg.IPFSProxyAddr.ValueForProtocol(ma.P_TCP)
	res, err := http.Get(fmt.Sprintf("http://%s:%s/api/v0/version",
		host,
		port))
	if err != nil {
		t.Fatal("should forward requests to ipfs host: ", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Error("the request should have succeeded")
	}

	defer res.Body.Close()
	resBytes, _ := ioutil.ReadAll(res.Body)

	var resp struct {
		Version string
	}
	err = json.Unmarshal(resBytes, &resp)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Version != "m.o.c.k" {
		t.Error("wrong version")
	}
}

func TestIPFSProxyPin(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	cfg := testingConfig()
	host, _ := cfg.IPFSProxyAddr.ValueForProtocol(ma.P_IP4)
	port, _ := cfg.IPFSProxyAddr.ValueForProtocol(ma.P_TCP)
	res, err := http.Get(fmt.Sprintf("http://%s:%s/api/v0/pin/add?arg=%s",
		host,
		port,
		testCid))
	if err != nil {
		t.Fatal("should have succeeded: ", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Error("the request should have succeeded")
	}
	resBytes, _ := ioutil.ReadAll(res.Body)

	var resp ipfsPinOpResp
	err = json.Unmarshal(resBytes, &resp)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Pins) != 1 || resp.Pins[0] != testCid {
		t.Error("wrong response")
	}
	res.Body.Close()

	// Try with a bad cid
	res, err = http.Get(fmt.Sprintf("http://%s:%s/api/v0/pin/add?arg=%s",
		host,
		port,
		errorCid))
	if err != nil {
		t.Fatal("request should work: ", err)
	}
	if res.StatusCode != http.StatusInternalServerError {
		t.Error("the request should return with InternalServerError")
	}

	resBytes, _ = ioutil.ReadAll(res.Body)
	var respErr ipfsError
	err = json.Unmarshal(resBytes, &respErr)
	if err != nil {
		t.Fatal(err)
	}

	if respErr.Message != errBadCid.Error() {
		t.Error("wrong response")
	}
	res.Body.Close()
}

func TestIPFSProxyUnpin(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	cfg := testingConfig()
	host, _ := cfg.IPFSProxyAddr.ValueForProtocol(ma.P_IP4)
	port, _ := cfg.IPFSProxyAddr.ValueForProtocol(ma.P_TCP)
	res, err := http.Get(fmt.Sprintf("http://%s:%s/api/v0/pin/rm?arg=%s",
		host,
		port,
		testCid))
	if err != nil {
		t.Fatal("should have succeeded: ", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Error("the request should have succeeded")
	}

	resBytes, _ := ioutil.ReadAll(res.Body)

	var resp ipfsPinOpResp
	err = json.Unmarshal(resBytes, &resp)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Pins) != 1 || resp.Pins[0] != testCid {
		t.Error("wrong response")
	}
	res.Body.Close()

	// Try with a bad cid
	res, err = http.Get(fmt.Sprintf("http://%s:%s/api/v0/pin/rm?arg=%s",
		host,
		port,
		errorCid))
	if err != nil {
		t.Fatal("request should work: ", err)
	}
	if res.StatusCode != http.StatusInternalServerError {
		t.Error("the request should return with InternalServerError")
	}

	resBytes, _ = ioutil.ReadAll(res.Body)
	var respErr ipfsError
	err = json.Unmarshal(resBytes, &respErr)
	if err != nil {
		t.Fatal(err)
	}

	if respErr.Message != errBadCid.Error() {
		t.Error("wrong response")
	}
	res.Body.Close()
}

func TestIPFSProxyPinLs(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	cfg := testingConfig()
	host, _ := cfg.IPFSProxyAddr.ValueForProtocol(ma.P_IP4)
	port, _ := cfg.IPFSProxyAddr.ValueForProtocol(ma.P_TCP)
	res, err := http.Get(fmt.Sprintf("http://%s:%s/api/v0/pin/ls?arg=%s",
		host,
		port,
		testCid))
	if err != nil {
		t.Fatal("should have succeeded: ", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Error("the request should have succeeded")
	}

	resBytes, _ := ioutil.ReadAll(res.Body)

	var resp ipfsPinLsResp
	err = json.Unmarshal(resBytes, &resp)
	if err != nil {
		t.Fatal(err)
	}

	_, ok := resp.Keys[testCid]
	if len(resp.Keys) != 1 || !ok {
		t.Error("wrong response")
	}
	res.Body.Close()

	res, err = http.Get(fmt.Sprintf("http://%s:%s/api/v0/pin/ls",
		host,
		port))
	if err != nil {
		t.Fatal("should have succeeded: ", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Error("the request should have succeeded")
	}

	resBytes, _ = ioutil.ReadAll(res.Body)
	err = json.Unmarshal(resBytes, &resp)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Keys) != 3 {
		t.Error("wrong response")
	}
	res.Body.Close()
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
