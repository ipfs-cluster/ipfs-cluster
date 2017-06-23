package test

import (
	"errors"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	rpc "gx/ipfs/QmYqnvVzUjjVddWPLGMAErUjNBqnyjoeeCgZUZFsAJeGHr/go-libp2p-gorpc"
	peer "gx/ipfs/QmZcUPvPhD1Xvk6mwijYF8AfR3mG31S1YsEfHG4khrFPRr/go-libp2p-peer"
	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

// ErrBadCid is returned when using ErrorCid. Operations with that CID always
// fail.
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

func (mock *mockService) Pin(in api.PinSerial, out *struct{}) error {
	if in.Cid == ErrorCid {
		return ErrBadCid
	}
	return nil
}

func (mock *mockService) Unpin(in api.PinSerial, out *struct{}) error {
	if in.Cid == ErrorCid {
		return ErrBadCid
	}
	return nil
}

func (mock *mockService) Pins(in struct{}, out *[]api.PinSerial) error {
	*out = []api.PinSerial{
		{
			Cid: TestCid1,
		},
		{
			Cid: TestCid2,
		},
		{
			Cid: TestCid3,
		},
	}
	return nil
}

func (mock *mockService) PinGet(in api.PinSerial, out *api.PinSerial) error {
	if in.Cid == ErrorCid {
		return errors.New("expected error when using ErrorCid")
	}
	*out = in
	return nil
}

func (mock *mockService) ID(in struct{}, out *api.IDSerial) error {
	//_, pubkey, _ := crypto.GenerateKeyPair(
	//	DefaultConfigCrypto,
	//	DefaultConfigKeyLength)
	*out = api.IDSerial{
		ID: TestPeerID1.Pretty(),
		//PublicKey: pubkey,
		Version: "0.0.mock",
		IPFS: api.IPFSIDSerial{
			ID: TestPeerID1.Pretty(),
			Addresses: api.MultiaddrsSerial{
				api.MultiaddrSerial("/ip4/127.0.0.1/tcp/4001/ipfs/" + TestPeerID1.Pretty()),
			},
		},
	}
	return nil
}

func (mock *mockService) Version(in struct{}, out *api.Version) error {
	*out = api.Version{
		Version: "0.0.mock",
	}
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

func (mock *mockService) Status(in api.PinSerial, out *api.GlobalPinInfoSerial) error {
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

func (mock *mockService) Sync(in api.PinSerial, out *api.GlobalPinInfoSerial) error {
	return mock.Status(in, out)
}

func (mock *mockService) StateSync(in struct{}, out *[]api.PinInfoSerial) error {
	*out = make([]api.PinInfoSerial, 0, 0)
	return nil
}

func (mock *mockService) Recover(in api.PinSerial, out *api.GlobalPinInfoSerial) error {
	return mock.Status(in, out)
}

func (mock *mockService) Track(in api.PinSerial, out *struct{}) error {
	return nil
}

func (mock *mockService) Untrack(in api.PinSerial, out *struct{}) error {
	return nil
}

/* PeerManager methods */

func (mock *mockService) PeerManagerPeers(in struct{}, out *[]peer.ID) error {
	*out = []peer.ID{TestPeerID1, TestPeerID2, TestPeerID3}
	return nil
}

func (mock *mockService) PeerManagerAddPeer(in api.MultiaddrSerial, out *struct{}) error {
	return nil
}

func (mock *mockService) PeerManagerRmPeer(in peer.ID, out *struct{}) error {
	return nil
}

/* IPFSConnector methods */

func (mock *mockService) IPFSPin(in api.PinSerial, out *struct{}) error {
	return nil
}

func (mock *mockService) IPFSUnpin(in api.PinSerial, out *struct{}) error {
	return nil
}

func (mock *mockService) IPFSPinLsCid(in api.PinSerial, out *api.IPFSPinStatus) error {
	if in.Cid == TestCid1 || in.Cid == TestCid3 {
		*out = api.IPFSPinStatusRecursive
	} else {
		*out = api.IPFSPinStatusUnpinned
	}
	return nil
}

func (mock *mockService) IPFSPinLs(in string, out *map[string]api.IPFSPinStatus) error {
	m := map[string]api.IPFSPinStatus{
		TestCid1: api.IPFSPinStatusRecursive,
		TestCid3: api.IPFSPinStatusRecursive,
	}
	*out = m
	return nil
}

func (mock *mockService) IPFSConnectSwarms(in struct{}, out *struct{}) error {
	return nil
}

func (mock *mockService) IPFSConfigKey(in string, out *interface{}) error {
	switch in {
	case "Datastore/StorageMax":
		*out = "100KB"
	default:
		return errors.New("configuration key not found")
	}
	return nil
}

func (mock *mockService) IPFSRepoSize(in struct{}, out *int) error {
	// since we have two pins. Assume each is 1KB.
	*out = 2000
	return nil
}
