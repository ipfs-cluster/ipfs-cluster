//go:build arm || 386 || (openbsd && amd64)

package main

const (
	defaultDatastore   = "badger3"
	datastoreFlagUsage = "select datastore: 'badger', 'badger3' or 'leveldb'"
)
