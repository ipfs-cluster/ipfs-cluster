//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package fd

import "math"

// GetNumFDs returns the File Descriptors for non unix systems as MaxInt.
func GetNumFDs() uint64 {
	return uint64(math.MaxUint64)
}
