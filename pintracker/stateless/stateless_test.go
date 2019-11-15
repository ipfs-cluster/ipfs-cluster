package stateless

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/datastore/inmem"
	"github.com/ipfs/ipfs-cluster/state"
	"github.com/ipfs/ipfs-cluster/state/dsstate"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	rpc "github.com/libp2p/go-libp2p-gorpc"
)

var (
	pinCancelCid      = test.Cid3
	unpinCancelCid    = test.Cid2
	ErrPinCancelCid   = errors.New("should not have received rpc.IPFSPin operation")
	ErrUnpinCancelCid = errors.New("should not have received rpc.IPFSUnpin operation")

	pinOpts = api.PinOptions{
		ReplicationFactorMax: -1,
		ReplicationFactorMin: -1,
	}
)

type mockIPFS struct{}

func mockRPCClient(t *testing.T) *rpc.Client {
	s := rpc.NewServer(nil, "mock")
	c := rpc.NewClientWithServer(nil, "mock", s)

	err := s.RegisterName("IPFSConnector", &mockIPFS{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (mock *mockIPFS) Pin(ctx context.Context, in *api.Pin, out *struct{}) error {
	switch in.Cid.String() {
	case test.SlowCid1.String():
		time.Sleep(2 * time.Second)
	case pinCancelCid.String():
		return ErrPinCancelCid
	}
	return nil
}

func (mock *mockIPFS) Unpin(ctx context.Context, in *api.Pin, out *struct{}) error {
	switch in.Cid.String() {
	case test.SlowCid1.String():
		time.Sleep(2 * time.Second)
	case unpinCancelCid.String():
		return ErrUnpinCancelCid
	}
	return nil
}

func (mock *mockIPFS) PinLs(ctx context.Context, in string, out *map[string]api.IPFSPinStatus) error {
	m := map[string]api.IPFSPinStatus{
		test.Cid1.String(): api.IPFSPinStatusRecursive,
	}
	*out = m
	return nil
}

func (mock *mockIPFS) PinLsCid(ctx context.Context, in cid.Cid, out *api.IPFSPinStatus) error {
	switch in.String() {
	case test.Cid1.String(), test.Cid2.String():
		*out = api.IPFSPinStatusRecursive
	default:
		*out = api.IPFSPinStatusUnpinned
	}
	return nil
}

func testSlowStatelessPinTracker(t *testing.T) *Tracker {
	cfg := &Config{}
	cfg.Default()
	cfg.ConcurrentPins = 1
	st, err := dsstate.New(inmem.New(), "", dsstate.DefaultHandle())
	if err != nil {
		t.Fatal(err)
	}
	getState := func(ctx context.Context) (state.ReadOnly, error) {
		return st, nil
	}
	spt := New(cfg, test.PeerID1, test.PeerName1, getState)
	spt.SetClient(mockRPCClient(t))
	return spt
}

func testStatelessPinTracker(t testing.TB) *Tracker {
	cfg := &Config{}
	cfg.Default()
	cfg.ConcurrentPins = 1
	st, err := dsstate.New(inmem.New(), "", dsstate.DefaultHandle())
	if err != nil {
		t.Fatal(err)
	}
	getState := func(ctx context.Context) (state.ReadOnly, error) {
		return st, nil
	}
	spt := New(cfg, test.PeerID1, test.PeerName1, getState)
	spt.SetClient(test.NewMockRPCClient(t))
	return spt
}

func TestStatelessPinTracker_New(t *testing.T) {
	ctx := context.Background()
	spt := testStatelessPinTracker(t)
	defer spt.Shutdown(ctx)
}

func TestStatelessPinTracker_Shutdown(t *testing.T) {
	ctx := context.Background()
	spt := testStatelessPinTracker(t)
	err := spt.Shutdown(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = spt.Shutdown(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUntrackTrack(t *testing.T) {
	ctx := context.Background()
	spt := testStatelessPinTracker(t)
	defer spt.Shutdown(ctx)

	h1 := test.Cid1

	// LocalPin
	c := api.PinWithOpts(h1, pinOpts)

	err := spt.Track(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	err = spt.Untrack(context.Background(), h1)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTrackUntrackWithCancel(t *testing.T) {
	ctx := context.Background()
	spt := testSlowStatelessPinTracker(t)
	defer spt.Shutdown(ctx)

	slowPinCid := test.SlowCid1

	// LocalPin
	slowPin := api.PinWithOpts(slowPinCid, pinOpts)

	err := spt.Track(context.Background(), slowPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond) // let pinning start

	pInfo := spt.optracker.Get(context.Background(), slowPin.Cid)
	if pInfo.Status == api.TrackerStatusUnpinned {
		t.Fatal("slowPin should be tracked")
	}

	if pInfo.Status == api.TrackerStatusPinning {
		go func() {
			err = spt.Untrack(context.Background(), slowPinCid)
			if err != nil {
				t.Fatal(err)
			}
		}()
		select {
		case <-spt.optracker.OpContext(context.Background(), slowPinCid).Done():
			return
		case <-time.Tick(100 * time.Millisecond):
			t.Errorf("operation context should have been cancelled by now")
		}
	} else {
		t.Error("slowPin should be pinning and is:", pInfo.Status)
	}
}

// This tracks a slow CID and then tracks a fast/normal one.
// Because we are pinning the slow CID, the fast one will stay
// queued. We proceed to untrack it then. Since it was never
// "pinning", it should simply be unqueued (or ignored), and no
// cancelling of the pinning operation happens (unlike on WithCancel).
func TestTrackUntrackWithNoCancel(t *testing.T) {
	ctx := context.Background()
	spt := testSlowStatelessPinTracker(t)
	defer spt.Shutdown(ctx)

	slowPinCid := test.SlowCid1
	fastPinCid := pinCancelCid

	// SlowLocalPin
	slowPin := api.PinWithOpts(slowPinCid, pinOpts)

	// LocalPin
	fastPin := api.PinWithOpts(fastPinCid, pinOpts)

	err := spt.Track(context.Background(), slowPin)
	if err != nil {
		t.Fatal(err)
	}

	// Otherwise fails when running with -race
	time.Sleep(300 * time.Millisecond)

	err = spt.Track(context.Background(), fastPin)
	if err != nil {
		t.Fatal(err)
	}

	// fastPin should be queued because slow pin is pinning
	fastPInfo := spt.optracker.Get(context.Background(), fastPin.Cid)
	if fastPInfo.Status == api.TrackerStatusUnpinned {
		t.Fatal("fastPin should be tracked")
	}
	if fastPInfo.Status == api.TrackerStatusPinQueued {
		err = spt.Untrack(context.Background(), fastPinCid)
		if err != nil {
			t.Fatal(err)
		}
		// pi := spt.get(fastPinCid)
		// if pi.Error == ErrPinCancelCid.Error() {
		// 	t.Fatal(ErrPinCancelCid)
		// }
	} else {
		t.Errorf("fastPin should be queued to pin but is %s", fastPInfo.Status)
	}

	pi := spt.optracker.Get(context.Background(), fastPin.Cid)
	if pi.Cid == cid.Undef {
		t.Error("fastPin should have been removed from tracker")
	}
}

func TestUntrackTrackWithCancel(t *testing.T) {
	ctx := context.Background()
	spt := testSlowStatelessPinTracker(t)
	defer spt.Shutdown(ctx)

	slowPinCid := test.SlowCid1

	// LocalPin
	slowPin := api.PinWithOpts(slowPinCid, pinOpts)

	err := spt.Track(context.Background(), slowPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	// Untrack should cancel the ongoing request
	// and unpin right away
	err = spt.Untrack(context.Background(), slowPinCid)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	pi := spt.optracker.Get(context.Background(), slowPin.Cid)
	if pi.Cid == cid.Undef {
		t.Fatal("expected slowPin to be tracked")
	}

	if pi.Status == api.TrackerStatusUnpinning {
		go func() {
			err = spt.Track(context.Background(), slowPin)
			if err != nil {
				t.Fatal(err)
			}
		}()
		select {
		case <-spt.optracker.OpContext(context.Background(), slowPinCid).Done():
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
	spt := testStatelessPinTracker(t)
	defer spt.Shutdown(ctx)

	slowPinCid := test.SlowCid1
	fastPinCid := unpinCancelCid

	// SlowLocalPin
	slowPin := api.PinWithOpts(slowPinCid, pinOpts)

	// LocalPin
	fastPin := api.PinWithOpts(fastPinCid, pinOpts)

	err := spt.Track(context.Background(), slowPin)
	if err != nil {
		t.Fatal(err)
	}

	err = spt.Track(context.Background(), fastPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)

	err = spt.Untrack(context.Background(), slowPin.Cid)
	if err != nil {
		t.Fatal(err)
	}

	err = spt.Untrack(context.Background(), fastPin.Cid)
	if err != nil {
		t.Fatal(err)
	}

	pi := spt.optracker.Get(context.Background(), fastPin.Cid)
	if pi.Cid == cid.Undef {
		t.Fatal("c untrack operation should be tracked")
	}

	if pi.Status == api.TrackerStatusUnpinQueued {
		err = spt.Track(context.Background(), fastPin)
		if err != nil {
			t.Fatal(err)
		}

		// pi := spt.get(fastPinCid)
		// if pi.Error == ErrUnpinCancelCid.Error() {
		// 	t.Fatal(ErrUnpinCancelCid)
		// }
	} else {
		t.Error("c should be queued to unpin")
	}
}

var sortPinInfoByCid = func(p []*api.PinInfo) {
	sort.Slice(p, func(i, j int) bool {
		return p[i].Cid.String() < p[j].Cid.String()
	})
}

func BenchmarkTracker_localStatus(b *testing.B) {
	tracker := testStatelessPinTracker(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.localStatus(context.Background(), true)
	}
}
