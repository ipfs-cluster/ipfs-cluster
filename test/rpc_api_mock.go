package test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/state"

	cid "github.com/ipfs/go-cid"
	gopath "github.com/ipfs/go-path"
	host "github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"
)

// ErrBadCid is returned when using ErrorCid. Operations with that CID always
// fail.
var ErrBadCid = errors.New("this is an expected error when using ErrorCid")

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
	err := s.RegisterName("Cluster", &mockCluster{})
	if err != nil {
		t.Fatal(err)
	}
	err = s.RegisterName("PinTracker", &mockPinTracker{})
	if err != nil {
		t.Fatal(err)
	}
	err = s.RegisterName("IPFSConnector", &mockIPFSConnector{})
	if err != nil {
		t.Fatal(err)
	}
	err = s.RegisterName("Consensus", &mockConsensus{})
	if err != nil {
		t.Fatal(err)
	}
	err = s.RegisterName("PeerMonitor", &mockPeerMonitor{})
	if err != nil {
		t.Fatal(err)
	}

	return c
}

type mockCluster struct{}
type mockPinTracker struct{}
type mockIPFSConnector struct{}
type mockConsensus struct{}
type mockPeerMonitor struct{}

func (mock *mockCluster) Pin(ctx context.Context, in *api.Pin, out *api.Pin) error {
	if in.Cid.Equals(ErrorCid) {
		return ErrBadCid
	}
	*out = *in
	return nil
}

func (mock *mockCluster) Unpin(ctx context.Context, in *api.Pin, out *api.Pin) error {
	if in.Cid.Equals(ErrorCid) {
		return ErrBadCid
	}
	if in.Cid.Equals(NotFoundCid) {
		return state.ErrNotFound
	}
	*out = *in
	return nil
}

func (mock *mockCluster) PinPath(ctx context.Context, in *api.PinPath, out *api.Pin) error {
	p, err := gopath.ParsePath(in.Path)
	if err != nil {
		return err
	}

	var pin *api.Pin
	if p.IsJustAKey() && !strings.HasPrefix(in.Path, "/ipns") {
		c, _, err := gopath.SplitAbsPath(p)
		if err != nil {
			return err
		}
		if c.Equals(ErrorCid) {
			return ErrBadCid
		}
		pin = api.PinWithOpts(c, in.PinOptions)
	} else {
		pin = api.PinWithOpts(CidResolved, in.PinOptions)
	}

	*out = *pin
	return nil
}

func (mock *mockCluster) UnpinPath(ctx context.Context, in *api.PinPath, out *api.Pin) error {
	if in.Path == NotFoundPath {
		return state.ErrNotFound
	}

	// Mock-Unpin behaves like pin (doing nothing).
	return mock.PinPath(ctx, in, out)
}

func (mock *mockCluster) Pins(ctx context.Context, in struct{}, out *[]*api.Pin) error {
	opts := api.PinOptions{
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	*out = []*api.Pin{
		api.PinWithOpts(Cid1, opts),
		api.PinCid(Cid2),
		api.PinWithOpts(Cid3, opts),
	}
	return nil
}

func (mock *mockCluster) PinGet(ctx context.Context, in cid.Cid, out *api.Pin) error {
	switch in.String() {
	case ErrorCid.String():
		return errors.New("this is an expected error when using ErrorCid")
	case Cid1.String(), Cid3.String():
		p := api.PinCid(in)
		p.ReplicationFactorMin = -1
		p.ReplicationFactorMax = -1
		*out = *p
		return nil
	case Cid2.String(): // This is a remote pin
		p := api.PinCid(in)
		p.ReplicationFactorMin = 1
		p.ReplicationFactorMax = 1
		*out = *p
	default:
		return errors.New("not found")
	}
	return nil
}

func (mock *mockCluster) ID(ctx context.Context, in struct{}, out *api.ID) error {
	//_, pubkey, _ := crypto.GenerateKeyPair(
	//	DefaultConfigCrypto,
	//	DefaultConfigKeyLength)

	addr, _ := api.NewMultiaddr("/ip4/127.0.0.1/tcp/4001/ipfs/" + PeerID1.Pretty())
	*out = api.ID{
		ID: PeerID1,
		//PublicKey: pubkey,
		Version: "0.0.mock",
		IPFS: &api.IPFSID{
			ID:        PeerID1,
			Addresses: []api.Multiaddr{addr},
		},
	}
	return nil
}

func (mock *mockCluster) Version(ctx context.Context, in struct{}, out *api.Version) error {
	*out = api.Version{
		Version: "0.0.mock",
	}
	return nil
}

func (mock *mockCluster) Peers(ctx context.Context, in struct{}, out *[]*api.ID) error {
	id := &api.ID{}
	mock.ID(ctx, in, id)

	*out = []*api.ID{id}
	return nil
}

func (mock *mockCluster) PeerAdd(ctx context.Context, in peer.ID, out *api.ID) error {
	id := api.ID{}
	mock.ID(ctx, struct{}{}, &id)
	*out = id
	return nil
}

func (mock *mockCluster) PeerRemove(ctx context.Context, in peer.ID, out *struct{}) error {
	return nil
}

func (mock *mockCluster) ConnectGraph(ctx context.Context, in struct{}, out *api.ConnectGraph) error {
	*out = api.ConnectGraph{
		ClusterID: PeerID1,
		IPFSLinks: map[string][]peer.ID{
			peer.IDB58Encode(PeerID4): []peer.ID{PeerID5, PeerID6},
			peer.IDB58Encode(PeerID5): []peer.ID{PeerID4, PeerID6},
			peer.IDB58Encode(PeerID6): []peer.ID{PeerID4, PeerID5},
		},
		ClusterLinks: map[string][]peer.ID{
			peer.IDB58Encode(PeerID1): []peer.ID{PeerID2, PeerID3},
			peer.IDB58Encode(PeerID2): []peer.ID{PeerID1, PeerID3},
			peer.IDB58Encode(PeerID3): []peer.ID{PeerID1, PeerID2},
		},
		ClustertoIPFS: map[string]peer.ID{
			peer.IDB58Encode(PeerID1): PeerID4,
			peer.IDB58Encode(PeerID2): PeerID5,
			peer.IDB58Encode(PeerID3): PeerID6,
		},
	}
	return nil
}

func (mock *mockCluster) StatusAll(ctx context.Context, in struct{}, out *[]*api.GlobalPinInfo) error {
	pid := peer.IDB58Encode(PeerID1)
	*out = []*api.GlobalPinInfo{
		{
			Cid: Cid1,
			PeerMap: map[string]*api.PinInfo{
				pid: {
					Cid:    Cid1,
					Peer:   PeerID1,
					Status: api.TrackerStatusPinned,
					TS:     time.Now(),
				},
			},
		},
		{
			Cid: Cid2,
			PeerMap: map[string]*api.PinInfo{
				pid: {
					Cid:    Cid2,
					Peer:   PeerID1,
					Status: api.TrackerStatusPinning,
					TS:     time.Now(),
				},
			},
		},
		{
			Cid: Cid3,
			PeerMap: map[string]*api.PinInfo{
				pid: {
					Cid:    Cid3,
					Peer:   PeerID1,
					Status: api.TrackerStatusPinError,
					TS:     time.Now(),
				},
			},
		},
	}
	return nil
}

func (mock *mockCluster) StatusAllLocal(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	return (&mockPinTracker{}).StatusAll(ctx, in, out)
}

func (mock *mockCluster) Status(ctx context.Context, in cid.Cid, out *api.GlobalPinInfo) error {
	if in.Equals(ErrorCid) {
		return ErrBadCid
	}
	*out = api.GlobalPinInfo{
		Cid: in,
		PeerMap: map[string]*api.PinInfo{
			peer.IDB58Encode(PeerID1): {
				Cid:    in,
				Peer:   PeerID1,
				Status: api.TrackerStatusPinned,
				TS:     time.Now(),
			},
		},
	}
	return nil
}

func (mock *mockCluster) StatusLocal(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	return (&mockPinTracker{}).Status(ctx, in, out)
}

func (mock *mockCluster) SyncAll(ctx context.Context, in struct{}, out *[]*api.GlobalPinInfo) error {
	return mock.StatusAll(ctx, in, out)
}

func (mock *mockCluster) SyncAllLocal(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	return mock.StatusAllLocal(ctx, in, out)
}

func (mock *mockCluster) Sync(ctx context.Context, in cid.Cid, out *api.GlobalPinInfo) error {
	return mock.Status(ctx, in, out)
}

func (mock *mockCluster) SyncLocal(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	return mock.StatusLocal(ctx, in, out)
}

func (mock *mockCluster) RecoverAll(ctx context.Context, in struct{}, out *[]*api.GlobalPinInfo) error {
	return mock.StatusAll(ctx, in, out)
}

func (mock *mockCluster) RecoverAllLocal(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	return (&mockPinTracker{}).RecoverAll(ctx, in, out)
}

func (mock *mockCluster) Recover(ctx context.Context, in cid.Cid, out *api.GlobalPinInfo) error {
	return mock.Status(ctx, in, out)
}

func (mock *mockCluster) RecoverLocal(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	return (&mockPinTracker{}).Recover(ctx, in, out)
}

func (mock *mockCluster) BlockAllocate(ctx context.Context, in *api.Pin, out *[]peer.ID) error {
	if in.ReplicationFactorMin > 1 {
		return errors.New("replMin too high: can only mock-allocate to 1")
	}
	*out = []peer.ID{""} // allocate to local peer
	return nil
}

func (mock *mockCluster) SendInformerMetric(ctx context.Context, in struct{}, out *api.Metric) error {
	return nil
}

/* Tracker methods */

func (mock *mockPinTracker) Track(ctx context.Context, in *api.Pin, out *struct{}) error {
	return nil
}

func (mock *mockPinTracker) Untrack(ctx context.Context, in *api.Pin, out *struct{}) error {
	return nil
}

func (mock *mockPinTracker) StatusAll(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	*out = []*api.PinInfo{
		{
			Cid:    Cid1,
			Peer:   PeerID1,
			Status: api.TrackerStatusPinned,
			TS:     time.Now(),
		},
		{
			Cid:    Cid3,
			Peer:   PeerID1,
			Status: api.TrackerStatusPinError,
			TS:     time.Now(),
		},
	}
	return nil
}

func (mock *mockPinTracker) Status(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	if in.Equals(ErrorCid) {
		return ErrBadCid
	}

	*out = api.PinInfo{
		Cid:    in,
		Peer:   PeerID2,
		Status: api.TrackerStatusPinned,
		TS:     time.Now(),
	}
	return nil
}

func (mock *mockPinTracker) RecoverAll(ctx context.Context, in struct{}, out *[]*api.PinInfo) error {
	*out = make([]*api.PinInfo, 0, 0)
	return nil
}

func (mock *mockPinTracker) Recover(ctx context.Context, in cid.Cid, out *api.PinInfo) error {
	*out = api.PinInfo{
		Cid:    in,
		Peer:   PeerID1,
		Status: api.TrackerStatusPinned,
		TS:     time.Now(),
	}
	return nil
}

/* PeerMonitor methods */

// LatestMetrics runs PeerMonitor.LatestMetrics().
func (mock *mockPeerMonitor) LatestMetrics(ctx context.Context, in string, out *[]*api.Metric) error {
	m := &api.Metric{
		Name:  "test",
		Peer:  PeerID1,
		Value: "0",
		Valid: true,
	}
	m.SetTTL(2 * time.Second)
	last := []*api.Metric{m}
	*out = last
	return nil
}

/* IPFSConnector methods */

func (mock *mockIPFSConnector) Pin(ctx context.Context, in *api.Pin, out *struct{}) error {
	return nil
}

func (mock *mockIPFSConnector) Unpin(ctx context.Context, in *api.Pin, out *struct{}) error {
	return nil
}

func (mock *mockIPFSConnector) PinLsCid(ctx context.Context, in cid.Cid, out *api.IPFSPinStatus) error {
	if in.Equals(Cid1) || in.Equals(Cid3) {
		*out = api.IPFSPinStatusRecursive
	} else {
		*out = api.IPFSPinStatusUnpinned
	}
	return nil
}

func (mock *mockIPFSConnector) PinLs(ctx context.Context, in string, out *map[string]api.IPFSPinStatus) error {
	m := map[string]api.IPFSPinStatus{
		Cid1.String(): api.IPFSPinStatusRecursive,
		Cid3.String(): api.IPFSPinStatusRecursive,
	}
	*out = m
	return nil
}

func (mock *mockIPFSConnector) SwarmPeers(ctx context.Context, in struct{}, out *[]peer.ID) error {
	*out = []peer.ID{PeerID2, PeerID3}
	return nil
}

func (mock *mockIPFSConnector) ConfigKey(ctx context.Context, in string, out *interface{}) error {
	switch in {
	case "Datastore/StorageMax":
		*out = "100KB"
	default:
		return errors.New("configuration key not found")
	}
	return nil
}

func (mock *mockIPFSConnector) RepoStat(ctx context.Context, in struct{}, out *api.IPFSRepoStat) error {
	// since we have two pins. Assume each is 1000B.
	stat := api.IPFSRepoStat{
		StorageMax: 100000,
		RepoSize:   2000,
	}
	*out = stat
	return nil
}

func (mock *mockIPFSConnector) BlockPut(ctx context.Context, in *api.NodeWithMeta, out *struct{}) error {
	return nil
}

func (mock *mockIPFSConnector) Resolve(ctx context.Context, in string, out *cid.Cid) error {
	switch in {
	case ErrorCid.String(), "/ipfs/" + ErrorCid.String():
		*out = ErrorCid
	default:
		*out = Cid2
	}
	return nil
}

func (mock *mockConsensus) AddPeer(ctx context.Context, in peer.ID, out *struct{}) error {
	return errors.New("mock rpc cannot redirect")
}

func (mock *mockConsensus) RmPeer(ctx context.Context, in peer.ID, out *struct{}) error {
	return errors.New("mock rpc cannot redirect")
}

func (mock *mockConsensus) Peers(ctx context.Context, in struct{}, out *[]peer.ID) error {
	*out = []peer.ID{PeerID1, PeerID2, PeerID3}
	return nil
}
