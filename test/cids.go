package test

import peer "github.com/libp2p/go-libp2p-peer"

// Common variables used all around tests.
var (
	TestCid1 = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq"
	TestCid2 = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmma"
	TestCid3 = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmb"
	// ErrorCid is meant to be used as a Cid which causes errors. i.e. the
	// ipfs mock fails when pinning this CID.
	ErrorCid       = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmc"
	TestPeerID1, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")
	TestPeerID2, _ = peer.IDB58Decode("QmUZ13osndQ5uL4tPWHXe3iBgBgq9gfewcBMSCAuMBsDJ6")
	TestPeerID3, _ = peer.IDB58Decode("QmPGDFvBkgWhvzEK9qaTWrWurSwqXNmhnK3hgELPdZZNPa")
	TestPeerID4, _ = peer.IDB58Decode("QmZ8naDy5mEz4GLuQwjWt9MPYqHTBbsm8tQBrNSjiq6zBc")
	TestPeerID5, _ = peer.IDB58Decode("QmZVAo3wd8s5eTTy2kPYs34J9PvfxpKPuYsePPYGjgRRjg")
	TestPeerID6, _ = peer.IDB58Decode("QmR8Vu6kZk7JvAN2rWVWgiduHatgBq2bb15Yyq8RRhYSbx")
)
