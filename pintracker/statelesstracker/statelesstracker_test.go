package stateless

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/optracker"
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

func testSlowStatelessPinTracker(t *testing.T) *Tracker {
	cfg := &Config{}
	cfg.Default()
	mpt := New(cfg, test.TestPeerID1)
	mpt.SetClient(mockRPCClient(t))
	return mpt
}

func testStatelessPinTracker(t *testing.T) *Tracker {
	cfg := &Config{}
	cfg.Default()
	spt := New(cfg, test.TestPeerID1)
	spt.SetClient(test.NewMockRPCClient(t))
	return spt
}

func TestStatelessPinTracker_New(t *testing.T) {
	spt := testStatelessPinTracker(t)
	defer spt.Shutdown()
}

func TestStatelessPinTracker_Shutdown(t *testing.T) {
	spt := testStatelessPinTracker(t)
	err := spt.Shutdown()
	if err != nil {
		t.Fatal(err)
	}
	err = spt.Shutdown()
	if err != nil {
		t.Fatal(err)
	}
}

func TestStatelessPinTracker_Track(t *testing.T) {
	type args struct {
		c api.Pin
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			"basic track",
			args{
				api.Pin{
					Cid:                  test.MustDecodeCid(test.TestCid1),
					Allocations:          []peer.ID{},
					ReplicationFactorMin: -1,
					ReplicationFactorMax: -1,
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStatelessPinTracker(t)
			if err := s.Track(tt.args.c); (err != nil) != tt.wantErr {
				t.Errorf("StatelessPinTracker.Track() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStatelessPinTracker_Untrack(t *testing.T) {
	type args struct {
		c *cid.Cid
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			"basic untrack",
			args{
				test.MustDecodeCid(test.TestCid1),
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStatelessPinTracker(t)
			if err := s.Untrack(tt.args.c); (err != nil) != tt.wantErr {
				t.Errorf("StatelessPinTracker.Untrack() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStatelessPinTracker_StatusAll(t *testing.T) {
	tests := []struct {
		name string
		want []api.PinInfo
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStatelessPinTracker(t)
			if got := s.StatusAll(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StatelessPinTracker.StatusAll() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatelessPinTracker_Status(t *testing.T) {
	type args struct {
		c *cid.Cid
	}
	tests := []struct {
		name string
		args args
		want api.PinInfo
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStatelessPinTracker(t)
			if got := s.Status(tt.args.c); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StatelessPinTracker.Status() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatelessPinTracker_SyncAll(t *testing.T) {
	tests := []struct {
		name    string
		want    []api.PinInfo
		wantErr bool
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStatelessPinTracker(t)
			got, err := s.SyncAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("StatelessPinTracker.SyncAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StatelessPinTracker.SyncAll() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatelessPinTracker_Sync(t *testing.T) {
	type args struct {
		c *cid.Cid
	}
	tests := []struct {
		name    string
		args    args
		want    api.PinInfo
		wantErr bool
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStatelessPinTracker(t)
			got, err := s.Sync(tt.args.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("StatelessPinTracker.Sync() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StatelessPinTracker.Sync() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatelessPinTracker_RecoverAll(t *testing.T) {
	tests := []struct {
		name    string
		want    []api.PinInfo
		wantErr bool
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStatelessPinTracker(t)
			got, err := s.RecoverAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("StatelessPinTracker.RecoverAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StatelessPinTracker.RecoverAll() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatelessPinTracker_Recover(t *testing.T) {
	type args struct {
		c *cid.Cid
	}
	tests := []struct {
		name    string
		args    args
		want    api.PinInfo
		wantErr bool
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStatelessPinTracker(t)
			got, err := s.Recover(tt.args.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("StatelessPinTracker.Recover() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StatelessPinTracker.Recover() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUntrackTrack(t *testing.T) {
	spt := testStatelessPinTracker(t)
	defer spt.Shutdown()

	h1 := test.MustDecodeCid(test.TestCid1)

	// LocalPin
	c := api.Pin{
		Cid:                  h1,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := spt.Track(c)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	err = spt.Untrack(h1)
	if err != nil {
		t.Fatal(err)
	}
}

func TestTrackUntrackWithCancel(t *testing.T) {
	spt := testSlowStatelessPinTracker(t)
	defer spt.Shutdown()

	slowPinCid := test.MustDecodeCid(test.TestSlowCid1)

	// LocalPin
	slowPin := api.Pin{
		Cid:                  slowPinCid,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := spt.Track(slowPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond) // let pinning start

	opc, ok := spt.optracker.Get(slowPin.Cid)
	if !ok {
		t.Fatal("slowPin should be tracked")
	}

	if opc.Phase == optracker.PhaseInProgress && opc.Op == optracker.OperationPin {
		go func() {
			err = spt.Untrack(slowPinCid)
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
	spt := testSlowStatelessPinTracker(t)
	defer spt.Shutdown()

	slowPinCid := test.MustDecodeCid(test.TestSlowCid1)
	fastPinCid := test.MustDecodeCid(pinCancelCid)

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

	err := spt.Track(slowPin)
	if err != nil {
		t.Fatal(err)
	}

	err = spt.Track(fastPin)
	if err != nil {
		t.Fatal(err)
	}

	// fastPin should be queued because slow pin is pinning
	opc, _ := spt.optracker.Get(fastPin.Cid)
	if opc.Phase == optracker.PhaseQueued && opc.Op == optracker.OperationPin {
		err = spt.Untrack(fastPinCid)
		if err != nil {
			t.Fatal(err)
		}
		// pi := spt.get(fastPinCid)
		// if pi.Error == ErrPinCancelCid.Error() {
		// 	t.Fatal(ErrPinCancelCid)
		// }
	} else {
		t.Error("fastPin should be queued to pin")
	}

	_, ok := spt.optracker.Get(fastPin.Cid)
	if ok {
		t.Error("fastPin should have been removed from tracker")
	}
}

func TestUntrackTrackWithCancel(t *testing.T) {
	spt := testSlowStatelessPinTracker(t)
	defer spt.Shutdown()

	slowPinCid := test.MustDecodeCid(test.TestSlowCid1)

	// LocalPin
	slowPin := api.Pin{
		Cid:                  slowPinCid,
		Allocations:          []peer.ID{},
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	err := spt.Track(slowPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second / 2)

	// Untrack should cancel the ongoing request
	// and unpin right away
	err = spt.Untrack(slowPinCid)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	opc, ok := spt.optracker.Get(slowPin.Cid)
	if !ok {
		t.Fatal("expected slowPin to be tracked")
	}

	if opc.Phase == optracker.PhaseInProgress && opc.Op == optracker.OperationUnpin {
		go func() {
			err = spt.Track(slowPin)
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
	spt := testStatelessPinTracker(t)
	defer spt.Shutdown()

	slowPinCid := test.MustDecodeCid(test.TestSlowCid1)
	fastPinCid := test.MustDecodeCid(unpinCancelCid)

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

	err := spt.Track(slowPin)
	if err != nil {
		t.Fatal(err)
	}

	err = spt.Track(fastPin)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)

	err = spt.Untrack(slowPin.Cid)
	if err != nil {
		t.Fatal(err)
	}

	err = spt.Untrack(fastPin.Cid)
	if err != nil {
		t.Fatal(err)
	}

	opc, ok := spt.optracker.Get(fastPin.Cid)
	if !ok {
		t.Fatal("c untrack operation should be tracked")
	}

	if opc.Phase == optracker.PhaseQueued && opc.Op == optracker.OperationUnpin {
		err = spt.Track(fastPin)
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
