package rest

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"

	p2phttp "github.com/hsanjuan/go-libp2p-http"
	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	SSLCertFile = "test/server.crt"
	SSLKeyFile  = "test/server.key"
)

func testAPI(t *testing.T) *API {
	apiMAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	h, err := libp2p.New(context.Background(), libp2p.ListenAddrs(apiMAddr))
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{}
	cfg.Default()
	cfg.HTTPListenAddr = apiMAddr

	rest, err := NewAPIWithHost(cfg, h)
	if err != nil {
		t.Fatal("should be able to create a new API: ", err)
	}

	// No keep alive for tests
	rest.server.SetKeepAlivesEnabled(false)
	rest.SetClient(test.NewMockRPCClient(t))

	return rest
}

func testHTTPSAPI(t *testing.T) *API {
	apiMAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	h, err := libp2p.New(context.Background(), libp2p.ListenAddrs(apiMAddr))
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{}
	cfg.Default()
	cfg.pathSSLCertFile = SSLCertFile
	cfg.pathSSLKeyFile = SSLKeyFile
	cfg.TLS, err = newTLSConfig(cfg.pathSSLCertFile, cfg.pathSSLKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	cfg.HTTPListenAddr = apiMAddr

	rest, err := NewAPIWithHost(cfg, h)
	if err != nil {
		t.Fatal("should be able to create a new https Api: ", err)
	}

	// No keep alive for tests
	rest.server.SetKeepAlivesEnabled(false)
	rest.SetClient(test.NewMockRPCClient(t))

	return rest
}

func processResp(t *testing.T, httpResp *http.Response, err error, resp interface{}) {
	if err != nil {
		t.Fatal("error making request: ", err)
	}
	body, err := ioutil.ReadAll(httpResp.Body)
	defer httpResp.Body.Close()
	if err != nil {
		t.Fatal("error reading body: ", err)
	}

	if len(body) != 0 {
		err = json.Unmarshal(body, resp)
		if err != nil {
			t.Error(string(body))
			t.Fatal("error parsing json: ", err)
		}
	}
}

func processStreamingResp(t *testing.T, httpResp *http.Response, err error, resp interface{}) {
	if err != nil {
		t.Fatal("error making streaming request: ", err)
	}

	if httpResp.StatusCode > 399 {
		// normal response with error
		processResp(t, httpResp, err, resp)
		return
	}

	defer httpResp.Body.Close()
	dec := json.NewDecoder(httpResp.Body)
	for {
		err := dec.Decode(&resp)
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func checkHeaders(t *testing.T, rest *API, url string, headers http.Header) {
	for k, v := range rest.config.Headers {
		if strings.Join(v, ",") != strings.Join(headers[k], ",") {
			t.Errorf("%s does not show configured headers: %s", url, k)
		}
	}
	if headers.Get("Content-Type") != "application/json" {
		t.Errorf("%s is not application/json", url)
	}
}

// makes a libp2p host that knows how to talk to the rest API host.
func makeHost(t *testing.T, rest *API) host.Host {
	h, err := libp2p.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	h.Peerstore().AddAddrs(
		rest.Host().ID(),
		rest.Host().Addrs(),
		peerstore.PermanentAddrTTL,
	)
	return h
}

type urlF func(a *API) string

func httpURL(a *API) string {
	u, _ := a.HTTPAddress()
	return fmt.Sprintf("http://%s", u)
}

func p2pURL(a *API) string {
	return fmt.Sprintf("libp2p://%s", peer.IDB58Encode(a.Host().ID()))
}

func httpsURL(a *API) string {
	u, _ := a.HTTPAddress()
	return fmt.Sprintf("https://%s", u)
}

func isHTTPS(url string) bool {
	return strings.HasPrefix(url, "https")
}

// supports both http/https and libp2p-tunneled-http
func httpClient(t *testing.T, h host.Host, isHTTPS bool) *http.Client {
	tr := &http.Transport{}
	if isHTTPS {
		certpool := x509.NewCertPool()
		cert, err := ioutil.ReadFile(SSLCertFile)
		if err != nil {
			t.Fatal("error reading cert for https client: ", err)
		}
		certpool.AppendCertsFromPEM(cert)
		tr = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: certpool,
			}}
	}
	if h != nil {
		tr.RegisterProtocol("libp2p", p2phttp.NewTransport(h))
	}
	return &http.Client{Transport: tr}
}

func makeGet(t *testing.T, rest *API, url string, resp interface{}) {
	h := makeHost(t, rest)
	defer h.Close()
	c := httpClient(t, h, isHTTPS(url))
	httpResp, err := c.Get(url)
	processResp(t, httpResp, err, resp)
	checkHeaders(t, rest, url, httpResp.Header)
}

func makePost(t *testing.T, rest *API, url string, body []byte, resp interface{}) {
	makePostWithContentType(t, rest, url, body, "application/json", resp)
}

func makePostWithContentType(t *testing.T, rest *API, url string, body []byte, contentType string, resp interface{}) {
	h := makeHost(t, rest)
	defer h.Close()
	c := httpClient(t, h, isHTTPS(url))
	httpResp, err := c.Post(url, contentType, bytes.NewReader(body))
	processResp(t, httpResp, err, resp)
	checkHeaders(t, rest, url, httpResp.Header)
}

func makeDelete(t *testing.T, rest *API, url string, resp interface{}) {
	h := makeHost(t, rest)
	defer h.Close()
	c := httpClient(t, h, isHTTPS(url))
	req, _ := http.NewRequest("DELETE", url, bytes.NewReader([]byte{}))
	httpResp, err := c.Do(req)
	processResp(t, httpResp, err, resp)
	checkHeaders(t, rest, url, httpResp.Header)
}

func makeStreamingPost(t *testing.T, rest *API, url string, body io.Reader, contentType string, resp interface{}) {
	h := makeHost(t, rest)
	defer h.Close()
	c := httpClient(t, h, isHTTPS(url))
	httpResp, err := c.Post(url, contentType, body)
	processStreamingResp(t, httpResp, err, resp)
	checkHeaders(t, rest, url, httpResp.Header)
}

type testF func(t *testing.T, url urlF)

func testBothEndpoints(t *testing.T, test testF) {
	t.Run("in-parallel", func(t *testing.T) {
		t.Run("http", func(t *testing.T) {
			t.Parallel()
			test(t, httpURL)
		})
		t.Run("libp2p", func(t *testing.T) {
			t.Parallel()
			test(t, p2pURL)
		})
	})
}

func testHTTPSEndPoint(t *testing.T, test testF) {
	t.Run("in-parallel", func(t *testing.T) {
		t.Run("https", func(t *testing.T) {
			t.Parallel()
			test(t, httpsURL)
		})
	})
}

func TestAPIShutdown(t *testing.T) {
	rest := testAPI(t)
	err := rest.Shutdown()
	if err != nil {
		t.Error("should shutdown cleanly: ", err)
	}
	// test shutting down twice
	rest.Shutdown()

}

func TestRestAPIIDEndpoint(t *testing.T) {
	rest := testAPI(t)
	httpsrest := testHTTPSAPI(t)
	defer rest.Shutdown()
	defer httpsrest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		id := api.IDSerial{}
		makeGet(t, rest, url(rest)+"/id", &id)
		if id.ID != test.TestPeerID1.Pretty() {
			t.Error("expected correct id")
		}
	}

	httpstf := func(t *testing.T, url urlF) {
		id := api.IDSerial{}
		makeGet(t, httpsrest, url(httpsrest)+"/id", &id)
		if id.ID != test.TestPeerID1.Pretty() {
			t.Error("expected correct id")
		}
	}

	testBothEndpoints(t, tf)
	testHTTPSEndPoint(t, httpstf)
}

func TestAPIVersionEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		ver := api.Version{}
		makeGet(t, rest, url(rest)+"/version", &ver)
		if ver.Version != "0.0.mock" {
			t.Error("expected correct version")
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIPeerstEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		var list []api.IDSerial
		makeGet(t, rest, url(rest)+"/peers", &list)
		if len(list) != 1 {
			t.Fatal("expected 1 element")
		}
		if list[0].ID != test.TestPeerID1.Pretty() {
			t.Error("expected a different peer id list: ", list)
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIPeerAddEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		id := api.IDSerial{}
		// post with valid body
		body := fmt.Sprintf("{\"peer_id\":\"%s\"}", test.TestPeerID1.Pretty())
		t.Log(body)
		makePost(t, rest, url(rest)+"/peers", []byte(body), &id)

		if id.ID != test.TestPeerID1.Pretty() {
			t.Error("expected correct ID")
		}
		if id.Error != "" {
			t.Error("did not expect an error")
		}

		// Send invalid body
		errResp := api.Error{}
		makePost(t, rest, url(rest)+"/peers", []byte("oeoeoeoe"), &errResp)
		if errResp.Code != 400 {
			t.Error("expected error with bad body")
		}
		// Send invalid peer id
		makePost(t, rest, url(rest)+"/peers", []byte("{\"peer_id\": \"ab\"}"), &errResp)
		if errResp.Code != 400 {
			t.Error("expected error with bad peer_id")
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIAddFileEndpointBadContentType(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		fmtStr1 := "/add?shard=true&repl_min=-1&repl_max=-1"
		localURL := url(rest) + fmtStr1

		errResp := api.Error{}
		makePost(t, rest, localURL, []byte("test"), &errResp)

		if errResp.Code != 400 {
			t.Error("expected error with bad content-type")
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIAddFileEndpointLocal(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	// This generates the testing files and
	// writes them to disk.
	// This is necessary here because we run tests
	// in parallel, and otherwise a write-race might happen.
	_, closer := sth.GetTreeMultiReader(t)
	closer.Close()

	tf := func(t *testing.T, url urlF) {
		fmtStr1 := "/add?shard=false&repl_min=-1&repl_max=-1&stream-channels=true"
		localURL := url(rest) + fmtStr1
		body, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		resp := api.AddedOutput{}
		mpContentType := "multipart/form-data; boundary=" + body.Boundary()
		makeStreamingPost(t, rest, localURL, body, mpContentType, &resp)

		// resp will contain the last object from the streaming
		if resp.Cid != test.ShardingDirBalancedRootCID {
			t.Error("Bad Cid after adding: ", resp.Cid)
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIAddFileEndpointShard(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	// This generates the testing files and
	// writes them to disk.
	// This is necessary here because we run tests
	// in parallel, and otherwise a write-race might happen.
	_, closer := sth.GetTreeMultiReader(t)
	closer.Close()

	tf := func(t *testing.T, url urlF) {
		body, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		mpContentType := "multipart/form-data; boundary=" + body.Boundary()
		resp := api.AddedOutput{}
		fmtStr1 := "/add?shard=true&repl_min=-1&repl_max=-1&stream-channels=true"
		shardURL := url(rest) + fmtStr1
		makeStreamingPost(t, rest, shardURL, body, mpContentType, &resp)
	}

	testBothEndpoints(t, tf)
}

func TestAPIAddFileEndpoint_StreamChannelsFalse(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	// This generates the testing files and
	// writes them to disk.
	// This is necessary here because we run tests
	// in parallel, and otherwise a write-race might happen.
	_, closer := sth.GetTreeMultiReader(t)
	closer.Close()

	tf := func(t *testing.T, url urlF) {
		body, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		fullBody, err := ioutil.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}
		mpContentType := "multipart/form-data; boundary=" + body.Boundary()
		resp := []api.AddedOutput{}
		fmtStr1 := "/add?shard=false&repl_min=-1&repl_max=-1&stream-channels=false"
		shardURL := url(rest) + fmtStr1

		makePostWithContentType(t, rest, shardURL, fullBody, mpContentType, &resp)
		lastHash := resp[len(resp)-1]
		if lastHash.Cid != test.ShardingDirBalancedRootCID {
			t.Error("Bad Cid after adding: ", lastHash.Cid)
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIPeerRemoveEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		makeDelete(t, rest, url(rest)+"/peers/"+test.TestPeerID1.Pretty(), &struct{}{})
	}

	testBothEndpoints(t, tf)
}

func TestConnectGraphEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		var cg api.ConnectGraphSerial
		makeGet(t, rest, url(rest)+"/health/graph", &cg)
		if cg.ClusterID != test.TestPeerID1.Pretty() {
			t.Error("unexpected cluster id")
		}
		if len(cg.IPFSLinks) != 3 {
			t.Error("unexpected number of ipfs peers")
		}
		if len(cg.ClusterLinks) != 3 {
			t.Error("unexpected number of cluster peers")
		}
		if len(cg.ClustertoIPFS) != 3 {
			t.Error("unexpected number of cluster to ipfs links")
		}
		// test a few link values
		pid1 := test.TestPeerID1.Pretty()
		pid4 := test.TestPeerID4.Pretty()
		if _, ok := cg.ClustertoIPFS[pid1]; !ok {
			t.Fatal("missing cluster peer 1 from cluster to peer links map")
		}
		if cg.ClustertoIPFS[pid1] != pid4 {
			t.Error("unexpected ipfs peer mapped to cluster peer 1 in graph")
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIPinEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		// test regular post
		makePost(t, rest, url(rest)+"/pins/"+test.TestCid1, []byte{}, &struct{}{})

		errResp := api.Error{}
		makePost(t, rest, url(rest)+"/pins/"+test.ErrorCid, []byte{}, &errResp)
		if errResp.Message != test.ErrBadCid.Error() {
			t.Error("expected different error: ", errResp.Message)
		}

		makePost(t, rest, url(rest)+"/pins/abcd", []byte{}, &errResp)
		if errResp.Code != 400 {
			t.Error("should fail with bad Cid")
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIUnpinEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		// test regular delete
		makeDelete(t, rest, url(rest)+"/pins/"+test.TestCid1, &struct{}{})

		errResp := api.Error{}
		makeDelete(t, rest, url(rest)+"/pins/"+test.ErrorCid, &errResp)
		if errResp.Message != test.ErrBadCid.Error() {
			t.Error("expected different error: ", errResp.Message)
		}

		makeDelete(t, rest, url(rest)+"/pins/abcd", &errResp)
		if errResp.Code != 400 {
			t.Error("should fail with bad Cid")
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIAllocationsEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		var resp []api.PinSerial
		makeGet(t, rest, url(rest)+"/allocations?filter=pin,meta-pin", &resp)
		if len(resp) != 3 ||
			resp[0].Cid != test.TestCid1 || resp[1].Cid != test.TestCid2 ||
			resp[2].Cid != test.TestCid3 {
			t.Error("unexpected pin list: ", resp)
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIAllocationEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		var resp api.PinSerial
		makeGet(t, rest, url(rest)+"/allocations/"+test.TestCid1, &resp)
		if resp.Cid != test.TestCid1 {
			t.Error("cid should be the same")
		}

		errResp := api.Error{}
		makeGet(t, rest, url(rest)+"/allocations/"+test.ErrorCid, &errResp)
		if errResp.Code != 404 {
			t.Error("a non-pinned cid should 404")
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIMetricsEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		var resp []api.MetricSerial
		makeGet(t, rest, url(rest)+"/monitor/metrics/somemetricstype", &resp)
		if len(resp) == 0 {
			t.Fatal("No metrics found")
		}
		for _, m := range resp {
			if m.Name != "test" {
				t.Error("Unexpected metric name: ", m.Name)
			}
			if m.Peer != test.TestPeerID1.Pretty() {
				t.Error("Unexpected peer id: ", m.Peer)
			}
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIStatusAllEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		var resp []api.GlobalPinInfoSerial
		makeGet(t, rest, url(rest)+"/pins", &resp)
		if len(resp) != 3 ||
			resp[0].Cid != test.TestCid1 ||
			resp[1].PeerMap[test.TestPeerID1.Pretty()].Status != "pinning" {
			t.Errorf("unexpected statusAll resp:\n %+v", resp)
		}

		// Test local=true
		var resp2 []api.GlobalPinInfoSerial
		makeGet(t, rest, url(rest)+"/pins?local=true", &resp2)
		if len(resp2) != 2 {
			t.Errorf("unexpected statusAll+local resp:\n %+v", resp)
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIStatusEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		var resp api.GlobalPinInfoSerial
		makeGet(t, rest, url(rest)+"/pins/"+test.TestCid1, &resp)

		if resp.Cid != test.TestCid1 {
			t.Error("expected the same cid")
		}
		info, ok := resp.PeerMap[test.TestPeerID1.Pretty()]
		if !ok {
			t.Fatal("expected info for test.TestPeerID1")
		}
		if info.Status != "pinned" {
			t.Error("expected different status")
		}

		// Test local=true
		var resp2 api.GlobalPinInfoSerial
		makeGet(t, rest, url(rest)+"/pins/"+test.TestCid1+"?local=true", &resp2)

		if resp2.Cid != test.TestCid1 {
			t.Error("expected the same cid")
		}
		info, ok = resp2.PeerMap[test.TestPeerID2.Pretty()]
		if !ok {
			t.Fatal("expected info for test.TestPeerID2")
		}
		if info.Status != "pinned" {
			t.Error("expected different status")
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPISyncAllEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		var resp []api.GlobalPinInfoSerial
		makePost(t, rest, url(rest)+"/pins/sync", []byte{}, &resp)

		if len(resp) != 3 ||
			resp[0].Cid != test.TestCid1 ||
			resp[1].PeerMap[test.TestPeerID1.Pretty()].Status != "pinning" {
			t.Errorf("unexpected syncAll resp:\n %+v", resp)
		}

		// Test local=true
		var resp2 []api.GlobalPinInfoSerial
		makePost(t, rest, url(rest)+"/pins/sync?local=true", []byte{}, &resp2)

		if len(resp2) != 2 {
			t.Errorf("unexpected syncAll+local resp:\n %+v", resp2)
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPISyncEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		var resp api.GlobalPinInfoSerial
		makePost(t, rest, url(rest)+"/pins/"+test.TestCid1+"/sync", []byte{}, &resp)

		if resp.Cid != test.TestCid1 {
			t.Error("expected the same cid")
		}
		info, ok := resp.PeerMap[test.TestPeerID1.Pretty()]
		if !ok {
			t.Fatal("expected info for test.TestPeerID1")
		}
		if info.Status != "pinned" {
			t.Error("expected different status")
		}

		// Test local=true
		var resp2 api.GlobalPinInfoSerial
		makePost(t, rest, url(rest)+"/pins/"+test.TestCid1+"/sync?local=true", []byte{}, &resp2)

		if resp2.Cid != test.TestCid1 {
			t.Error("expected the same cid")
		}
		info, ok = resp2.PeerMap[test.TestPeerID2.Pretty()]
		if !ok {
			t.Fatal("expected info for test.TestPeerID2")
		}
		if info.Status != "pinned" {
			t.Error("expected different status")
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIRecoverEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		var resp api.GlobalPinInfoSerial
		makePost(t, rest, url(rest)+"/pins/"+test.TestCid1+"/recover", []byte{}, &resp)

		if resp.Cid != test.TestCid1 {
			t.Error("expected the same cid")
		}
		info, ok := resp.PeerMap[test.TestPeerID1.Pretty()]
		if !ok {
			t.Fatal("expected info for test.TestPeerID1")
		}
		if info.Status != "pinned" {
			t.Error("expected different status")
		}
	}

	testBothEndpoints(t, tf)
}

func TestAPIRecoverAllEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	tf := func(t *testing.T, url urlF) {
		var resp []api.GlobalPinInfoSerial
		makePost(t, rest, url(rest)+"/pins/recover?local=true", []byte{}, &resp)

		if len(resp) != 0 {
			t.Fatal("bad response length")
		}

		var errResp api.Error
		makePost(t, rest, url(rest)+"/pins/recover", []byte{}, &errResp)
		if errResp.Code != 400 {
			t.Error("expected a different error")
		}
	}

	testBothEndpoints(t, tf)
}
