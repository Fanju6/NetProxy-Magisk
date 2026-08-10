//go:build !windows

package module

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

func detachServiceCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func signalServiceReload(pid int) error {
	return syscall.Kill(pid, syscall.SIGHUP)
}

func signalServiceStop(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

func signalServiceKill(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}

func serviceProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid)))
	return err == nil
}
