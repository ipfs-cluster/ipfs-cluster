package dsstate

import (
	"bytes"
	"context"
	"testing"

	"github.com/ipfs/ipfs-cluster/api"
	"github.com/ipfs/ipfs-cluster/datastore/inmem"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-core/peer"
)

var testCid1, _ = cid.Decode("QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq")
var testPeerID1, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")

var c = &api.Pin{
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

func newState(t *testing.T) *State {
	store := inmem.New()
	ds, err := New(store, "", DefaultHandle())
	if err != nil {
		t.Fatal(err)
	}
	return ds
}

func TestAdd(t *testing.T) {
	ctx := context.Background()
	st := newState(t)
	st.Add(ctx, c)
	if ok, err := st.Has(ctx, c.Cid); !ok || err != nil {
		t.Error("should have added it")
	}
}

func TestRm(t *testing.T) {
	ctx := context.Background()
	st := newState(t)
	st.Add(ctx, c)
	st.Rm(ctx, c.Cid)
	if ok, err := st.Has(ctx, c.Cid); ok || err != nil {
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
	st := newState(t)
	st.Add(ctx, c)
	get, err := st.Get(ctx, c.Cid)
	if err != nil {
		t.Fatal(err)
	}

	if get == nil {
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
	st := newState(t)
	st.Add(ctx, c)
	list, err := st.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Cid.String() != c.Cid.String() ||
		list[0].Allocations[0] != c.Allocations[0] ||
		list[0].ReplicationFactorMax != c.ReplicationFactorMax ||
		list[0].ReplicationFactorMin != c.ReplicationFactorMin {
		t.Error("returned something different")
	}
}

func TestMarshalUnmarshal(t *testing.T) {
	ctx := context.Background()
	st := newState(t)
	st.Add(ctx, c)
	buf := new(bytes.Buffer)
	err := st.Marshal(buf)
	if err != nil {
		t.Fatal(err)
	}
	st2 := newState(t)
	err = st2.Unmarshal(buf)
	if err != nil {
		t.Fatal(err)
	}

	get, err := st2.Get(ctx, c.Cid)
	if err != nil {
		t.Fatal(err)
	}
	if get == nil {
		t.Fatal("cannot get pin")
	}
	if get.Allocations[0] != testPeerID1 {
		t.Error("expected different peer id")
	}
	if !get.Cid.Equals(c.Cid) {
		t.Error("expected different cid")
	}
}
