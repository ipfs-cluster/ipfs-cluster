package api

import (
	"bytes"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/ugorji/go/codec"
)

var testTime = time.Date(2017, 12, 31, 15, 45, 50, 0, time.UTC)
var testMAddr, _ = ma.NewMultiaddr("/ip4/1.2.3.4")
var testMAddr2, _ = ma.NewMultiaddr("/dns4/a.b.c.d")
var testMAddr3, _ = ma.NewMultiaddr("/ip4/127.0.0.1/tcp/8081/ws/p2p/QmSoLer265NRgSp2LA3dPaeykiS1J6DifTC88f5uVQKNAd")
var testCid1, _ = cid.Decode("QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq")
var testCid2, _ = cid.Decode("QmYCLpFCj9Av8NFjkQogvtXspnTDFWaizLpVFEijHTH4eV")
var testCid3, _ = cid.Decode("QmZmdA3UZKuHuy9FrWsxJ82q21nbEh97NUnxTzF5EHxZia")
var testCid4, _ = cid.Decode("QmZbNfi13Sb2WUDMjiW1ZNhnds5KDk6mJB5hP9B5h9m5CJ")
var testPeerID1, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")
var testPeerID2, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabd")
var testPeerID3, _ = peer.IDB58Decode("QmPGDFvBkgWhvzEK9qaTWrWurSwqXNmhnK3hgELPdZZNPa")
var testPeerID4, _ = peer.IDB58Decode("QmZ8naDy5mEz4GLuQwjWt9MPYqHTBbsm8tQBrNSjiq6zBc")
var testPeerID5, _ = peer.IDB58Decode("QmZVAo3wd8s5eTTy2kPYs34J9PvfxpKPuYsePPYGjgRRjg")
var testPeerID6, _ = peer.IDB58Decode("QmR8Vu6kZk7JvAN2rWVWgiduHatgBq2bb15Yyq8RRhYSbx")

func TestTrackerFromString(t *testing.T) {
	testcases := []string{"cluster_error", "pin_error", "unpin_error", "pinned", "pinning", "unpinning", "unpinned", "remote"}
	for i, tc := range testcases {
		if TrackerStatusFromString(tc).String() != TrackerStatus(1<<uint(i+1)).String() {
			t.Errorf("%s does not match  TrackerStatus %d", tc, i)
		}
	}

	if TrackerStatusFromString("") != TrackerStatusUndefined ||
		TrackerStatusFromString("xyz") != TrackerStatusUndefined {
		t.Error("expected tracker status undefined for bad strings")
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

func BenchmarkIPFSPinStatusFromString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		IPFSPinStatusFromString("indirect")
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

	m.SetTTL(1 * time.Second)
	if m.Expired() {
		t.Error("metric should not be expired")
	}

	// let it expire
	time.Sleep(1500 * time.Millisecond)

	if !m.Expired() {
		t.Error("metric should be expired")
	}

	m.SetTTL(30 * time.Second)
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

func TestConvertPinType(t *testing.T) {
	for _, t1 := range []PinType{BadType, ShardType} {
		i := convertPinType(t1)
		t2 := PinType(1 << uint64(i))
		if t2 != t1 {
			t.Error("bad conversion")
		}
	}
}

func checkDupTags(t *testing.T, name string, typ reflect.Type, tags map[string]struct{}) {
	if tags == nil {
		tags = make(map[string]struct{})
	}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)

		if f.Type.Kind() == reflect.Struct && f.Anonymous {
			checkDupTags(t, name, f.Type, tags)
			continue
		}

		tag := f.Tag.Get(name)
		if tag == "" {
			continue
		}
		val := strings.Split(tag, ",")[0]

		t.Logf("%s: '%s:%s'", f.Name, name, val)
		_, ok := tags[val]
		if ok {
			t.Errorf("%s: tag %s already used", f.Name, val)
		}
		tags[val] = struct{}{}
	}
}

// TestDupTags checks that we are not re-using the same codec tag for
// different fields in the types objects.
func TestDupTags(t *testing.T) {
	typ := reflect.TypeOf(Pin{})
	checkDupTags(t, "codec", typ, nil)

	typ = reflect.TypeOf(ID{})
	checkDupTags(t, "codec", typ, nil)

	typ = reflect.TypeOf(GlobalPinInfo{})
	checkDupTags(t, "codec", typ, nil)

	typ = reflect.TypeOf(PinInfo{})
	checkDupTags(t, "codec", typ, nil)

	typ = reflect.TypeOf(ConnectGraph{})
	checkDupTags(t, "codec", typ, nil)

	typ = reflect.TypeOf(ID{})
	checkDupTags(t, "codec", typ, nil)

	typ = reflect.TypeOf(NodeWithMeta{})
	checkDupTags(t, "codec", typ, nil)

	typ = reflect.TypeOf(Metric{})
	checkDupTags(t, "codec", typ, nil)

	typ = reflect.TypeOf(Error{})
	checkDupTags(t, "codec", typ, nil)

	typ = reflect.TypeOf(IPFSRepoStat{})
	checkDupTags(t, "codec", typ, nil)

	typ = reflect.TypeOf(AddedOutput{})
	checkDupTags(t, "codec", typ, nil)
}

func TestPinOptionsQuery(t *testing.T) {
	testcases := []*PinOptions{
		&PinOptions{
			ReplicationFactorMax: 3,
			ReplicationFactorMin: 2,
			Name:                 "abc",
			ShardSize:            33,
			UserAllocations: StringsToPeers([]string{
				"QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc",
				"QmUZ13osndQ5uL4tPWHXe3iBgBgq9gfewcBMSCAuMBsDJ6",
			}),
			ExpireAt: time.Now().Add(12 * time.Hour),
			Metadata: map[string]string{
				"hello":  "bye",
				"hello2": "bye2",
			},
		},
		&PinOptions{
			ReplicationFactorMax: -1,
			ReplicationFactorMin: 0,
			Name:                 "",
			ShardSize:            0,
			UserAllocations:      []peer.ID{},
			Metadata:             nil,
		},
		&PinOptions{
			ReplicationFactorMax: -1,
			ReplicationFactorMin: 0,
			Name:                 "",
			ShardSize:            0,
			UserAllocations:      nil,
			Metadata: map[string]string{
				"": "bye",
			},
		},
	}

	for _, tc := range testcases {
		queryStr, err := tc.ToQuery()
		if err != nil {
			t.Fatal("error converting to query", err)
		}
		q, err := url.ParseQuery(queryStr)
		if err != nil {
			t.Error("error parsing query", err)
		}
		po2 := &PinOptions{}
		po2.FromQuery(q)
		if !tc.Equals(po2) {
			t.Error("expected equal PinOptions")
			t.Error(queryStr)
			t.Errorf("%+v\n", tc)
			t.Errorf("%+v\n", po2)
		}
	}
}

func TestIDCodec(t *testing.T) {
	TestPeerID1, _ := peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")
	TestPeerID2, _ := peer.IDB58Decode("QmUZ13osndQ5uL4tPWHXe3iBgBgq9gfewcBMSCAuMBsDJ6")
	TestPeerID3, _ := peer.IDB58Decode("QmPGDFvBkgWhvzEK9qaTWrWurSwqXNmhnK3hgELPdZZNPa")
	addr, _ := NewMultiaddr("/ip4/1.2.3.4")
	id := &ID{
		ID:                    TestPeerID1,
		Addresses:             []Multiaddr{addr},
		ClusterPeers:          []peer.ID{TestPeerID2},
		ClusterPeersAddresses: []Multiaddr{addr},
		Version:               "2",
		Commit:                "",
		RPCProtocolVersion:    "abc",
		Error:                 "",
		IPFS: &IPFSID{
			ID:        TestPeerID3,
			Addresses: []Multiaddr{addr},
			Error:     "",
		},
		Peername: "hi",
	}

	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf, &codec.MsgpackHandle{})
	err := enc.Encode(id)
	if err != nil {
		t.Fatal(err)
	}

	var buf2 = bytes.NewBuffer(buf.Bytes())
	dec := codec.NewDecoder(buf2, &codec.MsgpackHandle{})

	var id2 ID

	err = dec.Decode(&id2)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPinCodec(t *testing.T) {
	ci, _ := cid.Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")
	pin := PinCid(ci)
	var buf bytes.Buffer
	enc := codec.NewEncoder(&buf, &codec.MsgpackHandle{})
	err := enc.Encode(pin)
	if err != nil {
		t.Fatal(err)
	}

	var buf2 = bytes.NewBuffer(buf.Bytes())
	dec := codec.NewDecoder(buf2, &codec.MsgpackHandle{})

	var pin2 Pin

	err = dec.Decode(&pin2)
	if err != nil {
		t.Fatal(err)
	}

}
