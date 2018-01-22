package mapstate

import (
	"bytes"
	"testing"

	msgpack "github.com/multiformats/go-multicodec/msgpack"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
)

var testCid1, _ = cid.Decode("QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq")
var testPeerID1, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")

var c = api.Pin{
	Cid:                  testCid1,
	Allocations:          []peer.ID{testPeerID1},
	ReplicationFactorMax: -1,
	ReplicationFactorMin: -1,
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
		get.ReplicationFactorMax != c.ReplicationFactorMax ||
		get.ReplicationFactorMin != c.ReplicationFactorMin {
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
		list[0].ReplicationFactorMax != c.ReplicationFactorMax ||
		list[0].ReplicationFactorMin != c.ReplicationFactorMin {
		t.Error("returned something different")
	}
}

func TestMarshalUnmarshal(t *testing.T) {
	ms := NewMapState()
	ms.Add(c)
	b, err := ms.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	ms2 := NewMapState()
	err = ms2.Unmarshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if ms.Version != ms2.Version {
		t.Fatal(err)
	}
	get := ms2.Get(c.Cid)
	if get.Allocations[0] != testPeerID1 {
		t.Error("expected different peer id")
	}
}

func TestMigrateFromV1(t *testing.T) {
	// Construct the bytes of a v1 state
	var v1State mapStateV1
	v1State.PinMap = map[string]struct{}{
		c.Cid.String(): {}}
	v1State.Version = 1
	buf := new(bytes.Buffer)
	enc := msgpack.Multicodec(msgpack.DefaultMsgpackHandle()).Encoder(buf)
	err := enc.Encode(v1State)
	if err != nil {
		t.Fatal(err)
	}
	vCodec := make([]byte, 1)
	vCodec[0] = byte(v1State.Version)
	v1Bytes := append(vCodec, buf.Bytes()...)

	// Unmarshal first to check this is v1
	ms := NewMapState()
	err = ms.Unmarshal(v1Bytes)
	if err != nil {
		t.Error(err)
	}
	if ms.Version != 1 {
		t.Error("unmarshal picked up the wrong version")
	}
	// Migrate state to current version
	r := bytes.NewBuffer(v1Bytes)
	err = ms.Migrate(r)
	if err != nil {
		t.Error(err)
	}
	get := ms.Get(c.Cid)
	if get.ReplicationFactorMax != -1 || get.ReplicationFactorMin != -1 || !get.Cid.Equals(c.Cid) {
		t.Error("expected something different")
		t.Logf("%+v", get)
	}
}
