package sharding

import (
	"context"
	"fmt"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
)

// MockPinStore is used in VerifyShards
type MockPinStore interface {
	// Gets a pin
	PinGet(context.Context, cid.Cid) (api.Pin, error)
}

// MockBlockStore is used in VerifyShards
type MockBlockStore interface {
	// Gets a block
	BlockGet(context.Context, cid.Cid) ([]byte, error)
}

// VerifyShards checks that a sharded CID has been correctly formed and stored.
// This is a helper function for testing. It returns a map with all the blocks
// from all shards.
func VerifyShards(t *testing.T, rootCid cid.Cid, pins MockPinStore, ipfs MockBlockStore, expectedShards int) (map[string]struct{}, error) {
	ctx := context.Background()
	metaPin, err := pins.PinGet(ctx, rootCid)
	if err != nil {
		return nil, fmt.Errorf("meta pin was not pinned: %s", err)
	}

	if api.PinType(metaPin.Type) != api.MetaType {
		return nil, fmt.Errorf("bad MetaPin type")
	}

	clusterPin, err := pins.PinGet(ctx, metaPin.Reference)
	if err != nil {
		return nil, fmt.Errorf("cluster pin was not pinned: %s", err)
	}
	if api.PinType(clusterPin.Type) != api.ClusterDAGType {
		return nil, fmt.Errorf("bad ClusterDAGPin type")
	}

	if !clusterPin.Reference.Equals(metaPin.Cid) {
		return nil, fmt.Errorf("clusterDAG should reference the MetaPin")
	}

	clusterDAGBlock, err := ipfs.BlockGet(ctx, clusterPin.Cid)
	if err != nil {
		return nil, fmt.Errorf("cluster pin was not stored: %s", err)
	}

	clusterDAGNode, err := CborDataToNode(clusterDAGBlock, "cbor")
	if err != nil {
		return nil, err
	}

	shards := clusterDAGNode.Links()
	if len(shards) != expectedShards {
		return nil, fmt.Errorf("bad number of shards")
	}

	shardBlocks := make(map[string]struct{})
	var ref cid.Cid
	// traverse shards in order
	for i := 0; i < len(shards); i++ {
		sh, _, err := clusterDAGNode.ResolveLink([]string{fmt.Sprintf("%d", i)})
		if err != nil {
			return nil, err
		}

		shardPin, err := pins.PinGet(ctx, sh.Cid)
		if err != nil {
			return nil, fmt.Errorf("shard was not pinned: %s %s", sh.Cid, err)
		}

		if ref != cid.Undef && !shardPin.Reference.Equals(ref) {
			t.Errorf("Ref (%s) should point to previous shard (%s)", ref, shardPin.Reference)
		}
		ref = shardPin.Cid

		shardBlock, err := ipfs.BlockGet(ctx, shardPin.Cid)
		if err != nil {
			return nil, fmt.Errorf("shard block was not stored: %s", err)
		}
		shardNode, err := CborDataToNode(shardBlock, "cbor")
		if err != nil {
			return nil, err
		}
		for _, l := range shardNode.Links() {
			ci := l.Cid.String()
			_, ok := shardBlocks[ci]
			if ok {
				return nil, fmt.Errorf("block belongs to two shards: %s", ci)
			}
			shardBlocks[ci] = struct{}{}
		}
	}
	return shardBlocks, nil
}
