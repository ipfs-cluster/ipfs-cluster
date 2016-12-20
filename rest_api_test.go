package ipfscluster

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
)

var (
	apiHost = "http://127.0.0.1:10002" // should match testingConfig()
)

func testClusterApi(t *testing.T) *RESTAPI {
	//logging.SetDebugLogging()
	cfg := testingConfig()
	api, err := NewRESTAPI(cfg)
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
	pList := []GlobalPinInfo{
		GlobalPinInfo{
			Status: map[peer.ID]PinInfo{
				testPeerID: PinInfo{
					CidStr: testCid,
					Peer:   testPeerID,
					IPFS:   Pinned,
				},
			},
			Cid: c,
		},
		GlobalPinInfo{
			Status: map[peer.ID]PinInfo{
				testPeerID: PinInfo{
					CidStr: testCid2,
					Peer:   testPeerID,
					IPFS:   Unpinned,
				},
			},
			Cid: c2,
		},
		GlobalPinInfo{
			Status: map[peer.ID]PinInfo{
				testPeerID: PinInfo{
					CidStr: testCid3,
					Peer:   testPeerID,
					IPFS:   PinError,
				},
			},

			Cid: c3,
		},
		GlobalPinInfo{
			Status: map[peer.ID]PinInfo{
				testPeerID: PinInfo{
					CidStr: testCid,
					Peer:   testPeerID,
					IPFS:   UnpinError,
				},
			},
			Cid: c,
		},
		GlobalPinInfo{
			Status: map[peer.ID]PinInfo{
				testPeerID: PinInfo{
					CidStr: testCid2,
					Peer:   testPeerID,
					IPFS:   Unpinning,
				},
			},
			Cid: c2,
		},
		GlobalPinInfo{
			Status: map[peer.ID]PinInfo{
				testPeerID: PinInfo{
					CidStr: testCid3,
					Peer:   testPeerID,
					IPFS:   Pinning,
				},
			},
			Cid: c3,
		},
	}

	simulateAnswer(api.RpcChan(), pList, nil)
	var resp statusResp
	makeGet(t, "/status", &resp)
	if len(resp) != 6 {
		t.Errorf("unexpected statusResp:\n %+v", resp)
	}
}

func TestStatusCidEndpoint(t *testing.T) {
	c, _ := cid.Decode(testCid)
	api := testClusterApi(t)
	defer api.Shutdown()
	pin := GlobalPinInfo{
		Cid: c,
		Status: map[peer.ID]PinInfo{
			testPeerID: PinInfo{
				CidStr: testCid,
				Peer:   testPeerID,
				IPFS:   Unpinned,
			},
		},
	}
	simulateAnswer(api.RpcChan(), pin, nil)
	var resp statusCidResp
	makeGet(t, "/status/"+testCid, &resp)
	if resp.Cid != testCid {
		t.Error("expected the same cid")
	}
	info, ok := resp.Status[testPeerID.Pretty()]
	if !ok {
		t.Fatal("expected info for testPeerID")
	}
	if info.IPFS != "unpinned" {
		t.Error("expected different status")
	}
}

func TestStatusSyncEndpoint(t *testing.T) {
	c, _ := cid.Decode(testCid)
	c2, _ := cid.Decode(testCid2)
	c3, _ := cid.Decode(testCid3)
	api := testClusterApi(t)
	defer api.Shutdown()
	pList := []GlobalPinInfo{
		GlobalPinInfo{
			Status: map[peer.ID]PinInfo{
				testPeerID: PinInfo{
					CidStr: testCid,
					Peer:   testPeerID,
					IPFS:   PinError,
				},
			},
			Cid: c,
		},
		GlobalPinInfo{
			Status: map[peer.ID]PinInfo{
				testPeerID: PinInfo{
					CidStr: testCid2,
					Peer:   testPeerID,
					IPFS:   UnpinError,
				},
			},
			Cid: c2,
		},
		GlobalPinInfo{
			Status: map[peer.ID]PinInfo{
				testPeerID: PinInfo{
					CidStr: testCid3,
					Peer:   testPeerID,
					IPFS:   PinError,
				},
			},
			Cid: c3,
		},
	}

	simulateAnswer(api.RpcChan(), pList, nil)
	var resp statusResp
	makeGet(t, "/status", &resp)
	if len(resp) != 3 {
		t.Errorf("unexpected statusResp:\n %+v", resp)
	}
	if resp[0].Cid != testCid {
		t.Error("unexpected response info")
	}
}

func TestStatusSyncCidEndpoint(t *testing.T) {
	c, _ := cid.Decode(errorCid)
	api := testClusterApi(t)
	defer api.Shutdown()
	pin := GlobalPinInfo{
		Cid: c,
		Status: map[peer.ID]PinInfo{
			testPeerID: PinInfo{
				CidStr: errorCid,
				Peer:   testPeerID,
				IPFS:   PinError,
			},
		},
	}
	simulateAnswer(api.RpcChan(), pin, nil)
	var resp statusCidResp
	makePost(t, "/status/"+testCid, &resp)
	if resp.Cid != errorCid {
		t.Error("expected the same cid")
	}
	info, ok := resp.Status[testPeerID.Pretty()]
	if !ok {
		t.Fatal("expected info for testPeerID")
	}
	if info.IPFS != "pin_error" {
		t.Error("expected different status")
	}
}
