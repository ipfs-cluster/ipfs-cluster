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

func (rpcs *testRPC) PinGet(c *cid.Cid) (api.Pin, error) {
	pI, ok := rpcs.pins.Load(c.String())
	if !ok {
		return api.Pin{}, errors.New("not found")
	}
	return pI.(api.PinSerial).ToPin(), nil
}

func (rpcs *testRPC) BlockGet(c *cid.Cid) ([]byte, error) {
	bI, ok := rpcs.blocks.Load(c.String())
	if !ok {
		return nil, errors.New("not found")
	}
	return bI.([]byte), nil
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
