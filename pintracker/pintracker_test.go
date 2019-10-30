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
	"github.com/ipfs/ipfs-cluster/pintracker/stateless"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"
)

var (
	pinCancelCid      = test.Cid3
	unpinCancelCid    = test.Cid2
	ErrPinCancelCid   = errors.New("should not have received rpc.IPFSPin operation")
	ErrUnpinCancelCid = errors.New("should not have received rpc.IPFSUnpin operation")
	pinOpts           = api.PinOptions{
		ReplicationFactorMax: -1,
		ReplicationFactorMin: -1,
	}
)

type mockCluster struct{}
type mockIPFS struct{}

func mockRPCClient(t testing.TB) *rpc.Client {
	s := rpc.NewServer(nil, "mock")
	c := rpc.NewClientWithServer(nil, "mock", s)
	err := s.RegisterName("Cluster", &mockCluster{})
	if err != nil {
		t.Fatal(err)
	}
	err = s.RegisterName("IPFSConnector", &mockIPFS{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (mock *mockIPFS) Pin(ctx context.Context, in *api.Pin, out *struct{}) error {
	c := in.Cid
	switch c.String() {
	case test.SlowCid1.String():
		time.Sleep(3 * time.Second)
	case pinCancelCid.String():
		return ErrPinCancelCid
	}
	return nil
}

func (mock *mockIPFS) PinLsCid(ctx context.Context, in cid.Cid, out *api.IPFSPinStatus) error {
	switch in.String() {
	case test.Cid1.String(), test.Cid2.String():
		*out = api.IPFSPinStatusRecursive
	case test.Cid4.String():
		*out = api.IPFSPinStatusError
		return errors.New("an ipfs error")
	default:
		*out = api.IPFSPinStatusUnpinned
	}
	return nil
}

func (mock *mockIPFS) Unpin(ctx context.Context, in *api.Pin, out *struct{}) error {
	switch in.Cid.String() {
	case test.SlowCid1.String():
		time.Sleep(3 * time.Second)
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

func (mock *mockCluster) Pins(ctx context.Context, in struct{}, out *[]*api.Pin) error {
	*out = []*api.Pin{
		api.PinWithOpts(test.Cid1, pinOpts),
		api.PinWithOpts(test.Cid3, pinOpts),
	}
	return nil
}

func (mock *mockCluster) PinGet(ctx context.Context, in cid.Cid, out *api.Pin) error {
	switch in.String() {
	case test.ErrorCid.String():
		return errors.New("expected error when using ErrorCid")
	case test.Cid1.String(), test.Cid2.String():
		pin := api.PinWithOpts(in, pinOpts)
		*out = *pin
		return nil
	}
	pin := api.PinCid(in)
	*out = *pin
	return nil
}

var sortPinInfoByCid = func(p []*api.PinInfo) {
	sort.Slice(p, func(i, j int) bool {
		return p[i].Cid.String() < p[j].Cid.String()
	})
}

func testSlowStatelessPinTracker(t testing.TB) *stateless.Tracker {
	cfg := &stateless.Config{}
	cfg.Default()
	spt := stateless.New(cfg, test.PeerID1, test.PeerName1)
	spt.SetClient(mockRPCClient(t))
	return spt
}

func testStatelessPinTracker(t testing.TB) *stateless.Tracker {
	cfg := &stateless.Config{}
	cfg.Default()
	spt := stateless.New(cfg, test.PeerID1, test.PeerName1)
	spt.SetClient(test.NewMockRPCClient(t))
	return spt
}

func TestPinTracker_Track(t *testing.T) {
	type args struct {
		c       *api.Pin
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
				api.PinWithOpts(test.Cid1, pinOpts),
				testStatelessPinTracker(t),
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.args.tracker.Track(context.Background(), tt.args.c); (err != nil) != tt.wantErr {
				t.Errorf("PinTracker.Track() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func BenchmarkPinTracker_Track(b *testing.B) {
	type args struct {
		c       *api.Pin
		tracker ipfscluster.PinTracker
	}
	tests := []struct {
		name string
		args args
	}{
		{
			"basic stateless track",
			args{
				api.PinWithOpts(test.Cid1, pinOpts),
				testStatelessPinTracker(b),
			},
		},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := tt.args.tracker.Track(context.Background(), tt.args.c); err != nil {
					b.Errorf("PinTracker.Track() error = %v", err)
				}
			}
		})
	}
}

func TestPinTracker_Untrack(t *testing.T) {
	type args struct {
		c       cid.Cid
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
				test.Cid1,
				testStatelessPinTracker(t),
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.args.tracker.Untrack(context.Background(), tt.args.c); (err != nil) != tt.wantErr {
				t.Errorf("PinTracker.Untrack() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPinTracker_StatusAll(t *testing.T) {
	type args struct {
		c       *api.Pin
		tracker ipfscluster.PinTracker
	}
	tests := []struct {
		name string
		args args
		want []*api.PinInfo
	}{
		{
			"basic stateless statusall",
			args{
				api.PinWithOpts(test.Cid1, pinOpts),
				testStatelessPinTracker(t),
			},
			[]*api.PinInfo{
				{
					Cid:    test.Cid1,
					Status: api.TrackerStatusPinned,
				},
				{
					Cid:    test.Cid2,
					Status: api.TrackerStatusRemote,
				},
				{
					Cid:    test.Cid3,
					Status: api.TrackerStatusPinned,
				},
			},
		},
		{
			"slow stateless statusall",
			args{
				api.PinWithOpts(test.Cid1, pinOpts),
				testSlowStatelessPinTracker(t),
			},
			[]*api.PinInfo{
				{
					Cid:    test.Cid1,
					Status: api.TrackerStatusPinned,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.args.tracker.Track(context.Background(), tt.args.c); err != nil {
				t.Errorf("PinTracker.Track() error = %v", err)
			}
			time.Sleep(1 * time.Second)
			got := tt.args.tracker.StatusAll(context.Background())
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
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tt.args.tracker.StatusAll(context.Background())
			}
		})
	}
}

func TestPinTracker_Status(t *testing.T) {
	type args struct {
		c       cid.Cid
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
				test.Cid1,
				testStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.Cid1,
				Status: api.TrackerStatusPinned,
			},
		},
		{
			"basic stateless status/unpinned",
			args{
				test.Cid4,
				testStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.Cid4,
				Status: api.TrackerStatusUnpinned,
			},
		},
		{
			"slow stateless status",
			args{
				test.Cid1,
				testSlowStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.Cid1,
				Status: api.TrackerStatusPinned,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.args.tracker.Status(context.Background(), tt.args.c)

			if got.Cid.String() != tt.want.Cid.String() {
				t.Errorf("PinTracker.Status() = %v, want %v", got.Cid, tt.want.Cid)
			}

			if got.Status != tt.want.Status {
				t.Errorf("PinTracker.Status() = %v, want %v", got.Status, tt.want.Status)
			}
		})
	}
}

func TestPinTracker_RecoverAll(t *testing.T) {
	type args struct {
		tracker ipfscluster.PinTracker
		pin     *api.Pin // only used by maptracker
	}
	tests := []struct {
		name    string
		args    args
		want    []*api.PinInfo
		wantErr bool
	}{
		{
			"basic stateless recoverall",
			args{
				testStatelessPinTracker(t),
				&api.Pin{},
			},
			[]*api.PinInfo{
				{
					Cid:    test.Cid1,
					Status: api.TrackerStatusPinned,
				},
				{
					Cid:    test.Cid2,
					Status: api.TrackerStatusRemote,
				},
				{
					Cid:    test.Cid3,
					Status: api.TrackerStatusPinned,
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.args.tracker.RecoverAll(context.Background())
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
		c       cid.Cid
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
				test.Cid1,
				testStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.Cid1,
				Status: api.TrackerStatusPinned,
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.args.tracker.Recover(context.Background(), tt.args.c)
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
		c       cid.Cid
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
				test.Cid1,
				testStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.Cid1,
				Status: api.TrackerStatusPinned,
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.args.tracker.Track(context.Background(), api.PinWithOpts(tt.args.c, pinOpts))
			if err != nil {
				t.Fatal(err)
			}

			time.Sleep(time.Second / 2)

			err = tt.args.tracker.Untrack(context.Background(), tt.args.c)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTrackUntrackWithCancel(t *testing.T) {
	type args struct {
		c       cid.Cid
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
				test.SlowCid1,
				testSlowStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid:    test.SlowCid1,
				Status: api.TrackerStatusPinned,
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := api.PinWithOpts(tt.args.c, pinOpts)
			err := tt.args.tracker.Track(context.Background(), p)
			if err != nil {
				t.Fatal(err)
			}

			time.Sleep(100 * time.Millisecond) // let pinning start

			pInfo := tt.args.tracker.Status(context.Background(), tt.args.c)
			if pInfo.Status == api.TrackerStatusUnpinned {
				t.Fatal("slowPin should be tracked")
			}

			if pInfo.Status == api.TrackerStatusPinning {
				go func() {
					err = tt.args.tracker.Untrack(context.Background(), tt.args.c)
					if err != nil {
						t.Fatal(err)
					}
				}()
				var ctx context.Context
				switch trkr := tt.args.tracker.(type) {
				case *stateless.Tracker:
					ctx = trkr.OpContext(context.Background(), tt.args.c)
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

func TestPinTracker_RemoteIgnoresError(t *testing.T) {
	ctx := context.Background()
	testF := func(t *testing.T, pt ipfscluster.PinTracker) {
		remoteCid := test.Cid4

		remote := api.PinWithOpts(remoteCid, pinOpts)
		remote.Allocations = []peer.ID{test.PeerID2}
		remote.ReplicationFactorMin = 1
		remote.ReplicationFactorMax = 1

		err := pt.Track(ctx, remote)
		if err != nil {
			t.Fatal(err)
		}

		pi := pt.Status(ctx, remoteCid)
		if pi.Status != api.TrackerStatusRemote || pi.Error != "" {
			t.Error("Remote pin should not be in error")
		}
	}

	t.Run("stateless pintracker", func(t *testing.T) {
		pt := testStatelessPinTracker(t)
		testF(t, pt)
	})
}
