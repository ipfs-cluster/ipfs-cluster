// Package pebble provides a configurable Pebble database backend for use with
// IPFS Cluster.
package pebble

import (
	"context"
	"os"
	"time"

	"github.com/cockroachdb/pebble"
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

	// Deal with Pebble updates... user should try to be up to date with
	// latest Pebble table formats.
	fmv := cfg.PebbleOptions.FormatMajorVersion
	newest := pebble.FormatNewest
	if fmv < newest {
		logger.Warnf(`Pebble's format_major_version is set to %d, but newest version is %d.

It is recommended to increase format_major_version and restart. If an error
occurrs when increasing the number several versions at once, it may help to
increase them one by one, restarting the daemon every time.
`, fmv, newest)
	}

	db, err := pebbleds.NewDatastore(folder, &cfg.PebbleOptions)
	if err != nil {
		return nil, err
	}

	// Calling regularly DB's DiskUsage is a way to printout debug
	// database statistics.
	go func() {
		ctx := context.Background()
		db.DiskUsage(ctx)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			<-ticker.C
			db.DiskUsage(ctx)
		}
	}()
	return db, nil
}

// Cleanup deletes the badger datastore.
func Cleanup(cfg *Config) error {
	folder := cfg.GetFolder()
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(cfg.GetFolder())
}
