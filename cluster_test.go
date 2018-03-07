package ipfscluster

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/allocator/ascendalloc"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/informer/numpin"
	"github.com/ipfs/ipfs-cluster/monitor/basic"
	"github.com/ipfs/ipfs-cluster/pintracker/maptracker"
	"github.com/ipfs/ipfs-cluster/sharder"
	"github.com/ipfs/ipfs-cluster/state/mapstate"
	"github.com/ipfs/ipfs-cluster/test"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
)

type mockComponent struct {
	rpcClient   *rpc.Client
	returnError bool
}

func (c *mockComponent) Shutdown() error {
	return nil
}

func (c *mockComponent) SetClient(client *rpc.Client) {
	c.rpcClient = client
	return
}

type mockAPI struct {
	mockComponent
}

type mockConnector struct {
	mockComponent
}

func (ipfs *mockConnector) ID() (api.IPFSID, error) {
	if ipfs.returnError {
		return api.IPFSID{}, errors.New("")
	}
	return api.IPFSID{
		ID: test.TestPeerID1,
	}, nil
}

func (ipfs *mockConnector) Pin(c *cid.Cid) error {
	if ipfs.returnError {
		return errors.New("")
	}
	return nil
}

func (ipfs *mockConnector) Unpin(c *cid.Cid) error {
	if ipfs.returnError {
		return errors.New("")
	}
	return nil
}

func (ipfs *mockConnector) PinLsCid(c *cid.Cid) (api.IPFSPinStatus, error) {
	if ipfs.returnError {
		return api.IPFSPinStatusError, errors.New("")
	}
	return api.IPFSPinStatusRecursive, nil
}

func (ipfs *mockConnector) PinLs(filter string) (map[string]api.IPFSPinStatus, error) {
	if ipfs.returnError {
		return nil, errors.New("")
	}
	m := make(map[string]api.IPFSPinStatus)
	return m, nil
}

func (ipfs *mockConnector) SwarmPeers() (api.SwarmPeers, error) {
	return []peer.ID{test.TestPeerID4, test.TestPeerID5}, nil
}

func (ipfs *mockConnector) ConnectSwarms() error                          { return nil }
func (ipfs *mockConnector) ConfigKey(keypath string) (interface{}, error) { return nil, nil }
func (ipfs *mockConnector) FreeSpace() (uint64, error)                    { return 100, nil }
func (ipfs *mockConnector) RepoSize() (uint64, error)                     { return 0, nil }
func (ipfs *mockConnector) BlockPut(bwf api.NodeWithMeta) (string, error) { return "", nil }

func testingCluster(t *testing.T) (*Cluster, *mockAPI, *mockConnector, *mapstate.MapState, *maptracker.MapPinTracker) {
	clusterCfg, _, _, consensusCfg, trackerCfg, monCfg, _, sharderCfg := testingConfigs()

	api := &mockAPI{}
	ipfs := &mockConnector{}
	st := mapstate.NewMapState()
	tracker := maptracker.NewMapPinTracker(trackerCfg, clusterCfg.ID)
	monCfg.CheckInterval = 2 * time.Second
	mon, _ := basic.NewMonitor(monCfg)
	alloc := ascendalloc.NewAllocator()
	numpinCfg := &numpin.Config{}
	numpinCfg.Default()
	inf, _ := numpin.NewInformer(numpinCfg)
	sharder, _ := sharder.NewSharder(sharderCfg)

	cl, err := NewCluster(
		clusterCfg,
		consensusCfg,
		api,
		ipfs,
		st,
		tracker,
		mon,
		alloc,
		inf,
		sharder)
	if err != nil {
		t.Fatal("cannot create cluster:", err)
	}
	<-cl.Ready()
	return cl, api, ipfs, st, tracker
}

func cleanRaft() {
	raftDirs, _ := filepath.Glob("raftFolderFromTests*")
	for _, dir := range raftDirs {
		os.RemoveAll(dir)
	}
}

func testClusterShutdown(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	err := cl.Shutdown()
	if err != nil {
		t.Error("cluster shutdown failed:", err)
	}
	cl.Shutdown()
	cl, _, _, _, _ = testingCluster(t)
	err = cl.Shutdown()
	if err != nil {
		t.Error("cluster shutdown failed:", err)
	}
}

func TestClusterStateSync(t *testing.T) {
	cleanRaft()
	cl, _, _, st, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()
	_, err := cl.StateSync()
	if err == nil {
		t.Fatal("expected an error as there is no state to sync")
	}

	c, _ := cid.Decode(test.TestCid1)
	err = cl.Pin(api.PinCid(c))
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	_, err = cl.StateSync()
	if err != nil {
		t.Fatal("sync after pinning should have worked:", err)
	}

	// Modify state on the side so the sync does not
	// happen on an empty slide
	st.Rm(c)
	_, err = cl.StateSync()
	if err != nil {
		t.Fatal("sync with recover should have worked:", err)
	}
}

func TestClusterID(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()
	id := cl.ID()
	if len(id.Addresses) == 0 {
		t.Error("expected more addresses")
	}
	if id.ID == "" {
		t.Error("expected a cluster ID")
	}
	if id.Version != Version {
		t.Error("version should match current version")
	}
	//if id.PublicKey == nil {
	//	t.Error("publicKey should not be empty")
	//}
}

func TestClusterPin(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()

	c, _ := cid.Decode(test.TestCid1)
	err := cl.Pin(api.PinCid(c))
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	// test an error case
	cl.consensus.Shutdown()
	err = cl.Pin(api.Pin{
		Cid:                  c,
		ReplicationFactorMax: 1,
		ReplicationFactorMin: 1,
	})
	if err == nil {
		t.Error("expected an error but things worked")
	}
}

func TestClusterPins(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()

	c, _ := cid.Decode(test.TestCid1)
	err := cl.Pin(api.PinCid(c))
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	pins := cl.Pins()
	if len(pins) != 1 {
		t.Fatal("pin should be part of the state")
	}
	if !pins[0].Cid.Equals(c) || pins[0].ReplicationFactorMin != -1 || pins[0].ReplicationFactorMax != -1 {
		t.Error("the Pin does not look as expected")
	}
}

func TestClusterPinGet(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()

	c, _ := cid.Decode(test.TestCid1)
	err := cl.Pin(api.PinCid(c))
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	pin, err := cl.PinGet(c)
	if err != nil {
		t.Fatal(err)
	}
	if !pin.Cid.Equals(c) || pin.ReplicationFactorMin != -1 || pin.ReplicationFactorMax != -1 {
		t.Error("the Pin does not look as expected")
	}

	c2, _ := cid.Decode(test.TestCid2)
	_, err = cl.PinGet(c2)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestClusterUnpin(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()

	c, _ := cid.Decode(test.TestCid1)
	err := cl.Unpin(c)
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	// test an error case
	cl.consensus.Shutdown()
	err = cl.Unpin(c)
	if err == nil {
		t.Error("expected an error but things worked")
	}
}

func TestClusterPeers(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()
	peers := cl.Peers()
	if len(peers) != 1 {
		t.Fatal("expected 1 peer")
	}

	clusterCfg := &Config{}
	clusterCfg.LoadJSON(testingClusterCfg)
	if peers[0].ID != clusterCfg.ID {
		t.Error("bad member")
	}
}

func TestVersion(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()
	if cl.Version() != Version {
		t.Error("bad Version()")
	}
}

func TestClusterRecoverAllLocal(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()

	c, _ := cid.Decode(test.TestCid1)
	err := cl.Pin(api.PinCid(c))
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	time.Sleep(time.Second)

	recov, err := cl.RecoverAllLocal()
	if err != nil {
		t.Error("did not expect an error")
	}
	if len(recov) != 1 {
		t.Fatal("there should be only one pin")
	}
	if recov[0].Status != api.TrackerStatusPinned {
		t.Error("the pin should have been recovered")
	}
}
