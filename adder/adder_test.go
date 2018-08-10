package adder

import (
	"context"
	"mime/multipart"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
)

type mockCDAGServ struct {
	BaseDAGService
	resultCids map[string]struct{}
}

func (dag *mockCDAGServ) Add(ctx context.Context, node ipld.Node) error {
	dag.resultCids[node.Cid().String()] = struct{}{}
	return nil
}

func (dag *mockCDAGServ) AddMany(ctx context.Context, nodes []ipld.Node) error {
	for _, node := range nodes {
		err := dag.Add(ctx, node)
		if err != nil {
			return err
		}
	}
	return nil
}

func (dag *mockCDAGServ) Finalize(ctx context.Context, root *cid.Cid) (*cid.Cid, error) {
	return root, nil
}

func TestAdder(t *testing.T) {
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	mr, closer := sth.GetTreeMultiReader(t)
	defer closer.Close()
	r := multipart.NewReader(mr, mr.Boundary())
	p := api.DefaultAddParams()
	expectedCids := test.ShardingDirCids[:]

	dags := &mockCDAGServ{
		resultCids: make(map[string]struct{}),
	}

	adder := New(dags, p, nil)

	root, err := adder.FromMultipart(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}

	if root.String() != test.ShardingDirBalancedRootCID {
		t.Error("expected the right content root")
	}

	if len(expectedCids) != len(dags.resultCids) {
		t.Fatal("unexpected number of blocks imported")
	}

	for _, c := range expectedCids {
		_, ok := dags.resultCids[c]
		if !ok {
			t.Fatal("unexpected block emitted:", c)
		}
	}
}

func TestAdder_DoubleStart(t *testing.T) {
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	f := sth.GetTreeSerialFile(t)
	p := api.DefaultAddParams()

	dags := &mockCDAGServ{
		resultCids: make(map[string]struct{}),
	}

	adder := New(dags, p, nil)
	_, err := adder.FromFiles(context.Background(), f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	f = sth.GetTreeSerialFile(t)
	_, err = adder.FromFiles(context.Background(), f)
	f.Close()
	if err == nil {
		t.Fatal("expected an error: cannot run importer twice")
	}
}

func TestAdder_ContextCancelled(t *testing.T) {
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	mr, closer := sth.GetRandFileMultiReader(t, 50000) // 50 MB
	defer closer.Close()
	r := multipart.NewReader(mr, mr.Boundary())

	p := api.DefaultAddParams()

	dags := &mockCDAGServ{
		resultCids: make(map[string]struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	adder := New(dags, p, nil)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := adder.FromMultipart(ctx, r)
		if err == nil {
			t.Error("expected a context cancelled error")
		}
		t.Log(err)
	}()
	time.Sleep(200 * time.Millisecond)
	cancel()
	wg.Wait()
}
