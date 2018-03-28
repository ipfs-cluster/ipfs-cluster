package client

import (
	"context"
	"sync"
	"testing"
	"time"

	rpc "github.com/hsanjuan/go-libp2p-gorpc"
	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/api/rest"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	ma "github.com/multiformats/go-multiaddr"
)

func testClients(t *testing.T, api *rest.API, f func(*testing.T, *Client)) {
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

	testF := func(t *testing.T, c *Client) {
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

	testF := func(t *testing.T, c *Client) {
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

	testF := func(t *testing.T, c *Client) {
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

	testF := func(t *testing.T, c *Client) {
		addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/44444")
		c, _ = NewClient(&Config{APIAddr: addr, DisableKeepAlives: true})
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

	testF := func(t *testing.T, c *Client) {
		addr, _ := ma.NewMultiaddr("/ip4/1.2.3.4/tcp/1234/ipfs/" + test.TestPeerID1.Pretty())
		id, err := c.PeerAdd(addr)
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

	testF := func(t *testing.T, c *Client) {
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

	testF := func(t *testing.T, c *Client) {
		ci, _ := cid.Decode(test.TestCid1)
		err := c.Pin(ci, 6, 7, "hello")
		if err != nil {
			t.Fatal(err)
		}
	}

	testClients(t, api, testF)
}

func TestUnpin(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c *Client) {
		ci, _ := cid.Decode(test.TestCid1)
		err := c.Unpin(ci)
		if err != nil {
			t.Fatal(err)
		}
	}

	testClients(t, api, testF)
}

func TestAllocations(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c *Client) {
		pins, err := c.Allocations()
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

	testF := func(t *testing.T, c *Client) {
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

	testF := func(t *testing.T, c *Client) {
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

	testF := func(t *testing.T, c *Client) {
		pins, err := c.StatusAll(false)
		if err != nil {
			t.Fatal(err)
		}

		if len(pins) == 0 {
			t.Error("there should be some pins")
		}
	}

	testClients(t, api, testF)
}

func TestSync(t *testing.T) {
	api := testAPI(t)
	defer shutdown(api)

	testF := func(t *testing.T, c *Client) {
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

	testF := func(t *testing.T, c *Client) {
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

	testF := func(t *testing.T, c *Client) {
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

	testF := func(t *testing.T, c *Client) {
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

	testF := func(t *testing.T, c *Client) {
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

	testF := func(t *testing.T, c *Client) {
		ci, _ := cid.Decode(test.SlowCid)

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

			st, err := c.WaitFor(ctx, fp)
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
		err := c.Pin(ci, 0, 0, "test")
		if err != nil {
			t.Fatal(err)
		}
		wg.Wait()
	}

	testClients(t, tapi, testF)
}
