package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"

	cid "github.com/ipfs/go-cid"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
)

// ErrBadCid is returned when using ErrorCid. Operations with that CID always
// fail.
var ErrBadCid = errors.New("this is an expected error when using ErrorCid")

type mockService struct{}

// NewMockRPCClient creates a mock ipfs-cluster RPC server and returns
// a client to it.
func NewMockRPCClient(t testing.TB) *rpc.Client {
	return NewMockRPCClientWithHost(t, nil)
}

// NewMockRPCClientWithHost returns a mock ipfs-cluster RPC server
// initialized with a given host.
func NewMockRPCClientWithHost(t testing.TB, h host.Host) *rpc.Client {
	s := rpc.NewServer(h, "mock")
	c := rpc.NewClientWithServer(h, "mock", s)
	err := s.RegisterName("Cluster", &mockService{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (mock *mockService) Pin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	if in.Cid == ErrorCid {
		return ErrBadCid
	}
	return nil
}

func (mock *mockService) Unpin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	if in.Cid == ErrorCid {
		return ErrBadCid
	}
	return nil
}

func (mock *mockService) Pins(ctx context.Context, in struct{}, out *[]api.PinSerial) error {
	opts := api.PinOptions{
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	*out = []api.PinSerial{
		api.PinWithOpts(MustDecodeCid(TestCid1), opts).ToSerial(),
		api.PinCid(MustDecodeCid(TestCid2)).ToSerial(),
		api.PinWithOpts(MustDecodeCid(TestCid3), opts).ToSerial(),
	}
	return nil
}

func (mock *mockService) PinGet(ctx context.Context, in api.PinSerial, out *api.PinSerial) error {
	switch in.Cid {
	case ErrorCid:
		return errors.New("expected error when using ErrorCid")
	case TestCid1, TestCid3:
		p := api.PinCid(MustDecodeCid(in.Cid)).ToSerial()
		p.ReplicationFactorMin = -1
		p.ReplicationFactorMax = -1
		*out = p
		return nil
	case TestCid2: // This is a remote pin
		p := api.PinCid(MustDecodeCid(in.Cid)).ToSerial()
		p.ReplicationFactorMin = 1
		p.ReplicationFactorMax = 1
		*out = p
	default:
		return errors.New("not found")
	}
	return nil
}

func (mock *mockService) ID(ctx context.Context, in struct{}, out *api.IDSerial) error {
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

func (mock *mockService) Version(ctx context.Context, in struct{}, out *api.Version) error {
	*out = api.Version{
		Version: "0.0.mock",
	}
	return nil
}

func (mock *mockService) Peers(ctx context.Context, in struct{}, out *[]api.IDSerial) error {
	id := api.IDSerial{}
	mock.ID(ctx, in, &id)

	*out = []api.IDSerial{id}
	return nil
}

func (mock *mockService) PeerAdd(ctx context.Context, in string, out *api.IDSerial) error {
	id := api.IDSerial{}
	mock.ID(ctx, struct{}{}, &id)
	*out = id
	return nil
}

func (mock *mockService) PeerRemove(ctx context.Context, in peer.ID, out *struct{}) error {
	return nil
}

func (mock *mockService) ConnectGraph(ctx context.Context, in struct{}, out *api.ConnectGraphSerial) error {
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

func (mock *mockService) StatusAll(ctx context.Context, in struct{}, out *[]api.GlobalPinInfoSerial) error {
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

func (mock *mockService) StatusAllLocal(ctx context.Context, in struct{}, out *[]api.PinInfoSerial) error {
	return mock.TrackerStatusAll(ctx, in, out)
}

func (mock *mockService) Status(ctx context.Context, in api.PinSerial, out *api.GlobalPinInfoSerial) error {
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

func (mock *mockService) StatusLocal(ctx context.Context, in api.PinSerial, out *api.PinInfoSerial) error {
	return mock.TrackerStatus(ctx, in, out)
}

func (mock *mockService) SyncAll(ctx context.Context, in struct{}, out *[]api.GlobalPinInfoSerial) error {
	return mock.StatusAll(ctx, in, out)
}

func (mock *mockService) SyncAllLocal(ctx context.Context, in struct{}, out *[]api.PinInfoSerial) error {
	return mock.StatusAllLocal(ctx, in, out)
}

func (mock *mockService) Sync(ctx context.Context, in api.PinSerial, out *api.GlobalPinInfoSerial) error {
	return mock.Status(ctx, in, out)
}

func (mock *mockService) SyncLocal(ctx context.Context, in api.PinSerial, out *api.PinInfoSerial) error {
	return mock.StatusLocal(ctx, in, out)
}

func (mock *mockService) RecoverAllLocal(ctx context.Context, in struct{}, out *[]api.PinInfoSerial) error {
	return mock.TrackerRecoverAll(ctx, in, out)
}

func (mock *mockService) Recover(ctx context.Context, in api.PinSerial, out *api.GlobalPinInfoSerial) error {
	return mock.Status(ctx, in, out)
}

func (mock *mockService) RecoverLocal(ctx context.Context, in api.PinSerial, out *api.PinInfoSerial) error {
	return mock.TrackerRecover(ctx, in, out)
}

func (mock *mockService) BlockAllocate(ctx context.Context, in api.PinSerial, out *[]string) error {
	if in.ReplicationFactorMin > 1 {
		return errors.New("replMin too high: can only mock-allocate to 1")
	}
	*out = []string{""} // local peer
	return nil
}

func (mock *mockService) SendInformerMetric(ctx context.Context, in struct{}, out *api.Metric) error {
	return nil
}

/* Tracker methods */

func (mock *mockService) Track(ctx context.Context, in api.PinSerial, out *struct{}) error {
	return nil
}

func (mock *mockService) Untrack(ctx context.Context, in api.PinSerial, out *struct{}) error {
	return nil
}

func (mock *mockService) TrackerStatusAll(ctx context.Context, in struct{}, out *[]api.PinInfoSerial) error {
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

func (mock *mockService) TrackerStatus(ctx context.Context, in api.PinSerial, out *api.PinInfoSerial) error {
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

func (mock *mockService) TrackerRecoverAll(ctx context.Context, in struct{}, out *[]api.PinInfoSerial) error {
	*out = make([]api.PinInfoSerial, 0, 0)
	return nil
}

func (mock *mockService) TrackerRecover(ctx context.Context, in api.PinSerial, out *api.PinInfoSerial) error {
	in2 := in.ToPin()
	*out = api.PinInfo{
		Cid:    in2.Cid,
		Peer:   TestPeerID1,
		Status: api.TrackerStatusPinned,
		TS:     time.Now(),
	}.ToSerial()
	return nil
}

/* PeerMonitor methods */

// PeerMonitorLogMetric runs PeerMonitor.LogMetric().
func (mock *mockService) PeerMonitorLogMetric(ctx context.Context, in api.Metric, out *struct{}) error {
	return nil
}

// PeerMonitorLatestMetrics runs PeerMonitor.LatestMetrics().
func (mock *mockService) PeerMonitorLatestMetrics(ctx context.Context, in string, out *[]api.Metric) error {
	m := api.Metric{
		Name:  "test",
		Peer:  TestPeerID1,
		Value: "0",
		Valid: true,
	}
	m.SetTTL(2 * time.Second)
	last := []api.Metric{m}
	*out = last
	return nil
}

/* IPFSConnector methods */

func (mock *mockService) IPFSPin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	return nil
}

func (mock *mockService) IPFSUnpin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	return nil
}

func (mock *mockService) IPFSPinLsCid(ctx context.Context, in api.PinSerial, out *api.IPFSPinStatus) error {
	if in.Cid == TestCid1 || in.Cid == TestCid3 {
		*out = api.IPFSPinStatusRecursive
	} else {
		*out = api.IPFSPinStatusUnpinned
	}
	return nil
}

func (mock *mockService) IPFSPinLs(ctx context.Context, in string, out *map[string]api.IPFSPinStatus) error {
	m := map[string]api.IPFSPinStatus{
		TestCid1: api.IPFSPinStatusRecursive,
		TestCid3: api.IPFSPinStatusRecursive,
	}
	*out = m
	return nil
}

func (mock *mockService) IPFSConnectSwarms(ctx context.Context, in struct{}, out *struct{}) error {
	return nil
}

func (mock *mockService) IPFSSwarmPeers(ctx context.Context, in struct{}, out *api.SwarmPeersSerial) error {
	*out = []string{TestPeerID2.Pretty(), TestPeerID3.Pretty()}
	return nil
}

func (mock *mockService) IPFSConfigKey(ctx context.Context, in string, out *interface{}) error {
	switch in {
	case "Datastore/StorageMax":
		*out = "100KB"
	default:
		return errors.New("configuration key not found")
	}
	return nil
}

func (mock *mockService) IPFSRepoStat(ctx context.Context, in struct{}, out *api.IPFSRepoStat) error {
	// since we have two pins. Assume each is 1000B.
	stat := api.IPFSRepoStat{
		StorageMax: 100000,
		RepoSize:   2000,
	}
	*out = stat
	return nil
}

func (mock *mockService) IPFSBlockPut(ctx context.Context, in api.NodeWithMeta, out *struct{}) error {
	return nil
}

func (mock *mockService) ConsensusAddPeer(ctx context.Context, in peer.ID, out *struct{}) error {
	return errors.New("mock rpc cannot redirect")
}

func (mock *mockService) ConsensusRmPeer(ctx context.Context, in peer.ID, out *struct{}) error {
	return errors.New("mock rpc cannot redirect")
}

func (mock *mockService) ConsensusPeers(ctx context.Context, in struct{}, out *[]peer.ID) error {
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
