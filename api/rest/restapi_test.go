package rest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	test "github.com/ipfs-cluster/ipfs-cluster/api/common/test"
	clustertest "github.com/ipfs-cluster/ipfs-cluster/test"

	libp2p "github.com/libp2p/go-libp2p"
	peer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	SSLCertFile         = "test/server.crt"
	SSLKeyFile          = "test/server.key"
	clientOrigin        = "myorigin"
	validUserName       = "validUserName"
	validUserPassword   = "validUserPassword"
	adminUserName       = "adminUserName"
	adminUserPassword   = "adminUserPassword"
	invalidUserName     = "invalidUserName"
	invalidUserPassword = "invalidUserPassword"
)

func testAPIwithConfig(t *testing.T, cfg *Config, name string) *API {
	ctx := context.Background()
	apiMAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	h, err := libp2p.New(libp2p.ListenAddrs(apiMAddr))
	if err != nil {
		t.Fatal(err)
	}

	cfg.HTTPListenAddr = []ma.Multiaddr{apiMAddr}

	rest, err := NewAPIWithHost(ctx, cfg, h)
	if err != nil {
		t.Fatalf("should be able to create a new %s API: %s", name, err)
	}

	// No keep alive for tests
	rest.SetKeepAlivesEnabled(false)
	rest.SetClient(clustertest.NewMockRPCClient(t))

	return rest
}

func testAPI(t *testing.T) *API {
	cfg := NewConfig()
	cfg.Default()
	cfg.CORSAllowedOrigins = []string{clientOrigin}
	cfg.CORSAllowedMethods = []string{"GET", "POST", "DELETE"}
	//cfg.CORSAllowedHeaders = []string{"Content-Type"}
	cfg.CORSMaxAge = 10 * time.Minute

	return testAPIwithConfig(t, cfg, "basic")
}

func TestRestAPIIDEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		id := api.ID{}
		test.MakeGet(t, rest, url(rest)+"/id", &id)
		if id.ID != clustertest.PeerID1 {
			t.Error("expected correct id")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIVersionEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		ver := api.Version{}
		test.MakeGet(t, rest, url(rest)+"/version", &ver)
		if ver.Version != "0.0.mock" {
			t.Error("expected correct version")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIPeersEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		var list []api.ID
		test.MakeStreamingGet(t, rest, url(rest)+"/peers", &list, false)
		if len(list) != 1 {
			t.Fatal("expected 1 element")
		}
		if list[0].ID != clustertest.PeerID1 {
			t.Error("expected a different peer id list: ", list)
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIPeerAddEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		id := api.ID{}
		// post with valid body
		body := fmt.Sprintf("{\"peer_id\":\"%s\"}", clustertest.PeerID1)
		t.Log(body)
		test.MakePost(t, rest, url(rest)+"/peers", []byte(body), &id)
		if id.ID != clustertest.PeerID1 {
			t.Error("expected correct ID")
		}
		if id.Error != "" {
			t.Error("did not expect an error")
		}

		// Send invalid body
		errResp := api.Error{}
		test.MakePost(t, rest, url(rest)+"/peers", []byte("oeoeoeoe"), &errResp)
		if errResp.Code != 400 {
			t.Error("expected error with bad body")
		}
		// Send invalid peer id
		test.MakePost(t, rest, url(rest)+"/peers", []byte("{\"peer_id\": \"ab\"}"), &errResp)
		if errResp.Code != 400 {
			t.Error("expected error with bad peer_id")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIAddFileEndpointBadContentType(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		fmtStr1 := "/add?shard=true&repl_min=-1&repl_max=-1"
		localURL := url(rest) + fmtStr1

		errResp := api.Error{}
		test.MakePost(t, rest, localURL, []byte("test"), &errResp)

		if errResp.Code != 400 {
			t.Error("expected error with bad content-type")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIAddFileEndpointLocal(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	sth := clustertest.NewShardingTestHelper()
	defer sth.Clean(t)

	// This generates the testing files and
	// writes them to disk.
	// This is necessary here because we run tests
	// in parallel, and otherwise a write-race might happen.
	_, closer := sth.GetTreeMultiReader(t)
	closer.Close()

	tf := func(t *testing.T, url test.URLFunc) {
		fmtStr1 := "/add?shard=false&repl_min=-1&repl_max=-1&stream-channels=true"
		localURL := url(rest) + fmtStr1
		body, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		resp := api.AddedOutput{}
		mpContentType := "multipart/form-data; boundary=" + body.Boundary()
		test.MakeStreamingPost(t, rest, localURL, body, mpContentType, &resp)

		// resp will contain the last object from the streaming
		if resp.Cid.String() != clustertest.ShardingDirBalancedRootCID {
			t.Error("Bad Cid after adding: ", resp.Cid)
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIAddFileEndpointShard(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	sth := clustertest.NewShardingTestHelper()
	defer sth.Clean(t)

	// This generates the testing files and
	// writes them to disk.
	// This is necessary here because we run tests
	// in parallel, and otherwise a write-race might happen.
	_, closer := sth.GetTreeMultiReader(t)
	closer.Close()

	tf := func(t *testing.T, url test.URLFunc) {
		body, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		mpContentType := "multipart/form-data; boundary=" + body.Boundary()
		resp := api.AddedOutput{}
		fmtStr1 := "/add?shard=true&repl_min=-1&repl_max=-1&stream-channels=true&shard-size=1000000"
		shardURL := url(rest) + fmtStr1
		test.MakeStreamingPost(t, rest, shardURL, body, mpContentType, &resp)
	}

	test.BothEndpoints(t, tf)
}

func TestAPIAddFileEndpoint_StreamChannelsFalse(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	sth := clustertest.NewShardingTestHelper()
	defer sth.Clean(t)

	// This generates the testing files and
	// writes them to disk.
	// This is necessary here because we run tests
	// in parallel, and otherwise a write-race might happen.
	_, closer := sth.GetTreeMultiReader(t)
	closer.Close()

	tf := func(t *testing.T, url test.URLFunc) {
		body, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		fullBody, err := io.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}
		mpContentType := "multipart/form-data; boundary=" + body.Boundary()
		resp := []api.AddedOutput{}
		fmtStr1 := "/add?shard=false&repl_min=-1&repl_max=-1&stream-channels=false"
		shardURL := url(rest) + fmtStr1

		test.MakePostWithContentType(t, rest, shardURL, fullBody, mpContentType, &resp)
		lastHash := resp[len(resp)-1]
		if lastHash.Cid.String() != clustertest.ShardingDirBalancedRootCID {
			t.Error("Bad Cid after adding: ", lastHash.Cid)
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIPeerRemoveEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		test.MakeDelete(t, rest, url(rest)+"/peers/"+clustertest.PeerID1.String(), &struct{}{})
	}

	test.BothEndpoints(t, tf)
}

func TestConnectGraphEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		var cg api.ConnectGraph
		test.MakeGet(t, rest, url(rest)+"/health/graph", &cg)
		if cg.ClusterID != clustertest.PeerID1 {
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
		pid1 := clustertest.PeerID1
		pid4 := clustertest.PeerID4
		if _, ok := cg.ClustertoIPFS[pid1.String()]; !ok {
			t.Fatal("missing cluster peer 1 from cluster to peer links map")
		}
		if cg.ClustertoIPFS[pid1.String()] != pid4 {
			t.Error("unexpected ipfs peer mapped to cluster peer 1 in graph")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIPinEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		// test regular post
		test.MakePost(t, rest, url(rest)+"/pins/"+clustertest.Cid1.String(), []byte{}, &struct{}{})

		errResp := api.Error{}
		test.MakePost(t, rest, url(rest)+"/pins/"+clustertest.ErrorCid.String(), []byte{}, &errResp)
		if errResp.Message != clustertest.ErrBadCid.Error() {
			t.Error("expected different error: ", errResp.Message)
		}

		test.MakePost(t, rest, url(rest)+"/pins/abcd", []byte{}, &errResp)
		if errResp.Code != 400 {
			t.Error("should fail with bad Cid")
		}
	}

	test.BothEndpoints(t, tf)
}

type pathCase struct {
	path        string
	opts        api.PinOptions
	wantErr     bool
	code        int
	expectedCid string
}

func (p *pathCase) WithQuery(t *testing.T) string {
	query, err := p.opts.ToQuery()
	if err != nil {
		t.Fatal(err)
	}
	return p.path + "?" + query
}

var testPinOpts = api.PinOptions{
	ReplicationFactorMax: 7,
	ReplicationFactorMin: 6,
	Name:                 "hello there",
	UserAllocations:      []peer.ID{clustertest.PeerID1, clustertest.PeerID2},
	ExpireAt:             time.Now().Add(30 * time.Second),
}

var pathTestCases = []pathCase{
	{
		"/ipfs/QmaNJ5acV31sx8jq626qTpAWW4DXKw34aGhx53dECLvXbY",
		testPinOpts,
		false,
		http.StatusOK,
		"QmaNJ5acV31sx8jq626qTpAWW4DXKw34aGhx53dECLvXbY",
	},
	{
		"/ipfs/QmbUNM297ZwxB8CfFAznK7H9YMesDoY6Tt5bPgt5MSCB2u/im.gif",
		testPinOpts,
		false,
		http.StatusOK,
		clustertest.CidResolved.String(),
	},
	{
		"/ipfs/invalidhash",
		testPinOpts,
		true,
		http.StatusBadRequest,
		"",
	},
	{
		"/ipfs/bafyreiay3jpjk74dkckv2r74eyvf3lfnxujefay2rtuluintasq2zlapv4",
		testPinOpts,
		true,
		http.StatusNotFound,
		"",
	},
	// TODO: A case with trailing slash with paths
	// clustertest.PathIPNS2, clustertest.PathIPLD2, clustertest.InvalidPath1
}

func TestAPIPinEndpointWithPath(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		for _, testCase := range pathTestCases[:3] {
			c, _ := api.DecodeCid(testCase.expectedCid)
			resultantPin := api.PinWithOpts(
				c,
				testPinOpts,
			)

			if testCase.wantErr {
				errResp := api.Error{}
				q := testCase.WithQuery(t)
				test.MakePost(t, rest, url(rest)+"/pins"+q, []byte{}, &errResp)
				if errResp.Code != testCase.code {
					t.Errorf(
						"status code: expected: %d, got: %d, path: %s\n",
						testCase.code,
						errResp.Code,
						testCase.path,
					)
				}
				continue
			}
			pin := api.Pin{}
			q := testCase.WithQuery(t)
			test.MakePost(t, rest, url(rest)+"/pins"+q, []byte{}, &pin)
			if !pin.Equals(resultantPin) {
				t.Errorf("pin: expected: %+v", resultantPin)
				t.Errorf("pin: got: %+v", pin)
				t.Errorf("path: %s", testCase.path)
			}
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIUnpinEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		// test regular delete
		test.MakeDelete(t, rest, url(rest)+"/pins/"+clustertest.Cid1.String(), &struct{}{})

		errResp := api.Error{}
		test.MakeDelete(t, rest, url(rest)+"/pins/"+clustertest.ErrorCid.String(), &errResp)
		if errResp.Message != clustertest.ErrBadCid.Error() {
			t.Error("expected different error: ", errResp.Message)
		}

		test.MakeDelete(t, rest, url(rest)+"/pins/"+clustertest.NotFoundCid.String(), &errResp)
		if errResp.Code != http.StatusNotFound {
			t.Error("expected different error code: ", errResp.Code)
		}

		test.MakeDelete(t, rest, url(rest)+"/pins/abcd", &errResp)
		if errResp.Code != 400 {
			t.Error("expected different error code: ", errResp.Code)
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIUnpinEndpointWithPath(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		for _, testCase := range pathTestCases {
			if testCase.wantErr {
				errResp := api.Error{}
				test.MakeDelete(t, rest, url(rest)+"/pins"+testCase.path, &errResp)
				if errResp.Code != testCase.code {
					t.Errorf(
						"status code: expected: %d, got: %d, path: %s\n",
						testCase.code,
						errResp.Code,
						testCase.path,
					)
				}
				continue
			}
			pin := api.Pin{}
			test.MakeDelete(t, rest, url(rest)+"/pins"+testCase.path, &pin)
			if pin.Cid.String() != testCase.expectedCid {
				t.Errorf(
					"cid: expected: %s, got: %s, path: %s\n",
					clustertest.CidResolved,
					pin.Cid,
					testCase.path,
				)
			}
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIAllocationsEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		var resp []api.Pin
		test.MakeStreamingGet(t, rest, url(rest)+"/allocations?filter=pin,meta-pin", &resp, false)
		if len(resp) != 3 ||
			!resp[0].Cid.Equals(clustertest.Cid1) || !resp[1].Cid.Equals(clustertest.Cid2) ||
			!resp[2].Cid.Equals(clustertest.Cid3) {
			t.Error("unexpected pin list: ", resp)
		}

		test.MakeStreamingGet(t, rest, url(rest)+"/allocations", &resp, false)
		if len(resp) != 3 ||
			!resp[0].Cid.Equals(clustertest.Cid1) || !resp[1].Cid.Equals(clustertest.Cid2) ||
			!resp[2].Cid.Equals(clustertest.Cid3) {
			t.Error("unexpected pin list: ", resp)
		}

		errResp := api.Error{}
		test.MakeStreamingGet(t, rest, url(rest)+"/allocations?filter=invalid", &errResp, false)
		if errResp.Code != http.StatusBadRequest {
			t.Error("an invalid filter value should 400")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIAllocationEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		var resp api.Pin
		test.MakeGet(t, rest, url(rest)+"/allocations/"+clustertest.Cid1.String(), &resp)
		if !resp.Cid.Equals(clustertest.Cid1) {
			t.Errorf("cid should be the same: %s %s", resp.Cid, clustertest.Cid1)
		}

		errResp := api.Error{}
		test.MakeGet(t, rest, url(rest)+"/allocations/"+clustertest.Cid4.String(), &errResp)
		if errResp.Code != 404 {
			t.Error("a non-pinned cid should 404")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIMetricsEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		var resp []api.Metric
		test.MakeGet(t, rest, url(rest)+"/monitor/metrics/somemetricstype", &resp)
		if len(resp) == 0 {
			t.Fatal("No metrics found")
		}
		for _, m := range resp {
			if m.Name != "test" {
				t.Error("Unexpected metric name: ", m.Name)
			}
			if m.Peer != clustertest.PeerID1 {
				t.Error("Unexpected peer id: ", m.Peer)
			}
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIMetricNamesEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		var resp []string
		test.MakeGet(t, rest, url(rest)+"/monitor/metrics", &resp)
		if len(resp) == 0 {
			t.Fatal("No metric names found")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIAlertsEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		var resp []api.Alert
		test.MakeGet(t, rest, url(rest)+"/health/alerts", &resp)
		if len(resp) != 1 {
			t.Error("expected one alert")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIStatusAllEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		var resp []api.GlobalPinInfo

		test.MakeStreamingGet(t, rest, url(rest)+"/pins", &resp, false)

		// mockPinTracker returns 3 items for Cluster.StatusAll
		if len(resp) != 3 ||
			!resp[0].Cid.Equals(clustertest.Cid1) ||
			resp[1].PeerMap[clustertest.PeerID1.String()].Status.String() != "pinning" {
			t.Errorf("unexpected statusAll resp")
		}

		// Test local=true
		var resp2 []api.GlobalPinInfo
		test.MakeStreamingGet(t, rest, url(rest)+"/pins?local=true", &resp2, false)
		// mockPinTracker calls pintracker.StatusAll which returns 2
		// items.
		if len(resp2) != 2 {
			t.Errorf("unexpected statusAll+local resp:\n %+v", resp2)
		}

		// Test with filter
		var resp3 []api.GlobalPinInfo
		test.MakeStreamingGet(t, rest, url(rest)+"/pins?filter=queued", &resp3, false)
		if len(resp3) != 0 {
			t.Errorf("unexpected statusAll+filter=queued resp:\n %+v", resp3)
		}

		var resp4 []api.GlobalPinInfo
		test.MakeStreamingGet(t, rest, url(rest)+"/pins?filter=pinned", &resp4, false)
		if len(resp4) != 1 {
			t.Errorf("unexpected statusAll+filter=pinned resp:\n %+v", resp4)
		}

		var resp5 []api.GlobalPinInfo
		test.MakeStreamingGet(t, rest, url(rest)+"/pins?filter=pin_error", &resp5, false)
		if len(resp5) != 1 {
			t.Errorf("unexpected statusAll+filter=pin_error resp:\n %+v", resp5)
		}

		var resp6 []api.GlobalPinInfo
		test.MakeStreamingGet(t, rest, url(rest)+"/pins?filter=error", &resp6, false)
		if len(resp6) != 1 {
			t.Errorf("unexpected statusAll+filter=error resp:\n %+v", resp6)
		}

		var resp7 []api.GlobalPinInfo
		test.MakeStreamingGet(t, rest, url(rest)+"/pins?filter=error,pinned", &resp7, false)
		if len(resp7) != 2 {
			t.Errorf("unexpected statusAll+filter=error,pinned resp:\n %+v", resp7)
		}

		var errorResp api.Error
		test.MakeStreamingGet(t, rest, url(rest)+"/pins?filter=invalid", &errorResp, false)
		if errorResp.Code != http.StatusBadRequest {
			t.Error("an invalid filter value should 400")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIStatusAllWithCidsEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		var resp []api.GlobalPinInfo
		cids := []string{
			clustertest.Cid1.String(),
			clustertest.Cid2.String(),
			clustertest.Cid3.String(),
			clustertest.Cid4.String(),
		}
		test.MakeStreamingGet(t, rest, url(rest)+"/pins/?cids="+strings.Join(cids, ","), &resp, false)

		if len(resp) != 4 {
			t.Error("wrong number of responses")
		}

		// Test local=true
		var resp2 []api.GlobalPinInfo
		test.MakeStreamingGet(t, rest, url(rest)+"/pins/?local=true&cids="+strings.Join(cids, ","), &resp2, false)
		if len(resp2) != 4 {
			t.Error("wrong number of responses")
		}

		// Test with an error. This should produce a trailer error.
		cids = append(cids, clustertest.ErrorCid.String())
		var resp3 []api.GlobalPinInfo
		test.MakeStreamingGet(t, rest, url(rest)+"/pins/?local=true&cids="+strings.Join(cids, ","), &resp3, true)
		if len(resp3) != 4 {
			t.Error("wrong number of responses")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIStatusEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		var resp api.GlobalPinInfo
		test.MakeGet(t, rest, url(rest)+"/pins/"+clustertest.Cid1.String(), &resp)

		if !resp.Cid.Equals(clustertest.Cid1) {
			t.Error("expected the same cid")
		}
		info, ok := resp.PeerMap[clustertest.PeerID1.String()]
		if !ok {
			t.Fatal("expected info for clustertest.PeerID1")
		}
		if info.Status.String() != "pinned" {
			t.Error("expected different status")
		}

		// Test local=true
		var resp2 api.GlobalPinInfo
		test.MakeGet(t, rest, url(rest)+"/pins/"+clustertest.Cid1.String()+"?local=true", &resp2)

		if !resp2.Cid.Equals(clustertest.Cid1) {
			t.Error("expected the same cid")
		}
		info, ok = resp2.PeerMap[clustertest.PeerID2.String()]
		if !ok {
			t.Fatal("expected info for clustertest.PeerID2")
		}
		if info.Status.String() != "pinned" {
			t.Error("expected different status")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIRecoverEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		var resp api.GlobalPinInfo
		test.MakePost(t, rest, url(rest)+"/pins/"+clustertest.Cid1.String()+"/recover", []byte{}, &resp)

		if !resp.Cid.Equals(clustertest.Cid1) {
			t.Error("expected the same cid")
		}
		info, ok := resp.PeerMap[clustertest.PeerID1.String()]
		if !ok {
			t.Fatal("expected info for clustertest.PeerID1")
		}
		if info.Status.String() != "pinned" {
			t.Error("expected different status")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIRecoverAllEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		var resp []api.GlobalPinInfo
		test.MakeStreamingPost(t, rest, url(rest)+"/pins/recover?local=true", nil, "", &resp)
		if len(resp) != 0 {
			t.Fatal("bad response length")
		}

		var resp1 []api.GlobalPinInfo
		test.MakeStreamingPost(t, rest, url(rest)+"/pins/recover", nil, "", &resp1)
		if len(resp1) == 0 {
			t.Fatal("bad response length")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIIPFSGCEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	testGlobalRepoGC := func(t *testing.T, gRepoGC api.GlobalRepoGC) {
		if gRepoGC.PeerMap == nil {
			t.Fatal("expected a non-nil peer map")
		}

		if len(gRepoGC.PeerMap) != 1 {
			t.Error("expected repo gc information for one peer")
		}

		for _, repoGC := range gRepoGC.PeerMap {
			if repoGC.Peer == "" {
				t.Error("expected a cluster ID")
			}
			if repoGC.Error != "" {
				t.Error("did not expect any error")
			}
			if repoGC.Keys == nil {
				t.Fatal("expected a non-nil array of IPFSRepoGC")
			}
			if len(repoGC.Keys) == 0 {
				t.Fatal("expected at least one key, but found none")
			}
			if !repoGC.Keys[0].Key.Equals(clustertest.Cid1) {
				t.Errorf("expected a different cid, expected: %s, found: %s", clustertest.Cid1, repoGC.Keys[0].Key)
			}

		}
	}

	tf := func(t *testing.T, url test.URLFunc) {
		var resp api.GlobalRepoGC
		test.MakePost(t, rest, url(rest)+"/ipfs/gc?local=true", []byte{}, &resp)
		testGlobalRepoGC(t, resp)

		var resp1 api.GlobalRepoGC
		test.MakePost(t, rest, url(rest)+"/ipfs/gc", []byte{}, &resp1)
		testGlobalRepoGC(t, resp1)
	}

	test.BothEndpoints(t, tf)
}

func TestHealthEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		errResp := api.Error{}
		test.MakeGet(t, rest, url(rest)+"/health", &errResp)
		if errResp.Code != 0 || errResp.Message != "" {
			t.Error("expected no errors")
		}
	}

	test.BothEndpoints(t, tf)
}
