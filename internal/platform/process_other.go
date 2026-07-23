//go:build !windows

package platform

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func KillProcessTree(pid int) error {
	if pid <= 0 {
		return errors.New("invalid process id")
	}
	return syscall.Kill(pid, syscall.SIGKILL)
}

// StopGraceful sends SIGTERM to allow the process to exit cleanly.
func StopGraceful(pid int) error {
	if pid <= 0 {
		return errors.New("invalid process id")
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}

// WaitProcessExit polls ProcessExists up to timeout and returns true if the process exited.
func WaitProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !ProcessExists(pid) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func SetProcessGroup(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = nil
}

func FindExecutable(name string) string {
	if name == "" {
		return ""
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

func ProcessExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}
