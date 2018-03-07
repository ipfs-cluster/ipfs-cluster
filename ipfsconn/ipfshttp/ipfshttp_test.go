package ipfshttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	ma "github.com/multiformats/go-multiaddr"
)

func init() {
	_ = logging.Logger
}

func testIPFSConnector(t *testing.T) (*Connector, *test.IpfsMock) {
	mock := test.NewIpfsMock()
	nodeMAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d",
		mock.Addr, mock.Port))
	proxyMAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := &Config{}
	cfg.Default()
	cfg.NodeAddr = nodeMAddr
	cfg.ProxyAddr = proxyMAddr
	cfg.ConnectSwarmsDelay = 0

	ipfs, err := NewConnector(cfg)
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

	res, err := http.Post(fmt.Sprintf("%s/version", proxyURL(ipfs)), "", nil)
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

	res, err := http.Post(fmt.Sprintf("%s/pin/add?arg=%s", proxyURL(ipfs), test.TestCid1), "", nil)
	if err != nil {
		t.Fatal("should have succeeded: ", err)
	}
	defer res.Body.Close()

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

	// Try with a bad cid
	res2, err := http.Post(fmt.Sprintf("%s/pin/add?arg=%s", proxyURL(ipfs), test.ErrorCid), "", nil)
	if err != nil {
		t.Fatal("request should work: ", err)
	}
	defer res2.Body.Close()

	t.Log(fmt.Sprintf("%s/pin/add?arg=%s", proxyURL(ipfs), test.ErrorCid))
	if res2.StatusCode != http.StatusInternalServerError {
		t.Error("the request should return with InternalServerError")
	}

	resBytes, _ = ioutil.ReadAll(res2.Body)
	var respErr ipfsError
	err = json.Unmarshal(resBytes, &respErr)
	if err != nil {
		t.Fatal(err)
	}

	if respErr.Message != test.ErrBadCid.Error() {
		t.Error("wrong response")
	}
}

func TestIPFSProxyUnpin(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	res, err := http.Post(fmt.Sprintf("%s/pin/rm?arg=%s", proxyURL(ipfs), test.TestCid1), "", nil)
	if err != nil {
		t.Fatal("should have succeeded: ", err)
	}
	defer res.Body.Close()

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

	// Try with a bad cid
	res2, err := http.Post(fmt.Sprintf("%s/pin/rm?arg=%s", proxyURL(ipfs), test.ErrorCid), "", nil)
	if err != nil {
		t.Fatal("request should work: ", err)
	}
	defer res2.Body.Close()

	if res2.StatusCode != http.StatusInternalServerError {
		t.Error("the request should return with InternalServerError")
	}

	resBytes, _ = ioutil.ReadAll(res2.Body)
	var respErr ipfsError
	err = json.Unmarshal(resBytes, &respErr)
	if err != nil {
		t.Fatal(err)
	}

	if respErr.Message != test.ErrBadCid.Error() {
		t.Error("wrong response")
	}
}

func TestIPFSProxyPinLs(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	res, err := http.Post(fmt.Sprintf("%s/pin/ls?arg=%s", proxyURL(ipfs), test.TestCid1), "", nil)
	if err != nil {
		t.Fatal("should have succeeded: ", err)
	}
	defer res.Body.Close()
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

	res2, err := http.Post(fmt.Sprintf("%s/pin/ls", proxyURL(ipfs)), "", nil)
	if err != nil {
		t.Fatal("should have succeeded: ", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Error("the request should have succeeded")
	}

	resBytes, _ = ioutil.ReadAll(res2.Body)
	err = json.Unmarshal(resBytes, &resp)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Keys) != 3 {
		t.Error("wrong response")
	}

	res3, err := http.Post(fmt.Sprintf("%s/pin/ls?arg=%s", proxyURL(ipfs), test.ErrorCid), "", nil)
	if err != nil {
		t.Fatal("should have succeeded: ", err)
	}
	defer res3.Body.Close()
	if res3.StatusCode != http.StatusInternalServerError {
		t.Error("the request should have failed")
	}
}

func TestProxyAdd(t *testing.T) {
	// TODO: find a way to ensure that the calls to
	// rpc-api "Pin" happened.
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	urlQueries := []string{
		"",
		"pin=false",
		"progress=true",
		"wrap-with-directory",
	}

	reqs := make([]*http.Request, len(urlQueries), len(urlQueries))

	for i := 0; i < len(urlQueries); i++ {
		body := new(bytes.Buffer)
		w := multipart.NewWriter(body)
		part, err := w.CreateFormFile("file", "testfile")
		if err != nil {
			t.Fatal(err)
		}
		_, err = part.Write([]byte("this is a multipart file"))
		if err != nil {
			t.Fatal(err)
		}
		err = w.Close()
		if err != nil {
			t.Fatal(err)
		}
		url := fmt.Sprintf("%s/add?"+urlQueries[i], proxyURL(ipfs))
		req, _ := http.NewRequest("POST", url, body)
		req.Header.Set("Content-Type", w.FormDataContentType())
		reqs[i] = req
	}

	for i := 0; i < len(urlQueries); i++ {
		res, err := http.DefaultClient.Do(reqs[i])
		if err != nil {
			t.Fatal("should have succeeded: ", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatal("Bad response status")
		}

		var hash ipfsAddResp

		// We might return a progress notification, so we do it
		// like this to ignore it easily
		dec := json.NewDecoder(res.Body)
		for dec.More() {
			var resp ipfsAddResp
			err := dec.Decode(&resp)
			if err != nil {
				t.Fatal(err)
			}

			if resp.Bytes != 0 {
				continue
			} else {
				hash = resp
			}
		}

		if hash.Hash != test.TestCid3 {
			t.Logf("%+v", hash)
			t.Error("expected TestCid1 as it is hardcoded in ipfs mock")
		}
		if hash.Name != "testfile" {
			t.Logf("%+v", hash)
			t.Error("expected testfile for hash name")
		}
	}
}

func TestProxyAddError(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()
	res, err := http.Post(fmt.Sprintf("%s/add?recursive=true", proxyURL(ipfs)), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Log(res.StatusCode)
		t.Error("expected an error")
	}
}

func TestDecideRecursivePins(t *testing.T) {
	type testcases struct {
		addResps []ipfsAddResp
		query    url.Values
		expect   []string
	}

	tcs := []testcases{
		{
			[]ipfsAddResp{{"a", "cida", 0}},
			url.Values{},
			[]string{"cida"},
		},
		{
			[]ipfsAddResp{{"a/b", "cidb", 0}, {"a", "cida", 0}},
			url.Values{},
			[]string{"cida"},
		},
		{
			[]ipfsAddResp{{"a/b", "cidb", 0}, {"c", "cidc", 0}, {"a", "cida", 0}},
			url.Values{},
			[]string{"cidc", "cida"},
		},
		{
			[]ipfsAddResp{{"/a", "cida", 0}},
			url.Values{},
			[]string{"cida"},
		},
		{
			[]ipfsAddResp{{"a/b/c/d", "cidd", 0}},
			url.Values{},
			[]string{"cidd"},
		},
		{
			[]ipfsAddResp{{"a", "cida", 0}, {"b", "cidb", 0}, {"c", "cidc", 0}, {"d", "cidd", 0}},
			url.Values{},
			[]string{"cida", "cidb", "cidc", "cidd"},
		},
		{
			[]ipfsAddResp{{"a", "cida", 0}, {"b", "cidb", 0}, {"", "cidwrap", 0}},
			url.Values{
				"wrap-in-directory": []string{"true"},
			},
			[]string{"cidwrap"},
		},
		{

			[]ipfsAddResp{{"b", "", 0}, {"a", "cida", 0}},
			url.Values{},
			[]string{"cida"},
		},
	}

	for i, tc := range tcs {
		r := decideRecursivePins(tc.addResps, tc.query)
		for j, ritem := range r {
			if len(r) != len(tc.expect) {
				t.Errorf("testcase %d failed", i)
				break
			}
			if tc.expect[j] != ritem {
				t.Errorf("testcase %d failed for item %d", i, j)
			}
		}
	}
}

func TestProxyError(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	res, err := http.Post(fmt.Sprintf("%s/bad/command", proxyURL(ipfs)), "", nil)
	if err != nil {
		t.Fatal("should have succeeded: ", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 404 {
		t.Error("should have respected the status code")
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

func TestConnectSwarms(t *testing.T) {
	// In order to interactively test uncomment the following.
	// Otherwise there is no good way to test this with the
	// ipfs mock
	// logging.SetDebugLogging()

	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()
	time.Sleep(time.Second)
}

func TestSwarmPeers(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	swarmPeers, err := ipfs.SwarmPeers()
	if err != nil {
		t.Fatal(err)
	}
	if len(swarmPeers) != 2 {
		t.Fatal("expected 2 swarm peers")
	}
	if swarmPeers[0] != test.TestPeerID4 {
		t.Error("unexpected swarm peer")
	}
	if swarmPeers[1] != test.TestPeerID5 {
		t.Error("unexpected swarm peer")
	}
}

func TestBlockPut(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	data := []byte(test.TestCid4Data)
	resp, err := ipfs.BlockPut(api.NodeWithMeta{
		Data:   data,
		Format: "protobuf",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp != test.TestCid4 {
		t.Fatal("Unexpected resulting cid")
	}
}

func TestRepoSize(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	s, err := ipfs.RepoSize()
	if err != nil {
		t.Fatal(err)
	}
	// See the ipfs mock implementation
	if s != 0 {
		t.Error("expected 0 bytes of size")
	}

	c, _ := cid.Decode(test.TestCid1)
	err = ipfs.Pin(c)
	if err != nil {
		t.Error("expected success pinning cid")
	}

	s, err = ipfs.RepoSize()
	if err != nil {
		t.Fatal(err)
	}
	if s != 1000 {
		t.Error("expected 1000 bytes of size")
	}
}

func TestConfigKey(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	v, err := ipfs.ConfigKey("Datastore/StorageMax")
	if err != nil {
		t.Fatal(err)
	}
	sto, ok := v.(string)
	if !ok {
		t.Fatal("error converting to string")
	}
	if sto != "10G" {
		t.Error("StorageMax shouold be 10G")
	}

	v, err = ipfs.ConfigKey("Datastore")
	_, ok = v.(map[string]interface{})
	if !ok {
		t.Error("should have returned the whole Datastore config object")
	}

	_, err = ipfs.ConfigKey("")
	if err == nil {
		t.Error("should not work with an empty path")
	}

	_, err = ipfs.ConfigKey("Datastore/abc")
	if err == nil {
		t.Error("should not work with a bad path")
	}
}

func proxyURL(c *Connector) string {
	addr := c.listener.Addr()
	return fmt.Sprintf("http://%s/api/v0", addr.String())
}
