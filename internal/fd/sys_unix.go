//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package fd

import (
	"golang.org/x/sys/unix"
)

// GetNumFDs returns the File Descriptors limit.
func GetNumFDs() uint64 {
	var l unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &l); err != nil {
		return 0
	}
	return l.Cur
}
