package test

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/ipfs/go-ipfs-cmdkit/files"
)

const testDirName = "testingData"
const relRootDir = "src/github.com/ipfs/ipfs-cluster/test"

// GetTestingDirSerial returns a cmdkits serial file to the testing directory.
// $GOPATH must be set for this to work
func GetTestingDirSerial() (files.File, error) {
	rootPath := filepath.Join(os.Getenv("GOPATH"), relRootDir)
	testDir := filepath.Join(rootPath, testDirName)

	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		err := generateTestDirs(rootPath)
		if err != nil {
			return nil, err
		}
	}
	stat, err := os.Lstat(testDir)
	if err != nil {
		return nil, err
	}

	if !stat.IsDir() {
		return nil, fmt.Errorf("testDir should be seen as directory")
	}
	return files.NewSerialFile(testDirName, testDir, false, stat)
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

// generateTestDirs creates a testing directory structure on demand for testing
// leveraging random but deterministic strings.  Files are the same every run.
// Directory structure:
// - testingData
//     - A
//         - alpha
//             * small_file_0 (< 5 kB)
//         - beta
//             * small_file_1 (< 5 kB)
//         - delta
//             - empty
//         * small_file_2 (< 5 kB)
//         - gamma
//             * small_file_3 (< 5 kB)
//     - B
//         * medium_file (~.3 MB)
//         * big_file (3 MB)
//
// Take special care when modifying this function.  File data depends on order
// and each file size.  If this changes then hashes stored in test/cids.go
// recording the ipfs import hash tree must be updated manually.
func generateTestDirs(path string) error {
	// Prepare randomness source for writing files
	src := rand.NewSource(int64(1))
	ra := rand.New(src)

	// Make top level directory
	rootPath := filepath.Join(path, testDirName)
	err := os.Mkdir(rootPath, os.ModePerm)
	if err != nil {
		return err
	}

	// Make subdir A
	aPath := filepath.Join(rootPath, "A")
	err = os.Mkdir(aPath, os.ModePerm)
	if err != nil {
		return err
	}

	alphaPath := filepath.Join(aPath, "alpha")
	err = os.Mkdir(alphaPath, os.ModePerm)
	if err != nil {
		return err
	}

	sf0Path := filepath.Join(alphaPath, "small_file_0")
	f, err := os.Create(sf0Path)
	if err != nil {
		return err
	}
	err = writeRandFile(5, ra, f)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}

	betaPath := filepath.Join(aPath, "beta")
	err = os.Mkdir(betaPath, os.ModePerm)
	if err != nil {
		return err
	}

	sf1Path := filepath.Join(betaPath, "small_file_1")
	f, err = os.Create(sf1Path)
	if err != nil {
		return err
	}
	err = writeRandFile(5, ra, f)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}

	deltaPath := filepath.Join(aPath, "delta")
	err = os.Mkdir(deltaPath, os.ModePerm)
	if err != nil {
		return err
	}

	emptyPath := filepath.Join(deltaPath, "empty")
	err = os.Mkdir(emptyPath, os.ModePerm)
	if err != nil {
		return err
	}

	sf2Path := filepath.Join(aPath, "small_file_2")
	f, err = os.Create(sf2Path)
	if err != nil {
		return err
	}
	err = writeRandFile(5, ra, f)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}

	gammaPath := filepath.Join(aPath, "gamma")
	err = os.Mkdir(gammaPath, os.ModePerm)
	if err != nil {
		return err
	}

	sf3Path := filepath.Join(gammaPath, "small_file_3")
	f, err = os.Create(sf3Path)
	if err != nil {
		return err
	}
	err = writeRandFile(5, ra, f)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}

	// Make subdir B
	bPath := filepath.Join(rootPath, "B")
	err = os.Mkdir(bPath, os.ModePerm)
	if err != nil {
		return err
	}

	mfPath := filepath.Join(bPath, "medium_file")
	f, err = os.Create(mfPath)
	if err != nil {
		return err
	}
	err = writeRandFile(300, ra, f)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}

	bfPath := filepath.Join(bPath, "big_file")
	f, err = os.Create(bfPath)
	if err != nil {
		return err
	}
	err = writeRandFile(3000, ra, f)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}

	return nil
}

// writeRandFile takes in a source of randomness, a file, a number of kibibytes
// and a writing buffer and writes a kibibyte at a time from the randomness to
// the file
func writeRandFile(n int, ra *rand.Rand, f io.Writer) error {
	w := bufio.NewWriter(f)
	buf := make([]byte, 1024)
	i := 0
	for i < n {
		ra.Read(buf)
		if _, err := w.Write(buf); err != nil {
			return err
		}
		i++
	}
	return nil
}
