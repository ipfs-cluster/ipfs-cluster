package pstoremgr

import (
	"context"
	"os"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"

	libp2p "github.com/libp2p/go-libp2p"
	ma "github.com/multiformats/go-multiaddr"
)

var pid = "QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc"

func makeMgr(t *testing.T) *Manager {
	h, err := libp2p.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return New(h, "peerstore")
}

func clean(pm *Manager) {
	if path := pm.peerstorePath; path != "" {
		os.RemoveAll(path)
	}
}

func TestManager(t *testing.T) {
	pm := makeMgr(t)
	defer clean(pm)

	testPeer, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1234/ipfs/" + pid)

	err := pm.ImportPeer(testPeer, false)
	if err != nil {
		t.Fatal(err)
	}

	peers := api.StringsToPeers([]string{pid, pm.host.ID().Pretty()})
	addrs := pm.PeersAddresses(peers)
	if len(addrs) != 1 {
		t.Fatal("expected 1 address")
	}

	if !addrs[0].Equal(testPeer) {
		t.Error("expected same address as added")
	}

	pm.RmPeer(peers[0])
	addrs = pm.PeersAddresses(peers)
	if len(addrs) != 0 {
		t.Fatal("expected 0 addresses")
	}
}

func TestManagerDNS(t *testing.T) {
	pm := makeMgr(t)
	defer clean(pm)

	testPeer, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1234/ipfs/" + pid)
	testPeer2, _ := ma.NewMultiaddr("/dns4/localhost/tcp/1235/ipfs/" + pid)

	err := pm.ImportPeers([]ma.Multiaddr{testPeer, testPeer2}, false)
	if err != nil {
		t.Fatal(err)
	}

	addrs := pm.PeersAddresses(api.StringsToPeers([]string{pid}))
	if len(addrs) != 1 {
		t.Fatal("expected 1 address")
	}

	if !addrs[0].Equal(testPeer2) {
		t.Error("expected only the dns address")
	}
}

func TestPeerstore(t *testing.T) {
	pm := makeMgr(t)
	defer clean(pm)

	testPeer, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1234/ipfs/" + pid)
	testPeer2, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1235/ipfs/" + pid)

	err := pm.ImportPeers([]ma.Multiaddr{testPeer, testPeer2}, false)
	if err != nil {
		t.Fatal(err)
	}

	pm.SavePeerstoreForPeers(api.StringsToPeers([]string{pid}))

	pm2 := makeMgr(t)
	defer clean(pm2)

	err = pm2.ImportPeersFromPeerstore(false)
	if err != nil {
		t.Fatal(err)
	}

	if len(pm2.PeersAddresses(api.StringsToPeers([]string{pid}))) != 2 {
		t.Error("expected 2 addresses from the peerstore")
	}
}
