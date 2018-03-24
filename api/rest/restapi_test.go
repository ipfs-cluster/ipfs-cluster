package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"

	ma "github.com/multiformats/go-multiaddr"
)

func testAPI(t *testing.T) *API {
	//logging.SetDebugLogging()
	apiMAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := &Config{}
	cfg.Default()
	cfg.ListenAddr = apiMAddr

	rest, err := NewAPI(cfg)
	if err != nil {
		t.Fatal("should be able to create a new Api: ", err)
	}

	// No keep alive! Otherwise tests hang with
	// connections re-used from previous tests
	rest.server.SetKeepAlivesEnabled(false)
	rest.SetClient(test.NewMockRPCClient(t))
	return rest
}

func processResp(t *testing.T, httpResp *http.Response, err error, resp interface{}) {
	if err != nil {
		t.Fatal("error making get request: ", err)
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

func apiURL(a *API) string {
	return fmt.Sprintf("http://%s", a.HTTPAddress())
}

func makeGet(t *testing.T, url string, resp interface{}) {
	httpResp, err := http.Get(url)
	processResp(t, httpResp, err, resp)
}

func makePost(t *testing.T, url string, body []byte, resp interface{}) {
	httpResp, err := http.Post(url, "application/json", bytes.NewReader(body))
	processResp(t, httpResp, err, resp)
}

func makeDelete(t *testing.T, url string, resp interface{}) {
	req, _ := http.NewRequest("DELETE", url, bytes.NewReader([]byte{}))
	c := &http.Client{}
	httpResp, err := c.Do(req)
	processResp(t, httpResp, err, resp)
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
	defer rest.Shutdown()
	id := api.IDSerial{}
	makeGet(t, apiURL(rest)+"/id", &id)
	if id.ID != test.TestPeerID1.Pretty() {
		t.Error("expected correct id")
	}
}

func TestAPIVersionEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()
	ver := api.Version{}
	makeGet(t, apiURL(rest)+"/version", &ver)
	if ver.Version != "0.0.mock" {
		t.Error("expected correct version")
	}
}

func TestAPIPeerstEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	var list []api.IDSerial
	makeGet(t, apiURL(rest)+"/peers", &list)
	if len(list) != 1 {
		t.Fatal("expected 1 element")
	}
	if list[0].ID != test.TestPeerID1.Pretty() {
		t.Error("expected a different peer id list: ", list)
	}
}

func TestAPIPeerAddEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	id := api.IDSerial{}
	// post with valid body
	body := fmt.Sprintf("{\"peer_multiaddress\":\"/ip4/1.2.3.4/tcp/1234/ipfs/%s\"}", test.TestPeerID1.Pretty())
	t.Log(body)
	makePost(t, apiURL(rest)+"/peers", []byte(body), &id)

	if id.ID != test.TestPeerID1.Pretty() {
		t.Error("expected correct ID")
	}
	if id.Error != "" {
		t.Error("did not expect an error")
	}

	// Send invalid body
	errResp := api.Error{}
	makePost(t, apiURL(rest)+"/peers", []byte("oeoeoeoe"), &errResp)
	if errResp.Code != 400 {
		t.Error("expected error with bad body")
	}
	// Send invalid multiaddr
	makePost(t, apiURL(rest)+"/peers", []byte("{\"peer_multiaddress\": \"ab\"}"), &errResp)
	if errResp.Code != 400 {
		t.Error("expected error with bad multiaddress")
	}
}

func TestAPIPeerRemoveEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	makeDelete(t, apiURL(rest)+"/peers/"+test.TestPeerID1.Pretty(), &struct{}{})
}

func TestConnectGraphEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()
	var cg api.ConnectGraphSerial
	makeGet(t, apiURL(rest)+"/health/graph", &cg)
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

func TestAPIPinEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	// test regular post
	makePost(t, apiURL(rest)+"/pins/"+test.TestCid1, []byte{}, &struct{}{})

	errResp := api.Error{}
	makePost(t, apiURL(rest)+"/pins/"+test.ErrorCid, []byte{}, &errResp)
	if errResp.Message != test.ErrBadCid.Error() {
		t.Error("expected different error: ", errResp.Message)
	}

	makePost(t, apiURL(rest)+"/pins/abcd", []byte{}, &errResp)
	if errResp.Code != 400 {
		t.Error("should fail with bad Cid")
	}
}

func TestAPIUnpinEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	// test regular delete
	makeDelete(t, apiURL(rest)+"/pins/"+test.TestCid1, &struct{}{})

	errResp := api.Error{}
	makeDelete(t, apiURL(rest)+"/pins/"+test.ErrorCid, &errResp)
	if errResp.Message != test.ErrBadCid.Error() {
		t.Error("expected different error: ", errResp.Message)
	}

	makeDelete(t, apiURL(rest)+"/pins/abcd", &errResp)
	if errResp.Code != 400 {
		t.Error("should fail with bad Cid")
	}
}

func TestAPIAllocationsEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	var resp []api.PinSerial
	makeGet(t, apiURL(rest)+"/allocations?a=false", &resp)
	if len(resp) != 3 ||
		resp[0].Cid != test.TestCid1 || resp[1].Cid != test.TestCid2 ||
		resp[2].Cid != test.TestCid3 {
		t.Error("unexpected pin list: ", resp)
	}
}

func TestAPIAllocationEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	var resp api.PinSerial
	makeGet(t, apiURL(rest)+"/allocations/"+test.TestCid1, &resp)
	if resp.Cid != test.TestCid1 {
		t.Error("cid should be the same")
	}

	errResp := api.Error{}
	makeGet(t, apiURL(rest)+"/allocations/"+test.ErrorCid, &errResp)
	if errResp.Code != 404 {
		t.Error("a non-pinned cid should 404")
	}
}

func TestAPIStatusAllEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	var resp []api.GlobalPinInfoSerial
	makeGet(t, apiURL(rest)+"/pins", &resp)
	if len(resp) != 3 ||
		resp[0].Cid != test.TestCid1 ||
		resp[1].PeerMap[test.TestPeerID1.Pretty()].Status != "pinning" {
		t.Errorf("unexpected statusAll resp:\n %+v", resp)
	}

	// Test local=true
	var resp2 []api.GlobalPinInfoSerial
	makeGet(t, apiURL(rest)+"/pins?local=true", &resp2)
	if len(resp2) != 2 {
		t.Errorf("unexpected statusAll+local resp:\n %+v", resp)
	}
}

func TestAPIStatusEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	var resp api.GlobalPinInfoSerial
	makeGet(t, apiURL(rest)+"/pins/"+test.TestCid1, &resp)

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
	makeGet(t, apiURL(rest)+"/pins/"+test.TestCid1+"?local=true", &resp2)

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

func TestAPISyncAllEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	var resp []api.GlobalPinInfoSerial
	makePost(t, apiURL(rest)+"/pins/sync", []byte{}, &resp)

	if len(resp) != 3 ||
		resp[0].Cid != test.TestCid1 ||
		resp[1].PeerMap[test.TestPeerID1.Pretty()].Status != "pinning" {
		t.Errorf("unexpected syncAll resp:\n %+v", resp)
	}

	// Test local=true
	var resp2 []api.GlobalPinInfoSerial
	makePost(t, apiURL(rest)+"/pins/sync?local=true", []byte{}, &resp2)

	if len(resp2) != 2 {
		t.Errorf("unexpected syncAll+local resp:\n %+v", resp2)
	}
}

func TestAPISyncEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	var resp api.GlobalPinInfoSerial
	makePost(t, apiURL(rest)+"/pins/"+test.TestCid1+"/sync", []byte{}, &resp)

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
	makePost(t, apiURL(rest)+"/pins/"+test.TestCid1+"/sync?local=true", []byte{}, &resp2)

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

func TestAPIRecoverEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	var resp api.GlobalPinInfoSerial
	makePost(t, apiURL(rest)+"/pins/"+test.TestCid1+"/recover", []byte{}, &resp)

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

func TestAPIRecoverAllEndpoint(t *testing.T) {
	rest := testAPI(t)
	defer rest.Shutdown()

	var resp []api.GlobalPinInfoSerial
	makePost(t, apiURL(rest)+"/pins/recover?local=true", []byte{}, &resp)

	if len(resp) != 0 {
		t.Fatal("bad response length")
	}

	var errResp api.Error
	makePost(t, apiURL(rest)+"/pins/recover", []byte{}, &errResp)
	if errResp.Code != 400 {
		t.Error("expected a different error")
	}
}
