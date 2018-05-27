// Package test offers testing utilities to ipfs-cluster like
// mocks
package test

import cid "github.com/ipfs/go-cid"

// MustDecodeCid provides a test helper that ignores
// errors from cid.Decode.
func MustDecodeCid(v string) *cid.Cid {
	c, _ := cid.Decode(v)
	return c
}
