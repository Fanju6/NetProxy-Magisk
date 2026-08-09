package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunProcess 安装系统信号并运行 Worker。
func RunProcess(ctx context.Context, options Options, logger *log.Logger) error {
	processContext, wake, cleanup := withSignals(ctx)
	defer cleanup()
	return Run(processContext, options, wake, logger)
}

// Start 启动一个脱离当前命令通道的 Worker；没有自动订阅时不会驻留。
func Start(ctx context.Context, options Options, executable string) (Status, error) {
	if err := validateOptions(options); err != nil {
		return Status{}, err
	}
	if status, err := ReadStatus(options); err == nil && status.State == "running" {
		return status, nil
	}
	nearest, err := NextUpdate(options.Root, options.Now())
	if err != nil {
		return Status{}, err
	}
	if nearest == 0 && !(options.NetworkWatchEnabled && options.NetworkEvaluate != nil) {
		return Status{State: "stopped", Nearest: 0}, nil
	}
	if executable == "" {
		return Status{}, errors.New("Worker 可执行文件不能为空")
	}
	arguments := []string{"subworker", "run"}
	arguments = appendWorkerFlags(arguments, options)
	command := exec.CommandContext(ctx, executable, arguments...)
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return Status{}, err
	}
	defer devNull.Close()
	command.Stdin = devNull
	command.Stdout = devNull
	command.Stderr = devNull
	detachCommand(command)
	if err := command.Start(); err != nil {
		return Status{}, err
	}
	return Status{State: "running", PID: command.Process.Pid, Nearest: nearest}, nil
}

// Stop 请求 Worker 优雅退出。
func Stop(options Options) error {
	pid := readPID(options.PIDFile)
	if pid <= 0 || !isWorkerProcessPID(pid) {
		_ = os.Remove(options.PIDFile)
		return nil
	}
	if err := terminateProcess(pid); err != nil {
		return err
	}
	if !waitProcessStop(pid, 10*time.Second) {
		return fmt.Errorf("Worker 未在限定时间内退出: %d", pid)
	}
	return nil
}

// Wake 请求正在运行的 Worker 立即重新计算任务；未运行时按需启动。
func Wake(ctx context.Context, options Options, executable string) (Status, error) {
	status, err := ReadStatus(options)
	if err != nil {
		return Status{}, err
	}
	if status.State != "running" {
		return Start(ctx, options, executable)
	}
	if err := wakeProcess(status.PID); err != nil {
		return Status{}, err
	}
	return status, nil
}

func appendWorkerFlags(arguments []string, options Options) []string {
	arguments = append(arguments, "--root", options.Root, "--progress-dir", options.ProgressDir,
		"--pid-file", options.PIDFile, "--log-file", options.LogFile,
		"--module-conf", options.ModuleConf, "--reload-script", options.ReloadScript,
		"--sing-box", options.SingBoxPath, "--service-address", options.ServiceAddress,
		"--service-secret", options.ServiceSecret)
	return arguments
}

// OpenLogger 创建 Worker 日志。日志文件为空时返回 stderr logger。
func OpenLogger(path string) (*log.Logger, io.Closer, error) {
	if strings.TrimSpace(path) == "" {
		return log.New(os.Stderr, "", log.LstdFlags), io.NopCloser(strings.NewReader("")), nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	return log.New(file, "", log.LstdFlags), file, nil
}
