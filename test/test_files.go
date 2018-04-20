package test

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/ipfs/go-ipfs-cmdkit/files"
)

const relTestDir = "src/github.com/ipfs/ipfs-cluster/test/testingData"

// GetTestingDirSerial returns a cmdkits serial file to the testing directory.
// $GOPATH must be set for this to work
func GetTestingDirSerial() (files.File, error) {
	fpath := strings.Join([]string{os.Getenv("GOPATH"), relTestDir}, "/")
	stat, err := os.Lstat(fpath)
	if err != nil {
		return nil, err
	}

	if !stat.IsDir() {
		return nil, fmt.Errorf("testDir should be seen as directory")
	}
	return files.NewSerialFile(path.Base(fpath), fpath, false, stat)
}

// GetTestingDirMultiReader returns a cmdkits multifilereader to the testing
// directory.  $GOPATH must be set for this to work
func GetTestingDirMultiReader() (*files.MultiFileReader, error) {
	file, err := GetTestingDirSerial()
	if err != nil {
		return nil, err
	}
	sliceFile := files.NewSliceFile("", "", []files.File{file})
	return files.NewMultiFileReader(sliceFile, true), nil
}
