package ipfshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"testing"
	"time"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
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

func testPin(t *testing.T, method string) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	ipfs.config.PinMethod = method

	c, _ := cid.Decode(test.TestCid1)
	err := ipfs.Pin(ctx, c, true)
	if err != nil {
		t.Error("expected success pinning cid")
	}
	pinSt, err := ipfs.PinLsCid(ctx, c)
	if err != nil {
		t.Fatal("expected success doing ls")
	}
	if !pinSt.IsPinned() {
		t.Error("cid should have been pinned")
	}

	c2, _ := cid.Decode(test.ErrorCid)
	err = ipfs.Pin(ctx, c2, true)
	if err == nil {
		t.Error("expected error pinning cid")
	}
}

func TestIPFSPin(t *testing.T) {
	t.Run("method=pin", func(t *testing.T) { testPin(t, "pin") })
	t.Run("method=refs", func(t *testing.T) { testPin(t, "refs") })
}

func TestIPFSUnpin(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()
	c, _ := cid.Decode(test.TestCid1)
	err := ipfs.Unpin(ctx, c)
	if err != nil {
		t.Error("expected success unpinning non-pinned cid")
	}
	ipfs.Pin(ctx, c, true)
	err = ipfs.Unpin(ctx, c)
	if err != nil {
		t.Error("expected success unpinning pinned cid")
	}
}

func TestIPFSPinLsCid(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()
	c, _ := cid.Decode(test.TestCid1)
	c2, _ := cid.Decode(test.TestCid2)

	ipfs.Pin(ctx, c, true)
	ips, err := ipfs.PinLsCid(ctx, c)
	if err != nil || !ips.IsPinned() {
		t.Error("c should appear pinned")
	}

	ips, err = ipfs.PinLsCid(ctx, c2)
	if err != nil || ips != api.IPFSPinStatusUnpinned {
		t.Error("c2 should appear unpinned")
	}
}

func TestIPFSPinLs(t *testing.T) {
	ctx := context.Background()
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()
	c, _ := cid.Decode(test.TestCid1)
	c2, _ := cid.Decode(test.TestCid2)

	ipfs.Pin(ctx, c, true)
	ipfs.Pin(ctx, c2, true)
	ipsMap, err := ipfs.PinLs(ctx, "")
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

	type args struct {
		urlPath    string
		testCid    string
		statusCode int
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			"pin good cid query arg",
			args{
				"/pin/add?arg=",
				test.TestCid1,
				http.StatusOK,
			},
			test.TestCid1,
			false,
		},
		{
			"pin good cid url arg",
			args{
				"/pin/add/",
				test.TestCid1,
				http.StatusOK,
			},
			test.TestCid1,
			false,
		},
		{
			"pin bad cid query arg",
			args{
				"/pin/add?arg=",
				test.ErrorCid,
				http.StatusInternalServerError,
			},
			"",
			true,
		},
		{
			"pin bad cid url arg",
			args{
				"/pin/add/",
				test.ErrorCid,
				http.StatusInternalServerError,
			},
			"",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := fmt.Sprintf("%s%s%s", proxyURL(ipfs), tt.args.urlPath, tt.args.testCid)
			res, err := http.Post(u, "", nil)
			if err != nil {
				t.Fatal("should have succeeded: ", err)
			}
			defer res.Body.Close()

			if res.StatusCode != tt.args.statusCode {
				t.Errorf("statusCode: got = %v, want %v", res.StatusCode, tt.args.statusCode)
			}

			resBytes, _ := ioutil.ReadAll(res.Body)

			switch tt.wantErr {
			case false:
				var resp ipfsPinOpResp
				err = json.Unmarshal(resBytes, &resp)
				if err != nil {
					t.Fatal(err)
				}

				if len(resp.Pins) != 1 {
					t.Fatalf("wrong number of pins: got = %d, want %d", len(resp.Pins), 1)
				}

				if resp.Pins[0] != tt.want {
					t.Errorf("wrong pin cid: got = %s, want = %s", resp.Pins[0], tt.want)
				}
			case true:
				var respErr ipfsError
				err = json.Unmarshal(resBytes, &respErr)
				if err != nil {
					t.Fatal(err)
				}

				if respErr.Message != test.ErrBadCid.Error() {
					t.Errorf("wrong response: got = %s, want = %s", respErr.Message, test.ErrBadCid.Error())
				}
			}
		})
	}
}

func TestIPFSProxyUnpin(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	type args struct {
		urlPath    string
		testCid    string
		statusCode int
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			"unpin good cid query arg",
			args{
				"/pin/rm?arg=",
				test.TestCid1,
				http.StatusOK,
			},
			test.TestCid1,
			false,
		},
		{
			"unpin good cid url arg",
			args{
				"/pin/rm/",
				test.TestCid1,
				http.StatusOK,
			},
			test.TestCid1,
			false,
		},
		{
			"unpin bad cid query arg",
			args{
				"/pin/rm?arg=",
				test.ErrorCid,
				http.StatusInternalServerError,
			},
			"",
			true,
		},
		{
			"unpin bad cid url arg",
			args{
				"/pin/rm/",
				test.ErrorCid,
				http.StatusInternalServerError,
			},
			"",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := fmt.Sprintf("%s%s%s", proxyURL(ipfs), tt.args.urlPath, tt.args.testCid)
			res, err := http.Post(u, "", nil)
			if err != nil {
				t.Fatal("should have succeeded: ", err)
			}
			defer res.Body.Close()

			if res.StatusCode != tt.args.statusCode {
				t.Errorf("statusCode: got = %v, want %v", res.StatusCode, tt.args.statusCode)
			}

			resBytes, _ := ioutil.ReadAll(res.Body)

			switch tt.wantErr {
			case false:
				var resp ipfsPinOpResp
				err = json.Unmarshal(resBytes, &resp)
				if err != nil {
					t.Fatal(err)
				}

				if len(resp.Pins) != 1 {
					t.Fatalf("wrong number of pins: got = %d, want %d", len(resp.Pins), 1)
				}

				if resp.Pins[0] != tt.want {
					t.Errorf("wrong pin cid: got = %s, want = %s", resp.Pins[0], tt.want)
				}
			case true:
				var respErr ipfsError
				err = json.Unmarshal(resBytes, &respErr)
				if err != nil {
					t.Fatal(err)
				}

				if respErr.Message != test.ErrBadCid.Error() {
					t.Errorf("wrong response: got = %s, want = %s", respErr.Message, test.ErrBadCid.Error())
				}
			}
		})
	}
}

func TestIPFSProxyPinLs(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()

	t.Run("pin/ls query arg", func(t *testing.T) {
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
	})

	t.Run("pin/ls url arg", func(t *testing.T) {
		res, err := http.Post(fmt.Sprintf("%s/pin/ls/%s", proxyURL(ipfs), test.TestCid1), "", nil)
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
	})

	t.Run("pin/ls all no arg", func(t *testing.T) {
		res2, err := http.Post(fmt.Sprintf("%s/pin/ls", proxyURL(ipfs)), "", nil)
		if err != nil {
			t.Fatal("should have succeeded: ", err)
		}
		defer res2.Body.Close()
		if res2.StatusCode != http.StatusOK {
			t.Error("the request should have succeeded")
		}

		resBytes, _ := ioutil.ReadAll(res2.Body)
		var resp ipfsPinLsResp
		err = json.Unmarshal(resBytes, &resp)
		if err != nil {
			t.Fatal(err)
		}

		if len(resp.Keys) != 3 {
			t.Error("wrong response")
		}
	})

	t.Run("pin/ls bad cid query arg", func(t *testing.T) {
		res3, err := http.Post(fmt.Sprintf("%s/pin/ls?arg=%s", proxyURL(ipfs), test.ErrorCid), "", nil)
		if err != nil {
			t.Fatal("should have succeeded: ", err)
		}
		defer res3.Body.Close()
		if res3.StatusCode != http.StatusInternalServerError {
			t.Error("the request should have failed")
		}
	})
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
		t.Run(urlQueries[i], func(t *testing.T) {
			res, err := http.DefaultClient.Do(reqs[i])
			if err != nil {
				t.Fatal("should have succeeded: ", err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("Bad response status: got = %d, want = %d", res.StatusCode, http.StatusOK)
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
		})
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
		t.Errorf("wrong status code: got = %d, want = %d", res.StatusCode, http.StatusInternalServerError)
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

func TestBlockGet(t *testing.T) {
	ipfs, mock := testIPFSConnector(t)
	defer mock.Close()
	defer ipfs.Shutdown()
	shardCid, err := cid.Decode(test.TestShardCid)
	data, err := ipfs.BlockGet(shardCid)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, test.TestShardData) {
		t.Fatal("unexpected data returned")
	}
}

func TestRepoSize(t *testing.T) {
	ctx := context.Background()
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
	err = ipfs.Pin(ctx, c, true)
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

func Test_extractArgument(t *testing.T) {
	type args struct {
		handlePath string
		u          *url.URL
	}
	tests := []struct {
		name  string
		args  args
		want  string
		want1 bool
	}{
		{
			"pin/add url arg",
			args{
				"add",
				mustParseURL(fmt.Sprintf("/api/v0/pin/add/%s", test.TestCid1)),
			},
			test.TestCid1,
			true,
		},
		{
			"pin/add query arg",
			args{
				"add",
				mustParseURL(fmt.Sprintf("/api/v0/pin/add?arg=%s", test.TestCid1)),
			},
			test.TestCid1,
			true,
		},
		{
			"pin/ls url arg",
			args{
				"pin/ls",
				mustParseURL(fmt.Sprintf("/api/v0/pin/ls/%s", test.TestCid1)),
			},
			test.TestCid1,
			true,
		},
		{
			"pin/ls query arg",
			args{
				"pin/ls",
				mustParseURL(fmt.Sprintf("/api/v0/pin/ls?arg=%s", test.TestCid1)),
			},
			test.TestCid1,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := extractArgument(tt.args.u)
			if got != tt.want {
				t.Errorf("extractCid() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("extractCid() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func mustParseURL(rawurl string) *url.URL {
	u, err := url.Parse(rawurl)
	if err != nil {
		panic(err)
	}
	return u
}
