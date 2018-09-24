package main

import (
	"fmt"
	"io"
	"os"
	"path"

	fslock "github.com/ipfs/go-fs-lock"
)

// lock logic heavily inspired by go-ipfs/repo/fsrepo/lock/lock.go

// The name of the file used for locking
const lockFileName = "cluster.lock"

var locker *lock

// lock helps to coordinate procees via a lock file
type lock struct {
	lockCloser io.Closer
	path       string
}

func (l *lock) lock() error {
	if l.lockCloser != nil {
		return fmt.Errorf("cannot acquire lock twice")
	}

	if err := l.checkConfigExists(); err != nil {
		return err
	}

	// set the lock file within this function
	logger.Debug("checking lock")
	lk, err := fslock.Lock(l.path, lockFileName)
	if err != nil {
		logger.Debug(err)
		l.lockCloser = nil
		errStr := fmt.Sprintf(`could not obtain execution lock.  If no other process
is running, remove %s, or make sure that the config folder is
writable for the user running ipfs-cluster.  Run with -d for more information
about the error`, path.Join(l.path, lockFileName))
		logger.Error(errStr)
		return fmt.Errorf("could not obtain execution lock")
	}
	logger.Debug("Success! ipfs-cluster-service lock acquired")
	l.lockCloser = lk
	return nil
}

func (l *lock) tryUnlock() error {
	// Noop in the uninitialized case
	if l.lockCloser == nil {
		logger.Debug("locking not initialized, unlock is noop")
		return nil
	}
	err := l.lockCloser.Close()
	if err != nil {
		return err
	}
	logger.Debug("Successfully released execution lock")
	l.lockCloser = nil
	return nil
}

func (l *lock) checkConfigExists() error {
	if _, err := os.Stat(l.path); os.IsNotExist(err) {
		logger.Error("ipfs-cluster-service config hasn't been initialized.\nPlease run ipfs-cluster-service init")
		return err
	}
	if _, err := os.Stat(fmt.Sprintf("%s/%s", l.path, "service.json")); os.IsNotExist(err) {
		logger.Error("ipfs-cluster-service config hasn't been initialized.\nPlease run ipfs-cluster-service init")
		return err
	}
	return nil
}
