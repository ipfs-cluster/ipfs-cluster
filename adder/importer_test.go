package adder

import (
	"context"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

func TestImporter(t *testing.T) {
	sth := test.NewShardingTestHelper()
	defer sth.Clean()

	f := sth.GetTreeSerialFile(t)
	p := api.DefaultAddParams()

	out := make(chan *api.AddedOutput)
	go func() {
		for range out {
		}
	}()
	imp, err := NewImporter(f, p, out)
	if err != nil {
		t.Fatal(err)
	}

	expectedCids := test.ShardingDirCids[:]
	resultCids := make(map[string]struct{})

	blockHandler := func(ctx context.Context, n *api.NodeWithMeta) (string, error) {
		resultCids[n.Cid] = struct{}{}
		return n.Cid, nil
	}

	_, err = imp.Run(context.Background(), blockHandler)
	if err != nil {
		t.Fatal(err)
	}

	// for i, c := range expectedCids {
	// 	fmt.Printf("%d: %s\n", i, c)
	// }

	// for c := range resultCids {
	// 	fmt.Printf("%s\n", c)
	// }

	if len(expectedCids) != len(resultCids) {
		t.Fatal("unexpected number of blocks imported")
	}

	for _, c := range expectedCids {
		_, ok := resultCids[c]
		if !ok {
			t.Fatal("unexpected block emitted:", c)
		}

	}
}

func TestImporter_DoubleStart(t *testing.T) {
	sth := test.NewShardingTestHelper()
	defer sth.Clean()

	f := sth.GetTreeSerialFile(t)
	p := api.DefaultAddParams()

	out := make(chan *api.AddedOutput)
	go func() {
		for range out {
		}
	}()
	imp, err := NewImporter(f, p, out)
	if err != nil {
		t.Fatal(err)
	}

	blockHandler := func(ctx context.Context, n *api.NodeWithMeta) (string, error) {
		return "", nil
	}

	_, err = imp.Run(context.Background(), blockHandler)
	if err != nil {
		t.Fatal(err)
	}

	_, err = imp.Run(context.Background(), blockHandler)
	if err == nil {
		t.Fatal("expected an error: cannot run importer twice")
	}
}
