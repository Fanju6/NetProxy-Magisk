//go:build !windows

package module

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
	content, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	_, fields, found := strings.CutLast(string(content), ")")
	if !found {
		return false
	}
	state := strings.Fields(fields)
	// 当前命令仍是父进程时，已经完成退出的核心可能短暂保留为僵尸。
	return len(state) > 0 && state[0] != "Z" && state[0] != "X"
}
