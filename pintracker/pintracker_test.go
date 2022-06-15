// Package pintracker_test tests the multiple implementations
// of the PinTracker interface.
//
// These tests are legacy from the time when there were several
// pintracker implementations.
package pintracker_test

import (
	"context"
	"sort"
	"testing"
	"time"

	ipfscluster "github.com/ipfs-cluster/ipfs-cluster"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/inmem"
	"github.com/ipfs-cluster/ipfs-cluster/pintracker/stateless"
	"github.com/ipfs-cluster/ipfs-cluster/state"
	"github.com/ipfs-cluster/ipfs-cluster/state/dsstate"
	"github.com/ipfs-cluster/ipfs-cluster/test"

	peer "github.com/libp2p/go-libp2p-core/peer"
)

var (
	pinOpts = api.PinOptions{
		ReplicationFactorMax: -1,
		ReplicationFactorMin: -1,
	}
)

var sortPinInfoByCid = func(p []api.PinInfo) {
	sort.Slice(p, func(i, j int) bool {
		return p[i].Cid.String() < p[j].Cid.String()
	})
}

// prefilledState return a state instance with some pins:
// - Cid1 - pin everywhere
// - Cid2 - weird / remote // replication factor set to 0, no allocations
// - Cid3 - remote - this pin is on ipfs
// - Cid4 - pin everywhere - this pin is not on ipfs
func prefilledState(ctx context.Context) (state.ReadOnly, error) {
	st, err := dsstate.New(ctx, inmem.New(), "", dsstate.DefaultHandle())
	if err != nil {
		return nil, err
	}

	remote := api.PinWithOpts(test.Cid3, api.PinOptions{
		ReplicationFactorMax: 1,
		ReplicationFactorMin: 1,
	})
	remote.Allocations = []peer.ID{test.PeerID2}

	pins := []api.Pin{
		api.PinWithOpts(test.Cid1, pinOpts),
		api.PinCid(test.Cid2),
		remote,
		api.PinWithOpts(test.Cid4, pinOpts),
	}

	for _, pin := range pins {
		err = st.Add(ctx, pin)
		if err != nil {
			return nil, err
		}
	}
	return st, nil
}

func testStatelessPinTracker(t testing.TB) *stateless.Tracker {
	t.Helper()

	cfg := &stateless.Config{}
	cfg.Default()
	spt := stateless.New(cfg, test.PeerID1, test.PeerName1, prefilledState)
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
		c       api.Cid
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

func collectPinInfos(t *testing.T, out chan api.PinInfo) []api.PinInfo {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var pis []api.PinInfo
	for {
		select {
		case <-ctx.Done():
			t.Error("took too long")
			return nil
		case pi, ok := <-out:
			if !ok {
				return pis
			}
			pis = append(pis, pi)
		}
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
				api.PinWithOpts(test.Cid1, pinOpts),
				testStatelessPinTracker(t),
			},
			[]api.PinInfo{
				{
					Cid: test.Cid1,
					PinInfoShort: api.PinInfoShort{
						Status: api.TrackerStatusPinned,
					},
				},
				{
					Cid: test.Cid2,
					PinInfoShort: api.PinInfoShort{
						Status: api.TrackerStatusRemote,
					},
				},
				{
					Cid: test.Cid3,
					PinInfoShort: api.PinInfoShort{
						Status: api.TrackerStatusRemote,
					},
				},
				{
					// in state but not on IPFS
					Cid: test.Cid4,
					PinInfoShort: api.PinInfoShort{
						Status: api.TrackerStatusUnexpectedlyUnpinned,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.args.tracker.Track(context.Background(), tt.args.c); err != nil {
				t.Errorf("PinTracker.Track() error = %v", err)
			}
			time.Sleep(200 * time.Millisecond)
			infos := make(chan api.PinInfo)
			go func() {
				err := tt.args.tracker.StatusAll(context.Background(), api.TrackerStatusUndefined, infos)
				if err != nil {
					t.Error()
				}
			}()

			got := collectPinInfos(t, infos)

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
				if got[i].Cid != tt.want[i].Cid {
					t.Errorf("got: %v\nwant: %v", got, tt.want)
				}
				if got[i].Status != tt.want[i].Status {
					t.Errorf("for cid %v:\n got: %s\nwant: %s", got[i].Cid, got[i].Status, tt.want[i].Status)
				}
			}
		})
	}
}

func TestPinTracker_Status(t *testing.T) {
	type args struct {
		c       api.Cid
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
				Cid: test.Cid1,
				PinInfoShort: api.PinInfoShort{
					Status: api.TrackerStatusPinned,
				},
			},
		},
		{
			"basic stateless status/unpinned",
			args{
				test.Cid5,
				testStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid: test.Cid5,
				PinInfoShort: api.PinInfoShort{
					Status: api.TrackerStatusUnpinned,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.args.tracker.Status(context.Background(), tt.args.c)

			if got.Cid != tt.want.Cid {
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
			},
			// The only CID to recover is test.Cid4 which is in error.
			[]api.PinInfo{
				{
					// This will recover and status
					// is ignored as it could come back as
					// queued, pinning or error.

					Cid: test.Cid4,
					PinInfoShort: api.PinInfoShort{
						Status: api.TrackerStatusPinError,
					},
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			infos := make(chan api.PinInfo)
			go func() {
				err := tt.args.tracker.RecoverAll(context.Background(), infos)
				if (err != nil) != tt.wantErr {
					t.Errorf("PinTracker.RecoverAll() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
			}()

			got := collectPinInfos(t, infos)

			if len(got) != len(tt.want) {
				for _, pi := range got {
					t.Logf("pinfo: %v", pi)
				}
				t.Fatalf("got len = %d, want = %d", len(got), len(tt.want))
			}

			sortPinInfoByCid(got)
			sortPinInfoByCid(tt.want)

			for i := range tt.want {
				if got[i].Cid != tt.want[i].Cid {
					t.Errorf("\ngot: %v,\nwant: %v", got[i].Cid, tt.want[i].Cid)
				}

				// Cid4 needs to be recovered, we do not care
				// on what status it finds itself.
				if got[i].Cid == test.Cid4 {
					continue
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
		c       api.Cid
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
				Cid: test.Cid1,
				PinInfoShort: api.PinInfoShort{
					Status: api.TrackerStatusPinned,
				},
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

			if got.Cid != tt.want.Cid {
				t.Errorf("PinTracker.Recover() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUntrackTrack(t *testing.T) {
	type args struct {
		c       api.Cid
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
				Cid: test.Cid1,
				PinInfoShort: api.PinInfoShort{
					Status: api.TrackerStatusPinned,
				},
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

			time.Sleep(200 * time.Millisecond)

			err = tt.args.tracker.Untrack(context.Background(), tt.args.c)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTrackUntrackWithCancel(t *testing.T) {
	type args struct {
		c       api.Cid
		tracker ipfscluster.PinTracker
	}
	tests := []struct {
		name    string
		args    args
		want    api.PinInfo
		wantErr bool
	}{
		{
			"stateless tracker untrack w/ cancel",
			args{
				test.SlowCid1,
				testStatelessPinTracker(t),
			},
			api.PinInfo{
				Cid: test.SlowCid1,
				PinInfoShort: api.PinInfoShort{
					Status: api.TrackerStatusPinned,
				},
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

			time.Sleep(200 * time.Millisecond) // let pinning start

			pInfo := tt.args.tracker.Status(context.Background(), tt.args.c)
			if pInfo.Status == api.TrackerStatusUnpinned {
				t.Fatal("slowPin should be tracked")
			}

			if pInfo.Status == api.TrackerStatusPinning {
				go func() {
					err = tt.args.tracker.Untrack(context.Background(), tt.args.c)
					if err != nil {
						t.Error()
						return
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
				case <-time.Tick(150 * time.Millisecond):
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
		remoteCid := test.Cid3

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
			t.Error("Remote pin should not be in error", pi.Status, pi.Error)
		}
	}

	t.Run("stateless pintracker", func(t *testing.T) {
		pt := testStatelessPinTracker(t)
		testF(t, pt)
	})
}
