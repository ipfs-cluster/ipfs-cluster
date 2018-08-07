package adder

import (
	"context"
	"errors"
	"mime/multipart"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
)

type mockCDagServ struct {
	BaseDAGService
	resultCids map[string]struct{}
	lastCid    *cid.Cid
}

func (dag *mockCDagServ) Add(ctx context.Context, node ipld.Node) error {
	dag.resultCids[node.Cid().String()] = struct{}{}
	dag.lastCid = node.Cid()
	return nil
}

func (dag *mockCDagServ) AddMany(ctx context.Context, nodes []ipld.Node) error {
	for _, node := range nodes {
		err := dag.Add(ctx, node)
		if err != nil {
			return err
		}
	}
	return nil
}

func (dag *mockCDagServ) Finalize(ctx context.Context) (*cid.Cid, error) {
	if dag.lastCid == nil {
		return nil, errors.New("nothing added")
	}
	return dag.lastCid, nil
}

func TestAdder(t *testing.T) {
	sth := test.NewShardingTestHelper()
	defer sth.Clean()

	mr := sth.GetTreeMultiReader(t)
	r := multipart.NewReader(mr, mr.Boundary())
	p := api.DefaultAddParams()
	expectedCids := test.ShardingDirCids[:]

	dags := &mockCDagServ{
		resultCids: make(map[string]struct{}),
	}

	adder := New(context.Background(), dags, p, nil)

	root, err := adder.FromMultipart(r)
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
	defer sth.Clean()

	f := sth.GetTreeSerialFile(t)
	p := api.DefaultAddParams()

	dags := &mockCDagServ{
		resultCids: make(map[string]struct{}),
	}

	adder := New(context.Background(), dags, p, nil)
	_, err := adder.FromFiles(f)
	if err != nil {
		t.Fatal(err)
	}

	f = sth.GetTreeSerialFile(t)
	_, err = adder.FromFiles(f)
	if err == nil {
		t.Fatal("expected an error: cannot run importer twice")
	}
}

func TestAdder_ContextCancelled(t *testing.T) {
	sth := test.NewShardingTestHelper()
	defer sth.Clean()

	mr := sth.GetRandFileMultiReader(t, 50000) // 50 MB
	r := multipart.NewReader(mr, mr.Boundary())

	p := api.DefaultAddParams()

	dags := &mockCDagServ{
		resultCids: make(map[string]struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	adder := New(ctx, dags, p, nil)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := adder.FromMultipart(r)
		if err == nil {
			t.Error("expected a context cancelled error")
		}
		t.Log(err)
	}()
	time.Sleep(200 * time.Millisecond)
	cancel()
	wg.Wait()
}
