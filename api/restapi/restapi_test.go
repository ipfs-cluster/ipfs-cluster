package restapi

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

var (
	apiHost = "http://127.0.0.1:10002" // should match testingConfig()
)

func testRESTAPI(t *testing.T) *RESTAPI {
	//logging.SetDebugLogging()
	apiMAddr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/10002")
	rest, err := NewRESTAPI(apiMAddr)
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

func makeGet(t *testing.T, path string, resp interface{}) {
	httpResp, err := http.Get(apiHost + path)
	processResp(t, httpResp, err, resp)
}

func makePost(t *testing.T, path string, body []byte, resp interface{}) {
	httpResp, err := http.Post(apiHost+path, "application/json", bytes.NewReader(body))
	processResp(t, httpResp, err, resp)
}

func makeDelete(t *testing.T, path string, resp interface{}) {
	req, _ := http.NewRequest("DELETE", apiHost+path, bytes.NewReader([]byte{}))
	c := &http.Client{}
	httpResp, err := c.Do(req)
	processResp(t, httpResp, err, resp)
}

func TestRESTAPIShutdown(t *testing.T) {
	rest := testRESTAPI(t)
	err := rest.Shutdown()
	if err != nil {
		t.Error("should shutdown cleanly: ", err)
	}
	// test shutting down twice
	rest.Shutdown()
}

func TestRestAPIIDEndpoint(t *testing.T) {
	rest := testRESTAPI(t)
	defer rest.Shutdown()
	id := api.IDSerial{}
	makeGet(t, "/id", &id)
	if id.ID != test.TestPeerID1.Pretty() {
		t.Error("expected correct id")
	}
}

func TestRESTAPIVersionEndpoint(t *testing.T) {
	rest := testRESTAPI(t)
	defer rest.Shutdown()
	ver := api.Version{}
	makeGet(t, "/version", &ver)
	if ver.Version != "0.0.mock" {
		t.Error("expected correct version")
	}
}

func TestRESTAPIPeerstEndpoint(t *testing.T) {
	rest := testRESTAPI(t)
	defer rest.Shutdown()

	var list []api.IDSerial
	makeGet(t, "/peers", &list)
	if len(list) != 1 {
		t.Fatal("expected 1 element")
	}
	if list[0].ID != test.TestPeerID1.Pretty() {
		t.Error("expected a different peer id list: ", list)
	}
}

func TestRESTAPIPeerAddEndpoint(t *testing.T) {
	rest := testRESTAPI(t)
	defer rest.Shutdown()

	id := api.IDSerial{}
	// post with valid body
	body := fmt.Sprintf("{\"peer_multiaddress\":\"/ip4/1.2.3.4/tcp/1234/ipfs/%s\"}", test.TestPeerID1.Pretty())
	t.Log(body)
	makePost(t, "/peers", []byte(body), &id)

	if id.ID != test.TestPeerID1.Pretty() {
		t.Error("expected correct ID")
	}
	if id.Error != "" {
		t.Error("did not expect an error")
	}

	// Send invalid body
	errResp := api.Error{}
	makePost(t, "/peers", []byte("oeoeoeoe"), &errResp)
	if errResp.Code != 400 {
		t.Error("expected error with bad body")
	}
	// Send invalid multiaddr
	makePost(t, "/peers", []byte("{\"peer_multiaddress\": \"ab\"}"), &errResp)
	if errResp.Code != 400 {
		t.Error("expected error with bad multiaddress")
	}
}

func TestRESTAPIPeerRemoveEndpoint(t *testing.T) {
	rest := testRESTAPI(t)
	defer rest.Shutdown()

	makeDelete(t, "/peers/"+test.TestPeerID1.Pretty(), &struct{}{})
}

func TestRESTAPIPinEndpoint(t *testing.T) {
	rest := testRESTAPI(t)
	defer rest.Shutdown()

	// test regular post
	makePost(t, "/pins/"+test.TestCid1, []byte{}, &struct{}{})

	errResp := api.Error{}
	makePost(t, "/pins/"+test.ErrorCid, []byte{}, &errResp)
	if errResp.Message != test.ErrBadCid.Error() {
		t.Error("expected different error: ", errResp.Message)
	}

	makePost(t, "/pins/abcd", []byte{}, &errResp)
	if errResp.Code != 400 {
		t.Error("should fail with bad Cid")
	}
}

func TestRESTAPIUnpinEndpoint(t *testing.T) {
	rest := testRESTAPI(t)
	defer rest.Shutdown()

	// test regular delete
	makeDelete(t, "/pins/"+test.TestCid1, &struct{}{})

	errResp := api.Error{}
	makeDelete(t, "/pins/"+test.ErrorCid, &errResp)
	if errResp.Message != test.ErrBadCid.Error() {
		t.Error("expected different error: ", errResp.Message)
	}

	makeDelete(t, "/pins/abcd", &errResp)
	if errResp.Code != 400 {
		t.Error("should fail with bad Cid")
	}
}

func TestRESTAPIPinListEndpoint(t *testing.T) {
	rest := testRESTAPI(t)
	defer rest.Shutdown()

	var resp []api.PinSerial
	makeGet(t, "/pinlist", &resp)
	if len(resp) != 3 ||
		resp[0].Cid != test.TestCid1 || resp[1].Cid != test.TestCid2 ||
		resp[2].Cid != test.TestCid3 {
		t.Error("unexpected pin list: ", resp)
	}
}

func TestRESTAPIStatusAllEndpoint(t *testing.T) {
	rest := testRESTAPI(t)
	defer rest.Shutdown()

	var resp []api.GlobalPinInfoSerial
	makeGet(t, "/pins", &resp)
	if len(resp) != 3 ||
		resp[0].Cid != test.TestCid1 ||
		resp[1].PeerMap[test.TestPeerID1.Pretty()].Status != "pinning" {
		t.Errorf("unexpected statusResp:\n %+v", resp)
	}
}

func TestRESTAPIStatusEndpoint(t *testing.T) {
	rest := testRESTAPI(t)
	defer rest.Shutdown()

	var resp api.GlobalPinInfoSerial
	makeGet(t, "/pins/"+test.TestCid1, &resp)

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

func TestRESTAPISyncAllEndpoint(t *testing.T) {
	rest := testRESTAPI(t)
	defer rest.Shutdown()

	var resp []api.GlobalPinInfoSerial
	makePost(t, "/pins/sync", []byte{}, &resp)

	if len(resp) != 3 ||
		resp[0].Cid != test.TestCid1 ||
		resp[1].PeerMap[test.TestPeerID1.Pretty()].Status != "pinning" {
		t.Errorf("unexpected statusResp:\n %+v", resp)
	}
}

func TestRESTAPISyncEndpoint(t *testing.T) {
	rest := testRESTAPI(t)
	defer rest.Shutdown()

	var resp api.GlobalPinInfoSerial
	makePost(t, "/pins/"+test.TestCid1+"/sync", []byte{}, &resp)

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

func TestRESTAPIRecoverEndpoint(t *testing.T) {
	rest := testRESTAPI(t)
	defer rest.Shutdown()

	var resp api.GlobalPinInfoSerial
	makePost(t, "/pins/"+test.TestCid1+"/recover", []byte{}, &resp)

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
