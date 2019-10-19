package client

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	types "github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-core/peer"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	ma "github.com/multiformats/go-multiaddr"
)

func testClients(t *testing.T, api *rest.API, f func(*testing.T, Client)) {
	t.Run("in-parallel", func(t *testing.T) {
		t.Run("libp2p", func(t *testing.T) {
			t.Parallel()
			f(t, testClientLibp2p(t, api))
		})
		t.Run("http", func(t *testing.T) {
			t.Parallel()
			f(t, testClientHTTP(t, api))
		})
	})
}

func TestVersion(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		v, err := c.Version(ctx)
		if err != nil || v.Version == "" {
			t.Logf("%+v", v)
			t.Log(err)
			t.Error("expected something in version")
		}
	}

	testClients(t, api, testF)
}

func TestID(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		id, err := c.ID(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if id.ID == "" {
			t.Error("bad id")
		}
	}

	testClients(t, api, testF)
}

func TestPeers(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		ids, err := c.Peers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(ids) == 0 {
			t.Error("expected some peers")
		}
	}

	testClients(t, api, testF)
}

func TestPeersWithError(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/44444")
		c, _ = NewDefaultClient(&Config{APIAddr: addr, DisableKeepAlives: true})
		ids, err := c.Peers(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
		if ids != nil {
			t.Fatal("expected no ids")
		}
	}

	testClients(t, api, testF)
}

func TestPeerAdd(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		id, err := c.PeerAdd(ctx, test.PeerID1)
		if err != nil {
			t.Fatal(err)
		}
		if id.ID != test.PeerID1 {
			t.Error("bad peer")
		}
	}

	testClients(t, api, testF)
}

func TestPeerRm(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		err := c.PeerRm(ctx, test.PeerID1)
		if err != nil {
			t.Fatal(err)
		}
	}

	testClients(t, api, testF)
}

func TestPin(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		opts := types.PinOptions{
			ReplicationFactorMin: 6,
			ReplicationFactorMax: 7,
			Name:                 "hello there",
		}
		_, err := c.Pin(ctx, test.Cid1, opts)
		if err != nil {
			t.Fatal(err)
		}
	}

	testClients(t, api, testF)
}

func TestUnpin(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		_, err := c.Unpin(ctx, test.Cid1)
		if err != nil {
			t.Fatal(err)
		}
	}

	testClients(t, api, testF)
}

type pathCase struct {
	path        string
	wantErr     bool
	expectedCid string
}

var pathTestCases = []pathCase{
	{
		test.CidResolved.String(),
		false,
		test.CidResolved.String(),
	},
	{
		test.PathIPFS1,
		false,
		"QmaNJ5acV31sx8jq626qTpAWW4DXKw34aGhx53dECLvXbY",
	},
	{
		test.PathIPFS2,
		false,
		test.CidResolved.String(),
	},
	{
		test.PathIPNS1,
		false,
		test.CidResolved.String(),
	},
	{
		test.PathIPLD1,
		false,
		"QmaNJ5acV31sx8jq626qTpAWW4DXKw34aGhx53dECLvXbY",
	},
	{
		test.InvalidPath1,
		true,
		"",
	},
}

func TestPinPath(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	opts := types.PinOptions{
		ReplicationFactorMin: 6,
		ReplicationFactorMax: 7,
		Name:                 "hello there",
		UserAllocations:      []peer.ID{test.PeerID1, test.PeerID2},
	}

	testF := func(t *testing.T, c Client) {
		for _, testCase := range pathTestCases {
			ec, _ := cid.Decode(testCase.expectedCid)
			resultantPin := types.PinWithOpts(ec, opts)
			p := testCase.path
			pin, err := c.PinPath(ctx, p, opts)
			if err != nil {
				if testCase.wantErr {
					continue
				}
				t.Fatalf("unexpected error %s: %s", p, err)
			}

			if !pin.Equals(resultantPin) {
				t.Errorf("expected different pin: %s", p)
				t.Errorf("expected: %+v", resultantPin)
				t.Errorf("actual: %+v", pin)
			}

		}
	}

	testClients(t, api, testF)
}

func TestUnpinPath(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		for _, testCase := range pathTestCases {
			p := testCase.path
			pin, err := c.UnpinPath(ctx, p)
			if err != nil {
				if testCase.wantErr {
					continue
				}
				t.Fatalf("unepected error %s: %s", p, err)
			}

			if pin.Cid.String() != testCase.expectedCid {
				t.Errorf("bad resolved Cid: %s, %s", p, pin.Cid)
			}
		}
	}

	testClients(t, api, testF)
}

func TestAllocations(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		pins, err := c.Allocations(ctx, types.DataType|types.MetaType)
		if err != nil {
			t.Fatal(err)
		}
		if len(pins) == 0 {
			t.Error("should be some pins")
		}
	}

	testClients(t, api, testF)
}

func TestAllocation(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		pin, err := c.Allocation(ctx, test.Cid1)
		if err != nil {
			t.Fatal(err)
		}
		if !pin.Cid.Equals(test.Cid1) {
			t.Error("should be same pin")
		}
	}

	testClients(t, api, testF)
}

func TestStatus(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		pin, err := c.Status(ctx, test.Cid1, false)
		if err != nil {
			t.Fatal(err)
		}
		if !pin.Cid.Equals(test.Cid1) {
			t.Error("should be same pin")
		}
	}

	testClients(t, api, testF)
}

func TestStatusAll(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		pins, err := c.StatusAll(ctx, 0, false)
		if err != nil {
			t.Fatal(err)
		}

		if len(pins) == 0 {
			t.Error("there should be some pins")
		}

		// With local true
		pins, err = c.StatusAll(ctx, 0, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(pins) != 2 {
			t.Error("there should be two pins")
		}

		// With filter option
		pins, err = c.StatusAll(ctx, types.TrackerStatusPinning, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(pins) != 1 {
			t.Error("there should be one pin")
		}

		pins, err = c.StatusAll(ctx, types.TrackerStatusPinned|types.TrackerStatusError, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(pins) != 2 {
			t.Error("there should be two pins")
		}

		pins, err = c.StatusAll(ctx, 1<<25, false)
		if err == nil {
			t.Error("expected an error")
		}
	}

	testClients(t, api, testF)
}

func TestSync(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		pin, err := c.Sync(ctx, test.Cid1, false)
		if err != nil {
			t.Fatal(err)
		}
		if !pin.Cid.Equals(test.Cid1) {
			t.Error("should be same pin")
		}
	}

	testClients(t, api, testF)
}

func TestSyncAll(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		pins, err := c.SyncAll(ctx, false)
		if err != nil {
			t.Fatal(err)
		}

		if len(pins) == 0 {
			t.Error("there should be some pins")
		}
	}

	testClients(t, api, testF)
}

func TestRecover(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		pin, err := c.Recover(ctx, test.Cid1, false)
		if err != nil {
			t.Fatal(err)
		}
		if !pin.Cid.Equals(test.Cid1) {
			t.Error("should be same pin")
		}
	}

	testClients(t, api, testF)
}

func TestRecoverAll(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		_, err := c.RecoverAll(ctx, true)
		if err != nil {
			t.Fatal(err)
		}

		_, err = c.RecoverAll(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
	}

	testClients(t, api, testF)
}

func TestGetConnectGraph(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		cg, err := c.GetConnectGraph(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(cg.IPFSLinks) != 3 || len(cg.ClusterLinks) != 3 ||
			len(cg.ClustertoIPFS) != 3 {
			t.Fatal("Bad graph")
		}
	}

	testClients(t, api, testF)
}

func TestMetrics(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		m, err := c.Metrics(ctx, "somemetricstype")
		if err != nil {
			t.Fatal(err)
		}

		if len(m) == 0 {
			t.Fatal("No metrics found")
		}
	}

	testClients(t, api, testF)
}

type waitService struct {
	l        sync.Mutex
	pinStart time.Time
}

func (wait *waitService) Pin(ctx context.Context, in *api.Pin, out *api.Pin) error {
	wait.l.Lock()
	defer wait.l.Unlock()
	wait.pinStart = time.Now()
	*out = *in
	return nil
}

func (wait *waitService) Status(ctx context.Context, in cid.Cid, out *api.GlobalPinInfo) error {
	wait.l.Lock()
	defer wait.l.Unlock()
	if time.Now().After(wait.pinStart.Add(5 * time.Second)) { //pinned
		*out = api.GlobalPinInfo{
			Cid: in,
			PeerMap: map[string]*api.PinInfo{
				peer.IDB58Encode(test.PeerID1): {
					Cid:    in,
					Peer:   test.PeerID1,
					Status: api.TrackerStatusPinned,
					TS:     wait.pinStart,
				},
				peer.IDB58Encode(test.PeerID2): {
					Cid:    in,
					Peer:   test.PeerID2,
					Status: api.TrackerStatusPinned,
					TS:     wait.pinStart,
				},
			},
		}
	} else { // pinning
		*out = api.GlobalPinInfo{
			Cid: in,
			PeerMap: map[string]*api.PinInfo{
				peer.IDB58Encode(test.PeerID1): {
					Cid:    in,
					Peer:   test.PeerID1,
					Status: api.TrackerStatusPinning,
					TS:     wait.pinStart,
				},
				peer.IDB58Encode(test.PeerID2): {
					Cid:    in,
					Peer:   test.PeerID2,
					Status: api.TrackerStatusPinned,
					TS:     wait.pinStart,
				},
			},
		}
	}

	return nil
}

func TestWaitFor(t *testing.T) {
	ctx := context.Background()
	tapi := testAPI(t)
	defer shutdown(tapi)

	rpcS := rpc.NewServer(nil, "wait")
	rpcC := rpc.NewClientWithServer(nil, "wait", rpcS)
	err := rpcS.RegisterName("Cluster", &waitService{})
	if err != nil {
		t.Fatal(err)
	}

	tapi.SetClient(rpcC)

	testF := func(t *testing.T, c Client) {

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			fp := StatusFilterParams{
				Cid:       test.Cid1,
				Local:     false,
				Target:    api.TrackerStatusPinned,
				CheckFreq: time.Second,
			}
			start := time.Now()

			st, err := WaitFor(ctx, c, fp)
			if err != nil {
				t.Fatal(err)
			}
			if time.Now().Sub(start) <= 5*time.Second {
				t.Fatal("slow pin should have taken at least 5 seconds")
			}

			for _, pi := range st.PeerMap {
				if pi.Status != api.TrackerStatusPinned {
					t.Error("pin info should show the item is pinned")
				}
			}
		}()
		_, err := c.Pin(ctx, test.Cid1, types.PinOptions{ReplicationFactorMin: 0, ReplicationFactorMax: 0, Name: "test", ShardSize: 0})
		if err != nil {
			t.Fatal(err)
		}
		wg.Wait()
	}

	testClients(t, tapi, testF)
}

func TestAddMultiFile(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer api.Shutdown(ctx)

	sth := test.NewShardingTestHelper()
	defer sth.Clean(t)

	testF := func(t *testing.T, c Client) {
		mfr, closer := sth.GetTreeMultiReader(t)
		defer closer.Close()

		p := &types.AddParams{
			PinOptions: types.PinOptions{
				ReplicationFactorMin: -1,
				ReplicationFactorMax: -1,
				Name:                 "test something",
				ShardSize:            1024,
			},
			Shard:          false,
			Layout:         "",
			Chunker:        "",
			RawLeaves:      false,
			Hidden:         false,
			StreamChannels: true,
		}

		out := make(chan *types.AddedOutput, 1)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for v := range out {
				t.Logf("output: Name: %s. Hash: %s", v.Name, v.Cid)
			}
		}()

		err := c.AddMultiFile(ctx, mfr, p, out)
		if err != nil {
			t.Fatal(err)
		}

		wg.Wait()
	}

	testClients(t, api, testF)
}
