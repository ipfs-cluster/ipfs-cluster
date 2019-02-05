package maptracker

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"testing"
	"time"

	cid "github.com/ipfs/go-cid"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

var (
	pinCancelCid      = test.TestCid3
	unpinCancelCid    = test.TestCid2
	ErrPinCancelCid   = errors.New("should not have received rpc.IPFSPin operation")
	ErrUnpinCancelCid = errors.New("should not have received rpc.IPFSUnpin operation")
)

type mockService struct {
	rpcClient *rpc.Client
}

func mockRPCClient(t *testing.T) *rpc.Client {
	s := rpc.NewServer(nil, "mock")
	c := rpc.NewClientWithServer(nil, "mock", s)
	err := s.RegisterName("Cluster", &mockService{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (mock *mockService) IPFSPin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	c := in.ToPin().Cid
	switch c.String() {
	case test.TestSlowCid1:
		time.Sleep(2 * time.Second)
	case pinCancelCid:
		return ErrPinCancelCid
	}
	return nil
}

func (mock *mockService) IPFSUnpin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	c := in.ToPin().Cid
	switch c.String() {
	case test.TestSlowCid1:
		time.Sleep(2 * time.Second)
	case unpinCancelCid:
		return ErrUnpinCancelCid
	}
	return nil
}

func testPin(c cid.Cid, min, max int, allocs ...peer.ID) api.Pin {
	pin := api.PinCid(c)
	pin.ReplicationFactorMin = min
	pin.ReplicationFactorMax = max
	pin.Allocations = allocs
	return pin
}

func testSlowMapPinTracker(t *testing.T) *MapPinTracker {
	cfg := &Config{}
	cfg.Default()
	cfg.ConcurrentPins = 1
	mpt := NewMapPinTracker(cfg, test.TestPeerID1, test.TestPeerName1)
	mpt.SetClient(mockRPCClient(t))
	return mpt
}

func testMapPinTracker(t *testing.T) *MapPinTracker {
	cfg := &Config{}
	cfg.Default()
	cfg.ConcurrentPins = 1
	mpt := NewMapPinTracker(cfg, test.TestPeerID1, test.TestPeerName1)
	mpt.SetClient(test.NewMockRPCClient(t))
	return mpt
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown(ctx)
}

func TestShutdown(t *testing.T) {
	ctx := context.Background()
	mpt := testMapPinTracker(t)
	err := mpt.Shutdown(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = mpt.Shutdown(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTrack(t *testing.T) {
	ctx := context.Background()
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown(ctx)

	h, _ := cid.Decode(test.TestCid1)

	// Let's tart with a local pin
	c := testPin(h, -1, -1)

	err := mpt.Track(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond) // let it be pinned

	st := mpt.Status(context.Background(), h)
	if st.Status != api.TrackerStatusPinned {
		t.Fatalf("cid should be pinned and is %s", st.Status)
	}

	// Unpin and set remote
	c = testPin(h, 1, 1, test.TestPeerID2)
	err = mpt.Track(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond) // let it be unpinned

	st = mpt.Status(context.Background(), h)
	if st.Status != api.TrackerStatusRemote {
		t.Fatalf("cid should be pinned and is %s", st.Status)
	}
}

func TestUntrack(t *testing.T) {
	ctx := context.Background()
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown(ctx)

	h1, _ := cid.Decode(test.TestCid1)
	h2, _ := cid.Decode(test.TestCid2)

	// LocalPin
	c := testPin(h1, -1, -1)

	err := mpt.Track(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	// Remote pin
	c = testPin(h2, 1, 1, test.TestPeerID2)
	err = mpt.Track(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	err = mpt.Untrack(context.Background(), h2)
	if err != nil {
		t.Fatal(err)
	}
	err = mpt.Untrack(context.Background(), h1)
	if err != nil {
		t.Fatal(err)
	}
	err = mpt.Untrack(context.Background(), h1)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	st := mpt.Status(context.Background(), h1)
	if st.Status != api.TrackerStatusUnpinned {
		t.Fatalf("cid should be unpinned and is %s", st.Status)
	}

	st = mpt.Status(context.Background(), h2)
	if st.Status != api.TrackerStatusUnpinned {
		t.Fatalf("cid should be unpinned and is %s", st.Status)
	}
}

func TestStatusAll(t *testing.T) {
	ctx := context.Background()
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown(ctx)

	h1, _ := cid.Decode(test.TestCid1)
	h2, _ := cid.Decode(test.TestCid2)

	// LocalPin
	c := testPin(h1, -1, -1)
	mpt.Track(context.Background(), c)
	c = testPin(h2, 1, 1)
	mpt.Track(context.Background(), c)

	time.Sleep(200 * time.Millisecond)

	stAll := mpt.StatusAll(context.Background())
	if len(stAll) != 2 {
		t.Logf("%+v", stAll)
		t.Fatal("expected 2 pins")
	}

	for _, st := range stAll {
		if st.Cid.Equals(h1) && st.Status != api.TrackerStatusPinned {
			t.Fatal("expected pinned")
		}
		if st.Cid.Equals(h2) && st.Status != api.TrackerStatusRemote {
			t.Fatal("expected remote")
		}
	}
}

func TestSyncAndRecover(t *testing.T) {
	ctx := context.Background()
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown(ctx)

	h1, _ := cid.Decode(test.TestCid1)
	h2, _ := cid.Decode(test.TestCid2)

	c := testPin(h1, -1, -1)
	mpt.Track(context.Background(), c)
	c = testPin(h2, -1, -1)
	mpt.Track(context.Background(), c)

	time.Sleep(100 * time.Millisecond)

	// IPFSPinLS RPC returns unpinned for anything != Cid1 or Cid3
	info, err := mpt.Sync(context.Background(), h2)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != api.TrackerStatusPinError {
		t.Error("expected pin_error")
	}

	info, err = mpt.Sync(context.Background(), h1)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != api.TrackerStatusPinned {
		t.Error("expected pinned")
	}

	info, err = mpt.Recover(context.Background(), h1)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != api.TrackerStatusPinned {
		t.Error("expected pinned")
	}

	_, err = mpt.Recover(context.Background(), h2)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	info = mpt.Status(context.Background(), h2)
	if info.Status != api.TrackerStatusPinned {
		t.Error("expected pinned")
	}
}

func TestRecoverAll(t *testing.T) {
	ctx := context.Background()
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown(ctx)

	h1, _ := cid.Decode(test.TestCid1)

	c := testPin(h1, -1, -1)
	mpt.Track(context.Background(), c)
	time.Sleep(100 * time.Millisecond)
	mpt.optracker.SetError(context.Background(), h1, errors.New("fakeerror"))
	pins, err := mpt.RecoverAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 1 {
		t.Fatal("there should be only one pin")
	}

	time.Sleep(100 * time.Millisecond)
	info := mpt.Status(context.Background(), h1)

	if info.Status != api.TrackerStatusPinned {
		t.Error("the pin should have been recovered")
	}
}

func TestSyncAll(t *testing.T) {
	ctx := context.Background()
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown(ctx)

	synced, err := mpt.SyncAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// This relies on the rpc mock implementation

	if len(synced) != 0 {
		t.Fatal("should not have synced anything when it tracks nothing")
	}

	h1, _ := cid.Decode(test.TestCid1)
	h2, _ := cid.Decode(test.TestCid2)

	c := testPin(h1, -1, -1)
	mpt.Track(context.Background(), c)
	c = testPin(h2, -1, -1)
	mpt.Track(context.Background(), c)

	time.Sleep(100 * time.Millisecond)

	synced, err = mpt.SyncAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(synced) != 1 || synced[0].Status != api.TrackerStatusPinError {
		t.Logf("%+v", synced)
		t.Fatal("should have synced h2")
	}
}

func TestUntrackTrack(t *testing.T) {
	ctx := context.Background()
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown(ctx)

	h1, _ := cid.Decode(test.TestCid1)

	// LocalPin
	c := testPin(h1, -1, -1)
	err := mpt.Track(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	err = mpt.Untrack(context.Background(), h1)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTrackUntrackWithCancel(t *testing.T) {
	ctx := context.Background()
	mpt := testSlowMapPinTracker(t)
	defer mpt.Shutdown(ctx)

	slowPinCid, _ := cid.Decode(test.TestSlowCid1)

	// LocalPin
	slowPin := testPin(slowPinCid, -1, -1)

	err := mpt.Track(context.Background(), slowPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond) // let pinning start

	pInfo := mpt.Status(context.Background(), slowPin.Cid)
	if pInfo.Status == api.TrackerStatusUnpinned {
		t.Fatal("slowPin should be tracked")
	}

	if pInfo.Status == api.TrackerStatusPinning {
		go func() {
			err = mpt.Untrack(context.Background(), slowPinCid)
			if err != nil {
				t.Fatal(err)
			}
		}()
		select {
		case <-mpt.optracker.OpContext(context.Background(), slowPinCid).Done():
			return
		case <-time.Tick(100 * time.Millisecond):
			t.Errorf("operation context should have been cancelled by now")
		}
	} else {
		t.Error("slowPin should be pinning and is:", pInfo.Status)
	}
}

func TestTrackUntrackWithNoCancel(t *testing.T) {
	ctx := context.Background()
	mpt := testSlowMapPinTracker(t)
	defer mpt.Shutdown(ctx)

	slowPinCid, _ := cid.Decode(test.TestSlowCid1)
	fastPinCid, _ := cid.Decode(pinCancelCid)

	// SlowLocalPin
	slowPin := testPin(slowPinCid, -1, -1)

	// LocalPin
	fastPin := testPin(fastPinCid, -1, -1)

	err := mpt.Track(context.Background(), slowPin)
	if err != nil {
		t.Fatal(err)
	}

	err = mpt.Track(context.Background(), fastPin)
	if err != nil {
		t.Fatal(err)
	}

	// fastPin should be queued because slow pin is pinning
	pInfo := mpt.Status(context.Background(), fastPinCid)
	if pInfo.Status == api.TrackerStatusPinQueued {
		err = mpt.Untrack(context.Background(), fastPinCid)
		if err != nil {
			t.Fatal(err)
		}
		pi := mpt.Status(context.Background(), fastPinCid)
		if pi.Error == ErrPinCancelCid.Error() {
			t.Fatal(ErrPinCancelCid)
		}
	} else {
		t.Error("fastPin should be queued to pin:", pInfo.Status)
	}

	time.Sleep(100 * time.Millisecond)
	pInfo = mpt.Status(context.Background(), fastPinCid)
	if pInfo.Status != api.TrackerStatusUnpinned {
		t.Error("fastPin should have been removed from tracker:", pInfo.Status)
	}
}

func TestUntrackTrackWithCancel(t *testing.T) {
	ctx := context.Background()
	mpt := testSlowMapPinTracker(t)
	defer mpt.Shutdown(ctx)

	slowPinCid, _ := cid.Decode(test.TestSlowCid1)

	// LocalPin
	slowPin := testPin(slowPinCid, -1, -1)

	err := mpt.Track(context.Background(), slowPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	// Untrack should cancel the ongoing request
	// and unpin right away
	err = mpt.Untrack(context.Background(), slowPinCid)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	pInfo := mpt.Status(context.Background(), slowPinCid)
	if pInfo.Status == api.TrackerStatusUnpinned {
		t.Fatal("expected slowPin to be tracked")
	}

	if pInfo.Status == api.TrackerStatusUnpinning {
		go func() {
			err = mpt.Track(context.Background(), slowPin)
			if err != nil {
				t.Fatal(err)
			}
		}()
		select {
		case <-mpt.optracker.OpContext(context.Background(), slowPinCid).Done():
			return
		case <-time.Tick(100 * time.Millisecond):
			t.Errorf("operation context should have been cancelled by now")
		}
	} else {
		t.Error("slowPin should be in unpinning")
	}

}

func TestUntrackTrackWithNoCancel(t *testing.T) {
	ctx := context.Background()
	mpt := testSlowMapPinTracker(t)
	defer mpt.Shutdown(ctx)

	slowPinCid, _ := cid.Decode(test.TestSlowCid1)
	fastPinCid, _ := cid.Decode(unpinCancelCid)

	// SlowLocalPin
	slowPin := testPin(slowPinCid, -1, -1)

	// LocalPin
	fastPin := testPin(fastPinCid, -1, -1)

	err := mpt.Track(context.Background(), slowPin)
	if err != nil {
		t.Fatal(err)
	}

	err = mpt.Track(context.Background(), fastPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)

	err = mpt.Untrack(context.Background(), slowPin.Cid)
	if err != nil {
		t.Fatal(err)
	}

	err = mpt.Untrack(context.Background(), fastPin.Cid)
	if err != nil {
		t.Fatal(err)
	}

	pInfo := mpt.Status(context.Background(), fastPinCid)
	if pInfo.Status == api.TrackerStatusUnpinned {
		t.Fatal("c untrack operation should be tracked")
	}

	if pInfo.Status == api.TrackerStatusUnpinQueued {
		err = mpt.Track(context.Background(), fastPin)
		if err != nil {
			t.Fatal(err)
		}

		pi := mpt.Status(context.Background(), fastPinCid)
		if pi.Error == ErrUnpinCancelCid.Error() {
			t.Fatal(ErrUnpinCancelCid)
		}
	} else {
		t.Error("c should be queued to unpin")
	}
}

func TestTrackUntrackConcurrent(t *testing.T) {
	ctx := context.Background()
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown(ctx)

	h1, _ := cid.Decode(test.TestCid1)

	// LocalPin
	c := testPin(h1, -1, -1)

	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				var err error
				op := rand.Intn(2)
				if op == 1 {
					err = mpt.Track(context.Background(), c)
				} else {
					err = mpt.Untrack(context.Background(), c.Cid)
				}
				if err != nil {
					t.Error(err)
				}
			}
		}()
	}

	wg.Wait()

	time.Sleep(200 * time.Millisecond)
	st := mpt.Status(context.Background(), h1)
	t.Log(st.Status)
	if st.Status != api.TrackerStatusUnpinned && st.Status != api.TrackerStatusPinned {
		t.Fatal("should be pinned or unpinned")
	}
}
