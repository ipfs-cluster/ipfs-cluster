package ipfshttp

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

func testIPFSConnector(t *testing.T) (*Connector, *test.IpfsMock) {
	mock := test.NewIpfsMock()
	nodeMAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d",
		mock.Addr, mock.Port))
	proxyMAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/10001")

	ipfs, err := NewConnector(nodeMAddr, proxyMAddr)
	if err != nil {
		t.Fatal("creating an IPFSConnector should work: ", err)
	}
	ipfs.SetClient(test.NewMockRPCClient(t))
	return ipfs, mock
}

func TestNewConnector(t *testing.T) {
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
	if id.ID != test.TestPeerID1 {
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
	c, _ := cid.Decode(test.TestCid1)
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

	c2, _ := cid.Decode(test.ErrorCid)
	err = ipfs.Pin(c2)
	if err == nil {
		t.Error("expected error pinning cid")
	}
}

func TestIPFSUnpin(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()
	c, _ := cid.Decode(test.TestCid1)
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
	c, _ := cid.Decode(test.TestCid1)
	c2, _ := cid.Decode(test.TestCid2)

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
	c, _ := cid.Decode(test.TestCid1)
	c2, _ := cid.Decode(test.TestCid2)

	ipfs.Pin(c)
	ipfs.Pin(c2)
	ipsMap, err := ipfs.PinLs("")
	if err != nil {
		t.Error("should not error")
	}

	if len(ipsMap) != 2 {
		t.Fatal("the map does not contain expected keys")
	}

	if !ipsMap[test.TestCid1].IsPinned() || !ipsMap[test.TestCid2].IsPinned() {
		t.Error("c1 and c2 should appear pinned")
	}
}

func TestIPFSProxyVersion(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	res, err := http.Get(fmt.Sprintf("%s/version", proxyURL(ipfs)))
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

	res, err := http.Get(fmt.Sprintf("%s/pin/add?arg=%s", proxyURL(ipfs), test.TestCid1))
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

	if len(resp.Pins) != 1 || resp.Pins[0] != test.TestCid1 {
		t.Error("wrong response")
	}
	res.Body.Close()

	// Try with a bad cid
	res, err = http.Get(fmt.Sprintf("%s/pin/add?arg=%s", proxyURL(ipfs), test.ErrorCid))
	if err != nil {
		t.Fatal("request should work: ", err)
	}
	t.Log(fmt.Sprintf("%s/pin/add?arg=%s", proxyURL(ipfs), test.ErrorCid))
	if res.StatusCode != http.StatusInternalServerError {
		t.Error("the request should return with InternalServerError")
	}

	resBytes, _ = ioutil.ReadAll(res.Body)
	var respErr ipfsError
	err = json.Unmarshal(resBytes, &respErr)
	if err != nil {
		t.Fatal(err)
	}

	if respErr.Message != test.ErrBadCid.Error() {
		t.Error("wrong response")
	}
	res.Body.Close()
}

func TestIPFSProxyUnpin(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	res, err := http.Get(fmt.Sprintf("%s/pin/rm?arg=%s", proxyURL(ipfs), test.TestCid1))
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

	if len(resp.Pins) != 1 || resp.Pins[0] != test.TestCid1 {
		t.Error("wrong response")
	}
	res.Body.Close()

	// Try with a bad cid
	res, err = http.Get(fmt.Sprintf("%s/pin/rm?arg=%s", proxyURL(ipfs), test.ErrorCid))
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

	if respErr.Message != test.ErrBadCid.Error() {
		t.Error("wrong response")
	}
	res.Body.Close()
}

func TestIPFSProxyPinLs(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	res, err := http.Get(fmt.Sprintf("%s/pin/ls?arg=%s", proxyURL(ipfs), test.TestCid1))
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

	_, ok := resp.Keys[test.TestCid1]
	if len(resp.Keys) != 1 || !ok {
		t.Error("wrong response")
	}
	res.Body.Close()

	res, err = http.Get(fmt.Sprintf("%s/pin/ls", proxyURL(ipfs)))
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

func proxyURL(c *Connector) string {
	_, addr, _ := manet.DialArgs(c.proxyMAddr)
	return fmt.Sprintf("http://%s/api/v0", addr)
}
