package mapstate

import (
	"bytes"
	"fmt"
	"testing"

	cid "gx/ipfs/QmcTcsTvfaeEBRFo1TkFgT8sRmgi1n1LTZpecfVP8fzpGD/go-cid"
	peer "gx/ipfs/QmdS9KpbDyPrieswibZhkod1oXqRwZJrUPzxCofAMWpFGq/go-libp2p-peer"

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

func TestSnapshotRestore(t *testing.T) {
	ms := NewMapState()
	ms.Add(c)
	var buf bytes.Buffer
	err := ms.Snapshot(&buf)
	if err != nil {
		t.Fatal(err)
	}
	ms2 := NewMapState()
	err = ms2.Restore(&buf)
	if err != nil {
		t.Fatal(err)
	}
	get := ms2.Get(c.Cid)
	if get.Allocations[0] != testPeerID1 {
		t.Error("expected different peer id")
	}
}

func TestMigrateFromV1(t *testing.T) {
	v1 := []byte(fmt.Sprintf(`{
  "Version": 1,
  "PinMap": {
    "%s": {}
   }
}
`, c.Cid))
	buf := bytes.NewBuffer(v1)
	ms := NewMapState()
	err := ms.Restore(buf)
	if err != nil {
		t.Fatal(err)
	}
	get := ms.Get(c.Cid)
	if get.ReplicationFactor != -1 || !get.Cid.Equals(c.Cid) {
		t.Error("expected something different")
		t.Logf("%+v", get)
	}
}
