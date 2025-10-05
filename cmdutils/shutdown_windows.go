//go:build windows

package cmdutils

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procGenerateConsoleCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")
)

const (
	CTRL_C_EVENT = 0
)

// terminateProcess uses Windows-specific termination on Windows.
// It sends a CTRL_C_EVENT and waits for the process to exit.
// If the process does not exit within a timeout, it is killed.
func terminateProcess(process *os.Process, pid int) error {
	// Try GenerateConsoleCtrlEvent first (graceful)
	ret, _, err := procGenerateConsoleCtrlEvent.Call(
		uintptr(CTRL_C_EVENT),
		uintptr(pid),
	)

	if ret == 0 {
		// if we could not send the event, just try to kill it.
		fmt.Printf("Warning: GenerateConsoleCtrlCtrlEvent failed (%v), using process.Kill()\n", err)
		return process.Kill()
	}

	// Wait for the process to exit
	timeout := time.After(5 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-timeout:
			fmt.Println("Warning: process did not exit gracefully. Killing.")
			return process.Kill()
		case <-tick.C:
			// On Windows, FindProcess always succeeds, so we need to check
			// if the process is actually still running. A common way is to
			// send a 0 signal, which doesn't harm the process but returns
			// an error if the process is not running.
			err := process.Signal(syscall.Signal(0))
			if err != nil { // process is gone
				return nil
			}
		}
	}
} 