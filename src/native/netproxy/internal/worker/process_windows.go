//go:build windows

package worker

import (
	"errors"
	"os/exec"
	"time"
)

func detachCommand(command *exec.Cmd)                     {}
func isProcessRunningPID(pid int) bool                    { return false }
func isWorkerProcessPID(pid int) bool                     { return false }
func isProcessRunning(binary string) bool                 { return false }
func terminateProcess(pid int) error                      { return errors.New("Windows 不支持 Worker 进程信号") }
func wakeProcess(pid int) error                           { return errors.New("Windows 不支持 Worker 进程信号") }
func waitProcessStop(pid int, timeout time.Duration) bool { return true }
