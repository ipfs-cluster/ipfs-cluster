package test

import (
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
	ErrorCid       = "QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmc"
	TestPeerID1, _ = peer.IDB58Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")
	TestPeerID2, _ = peer.IDB58Decode("QmUZ13osndQ5uL4tPWHXe3iBgBgq9gfewcBMSCAuMBsDJ6")
	TestPeerID3, _ = peer.IDB58Decode("QmPGDFvBkgWhvzEK9qaTWrWurSwqXNmhnK3hgELPdZZNPa")
	TestPeerID4, _ = peer.IDB58Decode("QmZ8naDy5mEz4GLuQwjWt9MPYqHTBbsm8tQBrNSjiq6zBc")
	TestPeerID5, _ = peer.IDB58Decode("QmZVAo3wd8s5eTTy2kPYs34J9PvfxpKPuYsePPYGjgRRjg")
	TestPeerID6, _ = peer.IDB58Decode("QmR8Vu6kZk7JvAN2rWVWgiduHatgBq2bb15Yyq8RRhYSbx")

	TestPeerName1 = "TestPeer1"
	TestPeerName2 = "TestPeer2"
	TestPeerName3 = "TestPeer3"
	TestPeerName4 = "TestPeer4"
	TestPeerName5 = "TestPeer5"
	TestPeerName6 = "TestPeer6"

	TestCid5      = "QmaNJ5acV31sx8jq626qTpAWW4DXKw34aGhx53dECLvXbY"
	TestPathIPFS1 = "/ipfs/QmaNJ5acV31sx8jq626qTpAWW4DXKw34aGhx53dECLvXbY"
	TestPathIPFS2 = "/ipfs/QmbUNM297ZwxB8CfFAznK7H9YMesDoY6Tt5bPgt5MSCB2u/im.gif"
	TestPathIPFS3 = "/ipfs/QmbUNM297ZwxB8CfFAznK7H9YMesDoY6Tt5bPgt5MSCB2u/im.gif/"
	TestPathIPNS1 = "/ipns/QmbmSAQNnfGcBAB8M8AsSPxd1TY7cpT9hZ398kXAScn2Ka"
	TestPathIPNS2 = "/ipns/QmbmSAQNnfGcBAB8M8AsSPxd1TY7cpT9hZ398kXAScn2Ka/"
	TestPathIPLD1 = "/ipld/QmaNJ5acV31sx8jq626qTpAWW4DXKw34aGhx53dECLvXbY"
	TestPathIPLD2 = "/ipld/QmaNJ5acV31sx8jq626qTpAWW4DXKw34aGhx53dECLvXbY/"

	TestInvalidPath1 = "/invalidkeytype/QmaNJ5acV31sx8jq626qTpAWW4DXKw34aGhx53dECLvXbY/"
)

// MustDecodeCid provides a test helper that ignores
// errors from cid.Decode.
func MustDecodeCid(v string) cid.Cid {
	c, _ := cid.Decode(v)
	return c
}
