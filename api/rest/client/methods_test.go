package client

import (
	"context"
	"sync"
	"testing"
	"time"

	cid "github.com/ipfs/go-cid"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"

	types "github.com/ipfs/ipfs-cluster/api"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/test"
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
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		v, err := c.Version()
		if err != nil || v.Version == "" {
			t.Logf("%+v", v)
			t.Log(err)
			t.Error("expected something in version")
		}
	}

	testClients(t, api, testF)
}

func TestID(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		id, err := c.ID()
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
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		ids, err := c.Peers()
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
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/44444")
		c, _ = NewDefaultClient(&Config{APIAddr: addr, DisableKeepAlives: true})
		ids, err := c.Peers()
		if err == nil {
			t.Fatal("expected error")
		}
		if ids == nil || len(ids) != 0 {
			t.Fatal("expected no ids")
		}
	}

	testClients(t, api, testF)
}

func TestPeerAdd(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		id, err := c.PeerAdd(test.TestPeerID1)
		if err != nil {
			t.Fatal(err)
		}
		if id.ID != test.TestPeerID1 {
			t.Error("bad peer")
		}
	}

	testClients(t, api, testF)
}

func TestPeerRm(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		err := c.PeerRm(test.TestPeerID1)
		if err != nil {
			t.Fatal(err)
		}
	}

	testClients(t, api, testF)
}

func TestPin(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		ci, _ := cid.Decode(test.TestCid1)
		opts := types.PinOptions{
			ReplicationFactorMin: 6,
			ReplicationFactorMax: 7,
			Name:                 "hello there",
		}
		err := c.Pin(ci, opts)
		if err != nil {
			t.Fatal(err)
		}
	}

	testClients(t, api, testF)
}

func TestUnpin(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		ci, _ := cid.Decode(test.TestCid1)
		err := c.Unpin(ci)
		if err != nil {
			t.Fatal(err)
		}
	}

	testClients(t, api, testF)
}

type pathCase struct {
	path    string
	wantErr bool
}

var pathTestCases = []pathCase{
	{
		test.TestCidResolved,
		false,
	},
	{
		test.TestPathIPFS1,
		false,
	},
	{
		test.TestPathIPFS2,
		false,
	},
	{
		test.TestPathIPNS2,
		false,
	},
	{
		test.TestPathIPLD2,
		false,
	},
	{
		test.TestInvalidPath1,
		true,
	},
	{
		"/ipfs//QmbUNM297ZwxB8CfFAznK7H9YMesDoY6Tt5bPgt5MSCB2u/im.gif/",
		false,
	},
}

func TestPinPath(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	opts := types.PinOptions{
		ReplicationFactorMin: 6,
		ReplicationFactorMax: 7,
		Name:                 "hello there",
	}

	resultantPin := types.Pin{
		Cid:        test.MustDecodeCid(test.TestCidResolved),
		PinOptions: opts,
	}

	testF := func(t *testing.T, c Client) {

		for _, testCase := range pathTestCases {
			p := testCase.path
			pin, err := c.PinPath(p, opts)
			if err != nil {
				if testCase.wantErr {
					continue
				} else {
					t.Fatalf("unexpected error %s: %s", p, err)
				}
			}

			if !pin.Equals(resultantPin) {
				t.Errorf("expected different pin: %s", p)
				t.Errorf("expected: %+v", resultantPin.ToSerial())
				t.Errorf("actual: %+v", pin.ToSerial())
			}

		}
	}

	testClients(t, api, testF)
}

func TestUnpinPath(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		for _, testCase := range pathTestCases {
			p := testCase.path
			pin, err := c.UnpinPath(p)
			if err != nil {
				if testCase.wantErr {
					continue
				} else {
					t.Fatalf("unepected error %s: %s", p, err)
				}
			}

			if pin.Cid.String() != test.TestCidResolved {
				t.Errorf("bad resolved Cid: %s, %s", p, pin.Cid)
			}
		}
	}

	testClients(t, api, testF)
}

func TestAllocations(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		pins, err := c.Allocations(types.DataType | types.MetaType)
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
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		ci, _ := cid.Decode(test.TestCid1)
		pin, err := c.Allocation(ci)
		if err != nil {
			t.Fatal(err)
		}
		if pin.Cid.String() != test.TestCid1 {
			t.Error("should be same pin")
		}
	}

	testClients(t, api, testF)
}

func TestStatus(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		ci, _ := cid.Decode(test.TestCid1)
		pin, err := c.Status(ci, false)
		if err != nil {
			t.Fatal(err)
		}
		if pin.Cid.String() != test.TestCid1 {
			t.Error("should be same pin")
		}
	}

	testClients(t, api, testF)
}

func TestStatusAll(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		pins, err := c.StatusAll(0, false)
		if err != nil {
			t.Fatal(err)
		}

		if len(pins) == 0 {
			t.Error("there should be some pins")
		}

		// With local true
		pins, err = c.StatusAll(0, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(pins) != 2 {
			t.Error("there should be two pins")
		}

		// With filter option
		pins, err = c.StatusAll(types.TrackerStatusPinning, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(pins) != 1 {
			t.Error("there should be one pin")
		}

		pins, err = c.StatusAll(types.TrackerStatusPinned|types.TrackerStatusError, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(pins) != 2 {
			t.Error("there should be two pins")
		}

		pins, err = c.StatusAll(1<<25, false)
		if err == nil {
			t.Error("expected an error")
		}
	}

	testClients(t, api, testF)
}

func TestSync(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		ci, _ := cid.Decode(test.TestCid1)
		pin, err := c.Sync(ci, false)
		if err != nil {
			t.Fatal(err)
		}
		if pin.Cid.String() != test.TestCid1 {
			t.Error("should be same pin")
		}
	}

	testClients(t, api, testF)
}

func TestSyncAll(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		pins, err := c.SyncAll(false)
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
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		ci, _ := cid.Decode(test.TestCid1)
		pin, err := c.Recover(ci, false)
		if err != nil {
			t.Fatal(err)
		}
		if pin.Cid.String() != test.TestCid1 {
			t.Error("should be same pin")
		}
	}

	testClients(t, api, testF)
}

func TestRecoverAll(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		_, err := c.RecoverAll(true)
		if err != nil {
			t.Fatal(err)
		}
	}

	testClients(t, api, testF)
}

func TestGetConnectGraph(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		cg, err := c.GetConnectGraph()
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
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		m, err := c.Metrics("somemetricstype")
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

func (wait *waitService) Pin(ctx context.Context, in api.PinSerial, out *struct{}) error {
	wait.l.Lock()
	defer wait.l.Unlock()
	wait.pinStart = time.Now()
	return nil
}

func (wait *waitService) Status(ctx context.Context, in api.PinSerial, out *api.GlobalPinInfoSerial) error {
	wait.l.Lock()
	defer wait.l.Unlock()
	c1, _ := cid.Decode(in.Cid)
	if time.Now().After(wait.pinStart.Add(5 * time.Second)) { //pinned
		*out = api.GlobalPinInfo{
			Cid: c1,
			PeerMap: map[peer.ID]api.PinInfo{
				test.TestPeerID1: {
					Cid:    c1,
					Peer:   test.TestPeerID1,
					Status: api.TrackerStatusPinned,
					TS:     wait.pinStart,
				},
				test.TestPeerID2: {
					Cid:    c1,
					Peer:   test.TestPeerID2,
					Status: api.TrackerStatusPinned,
					TS:     wait.pinStart,
				},
			},
		}.ToSerial()
	} else { // pinning
		*out = api.GlobalPinInfo{
			Cid: c1,
			PeerMap: map[peer.ID]api.PinInfo{
				test.TestPeerID1: {
					Cid:    c1,
					Peer:   test.TestPeerID1,
					Status: api.TrackerStatusPinning,
					TS:     wait.pinStart,
				},
				test.TestPeerID2: {
					Cid:    c1,
					Peer:   test.TestPeerID2,
					Status: api.TrackerStatusPinned,
					TS:     wait.pinStart,
				},
			},
		}.ToSerial()
	}

	return nil
}

func TestWaitFor(t *testing.T) {
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
		ci, _ := cid.Decode(test.TestCid1)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			fp := StatusFilterParams{
				Cid:       ci,
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
		err := c.Pin(ci, types.PinOptions{0, 0, "test", 0})
		if err != nil {
			t.Fatal(err)
		}
		wg.Wait()
	}

	testClients(t, tapi, testF)
}

func TestAddMultiFile(t *testing.T) {
	api := testAPI(t)
	defer api.Shutdown()

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

		err := c.AddMultiFile(mfr, p, out)
		if err != nil {
			t.Fatal(err)
		}

		wg.Wait()
	}

	testClients(t, api, testF)
}
