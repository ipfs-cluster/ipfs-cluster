package ipfscluster

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	cid "github.com/ipfs/go-cid"

	crypto "github.com/libp2p/go-libp2p-crypto"
	peer "github.com/libp2p/go-libp2p-peer"
)

// This runs end-to-end tests using the standard components and the IPFS mock
// daemon.
// End-to-end means that all default implementations of components are tested
// together. It is not fully end-to-end because the ipfs daemon is a mock which
// never hangs.

// number of clusters to create
var nClusters = 3

// number of pins to pin/unpin/check
var nPins = 500

// ports
var clusterPort = 20000
var apiPort = 20500
var ipfsApiPort = 21000

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

func TestE2EVersion(t *testing.T) {
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

func TestE2EPin(t *testing.T) {
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
		status := c.tracker.LocalStatus()
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
		status := c.tracker.LocalStatus()
		if l := len(status); l != 0 {
			t.Errorf("Nothing should be pinned")
			//t.Errorf("%+v", status)
		}
	}
	runF(t, clusters, funpinned)
}

func TestE2EStatus(t *testing.T) {
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
