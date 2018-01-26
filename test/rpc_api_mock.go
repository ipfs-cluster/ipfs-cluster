package test

import (
	"errors"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	cid "github.com/ipfs/go-cid"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
)

// ErrBadCid is returned when using ErrorCid. Operations with that CID always
// fail.
var ErrBadCid = errors.New("this is an expected error when using ErrorCid")

type mockService struct{}

// NewMockRPCClient creates a mock ipfs-cluster RPC server and returns
// a client to it.
func NewMockRPCClient(t *testing.T) *rpc.Client {
	return NewMockRPCClientWithHost(t, nil)
}

// NewMockRPCClientWithHost returns a mock ipfs-cluster RPC server
// initialized with a given host.
func NewMockRPCClientWithHost(t *testing.T, h host.Host) *rpc.Client {
	s := rpc.NewServer(h, "mock")
	c := rpc.NewClientWithServer(h, "mock", s)
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

func (mock *mockService) ConnectGraph(in struct{}, out *api.ConnectGraphSerial) error {
	*out = api.ConnectGraphSerial{
		ClusterID: TestPeerID1.Pretty(),
		IPFSLinks: map[string][]string{
			TestPeerID4.Pretty(): []string{TestPeerID5.Pretty(), TestPeerID6.Pretty()},
			TestPeerID5.Pretty(): []string{TestPeerID4.Pretty(), TestPeerID6.Pretty()},
			TestPeerID6.Pretty(): []string{TestPeerID4.Pretty(), TestPeerID5.Pretty()},
		},
		ClusterLinks: map[string][]string{
			TestPeerID1.Pretty(): []string{TestPeerID2.Pretty(), TestPeerID3.Pretty()},
			TestPeerID2.Pretty(): []string{TestPeerID1.Pretty(), TestPeerID3.Pretty()},
			TestPeerID3.Pretty(): []string{TestPeerID1.Pretty(), TestPeerID2.Pretty()},
		},
		ClustertoIPFS: map[string]string{
			TestPeerID1.Pretty(): TestPeerID4.Pretty(),
			TestPeerID2.Pretty(): TestPeerID5.Pretty(),
			TestPeerID3.Pretty(): TestPeerID6.Pretty(),
		},
	}
	return nil
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

func (mock *mockService) StatusAllLocal(in struct{}, out *[]api.PinInfoSerial) error {
	return mock.TrackerStatusAll(in, out)
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

func (mock *mockService) StatusLocal(in api.PinSerial, out *api.PinInfoSerial) error {
	return mock.TrackerStatus(in, out)
}

func (mock *mockService) SyncAll(in struct{}, out *[]api.GlobalPinInfoSerial) error {
	return mock.StatusAll(in, out)
}

func (mock *mockService) SyncAllLocal(in struct{}, out *[]api.PinInfoSerial) error {
	return mock.StatusAllLocal(in, out)
}

func (mock *mockService) Sync(in api.PinSerial, out *api.GlobalPinInfoSerial) error {
	return mock.Status(in, out)
}

func (mock *mockService) SyncLocal(in api.PinSerial, out *api.PinInfoSerial) error {
	return mock.StatusLocal(in, out)
}

func (mock *mockService) StateSync(in struct{}, out *[]api.PinInfoSerial) error {
	*out = make([]api.PinInfoSerial, 0, 0)
	return nil
}

func (mock *mockService) RecoverAllLocal(in struct{}, out *[]api.PinInfoSerial) error {
	return mock.TrackerRecoverAll(in, out)
}

func (mock *mockService) Recover(in api.PinSerial, out *api.GlobalPinInfoSerial) error {
	return mock.Status(in, out)
}

func (mock *mockService) RecoverLocal(in api.PinSerial, out *api.PinInfoSerial) error {
	return mock.TrackerRecover(in, out)
}

/* Tracker methods */

func (mock *mockService) Track(in api.PinSerial, out *struct{}) error {
	return nil
}

func (mock *mockService) Untrack(in api.PinSerial, out *struct{}) error {
	return nil
}

func (mock *mockService) TrackerStatusAll(in struct{}, out *[]api.PinInfoSerial) error {
	c1, _ := cid.Decode(TestCid1)
	c3, _ := cid.Decode(TestCid3)

	*out = pinInfoSliceToSerial([]api.PinInfo{
		{
			Cid:    c1,
			Peer:   TestPeerID1,
			Status: api.TrackerStatusPinned,
			TS:     time.Now(),
		},
		{
			Cid:    c3,
			Peer:   TestPeerID1,
			Status: api.TrackerStatusPinError,
			TS:     time.Now(),
		},
	})
	return nil
}

func (mock *mockService) TrackerStatus(in api.PinSerial, out *api.PinInfoSerial) error {
	if in.Cid == ErrorCid {
		return ErrBadCid
	}
	c1, _ := cid.Decode(TestCid1)

	*out = api.PinInfo{
		Cid:    c1,
		Peer:   TestPeerID2,
		Status: api.TrackerStatusPinned,
		TS:     time.Now(),
	}.ToSerial()
	return nil
}

func (mock *mockService) TrackerRecoverAll(in struct{}, out *[]api.PinInfoSerial) error {
	*out = make([]api.PinInfoSerial, 0, 0)
	return nil
}

func (mock *mockService) TrackerRecover(in api.PinSerial, out *api.PinInfoSerial) error {
	in2 := in.ToPin()
	*out = api.PinInfo{
		Cid:    in2.Cid,
		Peer:   TestPeerID1,
		Status: api.TrackerStatusPinned,
		TS:     time.Now(),
	}.ToSerial()
	return nil
}

/* PeerManager methods */

func (mock *mockService) PeerManagerAddPeer(in api.MultiaddrSerial, out *struct{}) error {
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

func (mock *mockService) IPFSSwarmPeers(in struct{}, out *api.SwarmPeersSerial) error {
	*out = []string{TestPeerID2.Pretty(), TestPeerID3.Pretty()}
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

func (mock *mockService) IPFSRepoSize(in struct{}, out *uint64) error {
	// since we have two pins. Assume each is 1KB.
	*out = 2000
	return nil
}

func (mock *mockService) IPFSFreeSpace(in struct{}, out *uint64) error {
	// RepoSize is 2KB, StorageMax is 100KB
	*out = 98000
	return nil
}

func (mock *mockService) ConsensusAddPeer(in peer.ID, out *struct{}) error {
	return errors.New("mock rpc cannot redirect")
}

func (mock *mockService) ConsensusRmPeer(in peer.ID, out *struct{}) error {
	return errors.New("mock rpc cannot redirect")
}

func (mock *mockService) ConsensusPeers(in struct{}, out *[]peer.ID) error {
	*out = []peer.ID{TestPeerID1, TestPeerID2, TestPeerID3}
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

// FIXME: dup from util.go
func pinInfoSliceToSerial(pi []api.PinInfo) []api.PinInfoSerial {
	pis := make([]api.PinInfoSerial, len(pi), len(pi))
	for i, v := range pi {
		pis[i] = v.ToSerial()
	}
	return pis
}
