package sharding

import (
	"context"
	"errors"
	"mime/multipart"
	"sync"
	"testing"

	adder "github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	rpc "github.com/libp2p/go-libp2p-gorpc"
)

func init() {
	logging.SetLogLevel("shardingdags", "INFO")
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

func (rpcs *testRPC) BlockAllocate(ctx context.Context, in api.PinSerial, out *[]string) error {
	if in.ReplicationFactorMin > 1 {
		return errors.New("we can only replicate to 1 peer")
	}
	*out = []string{""}
	return nil
}

func (rpcs *testRPC) PinGet(ctx context.Context, c cid.Cid) (api.Pin, error) {
	pI, ok := rpcs.pins.Load(c.String())
	if !ok {
		return api.Pin{}, errors.New("not found")
	}
	return pI.(api.PinSerial).ToPin(), nil
}

func (rpcs *testRPC) BlockGet(ctx context.Context, c cid.Cid) ([]byte, error) {
	bI, ok := rpcs.blocks.Load(c.String())
	if !ok {
		return nil, errors.New("not found")
	}
	return bI.([]byte), nil
}

func makeAdder(t *testing.T, params *api.AddParams) (*adder.Adder, *testRPC) {
	rpcObj := &testRPC{}
	server := rpc.NewServer(nil, "mock")
	err := server.RegisterName("Cluster", rpcObj)
	if err != nil {
		t.Fatal(err)
	}
	client := rpc.NewClientWithServer(nil, "mock", server)

	out := make(chan *api.AddedOutput, 1)

	dags := New(client, params.PinOptions, out)
	add := adder.New(dags, params, out)

	go func() {
		for v := range out {
			t.Logf("Output: Name: %s. Cid: %s. Size: %d", v.Name, v.Cid, v.Size)
		}
	}()

	return add, rpcObj
}

func TestFromMultipart(t *testing.T) {
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	t.Run("Test tree", func(t *testing.T) {
		p := api.DefaultAddParams()
		// Total data is about
		p.ShardSize = 1024 * 300 // 300kB
		p.Name = "testingFile"
		p.Shard = true
		p.ReplicationFactorMin = 1
		p.ReplicationFactorMax = 2

		add, rpcObj := makeAdder(t, p)
		_ = rpcObj

		mr, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()
		r := multipart.NewReader(mr, mr.Boundary())

		rootCid, err := add.FromMultipart(context.Background(), r)
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
		shardBlocks, err := VerifyShards(t, rootCid, rpcObj, rpcObj, 14)
		if err != nil {
			t.Fatal(err)
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
		p := api.DefaultAddParams()
		// Total data is about
		p.ShardSize = 1024 * 1024 * 2 // 2MB
		p.Name = "testingFile"
		p.Shard = true
		p.ReplicationFactorMin = 1
		p.ReplicationFactorMax = 2

		add, rpcObj := makeAdder(t, p)
		_ = rpcObj

		mr, closer := sth.GetRandFileMultiReader(t, 1024*50) // 50 MB
		defer closer.Close()
		r := multipart.NewReader(mr, mr.Boundary())

		rootCid, err := add.FromMultipart(context.Background(), r)
		if err != nil {
			t.Fatal(err)
		}

		shardBlocks, err := VerifyShards(t, rootCid, rpcObj, rpcObj, 29)
		if err != nil {
			t.Fatal(err)
		}
		_ = shardBlocks
	})

}

func TestFromMultipart_Errors(t *testing.T) {
	type testcase struct {
		name   string
		params *api.AddParams
	}

	tcs := []*testcase{
		&testcase{
			name: "bad chunker",
			params: &api.AddParams{
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
			params: &api.AddParams{
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
			params: &api.AddParams{
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
	defer sth.Clean(t)
	for _, tc := range tcs {
		add, rpcObj := makeAdder(t, tc.params)
		_ = rpcObj

		f := sth.GetTreeSerialFile(t)

		_, err := add.FromFiles(context.Background(), f)
		if err == nil {
			t.Error(tc.name, ": expected an error")
		} else {
			t.Log(tc.name, ":", err)
		}
		f.Close()
	}
}
