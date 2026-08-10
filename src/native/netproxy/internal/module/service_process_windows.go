//go:build windows

package module

import (
	"os"
	"os/exec"
)

func detachServiceCommand(_ *exec.Cmd) {}

func signalServiceReload(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(os.Interrupt)
}

func signalServiceStop(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

func signalServiceKill(pid int) error {
	return signalServiceStop(pid)
}

func serviceProcessAlive(_ int) bool {
	return false
}
