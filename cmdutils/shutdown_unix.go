//go:build !windows

package cmdutils

import (
	"fmt"
	"os"
	"syscall"
)

// terminateProcess sends SIGTERM signal on Unix systems
func terminateProcess(process *os.Process, pid int) error {
	// Check if process is actually running
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("process %d is not running", pid)
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM to process %d: %w", pid, err)
	}

	return nil
}
