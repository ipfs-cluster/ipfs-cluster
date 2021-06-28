// Package leveldb provides a configurable LevelDB go-datastore for use with
// IPFS Cluster.
package leveldb

import (
	"os"

	ds "github.com/ipfs/go-datastore"
	leveldbds "github.com/ipfs/go-ds-leveldb"
	"github.com/pkg/errors"
)

// New returns a LevelDB datastore configured with the given
// configuration.
func New(cfg *Config) (ds.Datastore, error) {
	folder := cfg.GetFolder()
	err := os.MkdirAll(folder, 0700)
	if err != nil {
		return nil, errors.Wrap(err, "creating leveldb folder")
	}
	return leveldbds.NewDatastore(folder, (*leveldbds.Options)(&cfg.LevelDBOptions))
}

// Cleanup deletes the leveldb datastore.
func Cleanup(cfg *Config) error {
	folder := cfg.GetFolder()
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(cfg.GetFolder())

}
