package maptracker

import (
	"testing"
	"time"

	peer "gx/ipfs/QmZcUPvPhD1Xvk6mwijYF8AfR3mG31S1YsEfHG4khrFPRr/go-libp2p-peer"
	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/test"
)

func testMapPinTracker(t *testing.T) *MapPinTracker {
	mpt := NewMapPinTracker(test.TestPeerID1)
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
		Cid:               h,
		Allocations:       []peer.ID{},
		ReplicationFactor: -1,
	}

	err := mpt.Track(c)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond) // let it be pinned

	st := mpt.Status(h)
	if st.Status != api.TrackerStatusPinned {
		t.Fatalf("cid should be pinned and is %s", st.Status)
	}

	// Unpin and set remote
	c = api.Pin{
		Cid:               h,
		Allocations:       []peer.ID{test.TestPeerID2},
		ReplicationFactor: 1,
	}
	err = mpt.Track(c)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond) // let it be unpinned

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
		Cid:               h1,
		Allocations:       []peer.ID{},
		ReplicationFactor: -1,
	}

	err := mpt.Track(c)
	if err != nil {
		t.Fatal(err)
	}

	// Remote pin
	c = api.Pin{
		Cid:               h2,
		Allocations:       []peer.ID{test.TestPeerID2},
		ReplicationFactor: 1,
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
	c := api.Pin{Cid: h1, Allocations: []peer.ID{}, ReplicationFactor: -1}
	mpt.Track(c)
	c = api.Pin{Cid: h2, Allocations: []peer.ID{test.TestPeerID2}, ReplicationFactor: 1}
	mpt.Track(c)

	time.Sleep(100 * time.Millisecond)

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

	c := api.Pin{Cid: h1, Allocations: []peer.ID{}, ReplicationFactor: -1}
	mpt.Track(c)
	c = api.Pin{Cid: h2, Allocations: []peer.ID{}, ReplicationFactor: -1}
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

	c := api.Pin{Cid: h1, Allocations: []peer.ID{}, ReplicationFactor: -1}
	mpt.Track(c)
	c = api.Pin{Cid: h2, Allocations: []peer.ID{}, ReplicationFactor: -1}
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
