package pstoremgr

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/test"

	libp2p "github.com/libp2p/go-libp2p"
	peer "github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func makeMgr(t *testing.T) *Manager {
	h, err := libp2p.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return New(context.Background(), h, "peerstore")
}

func clean(pm *Manager) {
	if path := pm.peerstorePath; path != "" {
		os.RemoveAll(path)
	}
}

func testAddr(loc string, pid peer.ID) ma.Multiaddr {
	m, _ := ma.NewMultiaddr(loc + "/ipfs/" + peer.IDB58Encode(pid))
	return m
}

func TestManager(t *testing.T) {
	pm := makeMgr(t)
	defer clean(pm)

	loc := "/ip4/127.0.0.1/tcp/1234"
	testAddr := testAddr(loc, test.PeerID1)

	_, err := pm.ImportPeer(testAddr, false, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	peers := []peer.ID{test.PeerID1, pm.host.ID()}
	pinfos := pm.PeerInfos(peers)
	if len(pinfos) != 1 {
		t.Fatal("expected 1 peerinfo")
	}

	if pinfos[0].ID != test.PeerID1 {
		t.Error("expected same peer as added")
	}

	if len(pinfos[0].Addrs) != 1 {
		t.Fatal("expected an address")
	}

	if pinfos[0].Addrs[0].String() != loc {
		t.Error("expected same address as added")
	}

	pm.RmPeer(peers[0])
	pinfos = pm.PeerInfos(peers)
	if len(pinfos) != 0 {
		t.Fatal("expected 0 pinfos")
	}
}

func TestManagerDNS(t *testing.T) {
	pm := makeMgr(t)
	defer clean(pm)

	loc1 := "/ip4/127.0.0.1/tcp/1234"
	testAddr1 := testAddr(loc1, test.PeerID1)
	loc2 := "/dns4/localhost/tcp/1235"
	testAddr2 := testAddr(loc2, test.PeerID1)

	err := pm.ImportPeers([]ma.Multiaddr{testAddr1, testAddr2}, false, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	pinfos := pm.PeerInfos([]peer.ID{test.PeerID1})
	if len(pinfos) != 1 {
		t.Fatal("expected 1 pinfo")
	}

	if len(pinfos[0].Addrs) != 1 {
		t.Error("expected a single address")
	}

	if pinfos[0].Addrs[0].String() != "/dns4/localhost/tcp/1235" {
		t.Error("expected the dns address")
	}
}

func TestPeerstore(t *testing.T) {
	pm := makeMgr(t)
	defer clean(pm)

	loc1 := "/ip4/127.0.0.1/tcp/1234"
	testAddr1 := testAddr(loc1, test.PeerID1)
	loc2 := "/ip4/127.0.0.1/tcp/1235"
	testAddr2 := testAddr(loc2, test.PeerID1)

	err := pm.ImportPeers([]ma.Multiaddr{testAddr1, testAddr2}, false, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	err = pm.SavePeerstoreForPeers([]peer.ID{test.PeerID1})
	if err != nil {
		t.Error(err)
	}

	pm2 := makeMgr(t)
	defer clean(pm2)

	err = pm2.ImportPeersFromPeerstore(false, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	pinfos := pm2.PeerInfos([]peer.ID{test.PeerID1})
	if len(pinfos) != 1 {
		t.Fatal("expected 1 peer in the peerstore")
	}

	if len(pinfos[0].Addrs) != 2 {
		t.Error("expected 2 addresses")
	}
}

func TestPriority(t *testing.T) {
	pm := makeMgr(t)
	defer clean(pm)

	loc1 := "/ip4/127.0.0.1/tcp/1234"
	testAddr1 := testAddr(loc1, test.PeerID1)
	loc2 := "/ip4/127.0.0.2/tcp/1235"
	testAddr2 := testAddr(loc2, test.PeerID2)
	loc3 := "/ip4/127.0.0.3/tcp/1234"
	testAddr3 := testAddr(loc3, test.PeerID3)
	loc4 := "/ip4/127.0.0.4/tcp/1235"
	testAddr4 := testAddr(loc4, test.PeerID4)

	err := pm.ImportPeers([]ma.Multiaddr{testAddr1, testAddr2, testAddr3, testAddr4}, false, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	pinfos := pm.PeerInfos([]peer.ID{test.PeerID4, test.PeerID2, test.PeerID3, test.PeerID1})
	if len(pinfos) != 4 {
		t.Fatal("expected 4 pinfos")
	}

	if pinfos[0].ID != test.PeerID1 ||
		pinfos[1].ID != test.PeerID2 ||
		pinfos[2].ID != test.PeerID3 ||
		pinfos[3].ID != test.PeerID4 {
		t.Error("wrong order of peerinfos")
	}

	pm.SetPriority(test.PeerID1, 100)

	pinfos = pm.PeerInfos([]peer.ID{test.PeerID4, test.PeerID2, test.PeerID3, test.PeerID1})
	if len(pinfos) != 4 {
		t.Fatal("expected 4 pinfos")
	}

	if pinfos[3].ID != test.PeerID1 {
		t.Fatal("PeerID1 should be last in the list")
	}

	err = pm.SavePeerstoreForPeers([]peer.ID{test.PeerID4, test.PeerID2, test.PeerID3, test.PeerID1})
	if err != nil {
		t.Error(err)
	}

	pm2 := makeMgr(t)
	defer clean(pm2)

	err = pm2.ImportPeersFromPeerstore(false, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	pinfos = pm2.PeerInfos([]peer.ID{test.PeerID4, test.PeerID2, test.PeerID3, test.PeerID1})
	if len(pinfos) != 4 {
		t.Fatal("expected 4 pinfos")
	}

	if pinfos[0].ID != test.PeerID2 ||
		pinfos[1].ID != test.PeerID3 ||
		pinfos[2].ID != test.PeerID4 ||
		pinfos[3].ID != test.PeerID1 {
		t.Error("wrong order of peerinfos")
	}
}
