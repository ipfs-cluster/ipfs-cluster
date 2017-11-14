package raft

import (
	"fmt"
	"os"
	"path/filepath"
)

// RaftDataBackupKeep indicates the number of data folders we keep around
// after consensus.Clean() has been called.
var RaftDataBackupKeep = 5

// dataBackupHelper helps making and rotating backups from a folder.
// it will name them <folderName>.old.0, .old.1... and so on.
// when a new backup is made, the old.0 is renamed to old.1 and so on.
// when the RaftDataBackupKeep number is reached, the last is always
// discarded.
type dataBackupHelper struct {
	baseDir    string
	folderName string
}

func newDataBackupHelper(dataFolder string) *dataBackupHelper {
	return &dataBackupHelper{
		baseDir:    filepath.Dir(dataFolder),
		folderName: filepath.Base(dataFolder),
	}
}

func (dbh *dataBackupHelper) makeName(i int) string {
	return filepath.Join(dbh.baseDir, fmt.Sprintf("%s.old.%d", dbh.folderName, i))
}

func (dbh *dataBackupHelper) listBackups() []string {
	backups := []string{}
	for i := 0; i < RaftDataBackupKeep; i++ {
		name := dbh.makeName(i)
		if _, err := os.Stat(name); os.IsNotExist(err) {
			return backups
		}
		backups = append(backups, name)
	}
	return backups
}

func (dbh *dataBackupHelper) makeBackup() error {
	err := os.MkdirAll(dbh.baseDir, 0700)
	if err != nil {
		return err
	}
	backups := dbh.listBackups()
	// remove last / oldest
	if len(backups) >= RaftDataBackupKeep {
		os.RemoveAll(backups[len(backups)-1])
	} else {
		backups = append(backups, dbh.makeName(len(backups)))
	}

	// increase number for all backups folders
	for i := len(backups) - 1; i > 0; i-- {
		err := os.Rename(backups[i-1], backups[i])
		if err != nil {
			return err
		}
	}

	// save new as name.old.0
	return os.Rename(filepath.Join(dbh.baseDir, dbh.folderName), dbh.makeName(0))
}
