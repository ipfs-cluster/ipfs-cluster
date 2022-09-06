package dsstate

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/inmem"

	peer "github.com/libp2p/go-libp2p/core/peer"
)

var testCid1, _ = api.DecodeCid("QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq")
var testPeerID1, _ = peer.Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")

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

func newState(t *testing.T) *State {
	store := inmem.New()
	ds, err := New(context.Background(), store, "", DefaultHandle())
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
	out := make(chan api.Pin)
	go func() {
		err := st.List(ctx, out)
		if err != nil {
			t.Error(err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var list0 api.Pin
	for {
		select {
		case p, ok := <-out:
			if !ok && !list0.Cid.Defined() {
				t.Fatal("should have read list0 first")
			}
			if !ok {
				return
			}
			list0 = p
			if !p.Equals(c) {
				t.Error("returned something different")
			}
		case <-ctx.Done():
			t.Error("should have read from channel")
			return
		}
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
	if get.Allocations[0] != testPeerID1 {
		t.Error("expected different peer id")
	}
	if !get.Cid.Equals(c.Cid) {
		t.Error("expected different cid")
	}
}
