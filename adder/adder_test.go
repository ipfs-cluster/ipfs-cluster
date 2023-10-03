package adder

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"sync"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/test"
	"github.com/ipld/go-car"
	peer "github.com/libp2p/go-libp2p/core/peer"

	files "github.com/ipfs/boxo/files"
	cid "github.com/ipfs/go-cid"
)

type mockCDAGServ struct {
	*test.MockDAGService
}

func newMockCDAGServ() *mockCDAGServ {
	return &mockCDAGServ{
		// write-only DAGs.
		MockDAGService: test.NewMockDAGService(true),
	}
}

func newReadableMockCDAGServ() *mockCDAGServ {
	return &mockCDAGServ{
		MockDAGService: test.NewMockDAGService(false),
	}
}

// noop
func (dag *mockCDAGServ) Finalize(ctx context.Context, root api.Cid) (api.Cid, error) {
	return root, nil
}

func (dag *mockCDAGServ) Allocations() []peer.ID {
	return nil
}

func TestAdder(t *testing.T) {
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	mr, closer := sth.GetTreeMultiReader(t)
	defer closer.Close()
	r := multipart.NewReader(mr, mr.Boundary())
	p := api.DefaultAddParams()
	expectedCids := test.ShardingDirCids[:]

	dags := newMockCDAGServ()
	defer dags.Close()

	adder := New(dags, p, nil)

	root, err := adder.FromMultipart(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}

	if root.String() != test.ShardingDirBalancedRootCID {
		t.Error("expected the right content root")
	}

	if len(expectedCids) != len(dags.Nodes) {
		t.Fatal("unexpected number of blocks imported")
	}

	for _, c := range expectedCids {
		ci, _ := cid.Decode(c)
		_, ok := dags.Nodes[ci]
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

	dags := newMockCDAGServ()
	defer dags.Close()

	adder := New(dags, p, nil)
	_, err := adder.FromFiles(context.Background(), f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}

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

	lg, closer := sth.GetRandFileReader(t, 100000) // 50 MB
	st := sth.GetTreeSerialFile(t)
	defer closer.Close()
	defer st.Close()

	slf := files.NewMapDirectory(map[string]files.Node{
		"a": lg,
		"b": st,
	})
	mr := files.NewMultiFileReader(slf, true, false)

	r := multipart.NewReader(mr, mr.Boundary())

	p := api.DefaultAddParams()

	dags := newMockCDAGServ()
	defer dags.Close()

	ctx, cancel := context.WithCancel(context.Background())
	adder := New(dags, p, nil)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := adder.FromMultipart(ctx, r)
		if err == nil {
			t.Error("expected a context canceled error")
		}
		t.Log(err)
	}()
	// adder.FromMultipart will finish, if sleep more
	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()
}

func TestAdder_CAR(t *testing.T) {
	// prepare a CAR file
	ctx := context.Background()
	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	mr, closer := sth.GetTreeMultiReader(t)
	defer closer.Close()
	r := multipart.NewReader(mr, mr.Boundary())
	p := api.DefaultAddParams()
	dags := newReadableMockCDAGServ()
	adder := New(dags, p, nil)
	root, err := adder.FromMultipart(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	var carBuf bytes.Buffer
	// Make a CAR out of the files we added.
	err = car.WriteCar(ctx, dags, []cid.Cid{root.Cid}, &carBuf)
	if err != nil {
		t.Fatal(err)
	}

	// Make the CAR look like a multipart.
	carFile := files.NewReaderFile(&carBuf)
	carDir := files.NewMapDirectory(
		map[string]files.Node{"": carFile},
	)
	carMf := files.NewMultiFileReader(carDir, true, false)
	carMr := multipart.NewReader(carMf, carMf.Boundary())

	// Add the car, discarding old dags.
	dags = newMockCDAGServ()
	defer dags.Close()

	p.Format = "car"
	adder = New(dags, p, nil)
	root2, err := adder.FromMultipart(ctx, carMr)
	if err != nil {
		t.Fatal(err)
	}

	if !root.Equals(root2) {
		t.Error("Imported CAR file does not have expected root")
	}

	expectedCids := test.ShardingDirCids[:]
	for _, c := range expectedCids {
		ci, _ := cid.Decode(c)
		_, ok := dags.Nodes[ci]
		if !ok {
			t.Fatal("unexpected block extracted from CAR:", c)
		}
	}

}

func TestAdder_LargeFolder(t *testing.T) {
	items := 10000 // add 10000 items

	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	filesMap := make(map[string]files.Node)
	for i := 0; i < items; i++ {
		fstr := fmt.Sprintf("file%d", i)
		f := files.NewBytesFile([]byte(fstr))
		filesMap[fstr] = f
	}

	slf := files.NewMapDirectory(filesMap)

	p := api.DefaultAddParams()
	p.Wrap = true

	dags := newMockCDAGServ()
	defer dags.Close()

	adder := New(dags, p, nil)
	_, err := adder.FromFiles(context.Background(), slf)

	if err != nil {
		t.Fatal(err)
	}
}
