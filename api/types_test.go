package api

import (
	"reflect"
	"testing"
	"time"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
	ma "github.com/multiformats/go-multiaddr"
)

var testTime = time.Date(2017, 12, 31, 15, 45, 50, 0, time.UTC)
var testMAddr, _ = ma.NewMultiaddr("/ip4/1.2.3.4")
var testMAddr2, _ = ma.NewMultiaddr("/dns4/a.b.c.d")
var testMAddr3, _ = ma.NewMultiaddr("/ip4/127.0.0.1/tcp/8081/ws/ipfs/QmSoLer265NRgSp2LA3dPaeykiS1J6DifTC88f5uVQKNAd")
var testCid1, _ = cid.Decode("QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq")
var testPeerID1, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")
var testPeerID2, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabd")
var testPeerID3, _ = peer.IDB58Decode("QmPGDFvBkgWhvzEK9qaTWrWurSwqXNmhnK3hgELPdZZNPa")
var testPeerID4, _ = peer.IDB58Decode("QmZ8naDy5mEz4GLuQwjWt9MPYqHTBbsm8tQBrNSjiq6zBc")
var testPeerID5, _ = peer.IDB58Decode("QmZVAo3wd8s5eTTy2kPYs34J9PvfxpKPuYsePPYGjgRRjg")
var testPeerID6, _ = peer.IDB58Decode("QmR8Vu6kZk7JvAN2rWVWgiduHatgBq2bb15Yyq8RRhYSbx")

func TestTrackerFromString(t *testing.T) {
	testcases := []string{"bug", "cluster_error", "pin_error", "unpin_error", "pinned", "pinning", "unpinning", "unpinned", "remote"}
	for i, tc := range testcases {
		if TrackerStatusFromString(tc).String() != TrackerStatus(i).String() {
			t.Errorf("%s does not match  TrackerStatus %d", tc, i)
		}
	}
}

func TestIPFSPinStatusFromString(t *testing.T) {
	testcases := []string{"direct", "recursive", "indirect"}
	for i, tc := range testcases {
		if IPFSPinStatusFromString(tc) != IPFSPinStatus(i+2) {
			t.Errorf("%s does not match IPFSPinStatus %d", tc, i+2)
		}
	}
}

func TestGlobalPinInfoConv(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("paniced")
		}
	}()

	gpi := GlobalPinInfo{
		Cid: testCid1,
		PeerMap: map[peer.ID]PinInfo{
			testPeerID1: {
				Cid:    testCid1,
				Peer:   testPeerID1,
				Status: TrackerStatusPinned,
				TS:     testTime,
			},
		},
	}

	newgpi := gpi.ToSerial().ToGlobalPinInfo()
	if gpi.Cid.String() != newgpi.Cid.String() {
		t.Error("mismatching CIDs")
	}
	if gpi.PeerMap[testPeerID1].Cid.String() != newgpi.PeerMap[testPeerID1].Cid.String() {
		t.Error("mismatching PinInfo CIDs")
	}

	if !gpi.PeerMap[testPeerID1].TS.Equal(newgpi.PeerMap[testPeerID1].TS) {
		t.Error("bad time")
	}
}

func TestIDConv(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("paniced")
		}
	}()

	id := ID{
		ID:                    testPeerID1,
		Addresses:             []ma.Multiaddr{testMAddr},
		ClusterPeers:          []peer.ID{testPeerID2},
		ClusterPeersAddresses: []ma.Multiaddr{testMAddr2},
		Version:               "testv",
		Commit:                "ab",
		RPCProtocolVersion:    "testp",
		Error:                 "teste",
		IPFS: IPFSID{
			ID:        testPeerID2,
			Addresses: []ma.Multiaddr{testMAddr3},
			Error:     "abc",
		},
	}

	newid := id.ToSerial().ToID()

	if id.ID != newid.ID {
		t.Error("mismatching Peer IDs")
	}

	if !id.Addresses[0].Equal(newid.Addresses[0]) {
		t.Error("mismatching addresses")
	}

	if id.ClusterPeers[0] != newid.ClusterPeers[0] {
		t.Error("mismatching clusterPeers")
	}

	if !id.ClusterPeersAddresses[0].Equal(newid.ClusterPeersAddresses[0]) {
		t.Error("mismatching clusterPeersAddresses")
	}

	if id.Version != newid.Version ||
		id.Commit != newid.Commit ||
		id.RPCProtocolVersion != newid.RPCProtocolVersion ||
		id.Error != newid.Error {
		t.Error("some field didn't survive")
	}

	if id.IPFS.ID != newid.IPFS.ID {
		t.Error("ipfs daemon id mismatch")
	}

	if !id.IPFS.Addresses[0].Equal(newid.IPFS.Addresses[0]) {
		t.Error("mismatching addresses")
	}
	if id.IPFS.Error != newid.IPFS.Error {
		t.Error("ipfs error mismatch")
	}
}

func TestConnectGraphConv(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("paniced")
		}
	}()
	cg := ConnectGraph{
		ClusterID: testPeerID1,
		IPFSLinks: map[peer.ID][]peer.ID{
			testPeerID4: []peer.ID{testPeerID5, testPeerID6},
			testPeerID5: []peer.ID{testPeerID4, testPeerID6},
			testPeerID6: []peer.ID{testPeerID4, testPeerID5},
		},
		ClusterLinks: map[peer.ID][]peer.ID{
			testPeerID1: []peer.ID{testPeerID2, testPeerID3},
			testPeerID2: []peer.ID{testPeerID1, testPeerID3},
			testPeerID3: []peer.ID{testPeerID1, testPeerID2},
		},
		ClustertoIPFS: map[peer.ID]peer.ID{
			testPeerID1: testPeerID4,
			testPeerID2: testPeerID5,
			testPeerID3: testPeerID6,
		},
	}

	cgNew := cg.ToSerial().ToConnectGraph()
	if !reflect.DeepEqual(cg, cgNew) {
		t.Fatal("The new connect graph should be equivalent to the old")
	}
}

func TestMultiaddrConv(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("paniced")
		}
	}()
	addrs := []ma.Multiaddr{testMAddr2}
	new := MultiaddrsToSerial(addrs).ToMultiaddrs()
	if !addrs[0].Equal(new[0]) {
		t.Error("mismatch")
	}
}

func TestPinConv(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("paniced")
		}
	}()

	c := Pin{
		Cid:               testCid1,
		Allocations:       []peer.ID{testPeerID1},
		ReplicationFactor: -1,
	}

	newc := c.ToSerial().ToPin()
	if c.Cid.String() != newc.Cid.String() ||
		c.Allocations[0] != newc.Allocations[0] ||
		c.ReplicationFactor != newc.ReplicationFactor {
		t.Error("mismatch")
	}
}

func TestMetric(t *testing.T) {
	m := Metric{
		Name:  "hello",
		Value: "abc",
	}

	if !m.Expired() {
		t.Error("metric should be expired")
	}

	m.SetTTL(1)
	if m.Expired() {
		t.Error("metric should not be expired")
	}

	// let it expire
	time.Sleep(1500 * time.Millisecond)

	if !m.Expired() {
		t.Error("metric should be expired")
	}

	m.SetTTL(30)
	m.Valid = true

	if m.Discard() {
		t.Error("metric should be valid")
	}

	m.Valid = false
	if !m.Discard() {
		t.Error("metric should be invalid")
	}

	ttl := m.GetTTL()
	if ttl > 30*time.Second || ttl < 29*time.Second {
		t.Error("looks like a bad ttl")
	}
}
