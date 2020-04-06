package crdt

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/datastore/inmem"
	"github.com/ipfs/ipfs-cluster/test"

	cid "github.com/ipfs/go-cid"
	ipns "github.com/ipfs/go-ipns"
	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-core/host"
	peerstore "github.com/libp2p/go-libp2p-core/peerstore"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	record "github.com/libp2p/go-libp2p-record"
	routedhost "github.com/libp2p/go-libp2p/p2p/host/routed"
)

func makeTestingHost(t *testing.T) (host.Host, *pubsub.PubSub, *dht.IpfsDHT) {
	ctx := context.Background()
	h, err := libp2p.New(
		ctx,
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	if err != nil {
		t.Fatal(err)
	}

	psub, err := pubsub.NewGossipSub(
		ctx,
		h,
		pubsub.WithMessageSigning(true),
		pubsub.WithStrictSignatureVerification(true),
	)
	if err != nil {
		h.Close()
		t.Fatal(err)
	}

	idht, err := dht.New(ctx, h,
		dht.NamespacedValidator("pk", record.PublicKeyValidator{}),
		dht.NamespacedValidator("ipns", ipns.Validator{KeyBook: h.Peerstore()}),
		dht.Concurrency(10),
		dht.RoutingTableRefreshPeriod(200*time.Millisecond),
		dht.RoutingTableRefreshQueryTimeout(100*time.Millisecond),
	)
	if err != nil {
		h.Close()
		t.Fatal(err)
	}

	rHost := routedhost.Wrap(h, idht)
	return rHost, psub, idht
}

func testingConsensus(t *testing.T, idn int) *Consensus {
	h, psub, dht := makeTestingHost(t)

	cfg := &Config{}
	cfg.Default()
	cfg.DatastoreNamespace = fmt.Sprintf("crdttest-%d", idn)
	cfg.hostShutdown = true

	cc, err := New(h, dht, psub, cfg, inmem.New())
	if err != nil {
		t.Fatal("cannot create Consensus:", err)
	}
	cc.SetClient(test.NewMockRPCClientWithHost(t, h))
	<-cc.Ready(context.Background())
	return cc
}

func clean(t *testing.T, cc *Consensus) {
	err := cc.Clean(context.Background())
	if err != nil {
		t.Error(err)
	}
}

func testPin(c cid.Cid) *api.Pin {
	p := api.PinCid(c)
	p.ReplicationFactorMin = -1
	p.ReplicationFactorMax = -1
	return p
}

func TestShutdownConsensus(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	defer clean(t, cc)
	err := cc.Shutdown(ctx)
	if err != nil {
		t.Fatal("Consensus cannot shutdown:", err)
	}
	err = cc.Shutdown(ctx) // should be fine to shutdown twice
	if err != nil {
		t.Fatal("Consensus should be able to shutdown several times")
	}
}

func TestConsensusPin(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	defer clean(t, cc)
	defer cc.Shutdown(ctx)

	err := cc.LogPin(ctx, testPin(test.Cid1))
	if err != nil {
		t.Error(err)
	}

	time.Sleep(250 * time.Millisecond)
	st, err := cc.State(ctx)
	if err != nil {
		t.Fatal("error getting state:", err)
	}

	pins, err := st.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 1 || !pins[0].Cid.Equals(test.Cid1) {
		t.Error("the added pin should be in the state")
	}
}

func TestConsensusUnpin(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	defer clean(t, cc)
	defer cc.Shutdown(ctx)

	err := cc.LogPin(ctx, testPin(test.Cid1))
	if err != nil {
		t.Error(err)
	}

	err = cc.LogUnpin(ctx, api.PinCid(test.Cid1))
	if err != nil {
		t.Error(err)
	}
}

func TestConsensusUpdate(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	defer clean(t, cc)
	defer cc.Shutdown(ctx)

	// Pin first
	pin := testPin(test.Cid1)
	pin.Type = api.ShardType
	err := cc.LogPin(ctx, pin)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)

	// Update pin
	pin.Reference = &test.Cid2
	err = cc.LogPin(ctx, pin)
	if err != nil {
		t.Error(err)
	}

	time.Sleep(250 * time.Millisecond)
	st, err := cc.State(ctx)
	if err != nil {
		t.Fatal("error getting state:", err)
	}

	pins, err := st.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 1 || !pins[0].Cid.Equals(test.Cid1) {
		t.Error("the added pin should be in the state")
	}
	if !pins[0].Reference.Equals(test.Cid2) {
		t.Error("pin updated incorrectly")
	}
}

func TestConsensusAddRmPeer(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	cc2 := testingConsensus(t, 2)
	defer clean(t, cc)
	defer clean(t, cc)
	defer cc.Shutdown(ctx)
	defer cc2.Shutdown(ctx)

	cc.host.Peerstore().AddAddrs(cc2.host.ID(), cc2.host.Addrs(), peerstore.PermanentAddrTTL)
	_, err := cc.host.Network().DialPeer(ctx, cc2.host.ID())
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	err = cc.AddPeer(ctx, cc2.host.ID())
	if err != nil {
		t.Error("could not add peer:", err)
	}

	err = cc2.Trust(ctx, cc.host.ID())
	if err != nil {
		t.Error("could not trust peer:", err)
	}

	// Make a pin on peer1 and check it arrived to peer2
	err = cc.LogPin(ctx, testPin(test.Cid1))
	if err != nil {
		t.Error(err)
	}

	time.Sleep(250 * time.Millisecond)
	st, err := cc2.State(ctx)
	if err != nil {
		t.Fatal("error getting state:", err)
	}

	pins, err := st.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 1 || !pins[0].Cid.Equals(test.Cid1) {
		t.Error("the added pin should be in the state")
	}

	err = cc2.RmPeer(ctx, cc.host.ID())
	if err == nil {
		t.Error("crdt consensus should not remove pins")
	}
}

func TestConsensusDistrustPeer(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	cc2 := testingConsensus(t, 2)
	defer clean(t, cc)
	defer clean(t, cc)
	defer cc.Shutdown(ctx)
	defer cc2.Shutdown(ctx)

	cc.host.Peerstore().AddAddrs(cc2.host.ID(), cc2.host.Addrs(), peerstore.PermanentAddrTTL)
	_, err := cc.host.Network().DialPeer(ctx, cc2.host.ID())
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	err = cc2.Trust(ctx, cc.host.ID())
	if err != nil {
		t.Error("could not trust peer:", err)
	}

	// Make a pin on peer1 and check it arrived to peer2
	err = cc.LogPin(ctx, testPin(test.Cid1))
	if err != nil {
		t.Error(err)
	}

	time.Sleep(250 * time.Millisecond)

	err = cc2.Distrust(ctx, cc.host.ID())
	if err != nil {
		t.Error("could not distrust peer:", err)
	}

	// Another pin should never get to peer2
	err = cc.LogPin(ctx, testPin(test.Cid2))
	if err != nil {
		t.Error(err)
	}

	// Verify we only got the first pin
	st, err := cc2.State(ctx)
	if err != nil {
		t.Fatal("error getting state:", err)
	}

	pins, err := st.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 1 || !pins[0].Cid.Equals(test.Cid1) {
		t.Error("only first pin should be in the state")
	}
}

func TestPeers(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	defer clean(t, cc)
	defer cc.Shutdown(ctx)

	peers, err := cc.Peers(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// 1 is ourselves and the other comes from rpc
	// mock PeerMonitorLatestMetrics
	if len(peers) != 2 {
		t.Error("unexpected number of peers")
	}
}

func TestOfflineState(t *testing.T) {
	ctx := context.Background()
	cc := testingConsensus(t, 1)
	defer clean(t, cc)
	defer cc.Shutdown(ctx)

	// Make pin 1
	err := cc.LogPin(ctx, testPin(test.Cid1))
	if err != nil {
		t.Error(err)
	}

	// Make pin 2
	err = cc.LogPin(ctx, testPin(test.Cid2))
	if err != nil {
		t.Error(err)
	}

	err = cc.Shutdown(ctx)
	if err != nil {
		t.Fatal(err)
	}

	offlineState, err := OfflineState(cc.config, cc.store)
	if err != nil {
		t.Fatal(err)
	}

	pins, err := offlineState.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 2 {
		t.Error("there should be two pins in the state")
	}
}
