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

	"github.com/ipfs-cluster/ipfs-cluster/adder/sharding"
	"github.com/ipfs-cluster/ipfs-cluster/allocator/balanced"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/config"
	"github.com/ipfs-cluster/ipfs-cluster/informer/numpin"
	"github.com/ipfs-cluster/ipfs-cluster/monitor/pubsubmon"
	"github.com/ipfs-cluster/ipfs-cluster/pintracker/stateless"
	"github.com/ipfs-cluster/ipfs-cluster/state"
	"github.com/ipfs-cluster/ipfs-cluster/test"
	"github.com/ipfs-cluster/ipfs-cluster/version"

	gopath "github.com/ipfs/boxo/path"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

type mockComponent struct {
	rpcClient *rpc.Client
}

func (c *mockComponent) Shutdown(ctx context.Context) error {
	return nil
}

func (c *mockComponent) SetClient(client *rpc.Client) {
	c.rpcClient = client
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

func (ipfs *mockConnector) Ready(ctx context.Context) <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (ipfs *mockConnector) ID(ctx context.Context) (api.IPFSID, error) {
	return api.IPFSID{
		ID: test.PeerID1,
	}, nil
}

func (ipfs *mockConnector) Pin(ctx context.Context, pin api.Pin) error {
	if pin.Cid == test.ErrorCid {
		return errors.New("trying to pin ErrorCid")
	}
	ipfs.pins.Store(pin.Cid, pin.MaxDepth)
	return nil
}

func (ipfs *mockConnector) Unpin(ctx context.Context, c api.Cid) error {
	ipfs.pins.Delete(c)
	return nil
}

func (ipfs *mockConnector) PinLsCid(ctx context.Context, pin api.Pin) (api.IPFSPinStatus, error) {
	dI, ok := ipfs.pins.Load(pin.Cid)
	if !ok {
		return api.IPFSPinStatusUnpinned, nil
	}
	depth := dI.(api.PinDepth)
	if depth == 0 {
		return api.IPFSPinStatusDirect, nil
	}
	return api.IPFSPinStatusRecursive, nil
}

func (ipfs *mockConnector) PinLs(ctx context.Context, in []string, out chan<- api.IPFSPinInfo) error {
	defer close(out)

	var st api.IPFSPinStatus
	ipfs.pins.Range(func(k, v interface{}) bool {
		switch v.(api.PinDepth) {
		case 0:
			st = api.IPFSPinStatusDirect
		default:
			st = api.IPFSPinStatusRecursive
		}
		c := k.(api.Cid)

		out <- api.IPFSPinInfo{Cid: api.Cid(c), Type: st}
		return true
	})

	return nil
}

func (ipfs *mockConnector) SwarmPeers(ctx context.Context) ([]peer.ID, error) {
	return []peer.ID{test.PeerID4, test.PeerID5}, nil
}

func (ipfs *mockConnector) RepoStat(ctx context.Context) (api.IPFSRepoStat, error) {
	return api.IPFSRepoStat{RepoSize: 100, StorageMax: 1000}, nil
}

func (ipfs *mockConnector) RepoGC(ctx context.Context) (api.RepoGC, error) {
	return api.RepoGC{
		Keys: []api.IPFSRepoGC{
			{
				Key: test.Cid1,
			},
		},
	}, nil
}

func (ipfs *mockConnector) Resolve(ctx context.Context, path string) (api.Cid, error) {
	_, err := gopath.NewPath(path)
	if err != nil {
		return api.CidUndef, err
	}

	return test.CidResolved, nil
}
func (ipfs *mockConnector) ConnectSwarms(ctx context.Context) error       { return nil }
func (ipfs *mockConnector) ConfigKey(keypath string) (interface{}, error) { return nil, nil }

func (ipfs *mockConnector) BlockStream(ctx context.Context, in <-chan api.NodeWithMeta) error {
	for n := range in {
		ipfs.blocks.Store(n.Cid.String(), n.Data)
	}
	return nil
}

func (ipfs *mockConnector) BlockGet(ctx context.Context, c api.Cid) ([]byte, error) {
	d, ok := ipfs.blocks.Load(c.String())
	if !ok {
		return nil, errors.New("block not found")
	}
	return d.([]byte), nil
}

type mockTracer struct {
	mockComponent
}

func testingCluster(t *testing.T) (*Cluster, *mockAPI, *mockConnector, PinTracker) {
	ident, clusterCfg, _, _, _, badgerCfg, badger3Cfg, levelDBCfg, pebbleCfg, raftCfg, crdtCfg, statelesstrackerCfg, psmonCfg, _, _, _ := testingConfigs()
	ctx := context.Background()

	host, pubsub, dht := createHost(t, ident.PrivateKey, clusterCfg.Secret, clusterCfg.ListenAddr)

	folder := filepath.Join(testsFolder, host.ID().String())
	cleanState()
	clusterCfg.SetBaseDir(folder)
	raftCfg.DataFolder = folder
	badgerCfg.Folder = filepath.Join(folder, "badger")
	badger3Cfg.Folder = filepath.Join(folder, "badger3")
	levelDBCfg.Folder = filepath.Join(folder, "leveldb")
	pebbleCfg.Folder = filepath.Join(folder, "pebble")

	api := &mockAPI{}
	proxy := &mockProxy{}
	ipfs := &mockConnector{}

	tracer := &mockTracer{}

	store := makeStore(t, badgerCfg, badger3Cfg, levelDBCfg, pebbleCfg)
	cons := makeConsensus(t, store, host, pubsub, dht, raftCfg, false, crdtCfg)
	tracker := stateless.New(statelesstrackerCfg, ident.ID, clusterCfg.Peername, cons.State)

	var peersF func(context.Context) ([]peer.ID, error)
	if consensus == "raft" {
		peersF = cons.Peers
	}
	psmonCfg.CheckInterval = 2 * time.Second
	mon, err := pubsubmon.New(ctx, psmonCfg, pubsub, peersF)
	if err != nil {
		t.Fatal(err)
	}

	alloc, err := balanced.New(&balanced.Config{
		AllocateBy: []string{"numpin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	numpinCfg := &numpin.Config{}
	numpinCfg.Default()
	inf, _ := numpin.NewInformer(numpinCfg)

	ReadyTimeout = raftCfg.WaitForLeaderTimeout + 1*time.Second

	cl, err := NewCluster(
		ctx,
		host,
		dht,
		clusterCfg,
		store,
		cons,
		[]API{api, proxy},
		ipfs,
		tracker,
		mon,
		alloc,
		[]Informer{inf},
		tracer,
	)
	if err != nil {
		t.Fatal("cannot create cluster:", err)
	}
	<-cl.Ready()
	return cl, api, ipfs, tracker
}

func cleanState() {
	os.RemoveAll(testsFolder)
}

func shutdownTestingCluster(ctx context.Context, t *testing.T, cl *Cluster) {
	t.Helper()
	err := cl.Shutdown(ctx)
	if err != nil {
		t.Fatal("cluster shutdown failed:", err)
	}
	cl.dht.Close()
	cl.host.Close()
	cl.datastore.Close()
}

func TestClusterShutdown(t *testing.T) {
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	shutdownTestingCluster(ctx, t, cl)
	shutdownTestingCluster(ctx, t, cl)

	cl, _, _, _ = testingCluster(t)
	shutdownTestingCluster(ctx, t, cl)
	cleanState()
}

func TestClusterStateSync(t *testing.T) {
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)

	c := test.Cid1
	_, err := cl.Pin(ctx, c, api.PinOptions{})
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	err = cl.StateSync(ctx)
	if err != nil {
		t.Fatal("sync after pinning should have worked:", err)
	}

	// Modify state on the side so the sync does not
	// happen on an empty slide
	st, err := cl.consensus.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st.(state.State).Rm(ctx, c)
	err = cl.StateSync(ctx)
	if err != nil {
		t.Fatal("sync with recover should have worked:", err)
	}
}

func TestClusterID(t *testing.T) {
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)
	id := cl.ID(ctx)
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
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)

	c := test.Cid1
	res, err := cl.Pin(ctx, c, api.PinOptions{})
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	if res.Type != api.DataType {
		t.Error("unexpected pin type")
	}

	switch consensus {
	case "crdt":
		return
	case "raft":
		// test an error case
		cl.consensus.Shutdown(ctx)
		opts := api.PinOptions{
			ReplicationFactorMax: 1,
			ReplicationFactorMin: 1,
		}
		_, err = cl.Pin(ctx, c, opts)
		if err == nil {
			t.Error("expected an error but things worked")
		}
	}
}

func TestPinExpired(t *testing.T) {
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)

	c := test.Cid1
	_, err := cl.Pin(ctx, c, api.PinOptions{
		ExpireAt: time.Now(),
	})
	if err == nil {
		t.Fatal("pin should have errored")
	}
}

func TestClusterPinPath(t *testing.T) {
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)

	pin, err := cl.PinPath(ctx, test.PathIPFS2, api.PinOptions{})
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}
	if !pin.Cid.Equals(test.CidResolved) {
		t.Error("expected a different cid, found", pin.Cid.String())
	}

	// test an error case
	_, err = cl.PinPath(ctx, test.InvalidPath1, api.PinOptions{})
	if err == nil {
		t.Error("expected an error but things worked")
	}
}

func TestAddFile(t *testing.T) {
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	t.Run("local", func(t *testing.T) {
		params := api.DefaultAddParams()
		params.Shard = false
		params.Name = "testlocal"
		mfr, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		r := multipart.NewReader(mfr, mfr.Boundary())
		c, err := cl.AddFile(context.Background(), r, params)
		if err != nil {
			t.Fatal(err)
		}
		if c.String() != test.ShardingDirBalancedRootCID {
			t.Fatal("unexpected root CID for local add")
		}

		pinDelay()

		pin := cl.StatusLocal(ctx, c)
		if pin.Error != "" {
			t.Fatal(pin.Error)
		}
		if pin.Status != api.TrackerStatusPinned {
			t.Error("cid should be pinned")
		}

		cl.Unpin(ctx, c) // unpin so we can pin the shard in next test
		pinDelay()
	})

	t.Run("shard", func(t *testing.T) {
		params := api.DefaultAddParams()
		params.Shard = true
		params.Name = "testshard"
		mfr, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		r := multipart.NewReader(mfr, mfr.Boundary())
		c, err := cl.AddFile(context.Background(), r, params)
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
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	params := api.DefaultAddParams()
	params.Shard = true
	params.Name = "testshard"
	mfr, closer := sth.GetTreeMultiReader(t)
	defer closer.Close()
	r := multipart.NewReader(mfr, mfr.Boundary())
	root, err := cl.AddFile(context.Background(), r, params)
	if err != nil {
		t.Fatal(err)
	}

	pinDelay()

	// We know that this produces 14 shards.
	sharding.VerifyShards(t, root, cl, cl.ipfs, 14)

	// skipping errors, VerifyShards has checked
	pinnedCids := []api.Cid{}
	pinnedCids = append(pinnedCids, root)
	metaPin, _ := cl.PinGet(ctx, root)
	cDag, _ := cl.PinGet(ctx, *metaPin.Reference)
	pinnedCids = append(pinnedCids, cDag.Cid)
	cDagBlock, _ := cl.ipfs.BlockGet(ctx, cDag.Cid)
	cDagNode, _ := sharding.CborDataToNode(cDagBlock, "cbor")
	for _, l := range cDagNode.Links() {
		pinnedCids = append(pinnedCids, api.NewCid(l.Cid))
	}

	t.Run("unpin clusterdag should fail", func(t *testing.T) {
		_, err := cl.Unpin(ctx, cDag.Cid)
		if err == nil {
			t.Fatal("should not allow unpinning the cluster DAG directly")
		}
		t.Log(err)
	})

	t.Run("unpin shard should fail", func(t *testing.T) {
		_, err := cl.Unpin(ctx, api.NewCid(cDagNode.Links()[0].Cid))
		if err == nil {
			t.Fatal("should not allow unpinning shards directly")
		}
		t.Log(err)
	})

	t.Run("normal unpin", func(t *testing.T) {
		res, err := cl.Unpin(ctx, root)
		if err != nil {
			t.Fatal(err)
		}

		if res.Type != api.MetaType {
			t.Fatal("unexpected root pin type")
		}

		pinDelay()

		for _, c := range pinnedCids {
			st := cl.StatusLocal(ctx, c)
			if st.Status != api.TrackerStatusUnpinned {
				t.Errorf("%s should have been unpinned but is %s", c, st.Status)
			}

			st2, err := cl.ipfs.PinLsCid(context.Background(), api.PinCid(c))
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
// 	cShard, _ := cid.Decode(test.ShardCid)
// 	cCdag, _ := cid.Decode(test.CdagCid)
// 	cMeta, _ := cid.Decode(test.MetaRootCid)
// 	pinMeta(t, cl, []api.NewCid(cShard), cCdag, cMeta)
// }

// func pinMeta(t *testing.T, cl *Cluster, shardCids []api.Cid, cCdag, cMeta api.Cid) {
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
// 	cl, _, _, _ := testingCluster(t)
// 	defer cleanState()
// 	defer cl.Shutdown()

// 	singleShardedPin(t, cl)
// }

// func TestClusterUnpinShardFail(t *testing.T) {
// 	cl, _, _, _ := testingCluster(t)
// 	defer cleanState()
// 	defer cl.Shutdown()

// 	singleShardedPin(t, cl)
// 	// verify pins
// 	if len(cl.Pins()) != 3 {
// 		t.Fatal("should have 3 pins")
// 	}
// 	// Unpinning metadata should fail
// 	cShard, _ := cid.Decode(test.ShardCid)
// 	cCdag, _ := cid.Decode(test.CdagCid)

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
// 	cl, _, _, _ := testingCluster(t)
// 	defer cleanState()
// 	defer cl.Shutdown()

// 	singleShardedPin(t, cl)
// 	// verify pins
// 	if len(cl.Pins()) != 3 {
// 		t.Fatal("should have 3 pins")
// 	}
// 	// Unpinning from root should work
// 	cMeta, _ := cid.Decode(test.MetaRootCid)

// 	err := cl.Unpin(cMeta)
// 	if err != nil {
// 		t.Error(err)
// 	}
// }

// func pinTwoParentsOneShard(t *testing.T, cl *Cluster) {
// 	singleShardedPin(t, cl)

// 	cShard, _ := cid.Decode(test.ShardCid)
// 	cShard2, _ := cid.Decode(test.ShardCid2)
// 	cCdag2, _ := cid.Decode(test.CdagCid2)
// 	cMeta2, _ := cid.Decode(test.MetaRootCid2)
// 	pinMeta(t, cl, []api.Cid{cShard, cShard2}, cCdag2, cMeta2)

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
// 	cl, _, _, _ := testingCluster(t)
// 	defer cleanState()
// 	defer cl.Shutdown()

// 	pinTwoParentsOneShard(t, cl)

// 	cShard, _ := cid.Decode(test.ShardCid)
// 	shardPin, err := cl.PinGet(cShard)
// 	if err != nil {
// 		t.Fatal("double pinned shard should be pinned")
// 	}
// 	if shardPin.Parents == nil || shardPin.Parents.Len() != 2 {
// 		t.Fatal("double pinned shard should have two parents")
// 	}
// }

// func TestClusterUnpinShardSecondParent(t *testing.T) {
// 	cl, _, _, _ := testingCluster(t)
// 	defer cleanState()
// 	defer cl.Shutdown()

// 	pinTwoParentsOneShard(t, cl)
// 	if len(cl.Pins()) != 6 {
// 		t.Fatal("should have 6 pins")
// 	}
// 	cMeta2, _ := cid.Decode(test.MetaRootCid2)
// 	err := cl.Unpin(cMeta2)
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	pinDelay()

// 	if len(cl.Pins()) != 3 {
// 		t.Fatal("should have 3 pins")
// 	}

// 	cShard, _ := cid.Decode(test.ShardCid)
// 	cCdag, _ := cid.Decode(test.CdagCid)
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
// 	cl, _, _, _ := testingCluster(t)
// 	defer cleanState()
// 	defer cl.Shutdown()

// 	pinTwoParentsOneShard(t, cl)
// 	if len(cl.Pins()) != 6 {
// 		t.Fatal("should have 6 pins")
// 	}

// 	cMeta, _ := cid.Decode(test.MetaRootCid)
// 	err := cl.Unpin(cMeta)
// 	if err != nil {
// 		t.Error(err)
// 	}
// 	if len(cl.Pins()) != 4 {
// 		t.Fatal("should have 4 pins")
// 	}

// 	cShard, _ := cid.Decode(test.ShardCid)
// 	cShard2, _ := cid.Decode(test.ShardCid2)
// 	cCdag2, _ := cid.Decode(test.CdagCid2)
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
// 	cl, _, _, _ := testingCluster(t)
// 	defer cleanState()
// 	defer cl.Shutdown()

// 	// First pin normally then sharding pin fails
// 	c, _ := cid.Decode(test.MetaRootCid)
// 	err := cl.Pin(api.PinCid(c))
// 	if err != nil {
// 		t.Fatal("pin should have worked:", err)
// 	}

// 	cCdag, _ := cid.Decode(test.CdagCid)
// 	cMeta, _ := cid.Decode(test.MetaRootCid)
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
// 	cl, _, _, _ := testingCluster(t)
// 	defer cleanState()
// 	defer cl.Shutdown()

// 	cCdag, _ := cid.Decode(test.CdagCid)
// 	cShard, _ := cid.Decode(test.ShardCid)
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
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)

	c := test.Cid1
	_, err := cl.Pin(ctx, c, api.PinOptions{})
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	pinDelay()

	pins, err := cl.pinsSlice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 1 {
		t.Fatal("pin should be part of the state")
	}
	if !pins[0].Cid.Equals(c) || pins[0].ReplicationFactorMin != -1 || pins[0].ReplicationFactorMax != -1 {
		t.Error("the Pin does not look as expected")
	}
}

func TestClusterPinGet(t *testing.T) {
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)

	c := test.Cid1
	_, err := cl.Pin(ctx, c, api.PinOptions{})
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	pin, err := cl.PinGet(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if !pin.Cid.Equals(c) || pin.ReplicationFactorMin != -1 || pin.ReplicationFactorMax != -1 {
		t.Error("the Pin does not look as expected")
	}

	_, err = cl.PinGet(ctx, test.Cid2)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestClusterUnpin(t *testing.T) {
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)

	c := test.Cid1
	// Unpin should error without pin being committed to state
	_, err := cl.Unpin(ctx, c)
	if err == nil {
		t.Error("unpin should have failed")
	}

	// Unpin after pin should succeed
	_, err = cl.Pin(ctx, c, api.PinOptions{})
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}
	res, err := cl.Unpin(ctx, c)
	if err != nil {
		t.Error("unpin should have worked:", err)
	}

	if res.Type != api.DataType {
		t.Error("unexpected pin type returned")
	}

	// test another error case
	cl.consensus.Shutdown(ctx)
	_, err = cl.Unpin(ctx, c)
	if err == nil {
		t.Error("expected an error but things worked")
	}
}

func TestClusterUnpinPath(t *testing.T) {
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)

	// Unpin should error without pin being committed to state
	_, err := cl.UnpinPath(ctx, test.PathIPFS2)
	if err == nil {
		t.Error("unpin with path should have failed")
	}

	// Unpin after pin should succeed
	pin, err := cl.PinPath(ctx, test.PathIPFS2, api.PinOptions{})
	if err != nil {
		t.Fatal("pin with path should have worked:", err)
	}
	if !pin.Cid.Equals(test.CidResolved) {
		t.Error("expected a different cid, found", pin.Cid.String())
	}

	pin, err = cl.UnpinPath(ctx, test.PathIPFS2)
	if err != nil {
		t.Error("unpin with path should have worked:", err)
	}
	if !pin.Cid.Equals(test.CidResolved) {
		t.Error("expected a different cid, found", pin.Cid.String())
	}
}

func TestClusterPeers(t *testing.T) {
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)

	out := make(chan api.ID, 10)
	cl.Peers(ctx, out)
	if len(out) != 1 {
		t.Fatal("expected 1 peer")
	}

	ident := &config.Identity{}
	err := ident.LoadJSON(testingIdentity)
	if err != nil {
		t.Fatal(err)
	}

	p := <-out
	if p.ID != ident.ID {
		t.Error("bad member")
	}
}

func TestVersion(t *testing.T) {
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)
	if cl.Version() != version.Version.String() {
		t.Error("bad Version()")
	}
}

func TestClusterRecoverAllLocal(t *testing.T) {
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)

	_, err := cl.Pin(ctx, test.ErrorCid, api.PinOptions{})
	if err != nil {
		t.Fatal("pin should have worked:", err)
	}

	pinDelay()

	out := make(chan api.PinInfo, 10)
	go func() {
		err := cl.RecoverAllLocal(ctx, out)
		if err != nil {
			t.Error("did not expect an error")
		}
	}()

	recov := collectPinInfos(t, out)

	if len(recov) != 1 {
		t.Fatalf("there should be one pin recovered, got = %d", len(recov))
	}
	// Recovery will fail, but the pin appearing in the response is good enough to know it was requeued.
}

func TestClusterRepoGC(t *testing.T) {
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)

	gRepoGC, err := cl.RepoGC(ctx)
	if err != nil {
		t.Fatal("gc should have worked:", err)
	}

	if gRepoGC.PeerMap == nil {
		t.Fatal("expected a non-nil peer map")
	}

	if len(gRepoGC.PeerMap) != 1 {
		t.Error("expected repo gc information for one peer")
	}
	for _, repoGC := range gRepoGC.PeerMap {
		testRepoGC(t, repoGC)
	}

}

func TestClusterRepoGCLocal(t *testing.T) {
	ctx := context.Background()
	cl, _, _, _ := testingCluster(t)
	defer cleanState()
	defer shutdownTestingCluster(ctx, t, cl)

	repoGC, err := cl.RepoGCLocal(ctx)
	if err != nil {
		t.Fatal("gc should have worked:", err)
	}

	testRepoGC(t, repoGC)
}

func testRepoGC(t *testing.T, repoGC api.RepoGC) {
	if repoGC.Peer == "" {
		t.Error("expected a cluster ID")
	}
	if repoGC.Error != "" {
		t.Error("did not expect any error")
	}

	if repoGC.Keys == nil {
		t.Fatal("expected a non-nil array of IPFSRepoGC")
	}

	if len(repoGC.Keys) == 0 {
		t.Fatal("expected at least one key, but found none")
	}

	if !repoGC.Keys[0].Key.Equals(test.Cid1) {
		t.Errorf("expected a different cid, expected: %s, found: %s", test.Cid1, repoGC.Keys[0].Key)
	}
}
