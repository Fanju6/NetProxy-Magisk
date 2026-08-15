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
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
)

const (
	serviceReadyTimeout = 30 * time.Second
	serviceStopTimeout  = 10 * time.Second
)

var (
	writeServiceState        = WriteServiceState
	terminateServiceForStart = terminateService
)

// ServiceResult 描述一次服务操作结束后的统一状态快照。
type ServiceResult struct {
	Action string         `json:"action"`
	Status service.Status `json:"status"`
}

func toggleServiceAction(state string) (string, error) {
	switch state {
	case "ready":
		return "stop", nil
	case "", "stopped", "failed":
		return "start", nil
	case "preparing", "starting", "stopping":
		return "", fmt.Errorf("服务正在切换 (%s)", state)
	default:
		return "start", nil
	}
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
	if action != "start" && action != "stop" && action != "restart" && action != "reload" && action != "toggle" {
		return ServiceResult{}, fmt.Errorf("未知服务操作: %s", action)
	}

	lock, err := acquireLifecycleLock(options.StateFile)
	if err != nil {
		return ServiceResult{}, err
	}
	defer lock.release()

	if action == "toggle" {
		state, stateErr := ReadServiceState(options.StateFile)
		if stateErr != nil {
			return ServiceResult{}, fmt.Errorf("读取服务状态失败: %w", stateErr)
		}
		action, err = toggleServiceAction(state.State)
		if err != nil {
			return ServiceResult{}, err
		}
	}

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
			logService(options, "WARN", "worker.start", "failed", "后台 Worker 启动失败: %v", err)
		}
	}
	return serviceResult(ctx, options, action)
}

// StartService 生成运行时配置，启动 sing-box，并在控制面和 eBPF 就绪后写入 ready 状态。
func StartService(ctx context.Context, options Options) error {
	if err := validateLifecycleOptions(options); err != nil {
		return err
	}
	if err := recoverConfigApply(ctx, options); err != nil {
		return err
	}
	state, stateErr := ReadServiceState(options.StateFile)
	if stateErr != nil {
		return fmt.Errorf("读取服务状态失败: %w", stateErr)
	}
	if pid := service.FindProcess(options.SingBoxPath, int(state.PID)); pid > 0 {
		if err := ensureSingBoxRootCgroup(pid); err != nil {
			message := "sing-box 无法加入 root cgroup"
			stateErr := writeServiceState(options.StateFile, "failed", int64(pid), state.StartedAt, 0, message)
			logService(options, "ERROR", "service.cgroup", "failed", "%s: %v", message, err)
			return errors.Join(fmt.Errorf("%s: %w", message, err), stateErr)
		}
		startedAt, err := serviceStartedAt(ctx, options)
		if err != nil {
			message := "检测到无响应的 sing-box 进程"
			stateErr := writeServiceState(options.StateFile, "failed", int64(pid), state.StartedAt, 0, message)
			return errors.Join(fmt.Errorf("%s: %w", message, err), stateErr)
		}
		readyAt := state.ReadyAt
		if readyAt <= 0 {
			readyAt = time.Now().Unix()
		}
		if err := writeServiceState(options.StateFile, "ready", int64(pid), startedAt, readyAt, ""); err != nil {
			return err
		}
		logService(options, "WARN", "service.start", "already-running", "sing-box 已在运行 (PID: %d)", pid)
		return nil
	}

	logService(options, "INFO", "service.start", "started", "启动 sing-box 服务")
	if err := writeServiceState(options.StateFile, "preparing", 0, 0, 0, ""); err != nil {
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
	if err := ensureSingBoxRootCgroup(pid); err != nil {
		_ = logFile.Close()
		return failServiceStart(options, pid, 0, "sing-box 无法加入 root cgroup", err)
	}
	_ = command.Process.Release()
	_ = logFile.Close()
	startedAt := time.Now().Unix()
	if err := writeServiceState(options.StateFile, "starting", int64(pid), startedAt, 0, ""); err != nil {
		return failServiceStateWrite(options, pid, startedAt, "starting", err)
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
	if err := writeServiceState(options.StateFile, "ready", int64(pid), startedAt, readyAt, ""); err != nil {
		return failServiceStateWrite(options, pid, startedAt, "ready", err)
	}
	logService(options, "INFO", "service.start", "success", "sing-box 服务启动完成 (PID: %d)", pid)
	return nil
}

// StopService 终止 sing-box 并清理所有可重建运行时配置。
func StopService(_ context.Context, options Options) error {
	if err := ensureLifecycleStateDir(options); err != nil {
		return err
	}
	state, stateErr := ReadServiceState(options.StateFile)
	if stateErr != nil {
		state = ServiceState{Schema: 1, State: "stopped"}
	}
	pid := service.FindProcess(options.SingBoxPath, int(state.PID))
	logService(options, "INFO", "service.stop", "started", "停止 sing-box 服务")
	stateWriteErr := writeServiceState(options.StateFile, "stopping", int64(pid), state.StartedAt, 0, "")
	if pid > 0 {
		if err := terminateService(options, pid); err != nil {
			message := "sing-box 进程停止失败"
			failedWriteErr := writeServiceState(options.StateFile, "failed", int64(pid), state.StartedAt, 0, message)
			return errors.Join(fmt.Errorf("%s: %w", message, err), stateErr, stateWriteErr, failedWriteErr)
		}
	}
	cleanupRuntimeFiles(options)
	stoppedWriteErr := writeServiceState(options.StateFile, "stopped", 0, 0, 0, "")
	if err := errors.Join(stateErr, stateWriteErr, stoppedWriteErr); err != nil {
		return fmt.Errorf("停止 sing-box 已完成，但服务状态写入失败: %w", err)
	}
	logService(options, "INFO", "service.stop", "success", "sing-box 服务已停止")
	return nil
}

// ReloadService 原位重载已运行实例，并等待 Service API 报告新的启动时间。
func ReloadService(ctx context.Context, options Options) error {
	if err := validateLifecycleOptions(options); err != nil {
		return err
	}
	if err := recoverConfigApply(ctx, options); err != nil {
		return err
	}
	state, stateErr := ReadServiceState(options.StateFile)
	if stateErr != nil {
		return fmt.Errorf("读取服务状态失败: %w", stateErr)
	}
	pid := service.FindProcess(options.SingBoxPath, int(state.PID))
	if pid <= 0 {
		return errors.New("sing-box 未运行，无法重新加载")
	}
	if _, err := serviceStartedAtMillis(ctx, options); err != nil {
		return fmt.Errorf("Service API 未就绪，无法确认原位重新加载: %w", err)
	}
	logService(options, "INFO", "service.reload", "started", "重新加载 sing-box 配置")
	prepared, err := Prepare(ctx, options, false)
	if err != nil {
		return fmt.Errorf("重新加载配置生成失败: %w", err)
	}
	if err := checkPreparedConfiguration(ctx, options, prepared); err != nil {
		return err
	}
	return reloadPreparedService(ctx, options, prepared, true)
}

func reloadAppliedConfig(ctx context.Context, options Options) error {
	prepared, err := Prepare(ctx, options, false)
	if err != nil {
		return fmt.Errorf("重新加载配置生成失败: %w", err)
	}
	if err := checkPreparedConfiguration(ctx, options, prepared); err != nil {
		return err
	}
	reloadOptions := options
	reloadOptions.SkipServiceReload = true
	return reloadPreparedService(ctx, reloadOptions, prepared, true)
}

func reloadConfigSnapshot(ctx context.Context, options Options, journal configApplyJournal) error {
	prepared := prepareFromConfigJournal(options, journal)
	for name, path := range map[string]string{
		"providers": prepared.Providers,
		"outbounds": prepared.Outbounds,
		"ebpf":      prepared.EBPF,
	} {
		if path == "" {
			return fmt.Errorf("旧运行时快照缺少 %s", name)
		}
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("旧运行时快照 %s 不可用: %w", name, err)
		}
	}
	reloadOptions := options
	reloadOptions.SkipServiceReload = true
	return reloadPreparedService(ctx, reloadOptions, prepared, false)
}

func reloadPreparedService(ctx context.Context, options Options, prepared PrepareResult, syncSelection bool) error {
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
	if err := writeServiceState(options.StateFile, "starting", int64(pid), oldStartedAt, 0, ""); err != nil {
		return err
	}
	if err := signalServiceReload(pid); err != nil {
		return restoreReloadState(ctx, options, pid, oldStartedAt, state.ReadyAt, err)
	}
	startedAt, err := waitForServiceReady(ctx, options, pid, serviceReadyTimeout, oldStartedAtMillis)
	if err != nil {
		return restoreReloadState(ctx, options, pid, oldStartedAt, state.ReadyAt, err)
	}
	if syncSelection {
		syncOptions := options
		syncOptions.SkipServiceReload = true
		if err := syncRuntimeSelection(options.ModuleConfig, prepared.RuntimeResult); err != nil {
			return restoreReloadState(ctx, options, pid, startedAt, state.ReadyAt, err)
		}
		if _, err := SyncSelection(ctx, syncOptions); err != nil {
			return restoreReloadState(ctx, options, pid, startedAt, state.ReadyAt, err)
		}
	}
	if err := writeServiceState(options.StateFile, "ready", int64(pid), startedAt, time.Now().Unix(), ""); err != nil {
		return err
	}
	logService(options, "INFO", "service.reload", "success", "sing-box 配置重新加载完成")
	return nil
}

// CheckService 在隔离运行时目录中检查完整 sing-box 配置，不影响正在运行的实例。
func CheckService(ctx context.Context, options Options) error {
	if err := validateLifecycleOptions(options); err != nil {
		return err
	}
	lock, err := acquireLifecycleLock(options.StateFile)
	if err != nil {
		return err
	}
	defer lock.release()
	if err := recoverConfigApply(ctx, options); err != nil {
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
func Boot(ctx context.Context, options Options) error {
	if err := os.MkdirAll(options.LogDir, 0o700); err != nil {
		return err
	}
	logService(options, "INFO", "module.boot", "started", "NetProxy 开机服务启动")
	if err := exec.CommandContext(ctx, "resetprop", "-w", "sys.boot_completed").Run(); err != nil {
		return fmt.Errorf("等待系统启动完成失败: %w", err)
	}
	config, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return fmt.Errorf("加载模块配置失败: %w", err)
	}
	if config.AutoStart {
		if _, err := ManageService(ctx, options, "start"); err != nil {
			logService(options, "WARN", "service.start", "failed", "代理服务开机启动失败，可在导入节点或修正配置后手动启动: %v", err)
		}
	} else {
		logService(options, "INFO", "service.start", "skipped", "开机自启动已禁用，跳过启动")
	}
	if err := ensureWorker(ctx, options); err != nil {
		logService(options, "WARN", "worker.start", "failed", "后台 Worker 启动失败，可稍后手动重试: %v", err)
	}
	logService(options, "INFO", "module.boot", "success", "开机服务流程结束")
	return nil
}

func ensureWorker(ctx context.Context, options Options) error {
	executable := paths.New(options.ModuleDir).Native()
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
		"sing-box 配置目录": paths.SingBoxConfDir(options.SingBoxDir),
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
	command := exec.Command(options.SingBoxPath, "run", "-C", paths.SingBoxConfDir(options.SingBoxDir),
		"-c", prepared.Providers, "-c", prepared.Outbounds, "-c", prepared.EBPF)
	command.Dir = options.SingBoxDir
	command.Stdout = logFile
	command.Stderr = logFile
	detachServiceCommand(command)
	return command, logFile, nil
}

func checkPreparedConfiguration(ctx context.Context, options Options, prepared PrepareResult) error {
	command := exec.CommandContext(ctx, options.SingBoxPath, "check", "-C", paths.SingBoxConfDir(options.SingBoxDir),
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
			logService(options, "WARN", "service.stop", "forced", "sing-box 未在限定时间内退出，改用 SIGKILL")
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
	var stopErr error
	if pid > 0 {
		stopErr = terminateServiceForStart(options, pid)
	}
	cleanupRuntimeFiles(options)
	stateErr := writeServiceState(options.StateFile, "failed", int64(pid), startedAt, 0, message)
	logService(options, "ERROR", "service.start", "failed", "%s: %v", message, cause)
	var convergeErr error
	if stateErr != nil {
		if err := os.Remove(options.StateFile); err != nil && !os.IsNotExist(err) {
			convergeErr = fmt.Errorf("清理未收敛的服务状态失败: %w", err)
		}
	}
	return errors.Join(fmt.Errorf("%s: %w", message, cause), stopErr, stateErr, convergeErr)
}

func failServiceStateWrite(options Options, pid int, startedAt int64, state string, cause error) error {
	return failServiceStart(options, pid, startedAt, fmt.Sprintf("写入 sing-box %s 状态失败", state), cause)
}

func restoreReloadState(ctx context.Context, options Options, pid int, startedAt, readyAt int64, cause error) error {
	if serviceProcessAlive(pid) {
		if currentStartedAt, err := serviceStartedAt(ctx, options); err == nil {
			startedAt = currentStartedAt
		}
		stateErr := writeServiceState(options.StateFile, "ready", int64(pid), startedAt, readyAt, cause.Error())
		if stateErr != nil {
			cause = errors.Join(cause, stateErr)
		}
	}
	logService(options, "ERROR", "service.reload", "failed", "sing-box 原位重新加载失败: %v", cause)
	return fmt.Errorf("sing-box 原位重新加载失败: %w", cause)
}

func cleanupRuntimeFiles(options Options) {
	for _, name := range []string{"providers.json", "outbounds.json", "ebpf.json"} {
		_ = os.Remove(filepath.Join(options.RuntimeDir, name))
	}
}

func logService(options Options, level, event, result, format string, args ...any) {
	logEvent(options, level, "service", event, result, format, args...)
}
