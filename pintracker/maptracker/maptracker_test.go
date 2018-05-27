package maptracker

import (
	"context"
	"errors"
	"testing"
	"time"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/optracker"
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

func testSlowMapPinTracker(t *testing.T) *MapPinTracker {
	cfg := &Config{}
	cfg.Default()
	mpt := NewMapPinTracker(cfg, test.TestPeerID1)
	mpt.SetClient(mockRPCClient(t))
	return mpt
}

func testMapPinTracker(t *testing.T) *MapPinTracker {
	cfg := &Config{}
	cfg.Default()
	mpt := NewMapPinTracker(cfg, test.TestPeerID1)
	mpt.SetClient(test.NewMockRPCClient(t))
	return mpt
}

func TestNew(t *testing.T) {
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown()
}

func TestShutdown(t *testing.T) {
	mpt := testMapPinTracker(t)
	err := mpt.Shutdown()
	if err != nil {
		t.Fatal(err)
	}
	err = mpt.Shutdown()
	if err != nil {
		t.Fatal(err)
	}
}

func TestTrack(t *testing.T) {
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown()

	h, _ := cid.Decode(test.TestCid1)

	// Let's tart with a local pin
	c := api.Pin{
		Cid:                  h,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := mpt.Track(c)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond) // let it be pinned

	st := mpt.Status(h)
	if st.Status != api.TrackerStatusPinned {
		t.Fatalf("cid should be pinned and is %s", st.Status)
	}

	// Unpin and set remote
	c = api.Pin{
		Cid:                  h,
		Allocations:          []peer.ID{test.TestPeerID2},
		ReplicationFactorMin: 1,
		ReplicationFactorMax: 1,
	}
	err = mpt.Track(c)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond) // let it be unpinned

	st = mpt.Status(h)
	if st.Status != api.TrackerStatusRemote {
		t.Fatalf("cid should be pinned and is %s", st.Status)
	}
}

func TestUntrack(t *testing.T) {
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown()

	h1, _ := cid.Decode(test.TestCid1)
	h2, _ := cid.Decode(test.TestCid2)

	// LocalPin
	c := api.Pin{
		Cid:                  h1,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := mpt.Track(c)
	if err != nil {
		t.Fatal(err)
	}

	// Remote pin
	c = api.Pin{
		Cid:                  h2,
		Allocations:          []peer.ID{test.TestPeerID2},
		ReplicationFactorMin: 1,
		ReplicationFactorMax: 1,
	}
	err = mpt.Track(c)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	err = mpt.Untrack(h2)
	if err != nil {
		t.Fatal(err)
	}
	err = mpt.Untrack(h1)
	if err != nil {
		t.Fatal(err)
	}
	err = mpt.Untrack(h1)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	st := mpt.Status(h1)
	if st.Status != api.TrackerStatusUnpinned {
		t.Fatalf("cid should be unpinned and is %s", st.Status)
	}

	st = mpt.Status(h2)
	if st.Status != api.TrackerStatusUnpinned {
		t.Fatalf("cid should be unpinned and is %s", st.Status)
	}
}

func TestStatusAll(t *testing.T) {
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown()

	h1, _ := cid.Decode(test.TestCid1)
	h2, _ := cid.Decode(test.TestCid2)

	// LocalPin
	c := api.Pin{
		Cid:                  h1,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}
	mpt.Track(c)
	c = api.Pin{
		Cid:                  h2,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: 1,
		ReplicationFactorMax: 1,
	}
	mpt.Track(c)

	time.Sleep(200 * time.Millisecond)

	stAll := mpt.StatusAll()
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
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown()

	h1, _ := cid.Decode(test.TestCid1)
	h2, _ := cid.Decode(test.TestCid2)

	c := api.Pin{
		Cid:                  h1,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}
	mpt.Track(c)
	c = api.Pin{
		Cid:                  h2,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}
	mpt.Track(c)

	time.Sleep(100 * time.Millisecond)

	info, err := mpt.Sync(h2)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != api.TrackerStatusPinError {
		t.Error("expected pin_error")
	}

	info, err = mpt.Sync(h1)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != api.TrackerStatusPinned {
		t.Error("expected pinned")
	}

	mpt.set(h1, api.TrackerStatusPinning)
	info, err = mpt.Sync(h1)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != api.TrackerStatusPinned {
		t.Error("expected pinned")
	}

	info, err = mpt.Recover(h1)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != api.TrackerStatusPinned {
		t.Error("expected pinned")
	}

	info, err = mpt.Recover(h2)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != api.TrackerStatusPinned {
		t.Error("expected pinned")
	}
}

func TestRecoverAll(t *testing.T) {
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown()

	h1, _ := cid.Decode(test.TestCid1)

	c := api.Pin{
		Cid:                  h1,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}
	mpt.Track(c)
	time.Sleep(100 * time.Millisecond)
	mpt.set(h1, api.TrackerStatusPinError)
	pins, err := mpt.RecoverAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 1 {
		t.Fatal("there should be only one pin")
	}
	if pins[0].Status != api.TrackerStatusPinned {
		t.Error("the pin should have been recovered")
	}
}

func TestSyncAll(t *testing.T) {
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown()

	synced, err := mpt.SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	// This relies on the rpc mock implementation

	if len(synced) != 0 {
		t.Fatal("should not have synced anything when it tracks nothing")
	}

	h1, _ := cid.Decode(test.TestCid1)
	h2, _ := cid.Decode(test.TestCid2)

	c := api.Pin{
		Cid:                  h1,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	mpt.Track(c)
	c = api.Pin{
		Cid:                  h2,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}
	mpt.Track(c)

	time.Sleep(100 * time.Millisecond)

	synced, err = mpt.SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(synced) != 1 || synced[0].Status != api.TrackerStatusPinError {
		t.Logf("%+v", synced)
		t.Fatal("should have synced h2")
	}
}

func TestUntrackTrack(t *testing.T) {
	mpt := testMapPinTracker(t)
	defer mpt.Shutdown()

	h1, _ := cid.Decode(test.TestCid1)

	// LocalPin
	c := api.Pin{
		Cid:                  h1,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := mpt.Track(c)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	err = mpt.Untrack(h1)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTrackUntrackWithCancel(t *testing.T) {
	mpt := testSlowMapPinTracker(t)
	defer mpt.Shutdown()

	slowPinCid, _ := cid.Decode(test.TestSlowCid1)

	// LocalPin
	slowPin := api.Pin{
		Cid:                  slowPinCid,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := mpt.Track(slowPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond) // let pinning start

	opc, ok := mpt.optracker.Get(slowPin.Cid)
	if !ok {
		t.Fatal("slowPin should be tracked")
	}

	if opc.Phase == optracker.PhaseInProgress && opc.Op == optracker.OperationPin {
		go func() {
			err = mpt.Untrack(slowPinCid)
			if err != nil {
				t.Fatal(err)
			}
		}()
		select {
		case <-opc.Ctx.Done():
			return
		case <-time.Tick(100 * time.Millisecond):
			t.Errorf("operation context should have been cancelled by now")
		}
	} else {
		t.Error("slowPin should be pinning and is:", opc.Phase)
	}
}

func TestTrackUntrackWithNoCancel(t *testing.T) {
	mpt := testSlowMapPinTracker(t)
	defer mpt.Shutdown()

	slowPinCid, _ := cid.Decode(test.TestSlowCid1)
	fastPinCid, _ := cid.Decode(pinCancelCid)

	// SlowLocalPin
	slowPin := api.Pin{
		Cid:                  slowPinCid,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	// LocalPin
	fastPin := api.Pin{
		Cid:                  fastPinCid,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := mpt.Track(slowPin)
	if err != nil {
		t.Fatal(err)
	}

	err = mpt.Track(fastPin)
	if err != nil {
		t.Fatal(err)
	}

	// fastPin should be queued because slow pin is pinning
	opc, _ := mpt.optracker.Get(fastPin.Cid)
	if opc.Phase == optracker.PhaseQueued && opc.Op == optracker.OperationPin {
		err = mpt.Untrack(fastPinCid)
		if err != nil {
			t.Fatal(err)
		}
		pi := mpt.get(fastPinCid)
		if pi.Error == ErrPinCancelCid.Error() {
			t.Fatal(ErrPinCancelCid)
		}
	} else {
		t.Error("fastPin should be queued to pin")
	}

	_, ok := mpt.optracker.Get(fastPin.Cid)
	if ok {
		t.Error("fastPin should have been removed from tracker")
	}
}

func TestUntrackTrackWithCancel(t *testing.T) {
	mpt := testSlowMapPinTracker(t)
	defer mpt.Shutdown()

	slowPinCid, _ := cid.Decode(test.TestSlowCid1)

	// LocalPin
	slowPin := api.Pin{
		Cid:                  slowPinCid,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := mpt.Track(slowPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	// Untrack should cancel the ongoing request
	// and unpin right away
	err = mpt.Untrack(slowPinCid)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	opc, ok := mpt.optracker.Get(slowPin.Cid)
	if !ok {
		t.Fatal("expected slowPin to be tracked")
	}

	if opc.Phase == optracker.PhaseInProgress && opc.Op == optracker.OperationUnpin {
		go func() {
			err = mpt.Track(slowPin)
			if err != nil {
				t.Fatal(err)
			}
		}()
		select {
		case <-opc.Ctx.Done():
			return
		case <-time.Tick(100 * time.Millisecond):
			t.Errorf("operation context should have been cancelled by now")
		}
	} else {
		t.Error("slowPin should be in unpinning")
	}

}

func TestUntrackTrackWithNoCancel(t *testing.T) {
	mpt := testSlowMapPinTracker(t)
	defer mpt.Shutdown()

	slowPinCid, _ := cid.Decode(test.TestSlowCid1)
	fastPinCid, _ := cid.Decode(unpinCancelCid)

	// SlowLocalPin
	slowPin := api.Pin{
		Cid:                  slowPinCid,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	// LocalPin
	fastPin := api.Pin{
		Cid:                  fastPinCid,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := mpt.Track(slowPin)
	if err != nil {
		t.Fatal(err)
	}

	err = mpt.Track(fastPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)

	err = mpt.Untrack(slowPin.Cid)
	if err != nil {
		t.Fatal(err)
	}

	err = mpt.Untrack(fastPin.Cid)
	if err != nil {
		t.Fatal(err)
	}

	opc, ok := mpt.optracker.Get(fastPin.Cid)
	if !ok {
		t.Fatal("c untrack operation should be tracked")
	}

	if opc.Phase == optracker.PhaseQueued && opc.Op == optracker.OperationUnpin {
		err = mpt.Track(fastPin)
		if err != nil {
			t.Fatal(err)
		}

		pi := mpt.get(fastPinCid)
		if pi.Error == ErrUnpinCancelCid.Error() {
			t.Fatal(ErrUnpinCancelCid)
		}
	} else {
		t.Error("c should be queued to unpin")
	}
}
