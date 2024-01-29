package stateless

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/inmem"
	"github.com/ipfs-cluster/ipfs-cluster/state"
	"github.com/ipfs-cluster/ipfs-cluster/state/dsstate"
	"github.com/ipfs-cluster/ipfs-cluster/test"

	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

var (
	pinCancelCid      = test.Cid3
	unpinCancelCid    = test.Cid2
	pinErrCid         = test.ErrorCid
	errPinCancelCid   = errors.New("should not have received rpc.IPFSPin operation")
	errUnpinCancelCid = errors.New("should not have received rpc.IPFSUnpin operation")
	pinOpts           = api.PinOptions{
		ReplicationFactorMax: -1,
		ReplicationFactorMin: -1,
	}
)

// func TestMain(m *testing.M) {
// 	logging.SetLogLevel("pintracker", "debug")

// 	os.Exit(m.Run())
// }

// Overwrite Pin and Unpin methods on the normal mock in order to return
// special errors when unwanted operations have been triggered.
type mockIPFS struct{}

func (mock *mockIPFS) Pin(ctx context.Context, in api.Pin, out *struct{}) error {
	switch in.Cid {
	case pinCancelCid:
		return errPinCancelCid
	case test.SlowCid1:
		time.Sleep(time.Second)
	case pinErrCid:
		return errors.New("error pinning")
	}
	return nil
}

func (mock *mockIPFS) Unpin(ctx context.Context, in api.Pin, out *struct{}) error {
	switch in.Cid {
	case unpinCancelCid:
		return errUnpinCancelCid
	case test.SlowCid1:
		time.Sleep(time.Second)
	case pinErrCid:
		return errors.New("error unpinning")
	}

	return nil
}

func (mock *mockIPFS) PinLs(ctx context.Context, in <-chan []string, out chan<- api.IPFSPinInfo) error {
	out <- api.IPFSPinInfo{
		Cid:  api.Cid(test.Cid1),
		Type: api.IPFSPinStatusRecursive,
	}

	out <- api.IPFSPinInfo{
		Cid:  api.Cid(test.Cid2),
		Type: api.IPFSPinStatusRecursive,
	}
	close(out)
	return nil
}

func (mock *mockIPFS) PinLsCid(ctx context.Context, in api.Pin, out *api.IPFSPinStatus) error {
	switch in.Cid {
	case test.Cid1, test.Cid2:
		*out = api.IPFSPinStatusRecursive
	default:
		*out = api.IPFSPinStatusUnpinned
		return nil
	}
	return nil
}

type mockCluster struct{}

func (mock *mockCluster) IPFSID(ctx context.Context, in peer.ID, out *api.IPFSID) error {
	addr, _ := api.NewMultiaddr("/ip4/127.0.0.1/tcp/4001/p2p/" + test.PeerID1.String())
	*out = api.IPFSID{
		ID:        test.PeerID1,
		Addresses: []api.Multiaddr{addr},
	}
	return nil
}

func mockRPCClient(t testing.TB) *rpc.Client {
	t.Helper()

	s := rpc.NewServer(nil, "mock")
	c := rpc.NewClientWithServer(nil, "mock", s)

	err := s.RegisterName("IPFSConnector", &mockIPFS{})
	if err != nil {
		t.Fatal(err)
	}

	err = s.RegisterName("Cluster", &mockCluster{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func getStateFunc(t testing.TB, items ...api.Pin) func(context.Context) (state.ReadOnly, error) {
	t.Helper()
	ctx := context.Background()

	st, err := dsstate.New(ctx, inmem.New(), "", dsstate.DefaultHandle())
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range items {
		err := st.Add(ctx, item)
		if err != nil {
			t.Fatal(err)
		}
	}
	return func(ctx context.Context) (state.ReadOnly, error) {
		return st, nil
	}

}

func testStatelessPinTracker(t testing.TB, pins ...api.Pin) *Tracker {
	t.Helper()

	cfg := &Config{}
	cfg.Default()
	cfg.ConcurrentPins = 1
	cfg.PriorityPinMaxAge = 10 * time.Second
	cfg.PriorityPinMaxRetries = 1
	spt := New(cfg, test.PeerID1, test.PeerName1, getStateFunc(t, pins...))
	spt.SetClient(mockRPCClient(t))
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
	spt := testStatelessPinTracker(t)
	defer spt.Shutdown(ctx)

	slowPinCid := test.SlowCid1

	// LocalPin
	slowPin := api.PinWithOpts(slowPinCid, pinOpts)

	err := spt.Track(ctx, slowPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond) // let pinning start

	pInfo := spt.optracker.Get(ctx, slowPin.Cid, api.IPFSID{})
	if pInfo.Status == api.TrackerStatusUnpinned {
		t.Fatal("slowPin should be tracked")
	}

	if pInfo.Status == api.TrackerStatusPinning {
		go func() {
			err = spt.Untrack(ctx, slowPinCid)
			if err != nil {
				t.Error(err)
				return
			}
		}()
		select {
		case <-spt.optracker.OpContext(ctx, slowPinCid).Done():
			return
		case <-time.Tick(100 * time.Millisecond):
			t.Errorf("operation context should have been canceled by now")
		}
	} else {
		t.Error("slowPin should be pinning and is:", pInfo.Status)
	}
}

// This tracks a slow CID and then tracks a fast/normal one.
// Because we are pinning the slow CID, the fast one will stay
// queued. We proceed to untrack it then. Since it was never
// "pinning", it should simply be unqueued (or ignored), and no
// canceling of the pinning operation happens (unlike on WithCancel).
func TestTrackUntrackWithNoCancel(t *testing.T) {
	ctx := context.Background()
	spt := testStatelessPinTracker(t)
	defer spt.Shutdown(ctx)

	slowPinCid := test.SlowCid1
	fastPinCid := pinCancelCid

	// SlowLocalPin
	slowPin := api.PinWithOpts(slowPinCid, pinOpts)

	// LocalPin
	fastPin := api.PinWithOpts(fastPinCid, pinOpts)

	err := spt.Track(ctx, slowPin)
	if err != nil {
		t.Fatal(err)
	}

	// Otherwise fails when running with -race
	time.Sleep(300 * time.Millisecond)

	err = spt.Track(ctx, fastPin)
	if err != nil {
		t.Fatal(err)
	}

	// fastPin should be queued because slow pin is pinning
	fastPInfo := spt.optracker.Get(ctx, fastPin.Cid, api.IPFSID{})
	if fastPInfo.Status == api.TrackerStatusUnpinned {
		t.Fatal("fastPin should be tracked")
	}
	if fastPInfo.Status == api.TrackerStatusPinQueued {
		err = spt.Untrack(ctx, fastPinCid)
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

	pi := spt.optracker.Get(ctx, fastPin.Cid, api.IPFSID{})
	if !pi.Cid.Defined() {
		t.Error("fastPin should have been removed from tracker")
	}
}

func TestUntrackTrackWithCancel(t *testing.T) {
	ctx := context.Background()
	spt := testStatelessPinTracker(t)
	defer spt.Shutdown(ctx)

	slowPinCid := test.SlowCid1

	// LocalPin
	slowPin := api.PinWithOpts(slowPinCid, pinOpts)

	err := spt.Track(ctx, slowPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	// Untrack should cancel the ongoing request
	// and unpin right away
	err = spt.Untrack(ctx, slowPinCid)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	pi := spt.optracker.Get(ctx, slowPin.Cid, api.IPFSID{})
	if !pi.Cid.Defined() {
		t.Fatal("expected slowPin to be tracked")
	}

	if pi.Status == api.TrackerStatusUnpinning {
		go func() {
			err = spt.Track(ctx, slowPin)
			if err != nil {
				t.Error(err)
				return
			}
		}()
		select {
		case <-spt.optracker.OpContext(ctx, slowPinCid).Done():
			return
		case <-time.Tick(100 * time.Millisecond):
			t.Errorf("operation context should have been canceled by now")
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

	err := spt.Track(ctx, slowPin)
	if err != nil {
		t.Fatal(err)
	}

	err = spt.Track(ctx, fastPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)

	err = spt.Untrack(ctx, slowPin.Cid)
	if err != nil {
		t.Fatal(err)
	}

	err = spt.Untrack(ctx, fastPin.Cid)
	if err != nil {
		t.Fatal(err)
	}

	pi := spt.optracker.Get(ctx, fastPin.Cid, api.IPFSID{})
	if !pi.Cid.Defined() {
		t.Fatal("c untrack operation should be tracked")
	}

	if pi.Status == api.TrackerStatusUnpinQueued {
		err = spt.Track(ctx, fastPin)
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

// TestStatusAll checks that StatusAll correctly reports tracked
// items and mismatches between what's on IPFS and on the state.
func TestStatusAll(t *testing.T) {
	ctx := context.Background()

	normalPin := api.PinWithOpts(test.Cid1, pinOpts)
	normalPin2 := api.PinWithOpts(test.Cid4, pinOpts)

	// - Build a state with one pins (Cid1,Cid4)
	// - The IPFS Mock reports Cid1 and Cid2
	// - Track a SlowCid additionally
	slowPin := api.PinWithOpts(test.SlowCid1, pinOpts)
	spt := testStatelessPinTracker(t, normalPin, normalPin2, slowPin)
	defer spt.Shutdown(ctx)

	err := spt.Track(ctx, slowPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	// Needs to return:
	// * A slow CID pinning
	// * Cid1 is pinned
	// * Cid4 should be in PinError (it's in the state but not on IPFS)
	stAll := make(chan api.PinInfo, 10)
	err = spt.StatusAll(ctx, api.TrackerStatusUndefined, stAll)
	if err != nil {
		t.Fatal(err)
	}

	n := 0
	for pi := range stAll {
		n++
		switch pi.Cid {
		case test.Cid1:
			if pi.Status != api.TrackerStatusPinned {
				t.Error(test.Cid1, " should be pinned")
			}
		case test.Cid4:
			if pi.Status != api.TrackerStatusUnexpectedlyUnpinned {
				t.Error(test.Cid2, " should be in unexpectedly_unpinned status")
			}
		case test.SlowCid1:
			if pi.Status != api.TrackerStatusPinning {
				t.Error("slowCid1 should be pinning")
			}
		default:
			t.Error("Unexpected pin:", pi.Cid)
		}
		if pi.IPFS == "" {
			t.Error("IPFS field should be set")
		}
	}
	if n != 3 {
		t.Errorf("wrong status length. Expected 3, got: %d", n)
	}
}

// TestStatus checks that the Status calls correctly reports tracked
// items and mismatches between what's on IPFS and on the state.
func TestStatus(t *testing.T) {
	ctx := context.Background()

	normalPin := api.PinWithOpts(test.Cid1, pinOpts)
	normalPin2 := api.PinWithOpts(test.Cid4, pinOpts)

	// - Build a state with one pins (Cid1,Cid4)
	// - The IPFS Mock reports Cid1 and Cid2
	// - Track a SlowCid additionally

	spt := testStatelessPinTracker(t, normalPin, normalPin2)
	defer spt.Shutdown(ctx)

	slowPin := api.PinWithOpts(test.SlowCid1, pinOpts)
	err := spt.Track(ctx, slowPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	// Status needs to return:
	// * For slowCid1: A slow CID pinning
	// * For Cid1: pinned
	// * For Cid4: unexpectedly_unpinned

	st := spt.Status(ctx, test.Cid1)
	if st.Status != api.TrackerStatusPinned {
		t.Error("cid1 should be pinned")
	}

	st = spt.Status(ctx, test.Cid4)
	if st.Status != api.TrackerStatusUnexpectedlyUnpinned {
		t.Error("cid2 should be in unexpectedly_unpinned status")
	}

	st = spt.Status(ctx, test.SlowCid1)
	if st.Status != api.TrackerStatusPinning {
		t.Error("slowCid1 should be pinning")
	}

	if st.IPFS == "" {
		t.Error("IPFS field should be set")
	}
}

// Test
func TestAttemptCountAndPriority(t *testing.T) {
	ctx := context.Background()

	normalPin := api.PinWithOpts(test.Cid1, pinOpts)
	normalPin2 := api.PinWithOpts(test.Cid4, pinOpts)
	errPin := api.PinWithOpts(pinErrCid, pinOpts)

	spt := testStatelessPinTracker(t, normalPin, normalPin2, errPin)
	defer spt.Shutdown(ctx)

	st := spt.Status(ctx, test.Cid1)
	if st.AttemptCount != 0 {
		t.Errorf("errPin should have 0 attempts as it was already pinned: %+v", st)
	}

	err := spt.Track(ctx, errPin)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond) // let the pin be applied
	st = spt.Status(ctx, pinErrCid)
	if st.AttemptCount != 1 {
		t.Errorf("errPin should have 1 attempt count: %+v", st)
	}

	// Retry 1
	_, err = spt.Recover(ctx, pinErrCid)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond) // let the pin be applied
	st = spt.Status(ctx, pinErrCid)
	if st.AttemptCount != 2 || !st.PriorityPin {
		t.Errorf("errPin should have 2 attempt counts and be priority: %+v", st)
	}

	// Retry 2
	_, err = spt.Recover(ctx, pinErrCid)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond) // let the pin be applied
	st = spt.Status(ctx, pinErrCid)
	if st.AttemptCount != 3 || st.PriorityPin {
		t.Errorf("errPin should have 3 attempts and not be priority: %+v", st)
	}

	err = spt.Untrack(ctx, pinErrCid)
	time.Sleep(200 * time.Millisecond) // let the pin be applied
	if err != nil {
		t.Fatal(err)
	}
	st = spt.Status(ctx, pinErrCid)
	if st.AttemptCount != 1 {
		t.Errorf("errPin should have 1 attempt count to unpin: %+v", st)
	}

	err = spt.Untrack(ctx, pinErrCid)
	time.Sleep(200 * time.Millisecond) // let the pin be applied
	if err != nil {
		t.Fatal(err)
	}
	st = spt.Status(ctx, pinErrCid)
	if st.AttemptCount != 2 {
		t.Errorf("errPin should have 2 attempt counts to unpin: %+v", st)
	}
}
