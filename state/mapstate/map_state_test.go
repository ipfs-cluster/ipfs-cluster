package mapstate

import (
	"testing"

	peer "gx/ipfs/QmZcUPvPhD1Xvk6mwijYF8AfR3mG31S1YsEfHG4khrFPRr/go-libp2p-peer"
	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"

	"github.com/ipfs/ipfs-cluster/api"
)

var testCid1, _ = cid.Decode("QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq")
var testPeerID1, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")

var c = api.Pin{
	Cid:               testCid1,
	Allocations:       []peer.ID{testPeerID1},
	ReplicationFactor: -1,
}

func TestAdd(t *testing.T) {
	ms := NewMapState()
	ms.Add(c)
	if !ms.Has(c.Cid) {
		t.Error("should have added it")
	}
}

func TestRm(t *testing.T) {
	ms := NewMapState()
	ms.Add(c)
	ms.Rm(c.Cid)
	if ms.Has(c.Cid) {
		t.Error("should have removed it")
	}
}

func TestGet(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("paniced")
		}
	}()
	ms := NewMapState()
	ms.Add(c)
	get := ms.Get(c.Cid)
	if get.Cid.String() != c.Cid.String() ||
		get.Allocations[0] != c.Allocations[0] ||
		get.ReplicationFactor != c.ReplicationFactor {
		t.Error("returned something different")
	}
}

func TestList(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("paniced")
		}
	}()
	ms := NewMapState()
	ms.Add(c)
	list := ms.List()
	if list[0].Cid.String() != c.Cid.String() ||
		list[0].Allocations[0] != c.Allocations[0] ||
		list[0].ReplicationFactor != c.ReplicationFactor {
		t.Error("returned something different")
	}
}
