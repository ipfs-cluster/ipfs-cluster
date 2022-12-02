// Package pebble provides a configurable Pebble database backend for use with
// IPFS Cluster.
package pebble

import (
	"os"

	ds "github.com/ipfs/go-datastore"
	pebbleds "github.com/ipfs/go-ds-pebble"
	logging "github.com/ipfs/go-log/v2"
	"github.com/pkg/errors"
)

var logger = logging.Logger("pebble")

// New returns a BadgerDB datastore configured with the given
// configuration.
func New(cfg *Config) (ds.Datastore, error) {
	folder := cfg.GetFolder()
	err := os.MkdirAll(folder, 0700)
	if err != nil {
		return nil, errors.Wrap(err, "creating pebble folder")
	}
	return pebbleds.NewDatastore(folder, &cfg.PebbleOptions)
}

// Cleanup deletes the badger datastore.
func Cleanup(cfg *Config) error {
	folder := cfg.GetFolder()
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(cfg.GetFolder())
}
