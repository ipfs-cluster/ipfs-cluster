package ipfscluster

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

func testServer(t *testing.T) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//t.Log(r.URL.String())
		switch r.URL.Path {
		case "/api/v0/pin/add":
			if r.URL.RawQuery == fmt.Sprintf("arg=%s", testCid) {
				fmt.Fprintln(w, `{ "pinned": "`+testCid+`" }`)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
		case "/api/v0/pin/rm":
			if r.URL.RawQuery == fmt.Sprintf("arg=%s", testCid) {
				fmt.Fprintln(w, `{ "unpinned": "`+testCid+`" }`)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
		case "/api/v0/pin/ls":
			if r.URL.RawQuery == fmt.Sprintf("arg=%s", testCid) {
				fmt.Fprintln(w,
					`{"Keys":{"`+testCid+`":{"Type":"recursive"}}}`)
			} else {
				fmt.Fprintln(w,
					`{"Keys":{"`+testCid2+`":{"Type":"indirect"}}}`)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Log("test server url: ", ts.URL)
	return ts
}

func testIPFSConnectorConfig(ts *httptest.Server) *ClusterConfig {
	url, _ := url.Parse(ts.URL)
	h := strings.Split(url.Host, ":")
	i, _ := strconv.Atoi(h[1])

	return &ClusterConfig{
		IPFSHost:          h[0],
		IPFSPort:          i,
		IPFSAPIListenAddr: "127.0.0.1",
		IPFSAPIListenPort: 5000,
	}
}

func ipfsConnector(t *testing.T) (*IPFSHTTPConnector, *httptest.Server) {
	ts := testServer(t)
	cfg := testIPFSConnectorConfig(ts)

	ipfs, err := NewIPFSHTTPConnector(cfg)
	if err != nil {
		t.Fatal("creating an IPFSConnector should work: ", err)
	}
	return ipfs, ts
}

func TestNewIPFSHTTPConnector(t *testing.T) {
	ipfs, ts := ipfsConnector(t)
	defer ts.Close()
	defer ipfs.Shutdown()

	ch := ipfs.RpcChan()
	if ch == nil {
		t.Error("RpcCh should be created")
	}
}

func TestPin(t *testing.T) {
	ipfs, ts := ipfsConnector(t)
	defer ts.Close()
	defer ipfs.Shutdown()
	c, _ := cid.Decode(testCid)
	c2, _ := cid.Decode(testCid2)
	err := ipfs.Pin(c)
	if err != nil {
		t.Error("expected success pinning cid")
	}
	err = ipfs.Pin(c2)
	if err == nil {
		t.Error("expected error pinning cid")
	}
}

func TestUnpin(t *testing.T) {
	ipfs, ts := ipfsConnector(t)
	defer ts.Close()
	defer ipfs.Shutdown()
	c, _ := cid.Decode(testCid)
	c2, _ := cid.Decode(testCid2)
	c3, _ := cid.Decode(testCid3)
	err := ipfs.Unpin(c)
	if err != nil {
		t.Error("expected success unpinning cid")
	}

	err = ipfs.Unpin(c2)
	if err != nil {
		t.Error("expected error unpinning cid")
	}

	err = ipfs.Unpin(c3)
	if err == nil {
		t.Error("expected error unpinning cid")
	}
}

func TestIsPinned(t *testing.T) {
	ipfs, ts := ipfsConnector(t)
	defer ts.Close()
	defer ipfs.Shutdown()
	c, _ := cid.Decode(testCid)
	c2, _ := cid.Decode(testCid2)

	isp, err := ipfs.IsPinned(c)
	if err != nil || !isp {
		t.Error("c should appear pinned")
	}

	isp, err = ipfs.IsPinned(c2)
	if err != nil || isp {
		t.Error("c2 should appear unpinned")
	}
}

func TestProxy(t *testing.T) {
	ipfs, ts := ipfsConnector(t)
	defer ts.Close()
	defer ipfs.Shutdown()

	res, err := http.Get("http://127.0.0.1:5000/api/v0/add?arg=" + testCid)
	if err != nil {
		t.Fatal("should forward requests to ipfs host: ", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Error("the request should have succeeded")
	}
}

func TestShutdown(t *testing.T) {
	ipfs, ts := ipfsConnector(t)
	defer ts.Close()
	if err := ipfs.Shutdown(); err != nil {
		t.Error("expected a clean shutdown")
	}
}
