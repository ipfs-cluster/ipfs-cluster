package ipfscluster

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	cid "github.com/ipfs/go-cid"
	crypto "github.com/libp2p/go-libp2p-crypto"
	peer "github.com/libp2p/go-libp2p-peer"
)

var (
	testCid       = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq"
	testCid2      = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmma"
	testCid3      = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmb"
	errorCid      = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmc"
	testPeerID, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")
)

//TestClusters*
var (
	// number of clusters to create
	nClusters = 3

	// number of pins to pin/unpin/check
	nPins = 500

	// ports
	clusterPort = 20000
	apiPort     = 20500
	ipfsApiPort = 21000
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func checkErr(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

func randomBytes() []byte {
	bs := make([]byte, 64, 64)
	for i := 0; i < len(bs); i++ {
		b := byte(rand.Int())
		bs[i] = b
	}
	return bs
}

func createClusters(t *testing.T) ([]*Cluster, []*ipfsMock) {
	os.RemoveAll("./e2eTestRaft")
	ipfsMocks := make([]*ipfsMock, 0, nClusters)
	clusters := make([]*Cluster, 0, nClusters)
	cfgs := make([]*Config, 0, nClusters)

	type peerInfo struct {
		pid  string
		priv string
	}
	peers := make([]peerInfo, 0, nClusters)
	clusterpeers := make([]string, 0, nClusters)

	// Generate keys and ids
	for i := 0; i < nClusters; i++ {
		priv, pub, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
		checkErr(t, err)
		pid, err := peer.IDFromPublicKey(pub)
		checkErr(t, err)
		privBytes, err := priv.Bytes()
		checkErr(t, err)
		b64priv := base64.StdEncoding.EncodeToString(privBytes)
		ma := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/ipfs/%s",
			clusterPort+i,
			pid.Pretty())
		peers = append(peers, peerInfo{pid.Pretty(), b64priv})
		clusterpeers = append(clusterpeers, ma)
		//t.Log(ma)
	}

	// Generate nClusters configs
	for i := 0; i < nClusters; i++ {
		mock := newIpfsMock()
		ipfsMocks = append(ipfsMocks, mock)
		cfgs = append(cfgs, &Config{
			ID:                  peers[i].pid,
			PrivateKey:          peers[i].priv,
			ClusterPeers:        clusterpeers,
			ClusterAddr:         "127.0.0.1",
			ClusterPort:         clusterPort + i,
			ConsensusDataFolder: "./e2eTestRaft/" + peers[i].pid,
			APIAddr:             "127.0.0.1",
			APIPort:             apiPort + i,
			IPFSAPIAddr:         "127.0.0.1",
			IPFSAPIPort:         ipfsApiPort + i,
			IPFSAddr:            mock.addr,
			IPFSPort:            mock.port,
		})
	}

	for i := 0; i < nClusters; i++ {
		api, err := NewRESTAPI(cfgs[i])
		checkErr(t, err)
		ipfs, err := NewIPFSHTTPConnector(cfgs[i])
		checkErr(t, err)
		state := NewMapState()
		tracker := NewMapPinTracker(cfgs[i])
		remote := NewLibp2pRemote()

		cl, err := NewCluster(cfgs[i], api, ipfs, state, tracker, remote)
		checkErr(t, err)
		clusters = append(clusters, cl)
	}
	return clusters, ipfsMocks
}

func shutdownClusters(t *testing.T, clusters []*Cluster, m []*ipfsMock) {
	for i, c := range clusters {
		m[i].Close()
		err := c.Shutdown()
		if err != nil {
			t.Error(err)
		}
	}
	os.RemoveAll("./e2eTestRaft")
}

func runF(t *testing.T, clusters []*Cluster, f func(*testing.T, *Cluster)) {
	var wg sync.WaitGroup
	for _, c := range clusters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f(t, c)
		}()

	}
	wg.Wait()
}

func delay() {
	time.Sleep(time.Duration(nClusters) * time.Second)
}

func TestMakeRPC(t *testing.T) {
	testCh := make(chan RPC, 1)
	testReq := NewRPC(MemberListRPC, nil)
	testResp := RPCResponse{
		Data:  "hey",
		Error: nil,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Test wait for response
	go func() {
		req := <-testCh
		if req.Op() != MemberListRPC {
			t.Fatal("expected a MemberListRPC")
		}
		req.ResponseCh() <- testResp
	}()

	resp := MakeRPC(ctx, testCh, testReq, true)
	data, ok := resp.Data.(string)
	if !ok || data != "hey" {
		t.Error("bad response")
	}

	// Test not waiting for response
	resp = MakeRPC(ctx, testCh, testReq, false)
	if resp.Data != nil || resp.Error != nil {
		t.Error("expected empty response")
	}

	// Test full channel and cancel on send
	go func() {
		resp := MakeRPC(ctx, testCh, testReq, true)
		if resp.Error == nil || !strings.Contains(resp.Error.Error(), "timed out") {
			t.Fatal("the operation should have been waiting and then cancelled")
		}
	}()
	// Previous request still taking the channel
	time.Sleep(1 * time.Second)
	cancel()

	// Test cancelled while waiting for response context
	ctx, cancel = context.WithCancel(context.Background())
	testCh = make(chan RPC, 1)
	go func() {
		resp := MakeRPC(ctx, testCh, testReq, true)
		if resp.Error == nil || !strings.Contains(resp.Error.Error(), "timed out") {
			t.Fatal("the operation should have been waiting and then cancelled")
		}
	}()
	time.Sleep(1 * time.Second)
	cancel()
	time.Sleep(1 * time.Second)
}

func simulateAnswer(ch <-chan RPC, answer interface{}, err error) {
	go func() {
		req := <-ch
		req.ResponseCh() <- RPCResponse{
			Data:  answer,
			Error: CastRPCError(err),
		}
	}()
}

func TestClustersVersion(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	f := func(t *testing.T, c *Cluster) {
		v := c.Version()
		if v != Version {
			t.Error("Bad version")
		}
	}
	runF(t, clusters, f)
}

func TestClustersPin(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	delay()
	exampleCid, _ := cid.Decode(testCid)
	prefix := exampleCid.Prefix()
	for i := 0; i < nPins; i++ {
		j := rand.Intn(nClusters)           // choose a random cluster member
		h, err := prefix.Sum(randomBytes()) // create random cid
		checkErr(t, err)
		err = clusters[j].Pin(h)
		if err != nil {
			t.Errorf("error pinning %s: %s", h, err)
		}
		// Test re-pin
		err = clusters[j].Pin(h)
		if err != nil {
			t.Errorf("error repinning %s: %s", h, err)
		}
	}
	delay()
	fpinned := func(t *testing.T, c *Cluster) {
		status := c.tracker.Status()
		for _, v := range status {
			if v.IPFS != Pinned {
				t.Errorf("%s should have been pinned but it is %s",
					v.CidStr,
					v.IPFS.String())
			}
		}
		if l := len(status); l != nPins {
			t.Errorf("Pinned %d out of %d requests", l, nPins)
		}
	}
	runF(t, clusters, fpinned)

	// Unpin everything
	pinList := clusters[0].Pins()

	for i := 0; i < nPins; i++ {
		j := rand.Intn(nClusters) // choose a random cluster member
		err := clusters[j].Unpin(pinList[i])
		if err != nil {
			t.Errorf("error unpinning %s: %s", pinList[i], err)
		}
		// test re-unpin
		err = clusters[j].Unpin(pinList[i])
		if err != nil {
			t.Errorf("error re-unpinning %s: %s", pinList[i], err)
		}

	}
	delay()

	funpinned := func(t *testing.T, c *Cluster) {
		status := c.tracker.Status()
		if l := len(status); l != 0 {
			t.Errorf("Nothing should be pinned")
			//t.Errorf("%+v", status)
		}
	}
	runF(t, clusters, funpinned)
}

func TestClustersStatus(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	delay()
	h, _ := cid.Decode(testCid)
	clusters[0].Pin(h)
	delay()
	// Global status
	f := func(t *testing.T, c *Cluster) {
		statuses, err := c.Status()
		if err != nil {
			t.Error(err)
		}
		if len(statuses) == 0 {
			t.Fatal("bad status. Expected one item")
		}
		if statuses[0].Cid.String() != testCid {
			t.Error("bad cid in status")
		}
		info := statuses[0].Status
		if len(info) != nClusters {
			t.Error("bad info in status")
		}

		if info[c.host.ID()].IPFS != Pinned {
			t.Error("the hash should have been pinned")
		}

		status, err := c.StatusCid(h)
		if err != nil {
			t.Error(err)
		}

		pinfo, ok := status.Status[c.host.ID()]
		if !ok {
			t.Fatal("Host not in status")
		}

		if pinfo.IPFS != Pinned {
			t.Error("the status should show the hash as pinned")
		}
	}
	runF(t, clusters, f)
}

func TestClustersLocalSync(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	delay()
	h, _ := cid.Decode(errorCid) // This cid always fails
	h2, _ := cid.Decode(testCid2)
	clusters[0].Pin(h)
	clusters[0].Pin(h2)
	delay()
	f := func(t *testing.T, c *Cluster) {
		// Sync bad ID
		infos, err := c.LocalSync()
		if err != nil {
			// LocalSync() is asynchronous and should not show an
			// error even if Recover() fails.
			t.Error(err)
		}
		if len(infos) != 1 {
			t.Fatal("expected 1 elem slice")
		}
		// Last-known state may still be pinning
		if infos[0].IPFS != PinError && infos[0].IPFS != Pinning {
			t.Error("element should be in Pinning or PinError state")
		}
	}
	// Test Local syncs
	runF(t, clusters, f)
}

func TestClustersLocalSyncCid(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	delay()
	h, _ := cid.Decode(errorCid) // This cid always fails
	h2, _ := cid.Decode(testCid2)
	clusters[0].Pin(h)
	clusters[0].Pin(h2)
	delay()

	f := func(t *testing.T, c *Cluster) {
		info, err := c.LocalSyncCid(h)
		if err == nil {
			// LocalSyncCid is synchronous
			t.Error("expected an error")
		}
		if info.IPFS != PinError && info.IPFS != Pinning {
			t.Errorf("element is %s and not PinError", info.IPFS)
		}

		// Sync good ID
		info, err = c.LocalSyncCid(h2)
		if err != nil {
			t.Error(err)
		}
		if info.IPFS != Pinned {
			t.Error("element should be in Pinned state")
		}
	}
	// Test Local syncs
	runF(t, clusters, f)
}

func TestClustersGlobalSync(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	delay()
	h, _ := cid.Decode(errorCid) // This cid always fails
	h2, _ := cid.Decode(testCid2)
	clusters[0].Pin(h)
	clusters[0].Pin(h2)
	delay()

	j := rand.Intn(nClusters) // choose a random cluster member
	ginfos, err := clusters[j].GlobalSync()
	if err != nil {
		t.Fatal(err)
	}
	if len(ginfos) != 1 {
		t.Fatal("expected globalsync to have 1 elements")
	}
	if ginfos[0].Cid.String() != errorCid {
		t.Error("expected globalsync to have problems with errorCid")
	}
	for _, c := range clusters {
		inf, ok := ginfos[0].Status[c.host.ID()]
		if !ok {
			t.Fatal("GlobalPinInfo should have this cluster")
		}
		if inf.IPFS != PinError && inf.IPFS != Pinning {
			t.Error("should be PinError in all members")
		}
	}
}

func TestClustersGlobalSyncCid(t *testing.T) {
	clusters, mock := createClusters(t)
	defer shutdownClusters(t, clusters, mock)
	delay()
	h, _ := cid.Decode(errorCid) // This cid always fails
	h2, _ := cid.Decode(testCid2)
	clusters[0].Pin(h)
	clusters[0].Pin(h2)
	delay()

	j := rand.Intn(nClusters)
	ginfo, err := clusters[j].GlobalSyncCid(h)
	if err == nil {
		t.Error("expected an error")
	}
	if ginfo.Cid.String() != errorCid {
		t.Error("GlobalPinInfo should be for errorCid")
	}

	for _, c := range clusters {
		inf, ok := ginfo.Status[c.host.ID()]
		if !ok {
			t.Fatal("GlobalPinInfo should not be empty for this host")
		}

		if inf.IPFS != PinError && inf.IPFS != Pinning {
			t.Error("should be PinError or Pinning in all members")
		}
	}

	// Test with a good Cid
	j = rand.Intn(nClusters)
	ginfo, err = clusters[j].GlobalSyncCid(h2)
	if err != nil {
		t.Fatal(err)
	}
	if ginfo.Cid.String() != testCid2 {
		t.Error("GlobalPinInfo should be for testrCid2")
	}

	for _, c := range clusters {
		inf, ok := ginfo.Status[c.host.ID()]
		if !ok {
			t.Fatal("GlobalPinInfo should have this cluster")
		}
		if inf.IPFS != Pinned {
			t.Error("the GlobalPinInfo should show Pinned in all members")
		}
	}
}
