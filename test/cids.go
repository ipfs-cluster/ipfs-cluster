package test

import (
	"encoding/hex"

	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-peer"
)

// Common variables used all around tests.
var (
	TestCid1     = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq"
	TestCid2     = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmma"
	TestCid3     = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmb"
	TestCid4     = "zb2rhiKhUepkTMw7oFfBUnChAN7ABAvg2hXUwmTBtZ6yxuc57"
	TestCid4Data = "Cid4Data" // Cid resulting from block put NOT ipfs add
	TestSlowCid1 = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmd"
	// ErrorCid is meant to be used as a Cid which causes errors. i.e. the
	// ipfs mock fails when pinning this CID.
	ErrorCid = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmc"
	// Shard and Cdag Cids
	TestShardCid     = "zdpuAoiNm1ntWx6jpgcReTiCWFHJSTpvTw4bAAn9p6yDnznqh"
	TestShardCid2    = "zdpuAmUorxmxhrk96mVxQTuUi6QioKzKQKK8XvzU5WURU4Qea"
	TestCdagCid      = "zdpuAyVKsP6xvx1p81pKi7faxUs2GuD2ZG4o3CwMycvCLyuhK"
	TestCdagCid2     = "zdpuAynm14qkpVPMMazNjkz3nJYhtTXJ3TpRp5aEkoMHBwmKc"
	TestMetaRootCid  = "QmYCLpFCj9Av8NFjkQogvtXspnTDFWaizLpVFEijHTH4eV"
	TestMetaRootCid2 = "QmUatEiCFxtckNae8XyDVGwL1WZq8cVKTMacDkqz8zAE3q"

	TestShardData, _  = hex.DecodeString("a16130d82a58230012209273fd63ec94bed5abb219b2d9cb010cabe4af7b0177292d4335eff50464060a")
	TestShard2Data, _ = hex.DecodeString("a26130d82a5823001220a736215b7487e753686cc4e965ca62dc45d46303ef35349aee69a647c22ac57f6131d82a58230012205ccb8c75199ea2b1ef52a03ecaaa3186dd04fe0f8133aa0b2d5195c3c0844a10")
	TestCdagData, _   = hex.DecodeString("a16130d82a5825000171122030e9b9b4f1bc4b5a3759a93b4e77983cd053f84174e1b0cd628dc6c32fb0da14")
	TestCdagData2, _  = hex.DecodeString("a26130d82a582500017112200fb89a9189514be1d5418e0890367992b651af390f06d3751c672cba6c00b4276131d82a5825000171122030e9b9b4f1bc4b5a3759a93b4e77983cd053f84174e1b0cd628dc6c32fb0da14")

	TestPeerID1, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")
	TestPeerID2, _ = peer.IDB58Decode("QmUZ13osndQ5uL4tPWHXe3iBgBgq9gfewcBMSCAuMBsDJ6")
	TestPeerID3, _ = peer.IDB58Decode("QmPGDFvBkgWhvzEK9qaTWrWurSwqXNmhnK3hgELPdZZNPa")
	TestPeerID4, _ = peer.IDB58Decode("QmZ8naDy5mEz4GLuQwjWt9MPYqHTBbsm8tQBrNSjiq6zBc")
	TestPeerID5, _ = peer.IDB58Decode("QmZVAo3wd8s5eTTy2kPYs34J9PvfxpKPuYsePPYGjgRRjg")
	TestPeerID6, _ = peer.IDB58Decode("QmR8Vu6kZk7JvAN2rWVWgiduHatgBq2bb15Yyq8RRhYSbx")
)

// MustDecodeCid provides a test helper that ignores
// errors from cid.Decode.
func MustDecodeCid(v string) *cid.Cid {
	c, _ := cid.Decode(v)
	return c
}
