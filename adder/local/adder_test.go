package local

import (
	"context"
	"mime/multipart"
	"sync"
	"testing"

	"github.com/ipfs/ipfs-cluster/adder"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
)

type testRPC struct {
	blocks sync.Map
	pins   sync.Map
}

func (rpcs *testRPC) IPFSBlockPut(ctx context.Context, in api.NodeWithMeta, out *struct{}) error {
	rpcs.blocks.Store(in.Cid, in)
	return nil
}

func (rpcs *testRPC) Pin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	rpcs.pins.Store(in.Cid, in)
	return nil
}

func TestFromMultipart(t *testing.T) {
	defer test.CleanShardingDir(t)

	t.Run("balanced", func(t *testing.T) {
		rpcObj := &testRPC{}
		server := rpc.NewServer(nil, "mock")
		err := server.RegisterName("Cluster", rpcObj)
		if err != nil {
			t.Fatal(err)
		}
		client := rpc.NewClientWithServer(nil, "mock", server)

		add := New(client)

		mr := test.GetShardingDirMultiReader(t)
		r := multipart.NewReader(mr, mr.Boundary())
		err = add.FromMultipart(context.Background(), r, adder.DefaultParams())
		if err != nil {
			t.Fatal(err)
		}

		expected := test.ShardingDirCids[:]
		for _, c := range expected {
			_, ok := rpcObj.blocks.Load(c)
			if !ok {
				t.Error("no IPFSBlockPut for block", c)
			}
		}

		_, ok := rpcObj.pins.Load(test.ShardingDirBalancedRootCID)
		if !ok {
			t.Error("the tree wasn't pinned")
		}
	})

	t.Run("trickle", func(t *testing.T) {
		rpcObj := &testRPC{}
		server := rpc.NewServer(nil, "mock")
		err := server.RegisterName("Cluster", rpcObj)
		if err != nil {
			t.Fatal(err)
		}
		client := rpc.NewClientWithServer(nil, "mock", server)

		add := New(client)

		mr := test.GetShardingDirMultiReader(t)
		r := multipart.NewReader(mr, mr.Boundary())
		p := adder.DefaultParams()
		p.Layout = "trickle"

		err = add.FromMultipart(context.Background(), r, p)
		if err != nil {
			t.Fatal(err)
		}

		_, ok := rpcObj.pins.Load(test.ShardingDirTrickleRootCID)
		if !ok {
			t.Error("the tree wasn't pinned")
		}
	})
}
