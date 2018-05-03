package ipfscluster

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/allocator/ascendalloc"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/consensus/raft"
	"github.com/ipfs/ipfs-cluster/informer/numpin"
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

func (ipfs *mockConnector) Pin(ctx context.Context, c *cid.Cid, b bool) error {
	if ipfs.returnError {
		return errors.New("")
	}
	return nil
}

func (ipfs *mockConnector) Unpin(ctx context.Context, c *cid.Cid) error {
	if ipfs.returnError {
		return errors.New("")
	}
	return nil
}

func (ipfs *mockConnector) PinLsCid(ctx context.Context, c *cid.Cid) (api.IPFSPinStatus, error) {
	if ipfs.returnError {
		return api.IPFSPinStatusError, errors.New("")
	}
	return api.IPFSPinStatusRecursive, nil
}

func (ipfs *mockConnector) PinLs(ctx context.Context, filter string) (map[string]api.IPFSPinStatus, error) {
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
func (ipfs *mockConnector) BlockPut(nwm api.NodeWithMeta) error           { return nil }

func (ipfs *mockConnector) BlockGet(c *cid.Cid) ([]byte, error) {
	switch c.String() {
	case test.TestShardCid:
		return test.TestShardData, nil
	case test.TestCdagCid:
		return test.TestCdagData, nil
	case test.TestShardCid2:
		return test.TestShard2Data, nil
	case test.TestCdagCid2:
		return test.TestCdagData2, nil
	default:
		return nil, errors.New("block not found")
	}
}

func testingCluster(t *testing.T) (*Cluster, *mockAPI, *mockConnector, *mapstate.MapState, *maptracker.MapPinTracker) {
	clusterCfg, _, _, consensusCfg, trackerCfg, bmonCfg, psmonCfg, _, sharderCfg := testingConfigs()

	host, err := NewClusterHost(context.Background(), clusterCfg)
	if err != nil {
		t.Fatal(err)
	}

	api := &mockAPI{}
	ipfs := &mockConnector{}
	st := mapstate.NewMapState()
	tracker := maptracker.NewMapPinTracker(trackerCfg, clusterCfg.ID)

	raftcon, _ := raft.NewConsensus(host, consensusCfg, st, false)

	bmonCfg.CheckInterval = 2 * time.Second
	psmonCfg.CheckInterval = 2 * time.Second
	mon := makeMonitor(t, host, bmonCfg, psmonCfg)

	alloc := ascendalloc.NewAllocator()
	numpinCfg := &numpin.Config{}
	numpinCfg.Default()
	inf, _ := numpin.NewInformer(numpinCfg)
	sharder, _ := sharder.NewSharder(sharderCfg)

	ReadyTimeout = consensusCfg.WaitForLeaderTimeout + 1*time.Second

	cl, err := NewCluster(
		host,
		clusterCfg,
		raftcon,
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
	err := cl.StateSync()
	if err == nil {
		t.Fatal("expected an error as there is no state to sync")
	}

	c, _ := cid.Decode(test.TestCid1)
	err = cl.Pin(api.PinCid(c))
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	err = cl.StateSync()
	if err != nil {
		t.Fatal("sync after pinning should have worked:", err)
	}

	// Modify state on the side so the sync does not
	// happen on an empty slide
	st.Rm(c)
	err = cl.StateSync()
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

func singleShardedPin(t *testing.T, cl *Cluster) {
	cShard, _ := cid.Decode(test.TestShardCid)
	cCdag, _ := cid.Decode(test.TestCdagCid)
	cMeta, _ := cid.Decode(test.TestMetaRootCid)
	pinMeta(t, cl, []*cid.Cid{cShard}, cCdag, cMeta)
}

func pinMeta(t *testing.T, cl *Cluster, shardCids []*cid.Cid, cCdag, cMeta *cid.Cid) {
	for _, cShard := range shardCids {
		parents := cid.NewSet()
		parents.Add(cCdag)
		shardPin := api.Pin{
			Cid:                  cShard,
			Type:                 api.ShardType,
			ReplicationFactorMin: -1,
			ReplicationFactorMax: -1,
			Recursive:            true,
			Parents:              parents,
		}
		err := cl.Pin(shardPin)
		if err != nil {
			t.Fatal("shard pin should have worked:", err)
		}
	}

	parents := cid.NewSet()
	parents.Add(cMeta)
	cdagPin := api.Pin{
		Cid:                  cCdag,
		Type:                 api.CdagType,
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
		Recursive:            false,
		Parents:              parents,
	}
	err := cl.Pin(cdagPin)
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	metaPin := api.Pin{
		Cid:        cMeta,
		Type:       api.MetaType,
		Clusterdag: cCdag,
	}
	err = cl.Pin(metaPin)
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}
}

func TestClusterPinMeta(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()

	singleShardedPin(t, cl)
}

func TestClusterUnpinShardFail(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()

	singleShardedPin(t, cl)
	// verify pins
	if len(cl.Pins()) != 3 {
		t.Fatal("should have 3 pins")
	}
	// Unpinning metadata should fail
	cShard, _ := cid.Decode(test.TestShardCid)
	cCdag, _ := cid.Decode(test.TestCdagCid)

	err := cl.Unpin(cShard)
	if err == nil {
		t.Error("should error when unpinning shard")
	}
	err = cl.Unpin(cCdag)
	if err == nil {
		t.Error("should error when unpinning cluster dag")
	}
}

func TestClusterUnpinMeta(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()

	singleShardedPin(t, cl)
	// verify pins
	if len(cl.Pins()) != 3 {
		t.Fatal("should have 3 pins")
	}
	// Unpinning from root should work
	cMeta, _ := cid.Decode(test.TestMetaRootCid)

	err := cl.Unpin(cMeta)
	if err != nil {
		t.Error(err)
	}
}

func pinTwoParentsOneShard(t *testing.T, cl *Cluster) {
	singleShardedPin(t, cl)

	cShard, _ := cid.Decode(test.TestShardCid)
	cShard2, _ := cid.Decode(test.TestShardCid2)
	cCdag2, _ := cid.Decode(test.TestCdagCid2)
	cMeta2, _ := cid.Decode(test.TestMetaRootCid2)
	pinMeta(t, cl, []*cid.Cid{cShard, cShard2}, cCdag2, cMeta2)

	shardPin, err := cl.PinGet(cShard)
	if err != nil {
		t.Fatal("pin should be in state")
	}
	if shardPin.Parents.Len() != 2 {
		t.Fatal("unexpected parent set in shared shard")
	}

	shardPin2, err := cl.PinGet(cShard2)
	if shardPin2.Parents.Len() != 1 {
		t.Fatal("unexpected parent set in unshared shard")
	}
	if err != nil {
		t.Fatal("pin should be in state")
	}
}

func TestClusterPinShardTwoParents(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()

	pinTwoParentsOneShard(t, cl)

	cShard, _ := cid.Decode(test.TestShardCid)
	shardPin, err := cl.PinGet(cShard)
	if err != nil {
		t.Fatal("double pinned shard should be pinned")
	}
	if shardPin.Parents == nil || shardPin.Parents.Len() != 2 {
		t.Fatal("double pinned shard should have two parents")
	}
}

func TestClusterUnpinShardSecondParent(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()

	pinTwoParentsOneShard(t, cl)
	if len(cl.Pins()) != 6 {
		t.Fatal("should have 6 pins")
	}
	cMeta2, _ := cid.Decode(test.TestMetaRootCid2)
	err := cl.Unpin(cMeta2)
	if err != nil {
		t.Error(err)
	}

	pinDelay()

	if len(cl.Pins()) != 3 {
		t.Fatal("should have 3 pins")
	}

	cShard, _ := cid.Decode(test.TestShardCid)
	cCdag, _ := cid.Decode(test.TestCdagCid)
	shardPin, err := cl.PinGet(cShard)
	if err != nil {
		t.Fatal("double pinned shard node should still be pinned")
	}
	if shardPin.Parents == nil || shardPin.Parents.Len() != 1 ||
		!shardPin.Parents.Has(cCdag) {
		t.Fatalf("shard node should have single original parent %v", shardPin.Parents.Keys())
	}
}

func TestClusterUnpinShardFirstParent(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()

	pinTwoParentsOneShard(t, cl)
	if len(cl.Pins()) != 6 {
		t.Fatal("should have 6 pins")
	}

	cMeta, _ := cid.Decode(test.TestMetaRootCid)
	err := cl.Unpin(cMeta)
	if err != nil {
		t.Error(err)
	}
	if len(cl.Pins()) != 4 {
		t.Fatal("should have 4 pins")
	}

	cShard, _ := cid.Decode(test.TestShardCid)
	cShard2, _ := cid.Decode(test.TestShardCid2)
	cCdag2, _ := cid.Decode(test.TestCdagCid2)
	shardPin, err := cl.PinGet(cShard)
	if err != nil {
		t.Fatal("double pinned shard node should still be pinned")
	}
	if shardPin.Parents == nil || shardPin.Parents.Len() != 1 ||
		!shardPin.Parents.Has(cCdag2) {
		t.Fatal("shard node should have single original parent")
	}
	_, err = cl.PinGet(cShard2)
	if err != nil {
		t.Fatal("other shard shoud still be pinned too")
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
	// Unpin should error without pin being committed to state
	err := cl.Unpin(c)
	if err == nil {
		t.Error("unpin should have failed")
	}

	// Unpin after pin should succeed
	err = cl.Pin(api.PinCid(c))
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}
	err = cl.Unpin(c)
	if err != nil {
		t.Error("unpin should have worked:", err)
	}

	// test another error case
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

	pinDelay()

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
