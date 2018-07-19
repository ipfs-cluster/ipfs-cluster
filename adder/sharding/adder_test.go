package sharding

import (
	"context"
	"errors"
	"mime/multipart"
	"sync"
	"testing"

	"github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-cmdkit/files"
	logging "github.com/ipfs/go-log"
)

func init() {
	logging.SetLogLevel("addshard", "INFO")
	logging.SetLogLevel("adder", "INFO")
}

type testRPC struct {
	blocks sync.Map
	pins   sync.Map
}

func (rpcs *testRPC) IPFSBlockPut(ctx context.Context, in api.NodeWithMeta, out *struct{}) error {
	rpcs.blocks.Store(in.Cid, in.Data)
	return nil
}

func (rpcs *testRPC) Pin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	rpcs.pins.Store(in.Cid, in)
	return nil
}

func (rpcs *testRPC) Allocate(ctx context.Context, in api.PinSerial, out *[]string) error {
	if in.ReplicationFactorMin > 1 {
		return errors.New("we can only replicate to 1 peer")
	}
	*out = []string{""}
	return nil
}

func makeAdder(t *testing.T, multiReaderF func(*testing.T) *files.MultiFileReader) (*Adder, *testRPC, *multipart.Reader) {
	rpcObj := &testRPC{}
	server := rpc.NewServer(nil, "mock")
	err := server.RegisterName("Cluster", rpcObj)
	if err != nil {
		t.Fatal(err)
	}
	client := rpc.NewClientWithServer(nil, "mock", server)

	add := New(client)

	mr := multiReaderF(t)
	r := multipart.NewReader(mr, mr.Boundary())
	return add, rpcObj, r

}

// Given a rootCid it performs common checks and returns a map with all the blocks.
func verifyShards(t *testing.T, rootCid *cid.Cid, rpcObj *testRPC, expectedShards int) map[string]struct{} {
	metaPinI, ok := rpcObj.pins.Load(rootCid.String())
	if !ok {
		t.Fatal("meta pin was not pinned")
	}

	metaPin := metaPinI.(api.PinSerial)
	if api.PinType(metaPin.Type) != api.MetaType {
		t.Fatal("bad MetaPin type")
	}

	clusterPinI, ok := rpcObj.pins.Load(metaPin.ClusterDAG)
	if !ok {
		t.Fatal("cluster pin was not pinned")
	}
	clusterPin := clusterPinI.(api.PinSerial)
	if api.PinType(clusterPin.Type) != api.ClusterDAGType {
		t.Fatal("bad ClusterDAGPin type")
	}

	clusterDAGBlock, ok := rpcObj.blocks.Load(clusterPin.Cid)
	if !ok {
		t.Fatal("cluster pin was not stored")
	}

	clusterDAGNode, err := CborDataToNode(clusterDAGBlock.([]byte), "cbor")
	if err != nil {
		t.Fatal(err)
	}

	shards := clusterDAGNode.Links()
	if len(shards) != expectedShards {
		t.Fatal("bad number of shards")
	}

	shardBlocks := make(map[string]struct{})
	for _, sh := range shards {
		shardPinI, ok := rpcObj.pins.Load(sh.Cid.String())
		if !ok {
			t.Fatal("shard was not pinned:", sh.Cid)
		}
		shardPin := shardPinI.(api.PinSerial)
		shardBlock, ok := rpcObj.blocks.Load(shardPin.Cid)
		if !ok {
			t.Fatal("shard block was not stored")
		}
		shardNode, err := CborDataToNode(shardBlock.([]byte), "cbor")
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range shardNode.Links() {
			ci := l.Cid.String()
			_, ok := shardBlocks[ci]
			if ok {
				t.Fatal("block belongs to two shards:", ci)
			}
			shardBlocks[ci] = struct{}{}
		}
	}
	return shardBlocks
}

func TestFromMultipart(t *testing.T) {
	sth := test.NewShardingTestHelper()
	defer sth.Clean()

	t.Run("Test tree", func(t *testing.T) {
		add, rpcObj, r := makeAdder(t, sth.GetTreeMultiReader)
		_ = rpcObj

		p := adder.DefaultParams()
		// Total data is about
		p.ShardSize = 1024 * 300 // 300kB
		p.Name = "testingFile"
		p.Shard = true
		p.ReplicationFactorMin = 1
		p.ReplicationFactorMax = 2

		rootCid, err := add.FromMultipart(context.Background(), r, p)
		if err != nil {
			t.Fatal(err)
		}

		// Print all pins
		// rpcObj.pins.Range(func(k, v interface{}) bool {
		// 	p := v.(api.PinSerial)
		// 	j, _ := config.DefaultJSONMarshal(p)
		// 	fmt.Printf("%s", j)
		// 	return true
		// })

		if rootCid.String() != test.ShardingDirBalancedRootCID {
			t.Fatal("bad root CID")
		}

		// 14 has been obtained by carefully observing the logs
		// making sure that splitting happens in the right place.
		shardBlocks := verifyShards(t, rootCid, rpcObj, 14)
		for _, ci := range test.ShardingDirCids {
			_, ok := shardBlocks[ci]
			if !ok {
				t.Fatal("shards are missing a block:", ci)
			}
		}

		if len(test.ShardingDirCids) != len(shardBlocks) {
			t.Fatal("shards have some extra blocks")
		}
		for _, ci := range test.ShardingDirCids {
			_, ok := shardBlocks[ci]
			if !ok {
				t.Fatal("shards are missing a block:", ci)
			}
		}

		if len(test.ShardingDirCids) != len(shardBlocks) {
			t.Fatal("shards have some extra blocks")
		}

	})

	t.Run("Test file", func(t *testing.T) {
		mrF := func(t *testing.T) *files.MultiFileReader {
			return sth.GetRandFileMultiReader(t, 1024*50) // 50 MB
		}
		add, rpcObj, r := makeAdder(t, mrF)
		_ = rpcObj

		p := adder.DefaultParams()
		// Total data is about
		p.ShardSize = 1024 * 1024 * 2 // 2MB
		p.Name = "testingFile"
		p.Shard = true
		p.ReplicationFactorMin = 1
		p.ReplicationFactorMax = 2

		rootCid, err := add.FromMultipart(context.Background(), r, p)
		if err != nil {
			t.Fatal(err)
		}

		shardBlocks := verifyShards(t, rootCid, rpcObj, 29)
		_ = shardBlocks
	})

}

func TestFromMultipart_Errors(t *testing.T) {
	type testcase struct {
		name   string
		params *adder.Params
	}

	tcs := []*testcase{
		&testcase{
			name: "bad chunker",
			params: &adder.Params{
				Layout:    "",
				Chunker:   "aweee",
				RawLeaves: false,
				Hidden:    false,
				Shard:     true,
				PinOptions: api.PinOptions{
					ReplicationFactorMin: -1,
					ReplicationFactorMax: -1,
					Name:                 "test",
					ShardSize:            1024 * 1024,
				},
			},
		},
		&testcase{
			name: "shard size too small",
			params: &adder.Params{
				Layout:    "",
				Chunker:   "",
				RawLeaves: false,
				Hidden:    false,
				Shard:     true,
				PinOptions: api.PinOptions{
					ReplicationFactorMin: -1,
					ReplicationFactorMax: -1,
					Name:                 "test",
					ShardSize:            200,
				},
			},
		},
		&testcase{
			name: "replication too high",
			params: &adder.Params{
				Layout:    "",
				Chunker:   "",
				RawLeaves: false,
				Hidden:    false,
				Shard:     true,
				PinOptions: api.PinOptions{
					ReplicationFactorMin: 2,
					ReplicationFactorMax: 3,
					Name:                 "test",
					ShardSize:            1024 * 1024,
				},
			},
		},
	}

	sth := test.NewShardingTestHelper()
	defer sth.Clean()
	for _, tc := range tcs {
		add, _, r := makeAdder(t, sth.GetTreeMultiReader)
		_, err := add.FromMultipart(context.Background(), r, tc.params)
		if err == nil {
			t.Error(tc.name, ": expected an error")
		} else {
			t.Log(tc.name, ":", err)
		}
	}

	// Test running with a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	add, _, r := makeAdder(t, sth.GetTreeMultiReader)
	_, err := add.FromMultipart(ctx, r, adder.DefaultParams())
	if err != ctx.Err() {
		t.Error("expected context error:", err)
	}
}
