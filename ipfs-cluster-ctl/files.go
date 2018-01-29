package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/ipfs/go-ipfs-cmdkit/files"
)

func parseFileArgs(paths []string) (*files.MultiFileReader, error) {
	// logic largely drawn from go-ipfs-cmds/cli/parse.go: parseArgs
	parsedFiles := make([]files.File, len(paths), len(paths))
	for _, path := range paths {
		file, err := appendFile(path)
		if err != nil {
			return nil, err
		}
		parsedFiles = append(parsedFiles, file)
	}
	sliceFile := files.NewSliceFile("", "", parsedFiles)
	return files.NewMultiFileReader(sliceFile, true), nil
}

func appendFile(fpath string) (files.File, error) {
	// logic drawn from go-ipfs-cmds/cli/parse.go: appendFile
	if fpath == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cwd, err = filepath.EvalSymlinks(cwd)
		if err != nil {
			return nil, err
		}
		fpath = cwd
	}

	fpath = filepath.ToSlash(filepath.Clean(fpath))

	stat, err := os.Lstat(fpath)
	if err != nil {
		return nil, err
	}

	if stat.IsDir() {
		return nil, fmt.Errorf("path: %s points to dir, adding dirs not yet supported", fpath)
	}

	return files.NewSerialFile(path.Base(fpath), fpath, false, stat)
}
