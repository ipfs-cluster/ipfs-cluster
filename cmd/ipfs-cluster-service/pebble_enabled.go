//go:build !arm && !386 && !(openbsd && amd64)

package main

const (
	defaultDatastore   = "pebble"
	datastoreFlagUsage = "select datastore: 'badger', 'badger3', 'leveldb' or 'pebble'"
)
