//go:build !windows

package worker

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func detachCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func isProcessRunningPID(pid int) bool {
	if pid <= 0 {
		return false
	}
	if _, err := os.Stat("/proc/" + itoa(pid)); err != nil {
		return false
	}
	return true
}

func isWorkerProcessPID(pid int) bool {
	if !isProcessRunningPID(pid) {
		return false
	}
	content, err := os.ReadFile("/proc/" + itoa(pid) + "/cmdline")
	if err != nil {
		return false
	}
	arguments := strings.Split(string(content), "\x00")
	for index, argument := range arguments {
		if argument == "subworker" && index+1 < len(arguments) && arguments[index+1] == "run" {
			return true
		}
	}
	return false
}

func isProcessRunning(binary string) bool {
	if binary == "" {
		return false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid := entry.Name()
		if !allDigits(pid) {
			continue
		}
		content, err := os.ReadFile("/proc/" + pid + "/cmdline")
		if err != nil {
			continue
		}
		if len(content) > 0 && string(strings.Split(string(content), "\x00")[0]) == binary {
			return true
		}
	}
	return false
}

func terminateProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGTERM)
}

func wakeProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGUSR1)
}

func waitProcessStop(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isProcessRunningPID(pid) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return !isProcessRunningPID(pid)
}

func allDigits(value string) bool {
	return value != "" && strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) == -1
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
