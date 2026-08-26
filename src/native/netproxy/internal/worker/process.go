package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/logfile"
)

const workerStartTimeout = 2 * time.Second

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
	if status, err := ReadStatus(options); err == nil {
		if status.State == "running" {
			return status, nil
		}
		_ = os.Remove(options.PIDFile)
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
	arguments := []string{"__internal", "worker", "run"}
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
	pid := command.Process.Pid
	if err := waitForWorkerPID(ctx, options.PIDFile, pid, workerStartTimeout); err != nil {
		_ = terminateProcess(pid)
		return Status{}, err
	}
	return Status{State: "running", PID: pid, Nearest: nearest}, nil
}

func waitForWorkerPID(ctx context.Context, path string, pid int, timeout time.Duration) error {
	if pid <= 0 {
		return errors.New("Worker 启动失败：进程 PID 无效")
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if readPID(path) == pid {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("Worker 启动被取消: %w", ctx.Err())
		case <-deadline.C:
			return errors.New("Worker 启动失败：PID 状态未成功写入")
		case <-ticker.C:
		}
	}
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

func appendWorkerFlags(arguments []string, options Options) []string {
	arguments = append(arguments, "--root", options.Root, "--progress-dir", options.ProgressDir,
		"--pid-file", options.PIDFile, "--log-file", options.LogFile,
		"--module-conf", options.ModuleConf, "--executable", options.ExecutablePath,
		"--sing-box", options.SingBoxPath, "--service-address", options.ServiceAddress,
		"--service-secret", options.ServiceSecret)
	return arguments
}

// OpenLogger 创建 Worker 日志。日志文件为空时返回 stderr logger。
func OpenLogger(path string) (*log.Logger, io.Closer, error) {
	if strings.TrimSpace(path) == "" {
		return log.New(os.Stderr, "", log.LstdFlags), io.NopCloser(strings.NewReader("")), nil
	}
	if err := logfile.Prepare(path); err != nil {
		return nil, nil, err
	}
	return log.New(logfile.NewWriter(path), "", 0), io.NopCloser(strings.NewReader("")), nil
}
