package client

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	types "github.com/ipfs-cluster/ipfs-cluster/api"
	rest "github.com/ipfs-cluster/ipfs-cluster/api/rest"
	test "github.com/ipfs-cluster/ipfs-cluster/test"

	rpc "github.com/libp2p/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p/core/peer"
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
		out := make(chan types.ID, 10)
		err := c.Peers(ctx, out)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) == 0 {
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
		var _ = c
		c, _ = NewDefaultClient(&Config{APIAddr: addr, DisableKeepAlives: true})
		out := make(chan types.ID, 10)
		err := c.Peers(ctx, out)
		if err == nil {
			t.Fatal("expected error")
		}
		if len(out) > 0 {
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
			ec, _ := types.DecodeCid(testCase.expectedCid)
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
		pins := make(chan types.Pin)
		n := 0
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range pins {
				n++
			}
		}()

		err := c.Allocations(ctx, types.DataType|types.MetaType, pins)
		if err != nil {
			t.Fatal(err)
		}

		wg.Wait()
		if n == 0 {
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

func TestStatusCids(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		out := make(chan types.GlobalPinInfo)

		go func() {
			err := c.StatusCids(ctx, []types.Cid{test.Cid1}, false, out)
			if err != nil {
				t.Error(err)
			}
		}()

		pins := collectGlobalPinInfos(t, out)
		if len(pins) != 1 {
			t.Fatal("wrong number of pins returned")
		}
		if !pins[0].Cid.Equals(test.Cid1) {
			t.Error("should be same pin")
		}
	}

	testClients(t, api, testF)
}

func collectGlobalPinInfos(t *testing.T, out <-chan types.GlobalPinInfo) []types.GlobalPinInfo {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var gpis []types.GlobalPinInfo
	for {
		select {
		case <-ctx.Done():
			t.Error(ctx.Err())
			return gpis
		case gpi, ok := <-out:
			if !ok {
				return gpis
			}
			gpis = append(gpis, gpi)
		}
	}
}

func TestStatusAll(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		out := make(chan types.GlobalPinInfo)
		go func() {
			err := c.StatusAll(ctx, 0, false, out)
			if err != nil {
				t.Error(err)
			}
		}()
		pins := collectGlobalPinInfos(t, out)

		if len(pins) == 0 {
			t.Error("there should be some pins")
		}

		out2 := make(chan types.GlobalPinInfo)
		go func() {
			err := c.StatusAll(ctx, 0, true, out2)
			if err != nil {
				t.Error(err)
			}
		}()
		pins = collectGlobalPinInfos(t, out2)

		if len(pins) != 2 {
			t.Error("there should be two pins")
		}

		out3 := make(chan types.GlobalPinInfo)
		go func() {
			err := c.StatusAll(ctx, types.TrackerStatusPinning, false, out3)
			if err != nil {
				t.Error(err)
			}
		}()
		pins = collectGlobalPinInfos(t, out3)

		if len(pins) != 1 {
			t.Error("there should be one pin")
		}

		out4 := make(chan types.GlobalPinInfo)
		go func() {
			err := c.StatusAll(ctx, types.TrackerStatusPinned|types.TrackerStatusError, false, out4)
			if err != nil {
				t.Error(err)
			}
		}()
		pins = collectGlobalPinInfos(t, out4)

		if len(pins) != 2 {
			t.Error("there should be two pins")
		}

		out5 := make(chan types.GlobalPinInfo, 1)
		err := c.StatusAll(ctx, 1<<25, false, out5)
		if err == nil {
			t.Error("expected an error")
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
		out := make(chan types.GlobalPinInfo, 10)
		err := c.RecoverAll(ctx, true, out)
		if err != nil {
			t.Fatal(err)
		}

		out2 := make(chan types.GlobalPinInfo, 10)
		err = c.RecoverAll(ctx, false, out2)
		if err != nil {
			t.Fatal(err)
		}
	}

	testClients(t, api, testF)
}

func TestAlerts(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		alerts, err := c.Alerts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(alerts) != 1 {
			t.Fatal("expected 1 alert")
		}
		pID2 := test.PeerID2.String()
		if alerts[0].Peer != test.PeerID2 {
			t.Errorf("expected an alert from %s", pID2)
		}
	}

	testClients(t, api, testF)
}

func TestBandwidthByProtocol(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		bw, err := c.BandwidthByProtocol(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(bw) != 2 {
			t.Fatal("expected 2 protocols")
		}
		if p1 := bw["protocol1"]; p1.TotalIn != 10 {
			t.Error("expected different bandwidth stats")
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

func TestMetricNames(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		m, err := c.MetricNames(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if len(m) == 0 {
			t.Fatal("No metric names found")
		}
	}

	testClients(t, api, testF)
}

type waitService struct {
	l        sync.Mutex
	pinStart time.Time
}

func (wait *waitService) Pin(ctx context.Context, in types.Pin, out *types.Pin) error {
	wait.l.Lock()
	defer wait.l.Unlock()
	wait.pinStart = time.Now()
	*out = in
	return nil
}

func (wait *waitService) Status(ctx context.Context, in types.Cid, out *types.GlobalPinInfo) error {
	wait.l.Lock()
	defer wait.l.Unlock()
	if time.Now().After(wait.pinStart.Add(5 * time.Second)) { //pinned
		*out = types.GlobalPinInfo{
			Cid: in,
			PeerMap: map[string]types.PinInfoShort{
				test.PeerID1.String(): {
					Status: types.TrackerStatusPinned,
					TS:     wait.pinStart,
				},
				test.PeerID2.String(): {
					Status: types.TrackerStatusPinned,
					TS:     wait.pinStart,
				},
				test.PeerID3.String(): {
					Status: types.TrackerStatusPinning,
					TS:     wait.pinStart,
				},
				test.PeerID3.String(): {
					Status: types.TrackerStatusRemote,
					TS:     wait.pinStart,
				},
			},
		}
	} else { // pinning
		*out = types.GlobalPinInfo{
			Cid: in,
			PeerMap: map[string]types.PinInfoShort{
				test.PeerID1.String(): {
					Status: types.TrackerStatusPinning,
					TS:     wait.pinStart,
				},
				test.PeerID2.String(): {
					Status: types.TrackerStatusPinned,
					TS:     wait.pinStart,
				},
				test.PeerID3.String(): {
					Status: types.TrackerStatusPinning,
					TS:     wait.pinStart,
				},
				test.PeerID3.String(): {
					Status: types.TrackerStatusRemote,
					TS:     wait.pinStart,
				},
			},
		}
	}

	return nil
}

func (wait *waitService) PinGet(ctx context.Context, in types.Cid, out *types.Pin) error {
	p := types.PinCid(in)
	p.ReplicationFactorMin = 2
	p.ReplicationFactorMax = 3
	*out = p
	return nil
}

type waitServiceUnpin struct {
	l          sync.Mutex
	unpinStart time.Time
}

func (wait *waitServiceUnpin) Unpin(ctx context.Context, in types.Pin, out *types.Pin) error {
	wait.l.Lock()
	defer wait.l.Unlock()
	wait.unpinStart = time.Now()
	return nil
}

func (wait *waitServiceUnpin) Status(ctx context.Context, in types.Cid, out *types.GlobalPinInfo) error {
	wait.l.Lock()
	defer wait.l.Unlock()
	if time.Now().After(wait.unpinStart.Add(5 * time.Second)) { //unpinned
		*out = types.GlobalPinInfo{
			Cid: in,
			PeerMap: map[string]types.PinInfoShort{
				test.PeerID1.String(): {
					Status: types.TrackerStatusUnpinned,
					TS:     wait.unpinStart,
				},
				test.PeerID2.String(): {
					Status: types.TrackerStatusUnpinned,
					TS:     wait.unpinStart,
				},
			},
		}
	} else { // pinning
		*out = types.GlobalPinInfo{
			Cid: in,
			PeerMap: map[string]types.PinInfoShort{
				test.PeerID1.String(): {
					Status: types.TrackerStatusUnpinning,
					TS:     wait.unpinStart,
				},
				test.PeerID2.String(): {
					Status: types.TrackerStatusUnpinning,
					TS:     wait.unpinStart,
				},
			},
		}
	}

	return nil
}

func (wait *waitServiceUnpin) PinGet(ctx context.Context, in types.Cid, out *types.Pin) error {
	return errors.New("not found")
}

func TestWaitForPin(t *testing.T) {
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
				Target:    types.TrackerStatusPinned,
				CheckFreq: time.Second,
			}
			start := time.Now()

			st, err := WaitFor(ctx, c, fp)
			if err != nil {
				t.Error(err)
				return
			}
			if time.Since(start) <= 5*time.Second {
				t.Error("slow pin should have taken at least 5 seconds")
				return
			}

			totalPinned := 0
			for _, pi := range st.PeerMap {
				if pi.Status == types.TrackerStatusPinned {
					totalPinned++
				}
			}
			if totalPinned < 2 { // repl factor min
				t.Error("pin info should show the item is pinnedin two places at least")
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

func TestWaitForUnpin(t *testing.T) {
	ctx := context.Background()
	tapi := testAPI(t)
	defer shutdown(tapi)

	rpcS := rpc.NewServer(nil, "wait")
	rpcC := rpc.NewClientWithServer(nil, "wait", rpcS)
	err := rpcS.RegisterName("Cluster", &waitServiceUnpin{})
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
				Target:    types.TrackerStatusUnpinned,
				CheckFreq: time.Second,
			}
			start := time.Now()

			st, err := WaitFor(ctx, c, fp)
			if err != nil {
				t.Error(err)
				return
			}
			if time.Since(start) <= 5*time.Second {
				t.Error("slow unpin should have taken at least 5 seconds")
				return
			}

			for _, pi := range st.PeerMap {
				if pi.Status != types.TrackerStatusUnpinned {
					t.Error("the item should have been unpinned everywhere")
				}
			}
		}()
		_, err := c.Unpin(ctx, test.Cid1)
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

		p := types.AddParams{
			PinOptions: types.PinOptions{
				ReplicationFactorMin: -1,
				ReplicationFactorMax: -1,
				Name:                 "test something",
				ShardSize:            1024,
			},
			Shard:  false,
			Format: "",
			IPFSAddParams: types.IPFSAddParams{
				Chunker:   "",
				RawLeaves: false,
			},
			Hidden:         false,
			StreamChannels: true,
		}

		out := make(chan types.AddedOutput, 1)
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

func TestRepoGC(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		globalGC, err := c.RepoGC(ctx, false)
		if err != nil {
			t.Fatal(err)
		}

		if globalGC.PeerMap == nil {
			t.Fatal("expected a non-nil peer map")
		}

		for _, gc := range globalGC.PeerMap {
			if gc.Peer == "" {
				t.Error("bad id")
			}
			if gc.Error != "" {
				t.Error("did not expect any error")
			}
			if gc.Keys == nil {
				t.Error("expected a non-nil array of IPFSRepoGC")
			} else {
				if !gc.Keys[0].Key.Equals(test.Cid1) {
					t.Errorf("expected a different cid, expected: %s, found: %s", test.Cid1, gc.Keys[0].Key)
				}
			}
		}
	}

	testClients(t, api, testF)
}

func TestHealth(t *testing.T) {
	ctx := context.Background()
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c Client) {
		err := c.Health(ctx)
		if err != nil {
			t.Log(err)
			t.Error("expected no errors")
		}
	}

	testClients(t, api, testF)
}
