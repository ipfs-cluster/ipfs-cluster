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

// This runs tests using the standard components and the IPFS mock daemon.
// End-to-end means that all default implementations of components are tested
// together. It is not fully end-to-end because the ipfs daemon is a mock which
// never hangs.

// number of clusters to create
var nClusters = 3

// number of pins to pin/unpin/check
var nPins = 1000

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

func createClusters(t *testing.T) ([]*Cluster, *ipfsMock) {
	os.RemoveAll("./e2eTestRaft")
	ipfsmock := newIpfsMock()
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
			IPFSAddr:            ipfsmock.addr,
			IPFSPort:            ipfsmock.port,
		})
	}

	for i := 0; i < nClusters; i++ {
		api, err := NewRESTAPI(cfgs[i])
		checkErr(t, err)
		ipfs, err := NewIPFSHTTPConnector(cfgs[i])
		checkErr(t, err)
		state := NewMapState()
		tracker := NewMapPinTracker()
		remote := NewLibp2pRemote()

		cl, err := NewCluster(cfgs[i], api, ipfs, state, tracker, remote)
		checkErr(t, err)
		clusters = append(clusters, cl)
	}
	return clusters, ipfsmock
}

func shutdownClusters(t *testing.T, clusters []*Cluster, m *ipfsMock) {
	m.Close()
	for _, c := range clusters {
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
		fmt.Println(h)
		checkErr(t, err)
		err = clusters[j].Pin(h)
		if err != nil {
			t.Errorf("error pinning %s: %s", h, err)
		}
	}
	delay()
	f := func(t *testing.T, c *Cluster) {
		status := c.tracker.ListPins()
		for _, v := range status {
			if v.Status != Pinned {
				t.Errorf("%s should have been pinned but it is %s",
					v.Cid.String,
					v.Status.String())
			}
		}
		if l := len(status); l != nPins {
			t.Errorf("Pinned %d out of %d requests", l, nPins)
		}
	}
	runF(t, clusters, f)
}
