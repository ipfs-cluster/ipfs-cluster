package mapstate

import (
	"bytes"
	"context"
	"testing"

	msgpack "github.com/multiformats/go-multicodec/msgpack"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/ipfs/ipfs-cluster/api"
)

var testCid1, _ = cid.Decode("QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq")
var testPeerID1, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")

var c = api.Pin{
	Cid:         testCid1,
	Type:        api.DataType,
	Allocations: []peer.ID{testPeerID1},
	MaxDepth:    -1,
	PinOptions: api.PinOptions{
		ReplicationFactorMax: -1,
		ReplicationFactorMin: -1,
		Name:                 "test",
	},
}

func TestAdd(t *testing.T) {
	ctx := context.Background()
	ms := NewMapState()
	ms.Add(ctx, c)
	if !ms.Has(ctx, c.Cid) {
		t.Error("should have added it")
	}
}

func TestRm(t *testing.T) {
	ctx := context.Background()
	ms := NewMapState()
	ms.Add(ctx, c)
	ms.Rm(ctx, c.Cid)
	if ms.Has(ctx, c.Cid) {
		t.Error("should have removed it")
	}
}

func TestGet(t *testing.T) {
	ctx := context.Background()
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("paniced")
		}
	}()
	ms := NewMapState()
	ms.Add(ctx, c)
	get, ok := ms.Get(ctx, c.Cid)
	if !ok {
		t.Fatal("not found")
	}
	if get.Cid.String() != c.Cid.String() {
		t.Error("bad cid decoding: ", get.Cid)
	}

	if get.Allocations[0] != c.Allocations[0] {
		t.Error("bad allocations decoding:", get.Allocations)
	}

	if get.ReplicationFactorMax != c.ReplicationFactorMax ||
		get.ReplicationFactorMin != c.ReplicationFactorMin {
		t.Error("bad replication factors decoding")
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("paniced")
		}
	}()
	ms := NewMapState()
	ms.Add(ctx, c)
	list := ms.List(ctx)
	if list[0].Cid.String() != c.Cid.String() ||
		list[0].Allocations[0] != c.Allocations[0] ||
		list[0].ReplicationFactorMax != c.ReplicationFactorMax ||
		list[0].ReplicationFactorMin != c.ReplicationFactorMin {
		t.Error("returned something different")
	}
}

func TestMarshalUnmarshal(t *testing.T) {
	ctx := context.Background()
	ms := NewMapState()
	ms.Add(ctx, c)
	buf := new(bytes.Buffer)
	err := ms.Marshal(buf)
	if err != nil {
		t.Fatal(err)
	}
	ms2 := NewMapState()
	err = ms2.Unmarshal(buf)
	if err != nil {
		t.Fatal(err)
	}
	if ms.GetVersion() != ms2.GetVersion() {
		t.Fatal(err)
	}
	get, ok := ms2.Get(ctx, c.Cid)
	if !ok {
		t.Fatal("cannot get pin")
	}
	if get.Allocations[0] != testPeerID1 {
		t.Error("expected different peer id")
	}
	if !get.Cid.Equals(c.Cid) {
		t.Error("expected different cid")
	}
}

func TestMigrateFromV1(t *testing.T) {
	ctx := context.Background()
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

	buf2 := bytes.NewBuffer(v1Bytes)
	// Unmarshal first to check this is v1
	ms := NewMapState()
	err = ms.Unmarshal(buf2)
	if err != nil {
		t.Error(err)
	}
	if ms.GetVersion() != 1 {
		t.Error("unmarshal picked up the wrong version")
	}
	// Migrate state to current version
	r := bytes.NewBuffer(v1Bytes)
	err = ms.Migrate(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	get, ok := ms.Get(ctx, c.Cid)
	if !ok {
		t.Fatal("migrated state does not contain cid")
	}
	if get.ReplicationFactorMax != -1 || get.ReplicationFactorMin != -1 || !get.Cid.Equals(c.Cid) {
		t.Error("expected something different")
		t.Logf("%+v", get)
	}
}
