// lock logic heavily inspired by go-ipfs/repo/fsrepo/lock/lock.go
package main

import (
	"fmt"
	"io"
	"path"

	filelock "go4.org/lock"
)

// The name of the file used for locking
const lockFileName = "cluster.lock"

var locker *Locker

// Global file, close to relinquish lock access
type Locker struct {
	lockCloser io.Closer
	path       string
}

func (locker *Locker) lock() error {
	if locker.lockCloser != nil {
		return fmt.Errorf("cannot acquire lock twice")
	}
	// set the lock file within this function
	logger.Debug("checking lock")
	lk, err := filelock.Lock(path.Join(locker.path, lockFileName))
	if err != nil {
		logger.Debug(err)
		locker.lockCloser = nil
		errStr := `could not obtain execution lock.  If no other process
is running, remove ~/path/to/lock, or make sure that the config folder is 
writable for the user running ipfs-cluster.  Run with -d for more information
about the error`
		logger.Error(errStr)
		return fmt.Errorf("could not obtain execution lock.")
	}
	logger.Debug("Success! ipfs-cluster-service lock acquired")
	locker.lockCloser = lk
	return nil
}

func (locker *Locker) tryUnlock() error {
	// Noop in the uninitialized case
	if locker.lockCloser == nil {
		logger.Debug("locking not initialized, unlock is noop")
		return nil
	}
	err := locker.lockCloser.Close()
	if err != nil {
		return err
	}
	logger.Debug("Successfully released execution lock")
	locker.lockCloser = nil
	return nil
}
