//go:build !windows

package platform

import (
	"os/exec"
)

func KillProcessTree(pid int) error {
	return nil
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
