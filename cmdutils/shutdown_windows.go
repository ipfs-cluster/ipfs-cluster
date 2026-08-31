//go:build windows

package cmdutils

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procGenerateConsoleCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")
	procFreeConsole              = kernel32.NewProc("FreeConsole")
	procAttachConsole            = kernel32.NewProc("AttachConsole")
	procSetConsoleCtrlHandler    = kernel32.NewProc("SetConsoleCtrlHandler")
)

const (
	CTRL_BREAK_EVENT      = 1
	ctrlBreakHelperEnv    = "IPFS_CLUSTER_CTRL_BREAK_PID"
	STATUS_CONTROL_C_EXIT = 0xC000013A
)

func init() {
	if pidStr := os.Getenv(ctrlBreakHelperEnv); pidStr != "" {
		os.Exit(runCtrlBreakHelperProcess(pidStr))
	}
}

func terminateProcess(process *os.Process, pid int) error {
	fmt.Printf("sending CTRL_BREAK_EVENT to process %d...\n", pid)

	executable, err := os.Executable()
	if err != nil {
		fmt.Println("failed to get executable path, using Kill")
		return process.Kill()
	}

	cmd := exec.Command(executable)
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%d", ctrlBreakHelperEnv, pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}

	err = cmd.Run()
	if err == nil {
		return nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == STATUS_CONTROL_C_EXIT {
			fmt.Println("CTRL_BREAK_EVENT sent successfully via helper")
			return nil
		}
		fmt.Printf("helper process failed with exit code 0x%X, using Kill\n", exitErr.ExitCode())
		return process.Kill()
	}

	fmt.Printf("failed to run helper process (%s), using Kill\n", err)
	return process.Kill()
}

func runCtrlBreakHelperProcess(pidStr string) int {
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 1
	}

	// ignore original console CTRL+C and CTRL+BREAK signal
	procSetConsoleCtrlHandler.Call(0, 1)
	defer procSetConsoleCtrlHandler.Call(0, 0)

	// windows need to attach target console first, then send CTRL+BREAK signal
	procFreeConsole.Call()
	if ret, _, _ := procAttachConsole.Call(uintptr(pid)); ret == 0 {
		return 1
	}
	procGenerateConsoleCtrlEvent.Call(uintptr(CTRL_BREAK_EVENT), uintptr(pid))
	procFreeConsole.Call()

	return 0
}
