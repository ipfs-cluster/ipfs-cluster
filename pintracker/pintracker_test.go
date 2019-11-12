// Package pintracker_test tests the multiple implementations
// of the PinTracker interface.
package pintracker_test

import (
	"context"
	"sort"
	"testing"
	"time"

	ipfscluster "github.com/ipfs/ipfs-cluster"
	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/pintracker/stateless"
	"github.com/ipfs/ipfs-cluster/state"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-core/peer"
)

var (
	pinOpts = api.PinOptions{
		ReplicationFactorMax: -1,
		ReplicationFactorMin: -1,
	}
)

var sortPinInfoByCid = func(p []*api.PinInfo) {
	sort.Slice(p, func(i, j int) bool {
		return p[i].Cid.String() < p[j].Cid.String()
	})
}

type argsPin struct {
	c       *api.Pin
	tracker ipfscluster.PinTracker
	st      state.State
}

func newArgsPin(tb testing.TB, c *api.Pin, slow bool) argsPin {
	var tracker ipfscluster.PinTracker
	var st state.State
	args := argsPin{
		c: c,
	}

	if slow {
		tracker, st = stateless.SlowPinTracker(tb)
	} else {
		tracker, st = stateless.PinTracker(tb)
	}

	args.tracker = tracker
	args.st = st

	return args
}

type argsCid struct {
	c       cid.Cid
	tracker ipfscluster.PinTracker
	st      state.State
}

func newArgsCid(tb testing.TB, c cid.Cid, slow bool) argsCid {
	var tracker ipfscluster.PinTracker
	var st state.State
	args := argsCid{
		c: c,
	}

	if slow {
		tracker, st = stateless.SlowPinTracker(tb)
	} else {
		tracker, st = stateless.PinTracker(tb)
	}

	args.tracker = tracker
	args.st = st

	return args
}

func newStatelessPintracker(tb testing.TB, slow bool) *stateless.Tracker {
	if slow {
		tracker, _ := stateless.SlowPinTracker(tb)
		return tracker
	}
	tracker, _ := stateless.PinTracker(tb)
	return tracker
}

func TestPinTracker_Track(t *testing.T) {
	tests := []struct {
		name    string
		args    argsPin
		wantErr bool
	}{
		{
			"basic stateless track",
			newArgsPin(t, api.PinWithOpts(test.Cid1, pinOpts), false),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if err := tt.args.tracker.Track(ctx, tt.args.c); (err != nil) != tt.wantErr {
				t.Errorf("PinTracker.Track() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func BenchmarkPinTracker_Track(b *testing.B) {
	tests := []struct {
		name string
		args argsPin
	}{
		{
			"basic stateless track",
			newArgsPin(b, api.PinWithOpts(test.Cid1, pinOpts), false),
		},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx := context.Background()
				if err := tt.args.tracker.Track(ctx, tt.args.c); err != nil {
					b.Errorf("PinTracker.Track() error = %v", err)
				}
			}
		})
	}
}

func TestPinTracker_Untrack(t *testing.T) {
	tests := []struct {
		name    string
		args    argsCid
		wantErr bool
	}{
		{
			"basic stateless untrack",
			newArgsCid(t, test.Cid1, false),
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
	tests := []struct {
		name string
		args argsPin
		want []*api.PinInfo
	}{
		{
			"basic stateless statusall",
			newArgsPin(t, api.PinWithOpts(test.Cid1, pinOpts), false),
			[]*api.PinInfo{
				{
					Cid:    test.Cid1,
					Status: api.TrackerStatusPinned,
				},
			},
		},
		{
			"slow stateless statusall",
			newArgsPin(t, api.PinWithOpts(test.Cid1, pinOpts), true),
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
			ctx := context.Background()
			tt.args.st.Add(ctx, tt.args.c)
			if err := tt.args.tracker.Track(ctx, tt.args.c); err != nil {
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
				newStatelessPintracker(b, false),
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
	tests := []struct {
		name string
		args argsCid
		want api.PinInfo
	}{
		{
			"basic stateless status",
			newArgsCid(t, test.Cid1, false),
			api.PinInfo{
				Cid:    test.Cid1,
				Status: api.TrackerStatusPinned,
			},
		},
		{
			"basic stateless status/unpinned",
			newArgsCid(t, test.Cid4, false),
			api.PinInfo{
				Cid:    test.Cid4,
				Status: api.TrackerStatusUnpinned,
			},
		},
		{
			"slow stateless status",
			newArgsCid(t, test.Cid1, true),
			api.PinInfo{
				Cid:    test.Cid1,
				Status: api.TrackerStatusPinned,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pin := api.PinWithOpts(tt.args.c, pinOpts)
			switch tt.want.Status {
			case api.TrackerStatusRemote:
				pin.Allocations = []peer.ID{test.PeerID2}
				pin.ReplicationFactorMin = 1
				pin.ReplicationFactorMax = 1
				tt.args.st.Add(ctx, pin)
			case api.TrackerStatusPinned:
				tt.args.st.Add(ctx, pin)
				tt.args.tracker.Track(ctx, pin)
			default:
			}

			time.Sleep(1 * time.Second)
			got := tt.args.tracker.Status(ctx, tt.args.c)

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
		st      state.State
	}

	newArgs := func(slow bool) args {
		if slow {
			tracker, st := stateless.SlowPinTracker(t)
			return args{tracker, st}
		}
		tracker, st := stateless.PinTracker(t)
		return args{tracker, st}
	}

	tests := []struct {
		name    string
		args    args
		want    []*api.PinInfo
		wantErr bool
	}{
		{
			"basic stateless recoverall",
			newArgs(false),
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
			ctx := context.Background()
			for _, pinInfo := range tt.want {
				pin := api.PinWithOpts(pinInfo.Cid, pinOpts)
				if pinInfo.Status == api.TrackerStatusRemote {
					pin.Allocations = []peer.ID{test.PeerID2}
					pin.ReplicationFactorMin = 1
					pin.ReplicationFactorMax = 1
				}
				tt.args.st.Add(ctx, pin)
			}

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
	tests := []struct {
		name    string
		args    argsCid
		want    api.PinInfo
		wantErr bool
	}{
		{
			"basic stateless recover",
			newArgsCid(t, test.Cid1, false),
			api.PinInfo{
				Cid:    test.Cid1,
				Status: api.TrackerStatusPinned,
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.want.Status == api.TrackerStatusPinned {
				pin := api.PinWithOpts(tt.want.Cid, pinOpts)
				tt.args.st.Add(ctx, pin)
			}

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
	tests := []struct {
		name    string
		args    argsCid
		wantErr bool
	}{
		{
			"basic stateless untrack track",
			newArgsCid(t, test.Cid1, false),
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
	tests := []struct {
		name    string
		args    argsCid
		want    api.PinInfo
		wantErr bool
	}{
		{
			"slow stateless tracker untrack w/ cancel",
			newArgsCid(t, test.SlowCid1, true),
			api.PinInfo{
				Cid:    test.SlowCid1,
				Status: api.TrackerStatusPinned,
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			p := api.PinWithOpts(tt.args.c, pinOpts)
			if tt.want.Status != api.TrackerStatusUnpinned {
				tt.args.st.Add(ctx, p)
			}
			err := tt.args.tracker.Track(ctx, p)
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
	testF := func(t *testing.T, pt ipfscluster.PinTracker, st state.State) {
		remoteCid := test.Cid4

		remote := api.PinWithOpts(remoteCid, pinOpts)
		remote.Allocations = []peer.ID{test.PeerID2}
		remote.ReplicationFactorMin = 1
		remote.ReplicationFactorMax = 1

		st.Add(ctx, remote)
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
		pt, st := stateless.SlowPinTracker(t)
		testF(t, pt, st)
	})
}
