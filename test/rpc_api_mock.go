package test

import (
	"errors"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
)

var ErrBadCid = errors.New("this is an expected error when using ErrorCid")

type mockService struct{}

// NewMockRPCClient creates a mock ipfs-cluster RPC server and returns
// a client to it.
func NewMockRPCClient(t *testing.T) *rpc.Client {
	s := rpc.NewServer(nil, "mock")
	c := rpc.NewClientWithServer(nil, "mock", s)
	err := s.RegisterName("Cluster", &mockService{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (mock *mockService) Pin(in api.CidArgSerial, out *struct{}) error {
	if in.Cid == ErrorCid {
		return ErrBadCid
	}
	return nil
}

func (mock *mockService) Unpin(in api.CidArgSerial, out *struct{}) error {
	if in.Cid == ErrorCid {
		return ErrBadCid
	}
	return nil
}

func (mock *mockService) PinList(in struct{}, out *[]string) error {
	*out = []string{TestCid1, TestCid2, TestCid3}
	return nil
}

func (mock *mockService) ID(in struct{}, out *api.IDSerial) error {
	//_, pubkey, _ := crypto.GenerateKeyPair(
	//	DefaultConfigCrypto,
	//	DefaultConfigKeyLength)
	*out = api.ID{
		ID: TestPeerID1,
		//PublicKey: pubkey,
		Version: "0.0.mock",
		IPFS: api.IPFSID{
			ID: TestPeerID1,
		},
	}.ToSerial()
	return nil
}

func (mock *mockService) Version(in struct{}, out *api.Version) error {
	*out = api.Version{"0.0.mock"}
	return nil
}

func (mock *mockService) Peers(in struct{}, out *[]api.IDSerial) error {
	id := api.IDSerial{}
	mock.ID(in, &id)

	*out = []api.IDSerial{id}
	return nil
}

func (mock *mockService) PeerAdd(in api.MultiaddrSerial, out *api.IDSerial) error {
	id := api.IDSerial{}
	mock.ID(struct{}{}, &id)
	*out = id
	return nil
}

func (mock *mockService) PeerRemove(in peer.ID, out *struct{}) error {
	return nil
}

// FIXME: dup from util.go
func globalPinInfoSliceToSerial(gpi []api.GlobalPinInfo) []api.GlobalPinInfoSerial {
	gpis := make([]api.GlobalPinInfoSerial, len(gpi), len(gpi))
	for i, v := range gpi {
		gpis[i] = v.ToSerial()
	}
	return gpis
}

func (mock *mockService) StatusAll(in struct{}, out *[]api.GlobalPinInfoSerial) error {
	c1, _ := cid.Decode(TestCid1)
	c2, _ := cid.Decode(TestCid2)
	c3, _ := cid.Decode(TestCid3)
	*out = globalPinInfoSliceToSerial([]api.GlobalPinInfo{
		{
			Cid: c1,
			PeerMap: map[peer.ID]api.PinInfo{
				TestPeerID1: {
					Cid:    c1,
					Peer:   TestPeerID1,
					Status: api.TrackerStatusPinned,
					TS:     time.Now(),
				},
			},
		},
		{
			Cid: c2,
			PeerMap: map[peer.ID]api.PinInfo{
				TestPeerID1: {
					Cid:    c2,
					Peer:   TestPeerID1,
					Status: api.TrackerStatusPinning,
					TS:     time.Now(),
				},
			},
		},
		{
			Cid: c3,
			PeerMap: map[peer.ID]api.PinInfo{
				TestPeerID1: {
					Cid:    c3,
					Peer:   TestPeerID1,
					Status: api.TrackerStatusPinError,
					TS:     time.Now(),
				},
			},
		},
	})
	return nil
}

func (mock *mockService) Status(in api.CidArgSerial, out *api.GlobalPinInfoSerial) error {
	if in.Cid == ErrorCid {
		return ErrBadCid
	}
	c1, _ := cid.Decode(TestCid1)
	*out = api.GlobalPinInfo{
		Cid: c1,
		PeerMap: map[peer.ID]api.PinInfo{
			TestPeerID1: {
				Cid:    c1,
				Peer:   TestPeerID1,
				Status: api.TrackerStatusPinned,
				TS:     time.Now(),
			},
		},
	}.ToSerial()
	return nil
}

func (mock *mockService) SyncAll(in struct{}, out *[]api.GlobalPinInfoSerial) error {
	return mock.StatusAll(in, out)
}

func (mock *mockService) Sync(in api.CidArgSerial, out *api.GlobalPinInfoSerial) error {
	return mock.Status(in, out)
}

func (mock *mockService) StateSync(in struct{}, out *[]api.PinInfoSerial) error {
	*out = make([]api.PinInfoSerial, 0, 0)
	return nil
}

func (mock *mockService) Recover(in api.CidArgSerial, out *api.GlobalPinInfoSerial) error {
	return mock.Status(in, out)
}

func (mock *mockService) Track(in api.CidArgSerial, out *struct{}) error {
	return nil
}

func (mock *mockService) Untrack(in api.CidArgSerial, out *struct{}) error {
	return nil
}
