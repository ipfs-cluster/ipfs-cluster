package pinsvcapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/api/common/test"
	"github.com/ipfs/ipfs-cluster/api/pinsvcapi/pinsvc"
	clustertest "github.com/ipfs/ipfs-cluster/test"

	libp2p "github.com/libp2p/go-libp2p"
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

func TestAPIListEndpoint(t *testing.T) {
	ctx := context.Background()
	svcapi := testAPI(t)
	defer svcapi.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		var resp pinsvc.PinList
		test.MakeGet(t, svcapi, url(svcapi)+"/pins", &resp)

		// mockPinTracker returns 3 items for Cluster.StatusAll
		if resp.Count != 3 {
			t.Fatal("Count should be 3")
		}

		if len(resp.Results) != 3 {
			t.Fatal("There should be 3 results")
		}

		results := resp.Results
		if results[0].Pin.Cid != clustertest.Cid1.String() ||
			results[1].Status != pinsvc.StatusPinning {
			t.Errorf("unexpected statusAll resp: %+v", results)
		}

		// Test status filters
		var resp2 pinsvc.PinList
		test.MakeGet(t, svcapi, url(svcapi)+"/pins?status=pinning", &resp2)
		// mockPinTracker calls pintracker.StatusAll which returns 2
		// items.
		if resp2.Count != 1 {
			t.Errorf("unexpected statusAll+status=pinning resp:\n %+v", resp2)
		}

		var resp3 pinsvc.PinList
		test.MakeGet(t, svcapi, url(svcapi)+"/pins?status=queued", &resp3)
		if resp3.Count != 0 {
			t.Errorf("unexpected statusAll+status=queued resp:\n %+v", resp3)
		}

		var resp4 pinsvc.PinList
		test.MakeGet(t, svcapi, url(svcapi)+"/pins?status=pinned", &resp4)
		if resp4.Count != 1 {
			t.Errorf("unexpected statusAll+status=queued resp:\n %+v", resp4)
		}

		var resp5 pinsvc.PinList
		test.MakeGet(t, svcapi, url(svcapi)+"/pins?status=failed", &resp5)
		if resp5.Count != 1 {
			t.Errorf("unexpected statusAll+status=queued resp:\n %+v", resp5)
		}

		var resp6 pinsvc.PinList
		test.MakeGet(t, svcapi, url(svcapi)+"/pins?status=failed,pinned", &resp6)
		if resp6.Count != 2 {
			t.Errorf("unexpected statusAll+status=failed,pinned resp:\n %+v", resp6)
		}

		// Test with cids
		var resp7 pinsvc.PinList
		test.MakeGet(t, svcapi, url(svcapi)+"/pins?cid=QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq,QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmb", &resp7)
		if resp7.Count != 2 {
			t.Errorf("unexpected statusAll+cids resp:\n %+v", resp7)
		}

		// Test with cids+limit
		var resp8 pinsvc.PinList
		test.MakeGet(t, svcapi, url(svcapi)+"/pins?cid=QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq,QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmb&limit=1", &resp8)
		if resp8.Count != 1 {
			t.Errorf("unexpected statusAll+cids+limit resp:\n %+v", resp8)
		}

		// Test with limit
		var resp9 pinsvc.PinList
		test.MakeGet(t, svcapi, url(svcapi)+"/pins?limit=1", &resp9)
		if resp9.Count != 1 {
			t.Errorf("unexpected statusAll+limit=1 resp:\n %+v", resp9)
		}

		// Test with name-match
		var resp10 pinsvc.PinList
		test.MakeGet(t, svcapi, url(svcapi)+"/pins?name=C&match=ipartial", &resp10)
		if resp10.Count != 1 {
			t.Errorf("unexpected statusAll+name resp:\n %+v", resp10)
		}

		// Test with meta-match
		var resp11 pinsvc.PinList
		test.MakeGet(t, svcapi, url(svcapi)+`/pins?meta={"ccc":"3c"}`, &resp11)
		if resp11.Count != 1 {
			t.Errorf("unexpected statusAll+meta resp:\n %+v", resp11)
		}

		var errorResp pinsvc.APIError
		test.MakeGet(t, svcapi, url(svcapi)+"/pins?status=invalid", &errorResp)
		if errorResp.Reason == "" {
			t.Errorf("expected an error: %s", errorResp.Reason)
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIPinEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	ma, _ := api.NewMultiaddr("/ip4/1.2.3.4/ipfs/" + clustertest.PeerID1.String())

	tf := func(t *testing.T, url test.URLFunc) {
		// test normal pin
		pin := pinsvc.Pin{
			Cid:  clustertest.Cid3.String(),
			Name: "testname",
			Origins: []api.Multiaddr{
				ma,
			},
			Meta: map[string]string{
				"meta": "data",
			},
		}
		var status pinsvc.PinStatus
		pinJSON, err := json.Marshal(pin)
		if err != nil {
			t.Fatal(err)
		}
		test.MakePost(t, rest, url(rest)+"/pins", pinJSON, &status)

		if status.Pin.Cid != pin.Cid {
			t.Error("cids should match")
		}
		if status.Pin.Meta["meta"] != "data" {
			t.Errorf("metadata should match: %+v", status.Pin)
		}
		if len(status.Pin.Origins) != 1 {
			t.Errorf("expected origins: %+v", status.Pin)
		}
		if len(status.Delegates) != 3 {
			t.Errorf("expected 3 delegates: %+v", status)
		}

		var errName pinsvc.APIError
		pin2 := pinsvc.Pin{
			Cid:  clustertest.Cid1.String(),
			Name: pinsvc.PinName(make([]byte, 256)),
		}
		pinJSON, err = json.Marshal(pin2)
		if err != nil {
			t.Fatal(err)
		}
		test.MakePost(t, rest, url(rest)+"/pins", pinJSON, &errName)
		if !strings.Contains(errName.Reason, "255") {
			t.Error("expected name error")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIGetPinEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		// test existing pin
		var status pinsvc.PinStatus
		test.MakeGet(t, rest, url(rest)+"/pins/"+clustertest.Cid1.String(), &status)

		if status.Pin.Cid != clustertest.Cid1.String() {
			t.Error("Cid should be set")
		}

		if status.Pin.Meta["meta"] != "data" {
			t.Errorf("metadata should match: %+v", status.Pin)
		}
		if len(status.Delegates) != 1 {
			t.Errorf("expected 1 delegates: %+v", status)
		}

		var err pinsvc.APIError
		test.MakeGet(t, rest, url(rest)+"/pins/"+clustertest.ErrorCid.String(), &err)
		if err.Reason == "" {
			t.Error("expected an error")
		}
	}

	test.BothEndpoints(t, tf)
}

func TestAPIRemovePinEndpoint(t *testing.T) {
	ctx := context.Background()
	rest := testAPI(t)
	defer rest.Shutdown(ctx)

	tf := func(t *testing.T, url test.URLFunc) {
		// test existing pin
		test.MakeDelete(t, rest, url(rest)+"/pins/"+clustertest.Cid1.String(), nil)
	}

	test.BothEndpoints(t, tf)
}
