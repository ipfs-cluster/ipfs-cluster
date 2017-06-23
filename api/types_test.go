package api

import (
	"testing"
	"time"

	ma "gx/ipfs/QmSWLfmj5frN9xVLMMN846dMDriy5wN5jeghUm7aTW3DAG/go-multiaddr"
	peer "gx/ipfs/QmZcUPvPhD1Xvk6mwijYF8AfR3mG31S1YsEfHG4khrFPRr/go-libp2p-peer"
	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
)

var testTime = time.Date(2017, 12, 31, 15, 45, 50, 0, time.UTC)
var testMAddr, _ = ma.NewMultiaddr("/ip4/1.2.3.4")
var testCid1, _ = cid.Decode("QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq")
var testPeerID1, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")
var testPeerID2, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabd")

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
		ID:                 testPeerID1,
		Addresses:          []ma.Multiaddr{testMAddr},
		ClusterPeers:       []ma.Multiaddr{testMAddr},
		Version:            "testv",
		Commit:             "ab",
		RPCProtocolVersion: "testp",
		Error:              "teste",
		IPFS: IPFSID{
			ID:        testPeerID2,
			Addresses: []ma.Multiaddr{testMAddr},
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

	if !id.ClusterPeers[0].Equal(newid.ClusterPeers[0]) {
		t.Error("mismatching clusterPeers")
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

func TestMultiaddrConv(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("paniced")
		}
	}()
	addrs := []ma.Multiaddr{testMAddr}
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
