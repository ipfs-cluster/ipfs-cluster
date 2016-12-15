package ipfscluster

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
	peer "gx/ipfs/QmfMmLGoKzCHDN7cGgk64PJr4iipzidDRME8HABSJqvmhC/go-libp2p-peer"
)

var (
	apiHost = "http://127.0.0.1:10002" // should match testingConfig()
)

func testClusterApi(t *testing.T) *ClusterHTTPAPI {
	//logging.SetDebugLogging()
	cfg := testingConfig()
	api, err := NewHTTPAPI(cfg)
	// No keep alive! Otherwise tests hang with
	// connections re-used from previous tests
	api.server.SetKeepAlivesEnabled(false)
	if err != nil {
		t.Fatal("should be able to create a new Api: ", err)
	}

	if api.RpcChan() == nil {
		t.Fatal("should create the Rpc channel")
	}
	return api
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

func makePost(t *testing.T, path string, resp interface{}) {
	httpResp, err := http.Post(apiHost+path, "application/json", bytes.NewReader([]byte{}))
	processResp(t, httpResp, err, resp)
}

func makeDelete(t *testing.T, path string, resp interface{}) {
	req, _ := http.NewRequest("DELETE", apiHost+path, bytes.NewReader([]byte{}))
	c := &http.Client{}
	httpResp, err := c.Do(req)
	processResp(t, httpResp, err, resp)
}

func TestAPIShutdown(t *testing.T) {
	api := testClusterApi(t)
	err := api.Shutdown()
	if err != nil {
		t.Error("should shutdown cleanly: ", err)
	}
	api.Shutdown()
}

func TestVersionEndpoint(t *testing.T) {
	api := testClusterApi(t)
	api.server.SetKeepAlivesEnabled(false)
	defer api.Shutdown()
	simulateAnswer(api.RpcChan(), "v", nil)
	ver := versionResp{}
	makeGet(t, "/version", &ver)
	if ver.Version != "v" {
		t.Error("expected correct version")
	}

	simulateAnswer(api.RpcChan(), nil, errors.New("an error"))
	errResp := errorResp{}
	makeGet(t, "/version", &errResp)
	if errResp.Message != "an error" {
		t.Error("expected different error")
	}
}

func TestMemberListEndpoint(t *testing.T) {
	api := testClusterApi(t)
	api.server.SetKeepAlivesEnabled(false)
	defer api.Shutdown()
	pList := []peer.ID{
		testPeerID,
	}
	simulateAnswer(api.RpcChan(), pList, nil)
	var list []string
	makeGet(t, "/members", &list)
	if len(list) != 1 || list[0] != testPeerID.Pretty() {
		t.Error("expected a peer id list: ", list)
	}

	simulateAnswer(api.RpcChan(), nil, errors.New("an error"))
	errResp := errorResp{}
	makeGet(t, "/members", &errResp)
	if errResp.Message != "an error" {
		t.Error("expected different error")
	}
}

func TestPinEndpoint(t *testing.T) {
	api := testClusterApi(t)
	defer api.Shutdown()
	simulateAnswer(api.RpcChan(), nil, nil)
	var i interface{} = nil
	makePost(t, "/pins/"+testCid, &i)
	if i != nil {
		t.Error("pin should have returned an empty response")
	}

	simulateAnswer(api.RpcChan(), nil, errors.New("an error"))
	errResp := errorResp{}
	makePost(t, "/pins/"+testCid2, &errResp)
	if errResp.Message != "an error" {
		t.Error("expected different error")
	}

	makePost(t, "/pins/abcd", &errResp)
	if errResp.Code != 400 {
		t.Error("should fail with wrong Cid")
	}
}

func TestUnpinEndpoint(t *testing.T) {
	api := testClusterApi(t)
	defer api.Shutdown()
	simulateAnswer(api.RpcChan(), nil, nil)
	var i interface{} = nil
	makeDelete(t, "/pins/"+testCid, &i)
	if i != nil {
		t.Error("pin should have returned an empty response")
	}

	simulateAnswer(api.RpcChan(), nil, errors.New("an error"))
	errResp := errorResp{}
	makeDelete(t, "/pins/"+testCid2, &errResp)
	if errResp.Message != "an error" {
		t.Error("expected different error")
	}

	makeDelete(t, "/pins/abcd", &errResp)
	if errResp.Code != 400 {
		t.Error("should fail with wrong Cid")
	}
}

func TestPinListEndpoint(t *testing.T) {
	c, _ := cid.Decode(testCid)
	c2, _ := cid.Decode(testCid2)
	c3, _ := cid.Decode(testCid3)
	api := testClusterApi(t)
	defer api.Shutdown()
	pList := []*cid.Cid{
		c, c2, c3, c, c2,
	}

	simulateAnswer(api.RpcChan(), pList, nil)
	var resp []*cid.Cid
	makeGet(t, "/pins", &resp)
	if len(resp) != 5 {
		t.Error("unexpected pin list: ", resp)
	}
}

func TestStatusEndpoint(t *testing.T) {
	c, _ := cid.Decode(testCid)
	c2, _ := cid.Decode(testCid2)
	c3, _ := cid.Decode(testCid3)
	api := testClusterApi(t)
	defer api.Shutdown()
	pList := []Pin{
		Pin{
			Status: PinError,
			Cid:    c,
		},
		Pin{
			Status: UnpinError,
			Cid:    c,
		},
		Pin{
			Status: Pinned,
			Cid:    c3,
		},
		Pin{
			Status: Pinning,
			Cid:    c,
		},
		Pin{
			Status: Unpinning,
			Cid:    c2,
		},
	}

	simulateAnswer(api.RpcChan(), pList, nil)
	var resp statusResp
	makeGet(t, "/status", &resp)
	if len(resp) != 5 {
		t.Error("unexpected statusResp: ", resp)
	}
}
