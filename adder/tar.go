package adder

import (
	"archive/tar"
	"io"
	"sort"
	"strings"

	files "github.com/ipfs/go-ipfs-files"
)

func tarToSliceDirectory(tr *tar.Reader) (files.Directory, error) {
	// dirEntries contains our final directory entries
	dirEntries := []files.DirEntry{}

	// fileMap contains files which are not direct children
	// of our root directory
	fileMap := make(map[string][]files.DirEntry)

	// list of all directories except the root directory
	dirList := []string{}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		path := strings.TrimSuffix(header.Name, "/")
		splits := strings.Split(path, "/")
		if !header.FileInfo().IsDir() {
			b := make([]byte, header.Size)
			_, err := tr.Read(b)
			if err != nil && err != io.EOF {
				return nil, err
			}
			f := files.NewBytesFile(b)

			if len(splits) == 1 {
				// file that is direct children of root directory
				// add it directly to our final list of DirEntries
				dirEntries = append(dirEntries, files.FileEntry(path, f))
			} else {
				// file that is not direct children of root directory
				// add it to DirEntries of its parent
				parentDir := getParent(path)
				entries, ok := fileMap[parentDir]
				if !ok {
					entries = []files.DirEntry{}
				}
				entries = append(entries, files.FileEntry(getName(path), f))
				fileMap[parentDir] = entries
			}
		} else {
			// no data to read in case of directory, just add it to directory list
			dirList = append(dirList, path)
		}
	}

	// sort directories, starting with ones that are farther from root
	sort.Sort(byDepth(dirList))

	// add directories to their parents
	for _, v := range dirList {
		parent := getParent(v)
		if parent == "" {
			continue
		}

		entries, ok := fileMap[v]
		if !ok {
			entries = []files.DirEntry{}
		}
		dir := files.NewSliceDirectory(entries)

		parentEntries, ok := fileMap[parent]
		if !ok {
			parentEntries = []files.DirEntry{}
		}
		parentEntries = append(parentEntries, files.FileEntry(getName(v), dir))
		fileMap[parent] = parentEntries

		delete(fileMap, v)
	}

	// whatever has remained in fileMap are directories/files that are childen of
	// the root directory. So, add them to final dirEntries
	for k, v := range fileMap {
		dirEntries = append(dirEntries, files.FileEntry(getName(k), files.NewSliceDirectory(v)))
	}

	// add all entries in the file map to dirEntries
	return files.NewSliceDirectory(dirEntries), nil
}

func getName(path string) string {
	splits := strings.Split(path, "/")
	return splits[len(splits)-1]
}

func getParent(path string) string {
	splits := strings.Split(path, "/")
	if len(splits) == 1 {
		return ""
	}
	return strings.TrimSuffix(path, "/"+splits[len(splits)-1])
}

type byDepth []string

func (a byDepth) Len() int      { return len(a) }
func (a byDepth) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byDepth) Less(i, j int) bool {
	return len(strings.Split(a[i], "/")) > len(strings.Split(a[j], "/"))
}
