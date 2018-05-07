package raft

import (
	"fmt"
	"os"
	"testing"
)

func TestDataBackupHelper(t *testing.T) {
	keep := 5

	cleanup := func() {
		os.RemoveAll("data_helper_testing")
		for i := 0; i < 2*keep; i++ {
			os.RemoveAll(fmt.Sprintf("data_helper_testing.old.%d", i))
		}
	}
	cleanup()
	defer cleanup()

	os.MkdirAll("data_helper_testing", 0700)
	helper := newDataBackupHelper("data_helper_testing", keep)
	for i := 0; i < 2*keep; i++ {
		err := helper.makeBackup()
		if err != nil {
			t.Fatal(err)
		}
		backups := helper.listBackups()
		if (i < keep && len(backups) != i+1) ||
			(i >= keep && len(backups) != keep) {
			t.Fatal("incorrect number of backups saved")
		}
		os.MkdirAll("data_helper_testing", 0700)
	}
}
