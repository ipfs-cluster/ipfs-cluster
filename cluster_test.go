package ipfscluster

import (
	"context"
	"errors"
	"mime/multipart"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/elastos/Elastos.NET.Hive.Cluster/adder/sharding"
	"github.com/elastos/Elastos.NET.Hive.Cluster/allocator/ascendalloc"
	"github.com/elastos/Elastos.NET.Hive.Cluster/api"
	"github.com/elastos/Elastos.NET.Hive.Cluster/consensus/raft"
	"github.com/elastos/Elastos.NET.Hive.Cluster/informer/numpin"
	"github.com/elastos/Elastos.NET.Hive.Cluster/state"
	"github.com/elastos/Elastos.NET.Hive.Cluster/state/mapstate"
	"github.com/elastos/Elastos.NET.Hive.Cluster/test"
	"github.com/elastos/Elastos.NET.Hive.Cluster/version"

	cid "github.com/ipfs/go-cid"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p-peer"
)

type mockComponent struct {
	rpcClient *rpc.Client
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

type mockProxy struct {
	mockComponent
}

type mockConnector struct {
	mockComponent

	pins   sync.Map
	blocks sync.Map
}

func (ipfs *mockConnector) ID() (api.IPFSID, error) {
	return api.IPFSID{
		ID: test.TestPeerID1,
	}, nil
}

func (ipfs *mockConnector) Pin(ctx context.Context, c cid.Cid, maxDepth int) error {
	ipfs.pins.Store(c.String(), maxDepth)
	return nil
}

func (ipfs *mockConnector) Unpin(ctx context.Context, c cid.Cid) error {
	ipfs.pins.Delete(c.String())
	return nil
}

func (ipfs *mockConnector) PinLsCid(ctx context.Context, c cid.Cid) (api.IPFSPinStatus, error) {
	dI, ok := ipfs.pins.Load(c.String())
	if !ok {
		return api.IPFSPinStatusUnpinned, nil
	}
	depth := dI.(int)
	if depth == 0 {
		return api.IPFSPinStatusDirect, nil
	}
	return api.IPFSPinStatusRecursive, nil
}

func (ipfs *mockConnector) PinLs(ctx context.Context, filter string) (map[string]api.IPFSPinStatus, error) {
	m := make(map[string]api.IPFSPinStatus)
	var st api.IPFSPinStatus
	ipfs.pins.Range(func(k, v interface{}) bool {
		switch v.(int) {
		case 0:
			st = api.IPFSPinStatusDirect
		default:
			st = api.IPFSPinStatusRecursive
		}

		m[k.(string)] = st
		return true
	})

	return m, nil
}

func (ipfs *mockConnector) SwarmPeers() (api.SwarmPeers, error) {
	return []peer.ID{test.TestPeerID4, test.TestPeerID5}, nil
}

func (ipfs *mockConnector) RepoStat() (api.IPFSRepoStat, error) {
	return api.IPFSRepoStat{RepoSize: 100, StorageMax: 1000}, nil
}

func (ipfs *mockConnector) ConnectSwarms() error                          { return nil }
func (ipfs *mockConnector) ConfigKey(keypath string) (interface{}, error) { return nil, nil }

func (ipfs *mockConnector) BlockPut(nwm api.NodeWithMeta) error {
	ipfs.blocks.Store(nwm.Cid, nwm.Data)
	return nil
}

func (ipfs *mockConnector) BlockGet(c cid.Cid) ([]byte, error) {
	d, ok := ipfs.blocks.Load(c.String())
	if !ok {
		return nil, errors.New("block not found")
	}
	return d.([]byte), nil
}

func testingCluster(t *testing.T) (*Cluster, *mockAPI, *mockConnector, state.State, PinTracker) {
	clusterCfg, _, _, _, consensusCfg, maptrackerCfg, statelesstrackerCfg, bmonCfg, psmonCfg, _ := testingConfigs()

	host, err := NewClusterHost(context.Background(), clusterCfg)
	if err != nil {
		t.Fatal(err)
	}

	api := &mockAPI{}
	proxy := &mockProxy{}
	ipfs := &mockConnector{}
	st := mapstate.NewMapState()
	tracker := makePinTracker(t, clusterCfg.ID, maptrackerCfg, statelesstrackerCfg, clusterCfg.Peername)

	raftcon, _ := raft.NewConsensus(host, consensusCfg, st, false)

	bmonCfg.CheckInterval = 2 * time.Second
	psmonCfg.CheckInterval = 2 * time.Second
	mon := makeMonitor(t, host, bmonCfg, psmonCfg)

	alloc := ascendalloc.NewAllocator()
	numpinCfg := &numpin.Config{}
	numpinCfg.Default()
	inf, _ := numpin.NewInformer(numpinCfg)

	ReadyTimeout = consensusCfg.WaitForLeaderTimeout + 1*time.Second

	cl, err := NewCluster(
		host,
		clusterCfg,
		raftcon,
		[]API{api, proxy},
		ipfs,
		st,
		tracker,
		mon,
		alloc,
		inf,
	)
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
	if id.Version != version.Version.String() {
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
	pin := api.PinCid(c)
	pin.ReplicationFactorMax = 1
	pin.ReplicationFactorMin = 1
	err = cl.Pin(pin)
	if err == nil {
		t.Error("expected an error but things worked")
	}
}

func TestAddFile(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	t.Run("local", func(t *testing.T) {
		params := api.DefaultAddParams()
		params.Shard = false
		params.Name = "testlocal"
		mfr, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		r := multipart.NewReader(mfr, mfr.Boundary())
		c, err := cl.AddFile(r, params)
		if err != nil {
			t.Fatal(err)
		}
		if c.String() != test.ShardingDirBalancedRootCID {
			t.Fatal("unexpected root CID for local add")
		}

		pinDelay()

		pin := cl.StatusLocal(c)
		if pin.Error != "" {
			t.Fatal(pin.Error)
		}
		if pin.Status != api.TrackerStatusPinned {
			t.Error("cid should be pinned")
		}

		cl.Unpin(c) // unpin so we can pin the shard in next test
		pinDelay()
	})

	t.Run("shard", func(t *testing.T) {
		params := api.DefaultAddParams()
		params.Shard = true
		params.Name = "testshard"
		mfr, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		r := multipart.NewReader(mfr, mfr.Boundary())
		c, err := cl.AddFile(r, params)
		if err != nil {
			t.Fatal(err)
		}

		if c.String() != test.ShardingDirBalancedRootCID {
			t.Fatal("unexpected root CID for local add")
		}

		pinDelay()

		// We know that this produces 14 shards.
		sharding.VerifyShards(t, c, cl, cl.ipfs, 14)
	})
}

func TestUnpinShard(t *testing.T) {
	cl, _, _, _, _ := testingCluster(t)
	defer cleanRaft()
	defer cl.Shutdown()
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	params := api.DefaultAddParams()
	params.Shard = true
	params.Name = "testshard"
	mfr, closer := sth.GetTreeMultiReader(t)
	defer closer.Close()
	r := multipart.NewReader(mfr, mfr.Boundary())
	root, err := cl.AddFile(r, params)
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	// We know that this produces 14 shards.
	sharding.VerifyShards(t, root, cl, cl.ipfs, 14)

	// skipping errors, VerifyShards has checked
	pinnedCids := []cid.Cid{}
	pinnedCids = append(pinnedCids, root)
	metaPin, _ := cl.PinGet(root)
	cDag, _ := cl.PinGet(metaPin.Reference)
	pinnedCids = append(pinnedCids, cDag.Cid)
	cDagBlock, _ := cl.ipfs.BlockGet(cDag.Cid)
	cDagNode, _ := sharding.CborDataToNode(cDagBlock, "cbor")
	for _, l := range cDagNode.Links() {
		pinnedCids = append(pinnedCids, l.Cid)
	}

	t.Run("unpin clusterdag should fail", func(t *testing.T) {
		err := cl.Unpin(cDag.Cid)
		if err == nil {
			t.Fatal("should not allow unpinning the cluster DAG directly")
		}
		t.Log(err)
	})

	t.Run("unpin shard should fail", func(t *testing.T) {
		err := cl.Unpin(cDagNode.Links()[0].Cid)
		if err == nil {
			t.Fatal("should not allow unpinning shards directly")
		}
		t.Log(err)
	})

	t.Run("normal unpin", func(t *testing.T) {
		err := cl.Unpin(root)
		if err != nil {
			t.Fatal(err)
		}

		pinDelay()

		for _, c := range pinnedCids {
			st := cl.StatusLocal(c)
			if st.Status != api.TrackerStatusUnpinned {
				t.Errorf("%s should have been unpinned but is %s", c, st.Status)
			}

			st2, err := cl.ipfs.PinLsCid(context.Background(), c)
			if err != nil {
				t.Fatal(err)
			}
			if st2 != api.IPFSPinStatusUnpinned {
				t.Errorf("%s should have been unpinned in ipfs but is %d", c, st2)
			}
		}
	})
}

// func singleShardedPin(t *testing.T, cl *Cluster) {
// 	cShard, _ := cid.Decode(test.TestShardCid)
// 	cCdag, _ := cid.Decode(test.TestCdagCid)
// 	cMeta, _ := cid.Decode(test.TestMetaRootCid)
// 	pinMeta(t, cl, []cid.Cid{cShard}, cCdag, cMeta)
// }

// func pinMeta(t *testing.T, cl *Cluster, shardCids []cid.Cid, cCdag, cMeta cid.Cid) {
// 	for _, cShard := range shardCids {
// 		shardPin := api.Pin{
// 			Cid:      cShard,
// 			Type:     api.ShardType,
// 			MaxDepth: 1,
// 			PinOptions: api.PinOptions{
// 				ReplicationFactorMin: -1,
// 				ReplicationFactorMax: -1,
// 			},
// 		}
// 		err := cl.Pin(shardPin)
// 		if err != nil {
// 			t.Fatal("shard pin should have worked:", err)
// 		}
// 	}

// 	parents := cid.NewSet()
// 	parents.Add(cMeta)
// 	cdagPin := api.Pin{
// 		Cid:      cCdag,
// 		Type:     api.ClusterDAGType,
// 		MaxDepth: 0,
// 		PinOptions: api.PinOptions{
// 			ReplicationFactorMin: -1,
// 			ReplicationFactorMax: -1,
// 		},
// 	}
// 	err := cl.Pin(cdagPin)
// 	if err != nil {
// 		t.Fatal("pin should have worked:", err)
// 	}

// 	metaPin := api.Pin{
// 		Cid:        cMeta,
// 		Type:       api.MetaType,
// 		Clusterdag: cCdag,
// 	}
// 	err = cl.Pin(metaPin)
// 	if err != nil {
// 		t.Fatal("pin should have worked:", err)
// 	}
// }

// func TestClusterPinMeta(t *testing.T) {
// 	cl, _, _, _, _ := testingCluster(t)
// 	defer cleanRaft()
// 	defer cl.Shutdown()

// 	singleShardedPin(t, cl)
// }

// func TestClusterUnpinShardFail(t *testing.T) {
// 	cl, _, _, _, _ := testingCluster(t)
// 	defer cleanRaft()
// 	defer cl.Shutdown()

// 	singleShardedPin(t, cl)
// 	// verify pins
// 	if len(cl.Pins()) != 3 {
// 		t.Fatal("should have 3 pins")
// 	}
// 	// Unpinning metadata should fail
// 	cShard, _ := cid.Decode(test.TestShardCid)
// 	cCdag, _ := cid.Decode(test.TestCdagCid)

// 	err := cl.Unpin(cShard)
// 	if err == nil {
// 		t.Error("should error when unpinning shard")
// 	}
// 	err = cl.Unpin(cCdag)
// 	if err == nil {
// 		t.Error("should error when unpinning cluster dag")
// 	}
// }

// func TestClusterUnpinMeta(t *testing.T) {
// 	cl, _, _, _, _ := testingCluster(t)
// 	defer cleanRaft()
// 	defer cl.Shutdown()

// 	singleShardedPin(t, cl)
// 	// verify pins
// 	if len(cl.Pins()) != 3 {
// 		t.Fatal("should have 3 pins")
// 	}
// 	// Unpinning from root should work
// 	cMeta, _ := cid.Decode(test.TestMetaRootCid)

// 	err := cl.Unpin(cMeta)
// 	if err != nil {
// 		t.Error(err)
// 	}
// }

// func pinTwoParentsOneShard(t *testing.T, cl *Cluster) {
// 	singleShardedPin(t, cl)

// 	cShard, _ := cid.Decode(test.TestShardCid)
// 	cShard2, _ := cid.Decode(test.TestShardCid2)
// 	cCdag2, _ := cid.Decode(test.TestCdagCid2)
// 	cMeta2, _ := cid.Decode(test.TestMetaRootCid2)
// 	pinMeta(t, cl, []cid.Cid{cShard, cShard2}, cCdag2, cMeta2)

// 	shardPin, err := cl.PinGet(cShard)
// 	if err != nil {
// 		t.Fatal("pin should be in state")
// 	}
// 	if shardPin.Parents.Len() != 2 {
// 		t.Fatal("unexpected parent set in shared shard")
// 	}

// 	shardPin2, err := cl.PinGet(cShard2)
// 	if shardPin2.Parents.Len() != 1 {
// 		t.Fatal("unexpected parent set in unshared shard")
// 	}
// 	if err != nil {
// 		t.Fatal("pin should be in state")
// 	}
// }

// func TestClusterPinShardTwoParents(t *testing.T) {
// 	cl, _, _, _, _ := testingCluster(t)
// 	defer cleanRaft()
// 	defer cl.Shutdown()

// 	pinTwoParentsOneShard(t, cl)

// 	cShard, _ := cid.Decode(test.TestShardCid)
// 	shardPin, err := cl.PinGet(cShard)
// 	if err != nil {
// 		t.Fatal("double pinned shard should be pinned")
// 	}
// 	if shardPin.Parents == nil || shardPin.Parents.Len() != 2 {
// 		t.Fatal("double pinned shard should have two parents")
// 	}
// }

// func TestClusterUnpinShardSecondParent(t *testing.T) {
// 	cl, _, _, _, _ := testingCluster(t)
// 	defer cleanRaft()
// 	defer cl.Shutdown()

// 	pinTwoParentsOneShard(t, cl)
// 	if len(cl.Pins()) != 6 {
// 		t.Fatal("should have 6 pins")
// 	}
// 	cMeta2, _ := cid.Decode(test.TestMetaRootCid2)
// 	err := cl.Unpin(cMeta2)
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	pinDelay()

// 	if len(cl.Pins()) != 3 {
// 		t.Fatal("should have 3 pins")
// 	}

// 	cShard, _ := cid.Decode(test.TestShardCid)
// 	cCdag, _ := cid.Decode(test.TestCdagCid)
// 	shardPin, err := cl.PinGet(cShard)
// 	if err != nil {
// 		t.Fatal("double pinned shard node should still be pinned")
// 	}
// 	if shardPin.Parents == nil || shardPin.Parents.Len() != 1 ||
// 		!shardPin.Parents.Has(cCdag) {
// 		t.Fatalf("shard node should have single original parent %v", shardPin.Parents.Keys())
// 	}
// }

// func TestClusterUnpinShardFirstParent(t *testing.T) {
// 	cl, _, _, _, _ := testingCluster(t)
// 	defer cleanRaft()
// 	defer cl.Shutdown()

// 	pinTwoParentsOneShard(t, cl)
// 	if len(cl.Pins()) != 6 {
// 		t.Fatal("should have 6 pins")
// 	}

// 	cMeta, _ := cid.Decode(test.TestMetaRootCid)
// 	err := cl.Unpin(cMeta)
// 	if err != nil {
// 		t.Error(err)
// 	}
// 	if len(cl.Pins()) != 4 {
// 		t.Fatal("should have 4 pins")
// 	}

// 	cShard, _ := cid.Decode(test.TestShardCid)
// 	cShard2, _ := cid.Decode(test.TestShardCid2)
// 	cCdag2, _ := cid.Decode(test.TestCdagCid2)
// 	shardPin, err := cl.PinGet(cShard)
// 	if err != nil {
// 		t.Fatal("double pinned shard node should still be pinned")
// 	}
// 	if shardPin.Parents == nil || shardPin.Parents.Len() != 1 ||
// 		!shardPin.Parents.Has(cCdag2) {
// 		t.Fatal("shard node should have single original parent")
// 	}
// 	_, err = cl.PinGet(cShard2)
// 	if err != nil {
// 		t.Fatal("other shard shoud still be pinned too")
// 	}
// }

// func TestClusterPinTwoMethodsFail(t *testing.T) {
// 	cl, _, _, _, _ := testingCluster(t)
// 	defer cleanRaft()
// 	defer cl.Shutdown()

// 	// First pin normally then sharding pin fails
// 	c, _ := cid.Decode(test.TestMetaRootCid)
// 	err := cl.Pin(api.PinCid(c))
// 	if err != nil {
// 		t.Fatal("pin should have worked:", err)
// 	}

// 	cCdag, _ := cid.Decode(test.TestCdagCid)
// 	cMeta, _ := cid.Decode(test.TestMetaRootCid)
// 	metaPin := api.Pin{
// 		Cid:        cMeta,
// 		Type:       api.MetaType,
// 		Clusterdag: cCdag,
// 	}
// 	err = cl.Pin(metaPin)
// 	if err == nil {
// 		t.Fatal("pin should have failed:", err)
// 	}

// 	err = cl.Unpin(c)
// 	if err != nil {
// 		t.Fatal("unpin should have worked:", err)
// 	}

// 	singleShardedPin(t, cl)
// 	err = cl.Pin(api.PinCid(c))
// 	if err == nil {
// 		t.Fatal("pin should have failed:", err)
// 	}
// }

// func TestClusterRePinShard(t *testing.T) {
// 	cl, _, _, _, _ := testingCluster(t)
// 	defer cleanRaft()
// 	defer cl.Shutdown()

// 	cCdag, _ := cid.Decode(test.TestCdagCid)
// 	cShard, _ := cid.Decode(test.TestShardCid)
// 	shardPin := api.Pin{
// 		Cid:                  cShard,
// 		Type:                 api.ShardType,
// 		ReplicationFactorMin: -1,
// 		ReplicationFactorMax: -1,
// 		Recursive:            true,
// 	}
// 	err := cl.Pin(shardPin)
// 	if err != nil {
// 		t.Fatal("shard pin should have worked:", err)
// 	}

// 	parents := cid.NewSet()
// 	parents.Add(cCdag)
// 	shardPin.Parents = parents
// 	err = cl.Pin(shardPin)
// 	if err != nil {
// 		t.Fatal("repinning shard pin with different parents should have worked:", err)
// 	}

// 	shardPin.ReplicationFactorMin = 3
// 	shardPin.ReplicationFactorMax = 5
// 	err = cl.Pin(shardPin)
// 	if err == nil {
// 		t.Fatal("repinning shard pin with different repl factors should have failed:", err)
// 	}
// }

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
	if cl.Version() != version.Version.String() {
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
		t.Fatalf("there should be only one pin, got = %d", len(recov))
	}
	if recov[0].Status != api.TrackerStatusPinned {
		t.Errorf("the pin should have been recovered, got = %v", recov[0].Status)
	}
}
