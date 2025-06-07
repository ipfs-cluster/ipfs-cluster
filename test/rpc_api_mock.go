package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/state"

	gopath "github.com/ipfs/boxo/path"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	host "github.com/libp2p/go-libp2p/core/host"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

var (
	// ErrBadCid is returned when using ErrorCid. Operations with that CID always
	// fail.
	ErrBadCid = errors.New("this is an expected error when using ErrorCid")
	// ErrLinkNotFound is error returned when no link is found
	ErrLinkNotFound = errors.New("no link by that name")
)

// NewMockRPCClient creates a mock ipfs-cluster RPC server and returns
// a client to it.
func NewMockRPCClient(t testing.TB) *rpc.Client {
	return NewMockRPCClientWithHost(t, nil)
}

// NewMockRPCClientWithHost returns a mock ipfs-cluster RPC server
// initialized with a given host.
func NewMockRPCClientWithHost(t testing.TB, h host.Host) *rpc.Client {
	s := rpc.NewServer(h, "mock", rpc.WithStreamBufferSize(1024))
	c := rpc.NewClientWithServer(h, "mock", s, rpc.WithMultiStreamBufferSize(1024))
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

func (mock *mockCluster) Pin(ctx context.Context, in api.Pin, out *api.Pin) error {
	if in.Cid.Equals(ErrorCid) {
		return ErrBadCid
	}

	// a pin is never returned the replications set to 0.
	if in.ReplicationFactorMin == 0 {
		in.ReplicationFactorMin = -1
	}
	if in.ReplicationFactorMax == 0 {
		in.ReplicationFactorMax = -1
	}
	*out = in
	return nil
}

func (mock *mockCluster) Unpin(ctx context.Context, in api.Pin, out *api.Pin) error {
	if in.Cid.Equals(ErrorCid) {
		return ErrBadCid
	}
	if in.Cid.Equals(NotFoundCid) {
		return state.ErrNotFound
	}
	*out = in
	return nil
}

func (mock *mockCluster) PinPath(ctx context.Context, in api.PinPath, out *api.Pin) error {
	p, err := gopath.NewPath(in.Path)
	if err != nil {
		p, err = gopath.NewPath("/ipfs/" + in.Path)
		if err != nil {
			return err
		}
	}

	var pin api.Pin

	immPath, err := gopath.NewImmutablePath(p)
	if err == nil && len(immPath.Segments()) == 2 { // no need to resolve
		cc := api.NewCid(immPath.RootCid())
		if cc.Equals(ErrorCid) {
			return ErrBadCid
		}
		pin = api.PinWithOpts(cc, in.PinOptions)
	} else { // must resolve
		pin = api.PinWithOpts(CidResolved, in.PinOptions)
	}

	*out = pin
	return nil
}

func (mock *mockCluster) UnpinPath(ctx context.Context, in api.PinPath, out *api.Pin) error {
	if in.Path == NotFoundPath {
		return state.ErrNotFound
	}

	// Mock-Unpin behaves like pin (doing nothing).
	return mock.PinPath(ctx, in, out)
}

func (mock *mockCluster) Pins(ctx context.Context, in <-chan struct{}, out chan<- api.Pin) error {
	opts := api.PinOptions{
		ReplicationFactorMin: -1,
		ReplicationFactorMax: -1,
	}

	out <- api.PinWithOpts(Cid1, opts)
	out <- api.PinCid(Cid2)
	out <- api.PinWithOpts(Cid3, opts)
	close(out)
	return nil
}

func (mock *mockCluster) PinGet(ctx context.Context, in api.Cid, out *api.Pin) error {
	switch in.String() {
	case ErrorCid.String():
		return errors.New("this is an expected error when using ErrorCid")
	case Cid1.String(), Cid3.String():
		p := api.PinCid(in)
		p.ReplicationFactorMin = -1
		p.ReplicationFactorMax = -1
		*out = p
		return nil
	case Cid2.String(): // This is a remote pin
		p := api.PinCid(in)
		p.ReplicationFactorMin = 1
		p.ReplicationFactorMax = 1
		*out = p
	default:
		return state.ErrNotFound
	}
	return nil
}

func (mock *mockCluster) ID(ctx context.Context, in struct{}, out *api.ID) error {
	//_, pubkey, _ := crypto.GenerateKeyPair(
	//	DefaultConfigCrypto,
	//	DefaultConfigKeyLength)

	addr, _ := api.NewMultiaddr("/ip4/127.0.0.1/tcp/4001/p2p/" + PeerID1.String())
	*out = api.ID{
		ID: PeerID1,
		//PublicKey: pubkey,
		Version: "0.0.mock",
		IPFS: api.IPFSID{
			ID:        PeerID1,
			Addresses: []api.Multiaddr{addr},
		},
	}
	return nil
}

func (mock *mockCluster) IDStream(ctx context.Context, in <-chan struct{}, out chan<- api.ID) error {
	defer close(out)
	var id api.ID
	mock.ID(ctx, struct{}{}, &id)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- id:
	}
	return nil
}

func (mock *mockCluster) Version(ctx context.Context, in struct{}, out *api.Version) error {
	*out = api.Version{
		Version: "0.0.mock",
	}
	return nil
}

func (mock *mockCluster) Peers(ctx context.Context, in <-chan struct{}, out chan<- api.ID) error {
	id := api.ID{}
	mock.ID(ctx, struct{}{}, &id)
	out <- id
	close(out)
	return nil
}

func (mock *mockCluster) PeersWithFilter(ctx context.Context, in <-chan []peer.ID, out chan<- api.ID) error {
	inCh := make(chan struct{})
	close(inCh)
	return mock.Peers(ctx, inCh, out)
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
			PeerID4.String(): {PeerID5, PeerID6},
			PeerID5.String(): {PeerID4, PeerID6},
			PeerID6.String(): {PeerID4, PeerID5},
		},
		ClusterLinks: map[string][]peer.ID{
			PeerID1.String(): {PeerID2, PeerID3},
			PeerID2.String(): {PeerID1, PeerID3},
			PeerID3.String(): {PeerID1, PeerID2},
		},
		ClustertoIPFS: map[string]peer.ID{
			PeerID1.String(): PeerID4,
			PeerID2.String(): PeerID5,
			PeerID3.String(): PeerID6,
		},
	}
	return nil
}

func (mock *mockCluster) StatusAll(ctx context.Context, in <-chan api.TrackerStatus, out chan<- api.GlobalPinInfo) error {
	defer close(out)
	filter := <-in

	pid := PeerID1.String()
	gPinInfos := []api.GlobalPinInfo{
		{
			Cid:  Cid1,
			Name: "aaa",
			PeerMap: map[string]api.PinInfoShort{
				pid: {
					Status: api.TrackerStatusPinned,
					TS:     time.Now(),
				},
			},
		},
		{
			Cid:  Cid2,
			Name: "bbb",
			PeerMap: map[string]api.PinInfoShort{
				pid: {
					Status: api.TrackerStatusPinning,
					TS:     time.Now(),
				},
			},
		},
		{
			Cid:  Cid3,
			Name: "ccc",
			Metadata: map[string]string{
				"ccc": "3c",
			},
			PeerMap: map[string]api.PinInfoShort{
				pid: {
					Status: api.TrackerStatusPinError,
					TS:     time.Now(),
				},
			},
		},
	}
	// If there is no filter match, we will not return that status and we
	// will not have an entry for that peer in the peerMap.  In turn, when
	// a single peer, we will not have an entry for the cid at all.
	for _, gpi := range gPinInfos {
		for id, pi := range gpi.PeerMap {
			if !filter.Match(pi.Status) {
				delete(gpi.PeerMap, id)
			}
		}
	}
	for _, gpi := range gPinInfos {
		if len(gpi.PeerMap) > 0 {
			out <- gpi
		}
	}

	return nil
}

func (mock *mockCluster) StatusAllLocal(ctx context.Context, in <-chan api.TrackerStatus, out chan<- api.PinInfo) error {
	return (&mockPinTracker{}).StatusAll(ctx, in, out)
}

func (mock *mockCluster) Status(ctx context.Context, in api.Cid, out *api.GlobalPinInfo) error {
	if in.Equals(ErrorCid) {
		return ErrBadCid
	}
	ma, _ := api.NewMultiaddr("/ip4/1.2.3.4/ipfs/" + PeerID3.String())

	*out = api.GlobalPinInfo{
		Cid:         in,
		Name:        "test",
		Allocations: nil,
		Origins:     nil,
		Metadata: map[string]string{
			"meta": "data",
		},

		PeerMap: map[string]api.PinInfoShort{
			PeerID1.String(): {
				PeerName:      PeerName3,
				IPFS:          PeerID3,
				IPFSAddresses: []api.Multiaddr{ma},
				Status:        api.TrackerStatusPinned,
				TS:            time.Now(),
			},
		},
	}
	return nil
}

func (mock *mockCluster) StatusLocal(ctx context.Context, in api.Cid, out *api.PinInfo) error {
	return (&mockPinTracker{}).Status(ctx, in, out)
}

func (mock *mockCluster) RecoverAll(ctx context.Context, in <-chan struct{}, out chan<- api.GlobalPinInfo) error {
	f := make(chan api.TrackerStatus, 1)
	f <- api.TrackerStatusUndefined
	close(f)
	return mock.StatusAll(ctx, f, out)
}

func (mock *mockCluster) RecoverAllLocal(ctx context.Context, in <-chan struct{}, out chan<- api.PinInfo) error {
	return (&mockPinTracker{}).RecoverAll(ctx, in, out)
}

func (mock *mockCluster) Recover(ctx context.Context, in api.Cid, out *api.GlobalPinInfo) error {
	return mock.Status(ctx, in, out)
}

func (mock *mockCluster) RecoverLocal(ctx context.Context, in api.Cid, out *api.PinInfo) error {
	return (&mockPinTracker{}).Recover(ctx, in, out)
}

func (mock *mockCluster) BlockAllocate(ctx context.Context, in api.Pin, out *[]peer.ID) error {
	if in.ReplicationFactorMin > 1 {
		return errors.New("replMin too high: can only mock-allocate to 1")
	}
	*out = []peer.ID{""} // allocate to local peer
	return nil
}

func (mock *mockCluster) RepoGC(ctx context.Context, in struct{}, out *api.GlobalRepoGC) error {
	localrepoGC := api.RepoGC{}
	_ = mock.RepoGCLocal(ctx, struct{}{}, &localrepoGC)
	*out = api.GlobalRepoGC{
		PeerMap: map[string]api.RepoGC{
			PeerID1.String(): localrepoGC,
		},
	}
	return nil
}

func (mock *mockCluster) RepoGCLocal(ctx context.Context, in struct{}, out *api.RepoGC) error {
	*out = api.RepoGC{
		Peer: PeerID1,
		Keys: []api.IPFSRepoGC{
			{
				Key: Cid1,
			},
			{
				Key: Cid2,
			},
			{
				Key: Cid3,
			},
			{
				Key: Cid4,
			},
			{
				Error: ErrLinkNotFound.Error(),
			},
		},
	}

	return nil
}

func (mock *mockCluster) SendInformerMetrics(ctx context.Context, in struct{}, out *struct{}) error {
	return nil
}

func (mock *mockCluster) Alerts(ctx context.Context, in struct{}, out *[]api.Alert) error {
	*out = []api.Alert{
		{
			Metric: api.Metric{
				Name:       "ping",
				Peer:       PeerID2,
				Expire:     time.Now().Add(-30 * time.Second).UnixNano(),
				Valid:      true,
				ReceivedAt: time.Now().Add(-60 * time.Second).UnixNano(),
			},
			TriggeredAt: time.Now(),
		},
	}
	return nil
}

func (mock *mockCluster) BandwidthByProtocol(ctx context.Context, in struct{}, out *api.BandwidthByProtocol) error {
	*out = api.BandwidthByProtocol{
		"protocol1": api.Bandwidth{
			TotalIn:  10,
			TotalOut: 20,
			RateIn:   1,
			RateOut:  2,
		},
		"protocol2": api.Bandwidth{
			TotalIn:  30,
			TotalOut: 40,
			RateIn:   3,
			RateOut:  4,
		},
	}
	return nil
}

func (mock *mockCluster) IPFSID(ctx context.Context, in peer.ID, out *api.IPFSID) error {
	var id api.ID
	_ = mock.ID(ctx, struct{}{}, &id)
	*out = id.IPFS
	return nil
}

/* Tracker methods */

func (mock *mockPinTracker) Track(ctx context.Context, in api.Pin, out *struct{}) error {
	return nil
}

func (mock *mockPinTracker) Untrack(ctx context.Context, in api.Pin, out *struct{}) error {
	return nil
}

func (mock *mockPinTracker) StatusAll(ctx context.Context, in <-chan api.TrackerStatus, out chan<- api.PinInfo) error {
	defer close(out)
	filter := <-in

	pinInfos := []api.PinInfo{
		{
			Cid:  Cid1,
			Peer: PeerID1,
			PinInfoShort: api.PinInfoShort{
				Status: api.TrackerStatusPinned,
				TS:     time.Now(),
			},
		},
		{
			Cid:  Cid3,
			Peer: PeerID1,
			PinInfoShort: api.PinInfoShort{
				Status: api.TrackerStatusPinError,
				TS:     time.Now(),
			},
		},
	}
	for _, pi := range pinInfos {
		if filter.Match(pi.Status) {
			out <- pi
		}
	}
	return nil
}

func (mock *mockPinTracker) Status(ctx context.Context, in api.Cid, out *api.PinInfo) error {
	if in.Equals(ErrorCid) {
		return ErrBadCid
	}

	*out = api.PinInfo{
		Cid:  in,
		Peer: PeerID2,
		PinInfoShort: api.PinInfoShort{
			Status: api.TrackerStatusPinned,
			TS:     time.Now(),
		},
	}
	return nil
}

func (mock *mockPinTracker) RecoverAll(ctx context.Context, in <-chan struct{}, out chan<- api.PinInfo) error {
	close(out)
	return nil
}

func (mock *mockPinTracker) Recover(ctx context.Context, in api.Cid, out *api.PinInfo) error {
	*out = api.PinInfo{
		Cid:  in,
		Peer: PeerID1,
		PinInfoShort: api.PinInfoShort{
			Status: api.TrackerStatusPinned,
			TS:     time.Now(),
		},
	}
	return nil
}

func (mock *mockPinTracker) PinQueueSize(ctx context.Context, in struct{}, out *int64) error {
	*out = 10
	return nil
}

/* PeerMonitor methods */

// LatestMetrics runs PeerMonitor.LatestMetrics().
func (mock *mockPeerMonitor) LatestMetrics(ctx context.Context, in string, out *[]api.Metric) error {
	m := api.Metric{
		Name:  "test",
		Peer:  PeerID1,
		Value: "0",
		Valid: true,
	}
	m.SetTTL(2 * time.Second)
	last := []api.Metric{m}
	*out = last
	return nil
}

// MetricNames runs PeerMonitor.MetricNames().
func (mock *mockPeerMonitor) MetricNames(ctx context.Context, in struct{}, out *[]string) error {
	k := []string{"ping", "freespace"}
	*out = k
	return nil
}

/* IPFSConnector methods */

func (mock *mockIPFSConnector) Pin(ctx context.Context, in api.Pin, out *struct{}) error {
	switch in.Cid {
	case SlowCid1:
		time.Sleep(2 * time.Second)
	}
	return nil
}

func (mock *mockIPFSConnector) Unpin(ctx context.Context, in api.Pin, out *struct{}) error {
	switch in.Cid {
	case SlowCid1:
		time.Sleep(2 * time.Second)
	}
	return nil
}

func (mock *mockIPFSConnector) PinLsCid(ctx context.Context, in api.Pin, out *api.IPFSPinStatus) error {
	if in.Cid.Equals(Cid1) || in.Cid.Equals(Cid3) {
		*out = api.IPFSPinStatusRecursive
	} else {
		*out = api.IPFSPinStatusUnpinned
	}
	return nil
}

func (mock *mockIPFSConnector) PinLs(ctx context.Context, in <-chan []string, out chan<- api.IPFSPinInfo) error {
	out <- api.IPFSPinInfo{Cid: api.Cid(Cid1), Type: api.IPFSPinStatusRecursive}
	out <- api.IPFSPinInfo{Cid: api.Cid(Cid3), Type: api.IPFSPinStatusRecursive}
	close(out)
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

func (mock *mockIPFSConnector) BlockStream(ctx context.Context, in <-chan api.NodeWithMeta, out chan<- struct{}) error {
	close(out)
	return nil
}

func (mock *mockIPFSConnector) Resolve(ctx context.Context, in string, out *api.Cid) error {
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
