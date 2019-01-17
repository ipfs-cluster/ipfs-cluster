package ipfsproxy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/elastos/Elastos.NET.Hive.Cluster/api"
	"github.com/elastos/Elastos.NET.Hive.Cluster/test"

	logging "github.com/ipfs/go-log"
	ma "github.com/multiformats/go-multiaddr"
)

func init() {
	_ = logging.Logger
}

func testIPFSProxy(t *testing.T) (*Server, *test.IpfsMock) {
	mock := test.NewIpfsMock()
	nodeMAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d",
		mock.Addr, mock.Port))
	proxyMAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := &Config{}
	cfg.Default()
	cfg.NodeAddr = nodeMAddr
	cfg.ListenAddr = proxyMAddr

	proxy, err := New(cfg)
	if err != nil {
		t.Fatal("creating an IPFSProxy should work: ", err)
	}

	proxy.server.SetKeepAlivesEnabled(false)
	proxy.SetClient(test.NewMockRPCClient(t))
	return proxy, mock
}

func TestIPFSProxyVersion(t *testing.T) {
	proxy, mock := testIPFSProxy(t)
	defer mock.Close()
	defer proxy.Shutdown()

	res, err := http.Post(fmt.Sprintf("%s/version", proxyURL(proxy)), "", nil)
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
	proxy, mock := testIPFSProxy(t)
	defer mock.Close()
	defer proxy.Shutdown()

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
			u := fmt.Sprintf("%s%s%s", proxyURL(proxy), tt.args.urlPath, tt.args.testCid)
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
	proxy, mock := testIPFSProxy(t)
	defer mock.Close()
	defer proxy.Shutdown()

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
			u := fmt.Sprintf("%s%s%s", proxyURL(proxy), tt.args.urlPath, tt.args.testCid)
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
	proxy, mock := testIPFSProxy(t)
	defer mock.Close()
	defer proxy.Shutdown()

	t.Run("pin/ls query arg", func(t *testing.T) {
		res, err := http.Post(fmt.Sprintf("%s/pin/ls?arg=%s", proxyURL(proxy), test.TestCid1), "", nil)
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
		res, err := http.Post(fmt.Sprintf("%s/pin/ls/%s", proxyURL(proxy), test.TestCid1), "", nil)
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
		res2, err := http.Post(fmt.Sprintf("%s/pin/ls", proxyURL(proxy)), "", nil)
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
		res3, err := http.Post(fmt.Sprintf("%s/pin/ls?arg=%s", proxyURL(proxy), test.ErrorCid), "", nil)
		if err != nil {
			t.Fatal("should have succeeded: ", err)
		}
		defer res3.Body.Close()
		if res3.StatusCode != http.StatusInternalServerError {
			t.Error("the request should have failed")
		}
	})
}

func TestProxyRepoStat(t *testing.T) {
	proxy, mock := testIPFSProxy(t)
	defer mock.Close()
	defer proxy.Shutdown()
	res, err := http.Post(fmt.Sprintf("%s/repo/stat", proxyURL(proxy)), "", nil)
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Error("request should have succeeded")
	}

	resBytes, _ := ioutil.ReadAll(res.Body)
	var stat api.IPFSRepoStat
	err = json.Unmarshal(resBytes, &stat)
	if err != nil {
		t.Fatal(err)
	}

	// The mockRPC returns 3 peers. Since no host is set,
	// all calls are local.
	if stat.RepoSize != 6000 || stat.StorageMax != 300000 {
		t.Errorf("expected different stats: %+v", stat)
	}

}

func TestProxyAdd(t *testing.T) {
	proxy, mock := testIPFSProxy(t)
	defer mock.Close()
	defer proxy.Shutdown()

	type testcase struct {
		query       string
		expectedCid string
	}

	testcases := []testcase{
		testcase{
			query:       "",
			expectedCid: test.ShardingDirBalancedRootCID,
		},
		testcase{
			query:       "progress=true",
			expectedCid: test.ShardingDirBalancedRootCID,
		},
		testcase{
			query:       "wrap-with-directory=true",
			expectedCid: test.ShardingDirBalancedRootCIDWrapped,
		},
		testcase{
			query:       "trickle=true",
			expectedCid: test.ShardingDirTrickleRootCID,
		},
	}

	reqs := make([]*http.Request, len(testcases), len(testcases))

	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	for i, tc := range testcases {
		mr, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		cType := "multipart/form-data; boundary=" + mr.Boundary()
		url := fmt.Sprintf("%s/add?"+tc.query, proxyURL(proxy))
		req, _ := http.NewRequest("POST", url, mr)
		req.Header.Set("Content-Type", cType)
		reqs[i] = req
	}

	for i, tc := range testcases {
		t.Run(tc.query, func(t *testing.T) {
			res, err := http.DefaultClient.Do(reqs[i])
			if err != nil {
				t.Fatal("should have succeeded: ", err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("Bad response status: got = %d, want = %d", res.StatusCode, http.StatusOK)
			}

			var resp ipfsAddResp
			dec := json.NewDecoder(res.Body)
			for dec.More() {
				err := dec.Decode(&resp)
				if err != nil {
					t.Fatal(err)
				}
			}

			if resp.Hash != tc.expectedCid {
				t.Logf("%+v", resp.Hash)
				t.Error("expected CID does not match")
			}
		})
	}
}

func TestProxyAddError(t *testing.T) {
	proxy, mock := testIPFSProxy(t)
	defer mock.Close()
	defer proxy.Shutdown()
	res, err := http.Post(fmt.Sprintf("%s/add?recursive=true", proxyURL(proxy)), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("wrong status code: got = %d, want = %d", res.StatusCode, http.StatusInternalServerError)
	}
}

func TestProxyError(t *testing.T) {
	proxy, mock := testIPFSProxy(t)
	defer mock.Close()
	defer proxy.Shutdown()

	res, err := http.Post(fmt.Sprintf("%s/bad/command", proxyURL(proxy)), "", nil)
	if err != nil {
		t.Fatal("should have succeeded: ", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 404 {
		t.Error("should have respected the status code")
	}
}

func proxyURL(c *Server) string {
	addr := c.listener.Addr()
	return fmt.Sprintf("http://%s/api/v0", addr.String())
}

func TestIPFSProxy(t *testing.T) {
	proxy, mock := testIPFSProxy(t)
	defer mock.Close()
	if err := proxy.Shutdown(); err != nil {
		t.Error("expected a clean shutdown")
	}
	if err := proxy.Shutdown(); err != nil {
		t.Error("expected a second clean shutdown")
	}
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

func TestHeaderExtraction(t *testing.T) {
	proxy, mock := testIPFSProxy(t)
	defer mock.Close()
	defer proxy.Shutdown()

	res, err := http.Post(fmt.Sprintf("%s/pin/ls", proxyURL(proxy)), "", nil)
	if err != nil {
		t.Fatal("should forward requests to ipfs host: ", err)
	}
	res.Body.Close()

	if res.Header.Get("Access-Control-Allow-Headers") != "test-allow-header" {
		t.Error("the proxy should have extracted headers from ipfs")
	}

	if !strings.HasPrefix(res.Header.Get("Server"), "ipfs-cluster") {
		t.Error("wrong value for Server header")
	}
}
