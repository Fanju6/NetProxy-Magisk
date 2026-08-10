package module

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
)

const (
	serviceReadyTimeout = 30 * time.Second
	serviceStopTimeout  = 10 * time.Second
)

// ServiceResult 描述一次服务操作结束后的统一状态快照。
type ServiceResult struct {
	Action string         `json:"action"`
	Status service.Status `json:"status"`
}

// ManageService 执行 sing-box 生命周期操作。生命周期锁、状态落盘和进程管理全部在 Go 内完成。
func ManageService(ctx context.Context, options Options, action string) (ServiceResult, error) {
	action = strings.TrimSpace(action)
	if action == "status" {
		return serviceResult(ctx, options, action)
	}
	if action == "check" {
		if err := CheckService(ctx, options); err != nil {
			return ServiceResult{}, err
		}
		return serviceResult(ctx, options, action)
	}
	if action != "start" && action != "stop" && action != "restart" && action != "reload" {
		return ServiceResult{}, fmt.Errorf("未知服务操作: %s", action)
	}

	lock, err := acquireLifecycleLock(options.StateFile, action)
	if err != nil {
		return ServiceResult{}, err
	}
	defer lock.release()

	switch action {
	case "start":
		err = StartService(ctx, options)
	case "stop":
		err = StopService(ctx, options)
	case "restart":
		if err = StopService(ctx, options); err == nil {
			err = StartService(ctx, options)
		}
	case "reload":
		err = ReloadService(ctx, options)
	}
	if err != nil {
		return ServiceResult{}, err
	}
	if action == "start" || action == "restart" {
		if err := ensureWorker(ctx, options); err != nil {
			// Worker 独立于 sing-box，不能因为订阅调度启动失败而把已经就绪的核心判为失败。
			logService(options, "WARN", "订阅自动更新 Worker 启动失败: %v", err)
		}
	}
	return serviceResult(ctx, options, action)
}

// StartService 生成运行时配置，启动 sing-box，并在控制面和 eBPF 就绪后写入 ready 状态。
func StartService(ctx context.Context, options Options) error {
	if err := validateLifecycleOptions(options); err != nil {
		return err
	}
	state, _ := ReadServiceState(options.StateFile)
	if pid := service.FindProcess(options.SingBoxPath, int(state.PID)); pid > 0 {
		startedAt, err := serviceStartedAt(ctx, options)
		if err != nil {
			message := "检测到无响应的 sing-box 进程"
			_ = WriteServiceState(options.StateFile, "failed", int64(pid), state.StartedAt, 0, message)
			return fmt.Errorf("%s: %w", message, err)
		}
		readyAt := state.ReadyAt
		if readyAt <= 0 {
			readyAt = time.Now().Unix()
		}
		if err := WriteServiceState(options.StateFile, "ready", int64(pid), startedAt, readyAt, ""); err != nil {
			return err
		}
		logService(options, "WARN", "sing-box 已在运行 (PID: %d)", pid)
		return nil
	}

	logService(options, "INFO", "启动 sing-box 服务")
	if err := WriteServiceState(options.StateFile, "preparing", 0, 0, 0, ""); err != nil {
		return err
	}
	prepared, err := Prepare(ctx, options, false)
	if err != nil {
		return failServiceStart(options, 0, 0, "运行时配置生成失败", err)
	}
	if err := checkPreparedConfiguration(ctx, options, prepared); err != nil {
		return failServiceStart(options, 0, 0, "sing-box 配置检查失败", err)
	}
	if err := syncRuntimeSelection(options.ModuleConfig, prepared.RuntimeResult); err != nil {
		return failServiceStart(options, 0, 0, "运行时选择状态同步失败", err)
	}

	command, logFile, err := newSingBoxCommand(options, prepared)
	if err != nil {
		return failServiceStart(options, 0, 0, "sing-box 进程启动失败", err)
	}
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		return failServiceStart(options, 0, 0, "sing-box 进程启动失败", err)
	}
	pid := command.Process.Pid
	_ = command.Process.Release()
	_ = logFile.Close()
	startedAt := time.Now().Unix()
	if err := WriteServiceState(options.StateFile, "starting", int64(pid), startedAt, 0, ""); err != nil {
		_ = terminateService(options, pid)
		cleanupRuntimeFiles(options)
		return err
	}

	actualStartedAt, err := waitForServiceReady(ctx, options, pid, serviceReadyTimeout, 0)
	if err != nil {
		return failServiceStart(options, pid, startedAt, "核心或控制接口未在限定时间内就绪", err)
	}
	startedAt = actualStartedAt
	syncOptions := options
	syncOptions.SkipServiceReload = true
	if _, err := SyncSelection(ctx, syncOptions); err != nil {
		return failServiceStart(options, pid, startedAt, "运行时节点选择同步失败", err)
	}
	readyAt := time.Now().Unix()
	if err := WriteServiceState(options.StateFile, "ready", int64(pid), startedAt, readyAt, ""); err != nil {
		return err
	}
	logService(options, "INFO", "sing-box 服务启动完成 (PID: %d)", pid)
	return nil
}

// StopService 终止 sing-box 并清理所有可重建运行时配置。
func StopService(_ context.Context, options Options) error {
	if err := ensureLifecycleStateDir(options); err != nil {
		return err
	}
	state, _ := ReadServiceState(options.StateFile)
	pid := service.FindProcess(options.SingBoxPath, int(state.PID))
	logService(options, "INFO", "停止 sing-box 服务")
	if err := WriteServiceState(options.StateFile, "stopping", int64(pid), state.StartedAt, 0, ""); err != nil {
		return err
	}
	if pid > 0 {
		if err := terminateService(options, pid); err != nil {
			message := "sing-box 进程停止失败"
			_ = WriteServiceState(options.StateFile, "failed", int64(pid), state.StartedAt, 0, message)
			return fmt.Errorf("%s: %w", message, err)
		}
	}
	cleanupRuntimeFiles(options)
	if err := WriteServiceState(options.StateFile, "stopped", 0, 0, 0, ""); err != nil {
		return err
	}
	logService(options, "INFO", "sing-box 服务已停止")
	return nil
}

// ReloadService 原位重载已运行实例，并等待 Service API 报告新的启动时间。
func ReloadService(ctx context.Context, options Options) error {
	if err := validateLifecycleOptions(options); err != nil {
		return err
	}
	state, _ := ReadServiceState(options.StateFile)
	pid := service.FindProcess(options.SingBoxPath, int(state.PID))
	if pid <= 0 {
		return errors.New("sing-box 未运行，无法重新加载")
	}
	oldStartedAtMillis, err := serviceStartedAtMillis(ctx, options)
	if err != nil {
		return fmt.Errorf("Service API 未就绪，无法确认原位重新加载: %w", err)
	}
	oldStartedAt := oldStartedAtMillis / 1000
	logService(options, "INFO", "重新加载 sing-box 配置")
	prepared, err := Prepare(ctx, options, false)
	if err != nil {
		return fmt.Errorf("重新加载配置生成失败: %w", err)
	}
	if err := checkPreparedConfiguration(ctx, options, prepared); err != nil {
		return err
	}
	if err := syncRuntimeSelection(options.ModuleConfig, prepared.RuntimeResult); err != nil {
		return fmt.Errorf("运行时选择状态同步失败: %w", err)
	}
	if err := WriteServiceState(options.StateFile, "starting", int64(pid), oldStartedAt, 0, ""); err != nil {
		return err
	}
	if err := signalServiceReload(pid); err != nil {
		return restoreReloadState(ctx, options, pid, oldStartedAt, state.ReadyAt, err)
	}
	startedAt, err := waitForServiceReady(ctx, options, pid, serviceReadyTimeout, oldStartedAtMillis)
	if err != nil {
		return restoreReloadState(ctx, options, pid, oldStartedAt, state.ReadyAt, err)
	}
	syncOptions := options
	syncOptions.SkipServiceReload = true
	if _, err := SyncSelection(ctx, syncOptions); err != nil {
		return restoreReloadState(ctx, options, pid, startedAt, state.ReadyAt, err)
	}
	if err := WriteServiceState(options.StateFile, "ready", int64(pid), startedAt, time.Now().Unix(), ""); err != nil {
		return err
	}
	logService(options, "INFO", "sing-box 配置重新加载完成")
	return nil
}

// CheckService 在隔离运行时目录中检查完整 sing-box 配置，不影响正在运行的实例。
func CheckService(ctx context.Context, options Options) error {
	if err := validateLifecycleOptions(options); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(options.StateFile), "config-check-")
	if err != nil {
		return fmt.Errorf("创建配置检查目录失败: %w", err)
	}
	defer os.RemoveAll(temporary)
	checkOptions := options
	checkOptions.RuntimeDir = temporary
	prepared, err := Prepare(ctx, checkOptions, true)
	if err != nil {
		return err
	}
	return checkPreparedConfiguration(ctx, checkOptions, prepared)
}

// Boot 承载 service 阶段的最小开机流程：等待系统、按配置启动服务并拉起唯一 Worker。
func Boot(ctx context.Context, options Options, executable string) error {
	if err := os.MkdirAll(options.LogDir, 0o700); err != nil {
		return err
	}
	logService(options, "INFO", "NetProxy 开机服务启动")
	if err := exec.CommandContext(ctx, "resetprop", "-w", "sys.boot_completed").Run(); err != nil {
		return fmt.Errorf("等待系统启动完成失败: %w", err)
	}
	config, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return fmt.Errorf("加载模块配置失败: %w", err)
	}
	if config.AutoStart {
		if _, err := ManageService(ctx, options, "start"); err != nil {
			logService(options, "WARN", "代理服务开机启动失败，可在导入节点或修正配置后手动启动: %v", err)
		}
	} else {
		logService(options, "INFO", "开机自启动已禁用，跳过启动")
	}
	if executable == "" {
		executable = filepath.Join(options.ModuleDir, "bin", "netproxy-native")
	}
	if err := ensureWorker(ctx, options); err != nil {
		logService(options, "WARN", "订阅自动更新 Worker 启动失败，可稍后手动重试: %v", err)
	}
	logService(options, "INFO", "开机服务流程结束")
	return nil
}

func ensureWorker(ctx context.Context, options Options) error {
	executable := filepath.Join(options.ModuleDir, "bin", "netproxy-native")
	if _, err := worker.Start(ctx, workerOptions(options), executable); err != nil {
		return err
	}
	return nil
}

func serviceResult(ctx context.Context, options Options, action string) (ServiceResult, error) {
	status, err := service.ReadStatus(ctx, networkControlOptions(options))
	if err != nil {
		return ServiceResult{}, err
	}
	return ServiceResult{Action: action, Status: status}, nil
}

func validateLifecycleOptions(options Options) error {
	if err := options.validate(); err != nil {
		return err
	}
	for name, path := range map[string]string{
		"sing-box 二进制":  options.SingBoxPath,
		"sing-box 配置目录": filepath.Join(options.SingBoxDir, "confdir"),
		"Catalog":       options.CatalogRoot,
	} {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("%s不可用: %w", name, err)
		}
		if name != "sing-box 二进制" && !info.IsDir() {
			return fmt.Errorf("%s不是目录: %s", name, path)
		}
	}
	if _, err := os.Stat(options.EBPFConfig); err != nil {
		return fmt.Errorf("eBPF 配置不可用: %w", err)
	}
	return ensureLifecycleStateDir(options)
}

func ensureLifecycleStateDir(options Options) error {
	if err := os.MkdirAll(filepath.Dir(options.StateFile), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Dir(options.StateFile), 0o700); err != nil {
		return err
	}
	return os.MkdirAll(options.LogDir, 0o700)
}

func newSingBoxCommand(options Options, prepared PrepareResult) (*exec.Cmd, *os.File, error) {
	logPath := filepath.Join(options.LogDir, "sing-box.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, nil, err
	}
	command := exec.Command(options.SingBoxPath, "run", "-C", filepath.Join(options.SingBoxDir, "confdir"),
		"-c", prepared.Providers, "-c", prepared.Outbounds, "-c", prepared.EBPF)
	command.Dir = options.SingBoxDir
	command.Stdout = logFile
	command.Stderr = logFile
	detachServiceCommand(command)
	return command, logFile, nil
}

func checkPreparedConfiguration(ctx context.Context, options Options, prepared PrepareResult) error {
	command := exec.CommandContext(ctx, options.SingBoxPath, "check", "-C", filepath.Join(options.SingBoxDir, "confdir"),
		"-c", prepared.Providers, "-c", prepared.Outbounds, "-c", prepared.EBPF)
	command.Dir = options.SingBoxDir
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("sing-box 配置检查失败: %w", err)
	}
	return nil
}

func serviceStartedAt(ctx context.Context, options Options) (int64, error) {
	startedAt, err := serviceStartedAtMillis(ctx, options)
	if err != nil {
		return 0, err
	}
	return startedAt / 1000, nil
}

func serviceStartedAtMillis(ctx context.Context, options Options) (int64, error) {
	client, err := serviceapi.New(options.ServiceAddress, options.ServiceSecret)
	if err != nil {
		return 0, err
	}
	defer client.Close()
	requestContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	startedAt, err := client.Ready(requestContext)
	if err != nil {
		return 0, err
	}
	if startedAt.UnixMilli <= 0 {
		return 0, errors.New("Service API 返回的启动时间无效")
	}
	return startedAt.UnixMilli, nil
}

func waitForServiceReady(ctx context.Context, options Options, pid int, timeout time.Duration, previousStartedAtMillis int64) (int64, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !serviceProcessAlive(pid) {
			return 0, errors.New("sing-box 进程已退出")
		}
		if startedAtMillis, err := serviceStartedAtMillis(ctx, options); err == nil && (previousStartedAtMillis == 0 || startedAtMillis != previousStartedAtMillis) {
			return startedAtMillis / 1000, nil
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-deadline.C:
			return 0, errors.New("核心或控制接口未在限定时间内就绪")
		case <-ticker.C:
		}
	}
}

func terminateService(options Options, pid int) error {
	if pid <= 0 || !serviceProcessAlive(pid) {
		return nil
	}
	if err := signalServiceStop(pid); err != nil {
		return err
	}
	deadline := time.NewTimer(serviceStopTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for serviceProcessAlive(pid) {
		select {
		case <-deadline.C:
			logService(options, "WARN", "sing-box 未在限定时间内退出，改用 SIGKILL")
			if err := signalServiceKill(pid); err != nil {
				return err
			}
			return waitServiceExit(pid, time.Second)
		case <-ticker.C:
		}
	}
	return nil
}

func waitServiceExit(pid int, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for serviceProcessAlive(pid) {
		select {
		case <-deadline.C:
			return fmt.Errorf("sing-box 未在限定时间内退出: %d", pid)
		case <-ticker.C:
		}
	}
	return nil
}

func failServiceStart(options Options, pid int, startedAt int64, message string, cause error) error {
	if pid > 0 {
		_ = terminateService(options, pid)
	}
	cleanupRuntimeFiles(options)
	_ = WriteServiceState(options.StateFile, "failed", int64(pid), startedAt, 0, message)
	logService(options, "ERROR", "%s: %v", message, cause)
	return fmt.Errorf("%s: %w", message, cause)
}

func restoreReloadState(ctx context.Context, options Options, pid int, startedAt, readyAt int64, cause error) error {
	if serviceProcessAlive(pid) {
		if currentStartedAt, err := serviceStartedAt(ctx, options); err == nil {
			startedAt = currentStartedAt
		}
		_ = WriteServiceState(options.StateFile, "ready", int64(pid), startedAt, readyAt, cause.Error())
	}
	logService(options, "ERROR", "sing-box 原位重新加载失败: %v", cause)
	return fmt.Errorf("sing-box 原位重新加载失败: %w", cause)
}

func cleanupRuntimeFiles(options Options) {
	for _, name := range []string{"providers.json", "outbounds.json", "ebpf.json"} {
		_ = os.Remove(filepath.Join(options.RuntimeDir, name))
	}
}

func logService(options Options, level, format string, args ...any) {
	if strings.TrimSpace(options.LogDir) == "" {
		return
	}
	if err := os.MkdirAll(options.LogDir, 0o700); err != nil {
		return
	}
	file, err := os.OpenFile(filepath.Join(options.LogDir, "service.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	message := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(file, "[%s] [%s] [service] %s\n", time.Now().Format("2006-01-02 15:04:05"), level, message)
}
