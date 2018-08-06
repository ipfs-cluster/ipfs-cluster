package local

import (
	"context"
	"errors"
	"mime/multipart"
	"sync"
	"testing"

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

func (rpcs *testRPC) Allocate(ctx context.Context, in api.PinSerial, out *[]string) error {
	if in.ReplicationFactorMin > 1 {
		return errors.New("we can only replicate to 1 peer")
	}
	*out = []string{""}
	return nil
}

func TestFromMultipart(t *testing.T) {
	t.Run("balanced", func(t *testing.T) {
		rpcObj := &testRPC{}
		server := rpc.NewServer(nil, "mock")
		err := server.RegisterName("Cluster", rpcObj)
		if err != nil {
			t.Fatal(err)
		}
		client := rpc.NewClientWithServer(nil, "mock", server)

		add := New(client)

		sth := test.NewShardingTestHelper()
		defer sth.Clean()
		mr := sth.GetTreeMultiReader(t)
		r := multipart.NewReader(mr, mr.Boundary())
		params := api.DefaultAddParams()
		params.ShardSize = 0
		rootCid, err := add.FromMultipart(context.Background(), r, api.DefaultAddParams())
		if err != nil {
			t.Fatal(err)
		}

		if rootCid.String() != test.ShardingDirBalancedRootCID {
			t.Fatal("bad root cid")
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
		sth := test.NewShardingTestHelper()
		defer sth.Clean()
		mr := sth.GetTreeMultiReader(t)
		r := multipart.NewReader(mr, mr.Boundary())
		p := api.DefaultAddParams()
		p.Layout = "trickle"

		rootCid, err := add.FromMultipart(context.Background(), r, p)
		if err != nil {
			t.Fatal(err)
		}

		if rootCid.String() != test.ShardingDirTrickleRootCID {
			t.Fatal("bad root cid")
		}

		_, ok := rpcObj.pins.Load(test.ShardingDirTrickleRootCID)
		if !ok {
			t.Error("the tree wasn't pinned")
		}
	})
}
