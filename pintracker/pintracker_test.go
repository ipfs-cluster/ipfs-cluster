// Package pintracker_test tests the multiple implementations
// of the PinTracker interface.
package pintracker_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/pintracker/maptracker"
	"github.com/ipfs/ipfs-cluster/pintracker/stateless"
	"github.com/ipfs/ipfs-cluster/test"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
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

func mockRPCClient(t testing.TB) *rpc.Client {
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
		time.Sleep(3 * time.Second)
	case pinCancelCid:
		return ErrPinCancelCid
	}
	return nil
}

func (mock *mockService) IPFSPinLsCid(ctx context.Context, in api.PinSerial, out *api.IPFSPinStatus) error {
	switch in.Cid {
	case test.TestCid1, test.TestCid2:
		*out = api.IPFSPinStatusRecursive
	default:
		*out = api.IPFSPinStatusUnpinned
	}
	return nil
}

func (mock *mockService) IPFSUnpin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	c := in.ToPin().Cid
	switch c.String() {
	case test.TestSlowCid1:
		time.Sleep(3 * time.Second)
	case unpinCancelCid:
		return ErrUnpinCancelCid
	}
	return nil
}

func (mock *mockService) IPFSPinLs(ctx context.Context, in string, out *map[string]api.IPFSPinStatus) error {
	m := map[string]api.IPFSPinStatus{
		test.TestCid1: api.IPFSPinStatusRecursive,
	}
	*out = m
	return nil
}

func (mock *mockService) Status(ctx context.Context, in api.PinSerial, out *api.GlobalPinInfoSerial) error {
	switch in.Cid {
	case test.ErrorCid:
		return test.ErrBadCid
	case test.TestCid1:
		c1, _ := cid.Decode(test.TestCid1)
		*out = api.GlobalPinInfo{
			Cid: c1,
			PeerMap: map[peer.ID]api.PinInfo{
				test.TestPeerID1: {
					Cid:    c1,
					Peer:   test.TestPeerID1,
					Status: api.TrackerStatusPinned,
					TS:     time.Now(),
				},
			},
		}.ToSerial()
		// case test.TestSlowCid1:
		//	sc1 := test.MustDecodeCid(test.TestSlowCid1)
		// 	*out = api.GlobalPinInfo{
		// 		Cid: sc1,
		// 		PeerMap: map[peer.ID]api.PinInfo{
		// 			test.TestPeerID1: {
		// 				Cid:    sc1,
		// 				Peer:   test.TestPeerID1,
		// 				Status: api.TrackerStatusPinned,
		// 				TS:     time.Now(),
		// 			},
		// 		},
		// 	}.ToSerial()
	}
	return nil
}

func (mock *mockService) StatusAll(ctx context.Context, in struct{}, out *[]api.GlobalPinInfoSerial) error {
	c1, _ := cid.Decode(test.TestCid1)
	c2, _ := cid.Decode(test.TestCid2)
	c3, _ := cid.Decode(test.TestCid3)
	slowC1 := test.MustDecodeCid(test.TestSlowCid1)
	*out = ipfscluster.GlobalPinInfoSliceToSerial([]api.GlobalPinInfo{
		{
			Cid: c1,
			PeerMap: map[peer.ID]api.PinInfo{
				test.TestPeerID1: {
					Cid:    c1,
					Peer:   test.TestPeerID1,
					Status: api.TrackerStatusPinned,
					TS:     time.Now(),
				},
			},
		},
		{
			Cid: c2,
			PeerMap: map[peer.ID]api.PinInfo{
				test.TestPeerID1: {
					Cid:    c2,
					Peer:   test.TestPeerID1,
					Status: api.TrackerStatusPinning,
					TS:     time.Now(),
				},
			},
		},
		{
			Cid: c3,
			PeerMap: map[peer.ID]api.PinInfo{
				test.TestPeerID1: {
					Cid:    c3,
					Peer:   test.TestPeerID1,
					Status: api.TrackerStatusPinError,
					TS:     time.Now(),
				},
			},
		},
		{
			Cid: slowC1,
			PeerMap: map[peer.ID]api.PinInfo{
				test.TestPeerID1: {
					Cid:    slowC1,
					Peer:   test.TestPeerID1,
					Status: api.TrackerStatusPinning,
					TS:     time.Now(),
				},
			},
		},
	})
	return nil
}

func (mock *mockService) Pins(ctx context.Context, in struct{}, out *[]api.PinSerial) error {
	*out = []api.PinSerial{
		{Cid: test.TestCid1, ReplicationFactorMax: -1},
		{Cid: test.TestCid3, ReplicationFactorMax: -1},
	}
	return nil
}

func (mock *mockService) PinGet(ctx context.Context, in api.PinSerial, out *api.PinSerial) error {
	switch in.Cid {
	case test.ErrorCid:
		return errors.New("expected error when using ErrorCid")
	case test.TestCid1:
		*out = api.Pin{Cid: test.MustDecodeCid(in.Cid), ReplicationFactorMax: -1}.ToSerial()
		return nil
	case test.TestCid2:
		*out = api.Pin{Cid: test.MustDecodeCid(in.Cid), ReplicationFactorMax: -1}.ToSerial()
		return nil
	}
	*out = in
	return nil
}

var sortPinInfoByCid = func(p []api.PinInfo) {
	sort.Slice(p, func(i, j int) bool {
		return p[i].Cid.String() < p[j].Cid.String()
	})
}

func testSlowMapPinTracker(t testing.TB) *maptracker.MapPinTracker {
	cfg := &maptracker.Config{}
	cfg.Default()
	mpt := maptracker.NewMapPinTracker(cfg, test.TestPeerID1)
	mpt.SetClient(mockRPCClient(t))
	return mpt
}

func testMapPinTracker(t testing.TB) *maptracker.MapPinTracker {
	cfg := &maptracker.Config{}
	cfg.Default()
	mpt := maptracker.NewMapPinTracker(cfg, test.TestPeerID1)
	mpt.SetClient(test.NewMockRPCClient(t))
	return mpt
}

func testSlowStatelessPinTracker(t testing.TB) *stateless.Tracker {
	cfg := &stateless.Config{}
	cfg.Default()
	mpt := stateless.New(cfg, test.TestPeerID1)
	mpt.SetClient(mockRPCClient(t))
	return mpt
}

func testStatelessPinTracker(t testing.TB) *stateless.Tracker {
	cfg := &stateless.Config{}
	cfg.Default()
	spt := stateless.New(cfg, test.TestPeerID1)
	spt.SetClient(test.NewMockRPCClient(t))
	return spt
}

func TestPinTracker_Track(t *testing.T) {
	type args struct {
		c       api.Pin
		tracker ipfscluster.PinTracker
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			"basic stateless track",
			args{
				api.PinCid(test.MustDecodeCid(test.TestCid1)),
				testStatelessPinTracker(t),
			},
			false,
		},
		{
			"basic map track",
			args{
				api.PinCid(test.MustDecodeCid(test.TestCid1)),
				testMapPinTracker(t),
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.args.tracker.Track(tt.args.c); (err != nil) != tt.wantErr {
				t.Errorf("PinTracker.Track() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func BenchmarkPinTracker_Track(b *testing.B) {
	type args struct {
		c       api.Pin
		tracker ipfscluster.PinTracker
	}
	tests := []struct {
		name string
		args args
	}{
		{
			"basic stateless track",
			args{
				api.PinCid(test.MustDecodeCid(test.TestCid1)),
				testStatelessPinTracker(b),
			},
		},
		{
			"basic map track",
			args{
				api.PinCid(test.MustDecodeCid(test.TestCid1)),
				testMapPinTracker(b),
			},
		},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := tt.args.tracker.Track(tt.args.c); err != nil {
					b.Errorf("PinTracker.Track() error = %v", err)
				}
			}
		})
	}
}

func TestPinTracker_Untrack(t *testing.T) {
	type args struct {
		c       *cid.Cid
		tracker ipfscluster.PinTracker
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			"basic stateless untrack",
			args{
				test.MustDecodeCid(test.TestCid1),
				testStatelessPinTracker(t),
			},
			false,
		},
		{
			"basic map untrack",
			args{
				test.MustDecodeCid(test.TestCid1),
				testMapPinTracker(t),
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.args.tracker.Untrack(tt.args.c); (err != nil) != tt.wantErr {
				t.Errorf("PinTracker.Untrack() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPinTracker_StatusAll(t *testing.T) {
	type args struct {
		c       api.Pin
		tracker ipfscluster.PinTracker
	}
	tests := []struct {
		name string
		args args
		want []api.PinInfo
	}{
		{
			"basic stateless statusall",
			args{
				api.Pin{
					Cid:                  test.MustDecodeCid(test.TestCid1),
					Allocations:          []peer.ID{},
					ReplicationFactorMin: -1,
					ReplicationFactorMax: -1,
				},
				testStatelessPinTracker(t),
			},
			[]api.PinInfo{
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid1),
					Status: api.TrackerStatusPinned,
				},
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid2),
					Status: api.TrackerStatusRemote,
				},
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid3),
					Status: api.TrackerStatusPinned,
				},
			},
		},
		{
			"basic map statusall",
			args{
				api.Pin{
					Cid:                  test.MustDecodeCid(test.TestCid1),
					Allocations:          []peer.ID{},
					ReplicationFactorMin: -1,
					ReplicationFactorMax: -1,
				},
				testMapPinTracker(t),
			},
			[]api.PinInfo{
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid1),
					Status: api.TrackerStatusPinned,
				},
			},
		},
		{
			"slow stateless statusall",
			args{
				api.Pin{
					Cid:                  test.MustDecodeCid(test.TestCid1),
					Allocations:          []peer.ID{},
					ReplicationFactorMin: -1,
					ReplicationFactorMax: -1,
				},
				testSlowStatelessPinTracker(t),
			},
			[]api.PinInfo{
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid1),
					Status: api.TrackerStatusPinned,
				},
			},
		},
		{
			"slow map statusall",
			args{
				api.Pin{
					Cid:                  test.MustDecodeCid(test.TestCid1),
					Allocations:          []peer.ID{},
					ReplicationFactorMin: -1,
					ReplicationFactorMax: -1,
				},
				testSlowMapPinTracker(t),
			},
			[]api.PinInfo{
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid1),
					Status: api.TrackerStatusPinned,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.args.tracker.Track(tt.args.c); err != nil {
				t.Errorf("PinTracker.Track() error = %v", err)
			}
			time.Sleep(1 * time.Second)
			got := tt.args.tracker.StatusAll()
			if len(got) != len(tt.want) {
				for _, pi := range got {
					t.Logf("pinfo: %v", pi)
				}
				t.Errorf("got len = %d, want = %d", len(got), len(tt.want))
				t.FailNow()
			}

			sortPinInfoByCid(got)
			sortPinInfoByCid(tt.want)

			for i := range tt.want {
				if got[i].Cid.String() != tt.want[i].Cid.String() {
					t.Errorf("got: %v\nwant: %v", got, tt.want)
				}
				if got[i].Status != tt.want[i].Status {
					t.Errorf("for cid %v:\n got: %s\nwant: %s", got[i].Cid, got[i].Status, tt.want[i].Status)
				}
			}
		})
	}
}

func BenchmarkPinTracker_StatusAll(b *testing.B) {
	type args struct {
		tracker ipfscluster.PinTracker
	}
	tests := []struct {
		name string
		args args
	}{
		{
			"basic stateless track",
			args{
				testStatelessPinTracker(b),
			},
		},
		{
			"basic map track",
			args{
				testMapPinTracker(b),
			},
		},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tt.args.tracker.StatusAll()
			}
		})
	}
}

func TestPinTracker_Status(t *testing.T) {
	type args struct {
		c       *cid.Cid
		tracker ipfscluster.PinTracker
	}
	tests := []struct {
		name string
		args args
		want api.PinInfo
	}{
		{
			"basic stateless status",
			args{
				test.MustDecodeCid(test.TestCid1),
				testStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.MustDecodeCid(test.TestCid1),
				Status: api.TrackerStatusPinned,
			},
		},
		{
			"basic map status",
			args{
				test.MustDecodeCid(test.TestCid1),
				testMapPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.MustDecodeCid(test.TestCid1),
				Status: api.TrackerStatusPinned,
			},
		},
		{
			"slow stateless status",
			args{
				test.MustDecodeCid(test.TestCid1),
				testSlowStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.MustDecodeCid(test.TestCid1),
				Status: api.TrackerStatusPinned,
			},
		},
		{
			"slow map status",
			args{
				test.MustDecodeCid(test.TestCid1),
				testSlowMapPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.MustDecodeCid(test.TestCid1),
				Status: api.TrackerStatusPinned,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.args.tracker.(type) {
			case *maptracker.MapPinTracker:
				// the Track preps the internal map of the MapPinTracker
				// not required by the Stateless impl
				pin := api.Pin{
					Cid:                  test.MustDecodeCid(test.TestCid1),
					Allocations:          []peer.ID{},
					ReplicationFactorMin: -1,
					ReplicationFactorMax: -1,
				}
				if err := tt.args.tracker.Track(pin); err != nil {
					t.Errorf("PinTracker.Track() error = %v", err)
				}
				time.Sleep(1 * time.Second)
			}

			got := tt.args.tracker.Status(tt.args.c)

			if got.Cid.String() != tt.want.Cid.String() {
				t.Errorf("PinTracker.Status() = %v, want %v", got.Cid, tt.want.Cid)
			}

			if got.Status != tt.want.Status {
				t.Errorf("PinTracker.Status() = %v, want %v", got.Status, tt.want.Status)
			}
		})
	}
}

func TestPinTracker_SyncAll(t *testing.T) {
	type args struct {
		cs      []*cid.Cid
		tracker ipfscluster.PinTracker
	}
	tests := []struct {
		name    string
		args    args
		want    []api.PinInfo
		wantErr bool
	}{
		{
			"basic stateless syncall",
			args{
				[]*cid.Cid{
					test.MustDecodeCid(test.TestCid1),
					test.MustDecodeCid(test.TestCid2),
				},
				testStatelessPinTracker(t),
			},
			[]api.PinInfo{
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid1),
					Status: api.TrackerStatusPinned,
				},
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid2),
					Status: api.TrackerStatusPinned,
				},
			},
			false,
		},
		{
			"basic map syncall",
			args{
				[]*cid.Cid{
					test.MustDecodeCid(test.TestCid1),
					test.MustDecodeCid(test.TestCid2),
				},
				testMapPinTracker(t),
			},
			[]api.PinInfo{
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid1),
					Status: api.TrackerStatusPinned,
				},
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid2),
					Status: api.TrackerStatusPinned,
				},
			},
			false,
		},
		{
			"slow stateless syncall",
			args{
				[]*cid.Cid{
					test.MustDecodeCid(test.TestCid1),
					test.MustDecodeCid(test.TestCid2),
				},
				testSlowStatelessPinTracker(t),
			},
			[]api.PinInfo{
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid1),
					Status: api.TrackerStatusPinned,
				},
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid2),
					Status: api.TrackerStatusPinned,
				},
			},
			false,
		},
		{
			"slow map syncall",
			args{
				[]*cid.Cid{
					test.MustDecodeCid(test.TestCid1),
					test.MustDecodeCid(test.TestCid2),
				},
				testSlowMapPinTracker(t),
			},
			[]api.PinInfo{
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid1),
					Status: api.TrackerStatusPinned,
				},
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid2),
					Status: api.TrackerStatusPinned,
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.args.tracker.SyncAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("PinTracker.SyncAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(got) != 0 {
				t.Fatalf("should not have synced anything when it tracks nothing")
			}

			for _, c := range tt.args.cs {
				err := tt.args.tracker.Track(api.PinCid(c))
				if err != nil {
					t.Fatal(err)
				}
			}

			sortPinInfoByCid(got)
			sortPinInfoByCid(tt.want)

			for i := range got {
				if got[i].Cid.String() != tt.want[i].Cid.String() {
					t.Errorf("PinTracker.SyncAll() = %v, want %v", got, tt.want)
				}

				if got[i].Status != tt.want[i].Status {
					t.Errorf("PinTracker.SyncAll() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestPinTracker_Sync(t *testing.T) {
	type args struct {
		c       *cid.Cid
		tracker ipfscluster.PinTracker
	}
	tests := []struct {
		name    string
		args    args
		want    api.PinInfo
		wantErr bool
	}{
		{
			"basic stateless sync",
			args{
				test.MustDecodeCid(test.TestCid1),
				testStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.MustDecodeCid(test.TestCid1),
				Status: api.TrackerStatusPinned,
			},
			false,
		},
		{
			"basic map sync",
			args{
				test.MustDecodeCid(test.TestCid1),
				testMapPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.MustDecodeCid(test.TestCid1),
				Status: api.TrackerStatusPinned,
			},
			false,
		},
		{
			"slow stateless sync",
			args{
				test.MustDecodeCid(test.TestCid1),
				testSlowStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.MustDecodeCid(test.TestCid1),
				Status: api.TrackerStatusPinned,
			},
			false,
		},
		{
			"slow map sync",
			args{
				test.MustDecodeCid(test.TestCid1),
				testSlowMapPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.MustDecodeCid(test.TestCid1),
				Status: api.TrackerStatusPinned,
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.args.tracker.(type) {
			case *maptracker.MapPinTracker:
				// the Track preps the internal map of the MapPinTracker; not required by the Stateless impl
				pin := api.Pin{
					Cid:                  test.MustDecodeCid(test.TestCid1),
					Allocations:          []peer.ID{},
					ReplicationFactorMin: -1,
					ReplicationFactorMax: -1,
				}
				if err := tt.args.tracker.Track(pin); err != nil {
					t.Errorf("PinTracker.Track() error = %v", err)
				}
				time.Sleep(1 * time.Second)
			}

			got, err := tt.args.tracker.Sync(tt.args.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("PinTracker.Sync() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			t.Logf("got: %+v\n", got)
			if got.Cid.String() != tt.want.Cid.String() {
				t.Errorf("PinTracker.Sync() = %v, want %v", got.Cid.String(), tt.want.Cid.String())
			}

			if got.Status != tt.want.Status {
				t.Errorf("PinTracker.Sync() = %v, want %v", got.Status, tt.want.Status)
			}
		})
	}
}

func TestPinTracker_RecoverAll(t *testing.T) {
	type args struct {
		tracker ipfscluster.PinTracker
		pin     api.Pin // only used by maptracker
	}
	tests := []struct {
		name    string
		args    args
		want    []api.PinInfo
		wantErr bool
	}{
		{
			"basic stateless recoverall",
			args{
				testStatelessPinTracker(t),
				api.Pin{},
			},
			[]api.PinInfo{
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid1),
					Status: api.TrackerStatusPinned,
				},
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid2),
					Status: api.TrackerStatusRemote,
				},
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid3),
					Status: api.TrackerStatusPinned,
				},
			},
			false,
		},
		{
			"basic map recoverall",
			args{
				testMapPinTracker(t),
				api.Pin{Cid: test.MustDecodeCid(test.TestCid1), ReplicationFactorMax: -1},
			},
			[]api.PinInfo{
				api.PinInfo{
					Cid:    test.MustDecodeCid(test.TestCid1),
					Status: api.TrackerStatusPinned,
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.args.tracker.(type) {
			case *maptracker.MapPinTracker:
				// the Track preps the internal map of the MapPinTracker; not required by the Stateless impl
				if err := tt.args.tracker.Track(tt.args.pin); err != nil {
					t.Errorf("PinTracker.Track() error = %v", err)
				}
				time.Sleep(1 * time.Second)
			}

			got, err := tt.args.tracker.RecoverAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("PinTracker.RecoverAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(got) != len(tt.want) {
				for _, pi := range got {
					t.Logf("pinfo: %v", pi)
				}
				t.Errorf("got len = %d, want = %d", len(got), len(tt.want))
				t.FailNow()
			}

			sortPinInfoByCid(got)
			sortPinInfoByCid(tt.want)

			for i := range tt.want {
				if got[i].Cid.String() != tt.want[i].Cid.String() {
					t.Errorf("\ngot: %v,\nwant: %v", got[i].Cid, tt.want[i].Cid)
				}

				if got[i].Status != tt.want[i].Status {
					t.Errorf("for cid: %v:\ngot: %v,\nwant: %v", tt.want[i].Cid, got[i].Status, tt.want[i].Status)
				}
			}
		})
	}
}

func TestPinTracker_Recover(t *testing.T) {
	type args struct {
		c       *cid.Cid
		tracker ipfscluster.PinTracker
	}
	tests := []struct {
		name    string
		args    args
		want    api.PinInfo
		wantErr bool
	}{
		{
			"basic stateless recover",
			args{
				test.MustDecodeCid(test.TestCid1),
				testStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.MustDecodeCid(test.TestCid1),
				Status: api.TrackerStatusPinned,
			},
			false,
		},
		{
			"basic map recover",
			args{
				test.MustDecodeCid(test.TestCid1),
				testMapPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.MustDecodeCid(test.TestCid1),
				Status: api.TrackerStatusPinned,
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.args.tracker.Recover(tt.args.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("PinTracker.Recover() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got.Cid.String() != tt.want.Cid.String() {
				t.Errorf("PinTracker.Recover() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUntrackTrack(t *testing.T) {
	type args struct {
		c       *cid.Cid
		tracker ipfscluster.PinTracker
	}
	tests := []struct {
		name    string
		args    args
		want    api.PinInfo
		wantErr bool
	}{
		{
			"basic stateless untrack track",
			args{
				test.MustDecodeCid(test.TestCid1),
				testStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.MustDecodeCid(test.TestCid1),
				Status: api.TrackerStatusPinned,
			},
			false,
		},
		{
			"basic map untrack track",
			args{
				test.MustDecodeCid(test.TestCid1),
				testMapPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.MustDecodeCid(test.TestCid1),
				Status: api.TrackerStatusPinned,
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.args.tracker.Track(api.PinCid(tt.args.c))
			if err != nil {
				t.Fatal(err)
			}

			time.Sleep(time.Second / 2)

			err = tt.args.tracker.Untrack(tt.args.c)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTrackUntrackWithCancel(t *testing.T) {
	type args struct {
		c       *cid.Cid
		tracker ipfscluster.PinTracker
	}
	tests := []struct {
		name    string
		args    args
		want    api.PinInfo
		wantErr bool
	}{
		{
			"slow stateless tracker untrack w/ cancel",
			args{
				test.MustDecodeCid(test.TestSlowCid1),
				testSlowStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.MustDecodeCid(test.TestSlowCid1),
				Status: api.TrackerStatusPinned,
			},
			false,
		},
		{
			"slow map tracker untrack w/ cancel",
			args{
				test.MustDecodeCid(test.TestSlowCid1),
				testSlowMapPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.MustDecodeCid(test.TestSlowCid1),
				Status: api.TrackerStatusPinned,
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := api.Pin{
				Cid:                  tt.args.c,
				ReplicationFactorMax: -1,
				ReplicationFactorMin: -1,
			}
			err := tt.args.tracker.Track(p)
			if err != nil {
				t.Fatal(err)
			}

			time.Sleep(100 * time.Millisecond) // let pinning start

			pInfo := tt.args.tracker.Status(tt.args.c)
			if pInfo.Status == api.TrackerStatusUnpinned {
				t.Fatal("slowPin should be tracked")
			}

			if pInfo.Status == api.TrackerStatusPinning {
				go func() {
					err = tt.args.tracker.Untrack(tt.args.c)
					if err != nil {
						t.Fatal(err)
					}
				}()
				var ctx context.Context
				switch trkr := tt.args.tracker.(type) {
				case *maptracker.MapPinTracker:
					ctx = trkr.OpContext(tt.args.c)
				case *stateless.Tracker:
					ctx = trkr.OpContext(tt.args.c)
				}
				select {
				case <-ctx.Done():
					return
				case <-time.Tick(100 * time.Millisecond):
					t.Errorf("operation context should have been cancelled by now")
				}
			} else {
				t.Error("slowPin should be pinning and is:", pInfo.Status)
			}
		})
	}
}
