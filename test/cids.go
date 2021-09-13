package test

import (
	cid "github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p-core/peer"
)

// Common variables used all around tests.
var (
	Cid1, _  = cid.Decode("QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmq")
	Cid2, _  = cid.Decode("QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmma")
	Cid3, _  = cid.Decode("QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmb")
	Cid4Data = "Cid4Data"
	// Cid resulting from block put using blake2b-256 and raw format
	Cid4, _ = cid.Decode("bafk2bzaceawsyhsnrwwy5mtit2emnjfalkxsyq2p2ptd6fuliolzwwjbs42fq")

	// Cid resulting from block put using format "v0" defaults
	Cid5, _        = cid.Decode("QmbgmXgsFjxAJ7cEaziL2NDSptHAkPwkEGMmKMpfyYeFXL")
	Cid5Data       = "Cid5Data"
	SlowCid1, _    = cid.Decode("QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmd")
	CidResolved, _ = cid.Decode("zb2rhiKhUepkTMw7oFfBUnChAN7ABAvg2hXUwmTBtZ6yxuabc")
	// ErrorCid is meant to be used as a Cid which causes errors. i.e. the
	// ipfs mock fails when pinning this CID.
	ErrorCid, _ = cid.Decode("QmP63DkAFEnDYNjDYBpyNDfttu1fvUw99x1brscPzpqmmc")
	// NotFoundCid is meant to be used as a CID that doesn't exist in the
	// pinset.
	NotFoundCid, _ = cid.Decode("bafyreiay3jpjk74dkckv2r74eyvf3lfnxujefay2rtuluintasq2zlapv4")
	PeerID1, _     = peer.Decode("QmXZrtE5jQwXNqCJMfHUTQkvhQ4ZAnqMnmzFMJfLewuabc")
	PeerID2, _     = peer.Decode("QmUZ13osndQ5uL4tPWHXe3iBgBgq9gfewcBMSCAuMBsDJ6")
	PeerID3, _     = peer.Decode("QmPGDFvBkgWhvzEK9qaTWrWurSwqXNmhnK3hgELPdZZNPa")
	PeerID4, _     = peer.Decode("QmZ8naDy5mEz4GLuQwjWt9MPYqHTBbsm8tQBrNSjiq6zBc")
	PeerID5, _     = peer.Decode("QmZVAo3wd8s5eTTy2kPYs34J9PvfxpKPuYsePPYGjgRRjg")
	PeerID6, _     = peer.Decode("QmR8Vu6kZk7JvAN2rWVWgiduHatgBq2bb15Yyq8RRhYSbx")
	PeerID7, _     = peer.Decode("12D3KooWGHTKzeT4KaLGLrbKKyT8zKrBPXAUBRzCAN6ZMDMo4M6M")
	PeerID8, _     = peer.Decode("12D3KooWFBFCDQzAkQSwPZLV883pKdsmb6urQ3sMjfJHUxn5GCVv")
	PeerID9, _     = peer.Decode("12D3KooWKuJ8LPTyHbyX4nt4C7uWmUobzFsiceTVoFw7HpmoNakM")

	PeerName1 = "TestPeer1"
	PeerName2 = "TestPeer2"
	PeerName3 = "TestPeer3"
	PeerName4 = "TestPeer4"
	PeerName5 = "TestPeer5"
	PeerName6 = "TestPeer6"

	PathIPFS1 = "/ipfs/QmaNJ5acV31sx8jq626qTpAWW4DXKw34aGhx53dECLvXbY"
	PathIPFS2 = "/ipfs/QmbUNM297ZwxB8CfFAznK7H9YMesDoY6Tt5bPgt5MSCB2u/im.gif"
	PathIPFS3 = "/ipfs/QmbUNM297ZwxB8CfFAznK7H9YMesDoY6Tt5bPgt5MSCB2u/im.gif/"
	PathIPNS1 = "/ipns/QmbmSAQNnfGcBAB8M8AsSPxd1TY7cpT9hZ398kXAScn2Ka"
	PathIPNS2 = "/ipns/QmbmSAQNnfGcBAB8M8AsSPxd1TY7cpT9hZ398kXAScn2Ka/"
	PathIPLD1 = "/ipld/QmaNJ5acV31sx8jq626qTpAWW4DXKw34aGhx53dECLvXbY"
	PathIPLD2 = "/ipld/QmaNJ5acV31sx8jq626qTpAWW4DXKw34aGhx53dECLvXbY/"

	// NotFoundPath is meant to be used as a path that resolves into a CID that doesn't exist in the
	// pinset.
	NotFoundPath = "/ipfs/bafyreiay3jpjk74dkckv2r74eyvf3lfnxujefay2rtuluintasq2zlapv4"
	InvalidPath1 = "/invalidkeytype/QmaNJ5acV31sx8jq626qTpAWW4DXKw34aGhx53dECLvXbY/"
	InvalidPath2 = "/ipfs/invalidhash"
	InvalidPath3 = "/ipfs/"
)
