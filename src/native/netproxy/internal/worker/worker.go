package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/logfile"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

const defaultServiceSecret = "singbox"

const (
	workerTransientRetryBase = time.Minute
	workerTransientRetryMax  = time.Hour
	workerPermanentRetryBase = 15 * time.Minute
	workerPermanentRetryMax  = 6 * time.Hour
	runtimeVerifyTimeout     = 5 * time.Second
	runtimeVerifyInterval    = 100 * time.Millisecond
)

var (
	workerProcessRunning = isProcessRunning
	workerProcessPID     = isWorkerProcessPID
	workerVerifyRuntime  = verifyRuntimeState
)

// logWorker 按 Native 统一事件格式写入 Worker 日志。
func logWorker(logger *log.Logger, level, event, result, format string, args ...any) {
	if logger != nil {
		logger.Print(logfile.FormatEntry(logfile.Entry{
			Level: level, Component: "worker", Event: event, Result: result,
			Message: fmt.Sprintf(format, args...),
		}))
	}
}

// Timer 描述 Worker 调度所需的最小定时器接口。
type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

// TimerFactory 创建 Worker 调度定时器；测试可以用虚拟时钟替换系统时间。
type TimerFactory func(time.Duration) Timer

// Options 描述后台 Worker 的运行环境。
type Options struct {
	Root           string
	ProgressDir    string
	PIDFile        string
	LogFile        string
	ModuleConf     string
	SingBoxPath    string
	ServiceAddress string
	ServiceSecret  string
	ProxyURL       string
	FallbackDirect bool
	// PersistedBeforeUpdate 表示订阅编辑已保存设置，更新失败时不得误报为未保存。
	PersistedBeforeUpdate   bool
	NetworkWatchEnabled     bool
	SyncCatalog             func(context.Context, string, bool) (string, bool, error)
	NetworkEvaluate         func(context.Context, string, string) error
	NetworkEventSource      NetworkEventSource
	NetworkStateReader      NetworkStateReader
	NetworkDebounceInterval time.Duration
	Now                     func() time.Time
	NewTimer                TimerFactory
	CoreRunning             func() bool
	Telemetry               interface {
		Run(context.Context, func() bool, func())
		Notify()
	}
}

// Summary 是一次调度轮次的结果。
type Summary struct {
	Updated []string `json:"updated"`
	Failed  []string `json:"failed"`
	Nearest int64    `json:"nearest"`
}

type subscriptionRetry struct {
	attempt int
	epoch   int64
}

type workerFailureKind uint8

const (
	workerFailurePermanent workerFailureKind = iota
	workerFailureTransient
)

// Status 描述 Worker 的进程和下一次任务。
type Status struct {
	State   string `json:"state"`
	PID     int    `json:"pid,omitzero"`
	Nearest int64  `json:"nearest"`
}

// NewOptions 返回模块默认的 Worker 配置。
func NewOptions(root string) Options {
	layout := paths.Default()
	return Options{
		Root:           root,
		ProgressDir:    layout.ProgressDir(),
		PIDFile:        layout.WorkerPID(),
		ServiceAddress: "127.0.0.1:9090",
		ServiceSecret:  defaultServiceSecret,
		Now:            time.Now,
	}
}

// Run 执行订阅调度循环。wake 通道收到信号后立即重新计算任务。
func Run(ctx context.Context, options Options, wake <-chan struct{}, logger *log.Logger) error {
	if err := validateOptions(options); err != nil {
		return err
	}
	if logger == nil {
		logger = log.New(os.Stderr, "", 0)
	}
	if err := acquirePID(options.PIDFile); err != nil {
		return err
	}
	defer releasePID(options.PIDFile)
	background, cancelBackground := context.WithCancel(ctx)
	var telemetryDone chan struct{}
	if options.Telemetry != nil {
		telemetryDone = make(chan struct{})
		go func() {
			defer close(telemetryDone)
			options.Telemetry.Run(background, options.CoreRunning, func() {
				logWorker(logger, "WARN", "telemetry.upload", "waiting", "设备统计暂未完成，将稍后重试")
			})
		}()
	}

	networkWatchEnabled := options.NetworkWatchEnabled && options.NetworkEvaluate != nil
	var networkDone chan struct{}
	if networkWatchEnabled {
		networkDone = make(chan struct{})
		go func() {
			defer close(networkDone)
			runNetworkWatcher(background, options, logger)
		}()
		logWorker(logger, "INFO", "network.watch", "started", "Android 网络事件监听已启动")
	}
	defer func() {
		cancelBackground()
		if telemetryDone != nil {
			<-telemetryDone
		}
		if networkDone != nil {
			<-networkDone
		}
	}()

	logWorker(logger, "INFO", "worker.run", "started", "后台 Worker 已启动")
	consecutiveFailures := 0
	retries := make(map[string]subscriptionRetry)
	for {
		now := options.Now()
		_, err := runDue(ctx, options, now, logger, retries)
		var nearest int64
		if err == nil {
			now = options.Now()
			var schedule catalog.ScheduleResult
			schedule, err = subscriptionSchedule(ctx, options.Root, now.Unix(), retries)
			nearest = schedule.Nearest
		}
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			logWorker(logger, "ERROR", "subscription.schedule", "failed", "读取订阅调度失败: %v", err)
			consecutiveFailures++
			nearest = now.Unix() + int64(workerRetryDelay(consecutiveFailures, classifyWorkerError(err))/time.Second)
		} else {
			consecutiveFailures = 0
		}
		if nearest == 0 && !networkWatchEnabled && options.Telemetry == nil {
			logWorker(logger, "INFO", "worker.run", "stopped", "没有启用自动更新的订阅，Worker 退出")
			return nil
		}
		if nearest == 0 {
			nearest = now.Unix() + int64((24*time.Hour)/time.Second)
		}
		delay := time.Duration(nearest-now.Unix()) * time.Second
		if delay < time.Second {
			delay = time.Second
		}
		timer := newTimer(options, delay)
	wait:
		for {
			select {
			case <-ctx.Done():
				stopTimer(timer)
				logWorker(logger, "INFO", "worker.run", "stopped", "后台 Worker 已停止")
				return nil
			case <-wake:
				if options.Telemetry != nil {
					options.Telemetry.Notify()
				}
				stopTimer(timer)
				break wait
			case <-timer.C():
				break wait
			}
		}
	}
}

func subscriptionSchedule(ctx context.Context, root string, now int64, retries map[string]subscriptionRetry) (catalog.ScheduleResult, error) {
	schedule, err := catalog.Schedule(ctx, root, now)
	if err != nil {
		return schedule, err
	}
	for groupID := range retries {
		if _, enabled := schedule.NextByGroup[groupID]; !enabled {
			delete(retries, groupID)
		}
	}
	if len(retries) == 0 {
		return schedule, nil
	}
	schedule.Nearest, schedule.Due = 0, schedule.Due[:0]
	for _, groupID := range slices.Sorted(maps.Keys(schedule.NextByGroup)) {
		epoch := schedule.NextByGroup[groupID]
		if retry, exists := retries[groupID]; exists {
			epoch = retry.epoch
		}
		if schedule.Nearest == 0 || epoch < schedule.Nearest {
			schedule.Nearest = epoch
		}
		if epoch <= now {
			schedule.Due = append(schedule.Due, groupID)
		}
	}
	return schedule, nil
}

type systemTimer struct {
	timer *time.Timer
}

func (timer systemTimer) C() <-chan time.Time {
	return timer.timer.C
}

func (timer systemTimer) Stop() bool {
	return timer.timer.Stop()
}

func newTimer(options Options, duration time.Duration) Timer {
	if options.NewTimer != nil {
		return options.NewTimer(duration)
	}
	return systemTimer{timer: time.NewTimer(duration)}
}

func stopTimer(timer Timer) {
	if timer == nil || timer.Stop() {
		return
	}
	select {
	case <-timer.C():
	default:
	}
}

func runDue(ctx context.Context, options Options, now time.Time, logger *log.Logger, retries map[string]subscriptionRetry) (Summary, error) {
	schedule, err := subscriptionSchedule(ctx, options.Root, now.Unix(), retries)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Updated: []string{}, Failed: []string{}, Nearest: schedule.Nearest}
	for _, groupID := range schedule.Due {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if logger != nil {
			logWorker(logger, "INFO", "subscription.update", "started", "自动更新到期订阅: %s", groupID)
		}
		updated, updateErr := UpdateGroup(ctx, options, groupID, now, logger)
		if updateErr != nil {
			summary.Failed = append(summary.Failed, groupID)
			if retries != nil {
				attempt := retries[groupID].attempt + 1
				delay := workerRetryDelay(attempt, classifyWorkerError(updateErr))
				retries[groupID] = subscriptionRetry{attempt: attempt, epoch: options.Now().Add(delay).Unix()}
			}
			if logger != nil {
				logWorker(logger, "ERROR", "subscription.update", "failed", "订阅更新失败: %s: %v", groupID, updateErr)
			}
			continue
		}
		delete(retries, groupID)
		summary.Updated = append(summary.Updated, groupID)
		logWorker(logger, "INFO", "subscription.update", "success", "订阅更新完成: %s，节点 %d，运行时状态 %s", groupID, updated.NodeCount, updated.RuntimeSyncState)
	}
	return summary, nil
}

func workerRetryDelay(attempt int, kind workerFailureKind) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	base, maximum := workerTransientRetryBase, workerTransientRetryMax
	if kind == workerFailurePermanent {
		base, maximum = workerPermanentRetryBase, workerPermanentRetryMax
	}
	delay := base
	for index := 1; index < attempt && delay < maximum; index++ {
		if delay > maximum/2 {
			delay = maximum
			break
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func classifyWorkerError(err error) workerFailureKind {
	if err == nil {
		return workerFailurePermanent
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return workerFailureTransient
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return workerFailureTransient
	}
	if subscriptionError, ok := errors.AsType[*subscription.Error](err); ok {
		switch subscriptionError.Code {
		case "subscription.runtime_sync_failed", "subscription.busy":
			return workerFailureTransient
		case "subscription.conflict":
			return workerFailurePermanent
		case "subscription.convert_failed":
			cause := strings.ToLower(subscriptionErrorCause(subscriptionError))
			if strings.Contains(cause, "request failed") || strings.Contains(cause, "timeout") ||
				strings.Contains(cause, "connection") || strings.Contains(cause, "dns") ||
				strings.Contains(cause, "http ") {
				return workerFailureTransient
			}
			return workerFailurePermanent
		default:
			return workerFailurePermanent
		}
	}
	return workerFailurePermanent
}

func subscriptionErrorCause(value *subscription.Error) string {
	if value == nil {
		return ""
	}
	data, ok := value.Data.(map[string]any)
	if !ok {
		return ""
	}
	cause, _ := data["cause"].(string)
	return cause
}

// UpdateGroup 执行单个订阅更新，并统一处理更新后的运行时状态。
func UpdateGroup(ctx context.Context, options Options, groupID string, now time.Time, logger *log.Logger) (subscription.Result, error) {
	runtimeRunning := workerProcessRunning(options.SingBoxPath)
	result, err := subscription.Update(ctx, subscription.UpdateOptions{
		Root: options.Root, GroupID: groupID, ProgressDir: options.ProgressDir,
		ProxyURL: options.ProxyURL, FallbackDirect: options.FallbackDirect,
		UseConfiguredProxy: true, RuntimeSyncPending: runtimeRunning,
		PersistedBeforeUpdate: options.PersistedBeforeUpdate, Now: now,
	})
	if err != nil {
		providerPersisted := result.Persisted
		if options.PersistedBeforeUpdate && !providerPersisted {
			result.GroupID = groupID
			result.Persisted = true
			return result, subscription.MarkPersistedError(err)
		}
		if result.Persisted {
			// Provider 与 metadata 已提交时，历史写入失败不能阻止运行中的核心应用新 Provider。
			// 原始历史错误仍作为主错误返回，避免客户端把持久化副作用误报为未保存。
			synced, syncErr := applyRuntimeSync(ctx, options, result, groupID, logger, false, now)
			if syncErr != nil {
				return synced, errors.Join(err, syncErr)
			}
			return synced, err
		}
		return result, err
	}
	return applyRuntimeSync(ctx, options, result, groupID, logger, false, now)
}

// SyncEditedGroup 将已持久化的订阅编辑通过统一运行时流程应用到 sing-box。
func SyncEditedGroup(ctx context.Context, options Options, groupID string, now time.Time, logger *log.Logger) (subscription.Result, error) {
	result := subscription.Result{GroupID: groupID, Persisted: true}
	localContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	metadata, err := catalog.LoadMetadata(localContext, filepath.Join(options.Root, groupID, "meta.json"), groupID)
	if err != nil {
		return result, persistedEffectFailure(result, err)
	}
	result.NodeCount = metadata.NodeCount
	result.Revision = metadata.Revision
	result.RuntimeSyncState = metadata.RuntimeSyncState
	result.RuntimeSyncPending = metadata.RuntimeSyncPending
	if workerProcessRunning(options.SingBoxPath) {
		result.RuntimeSyncPending = true
	}
	if now.IsZero() {
		now = time.Now()
	}
	return applyRuntimeSync(ctx, options, result, groupID, logger, true, now)
}

func applyRuntimeSync(ctx context.Context, options Options, result subscription.Result, groupID string, logger *log.Logger, forceReload bool, now time.Time) (subscription.Result, error) {
	runtimeState, runtimeAttempted, effectErr := applyUpdateEffects(ctx, options, result, groupID, forceReload)
	// 已提交的同步状态必须落盘，取消只阻止运行时操作。
	localContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	result.RuntimeSyncState = runtimeState
	result.RuntimeSynced = runtimeState == subscription.RuntimeSyncApplied
	if effectErr != nil {
		if runtimeAttempted {
			if err := subscription.RecordRuntimeSyncFailure(localContext, options.Root, result.GroupID, effectErr, now); err != nil {
				effectErr = errors.Join(effectErr, err)
			}
			result.RuntimeSyncPending = true
			if logger != nil {
				logWorker(logger, "ERROR", "subscription.runtime-sync", "failed", "订阅已持久化，但运行时应用失败: %s: %v", groupID, effectErr)
			}
			return result, runtimeSyncFailure(result, effectErr)
		}
		pending := result.RuntimeSyncPending || runtimeState != subscription.RuntimeSyncNotRunning
		if err := subscription.RecordPersistedEffectFailure(localContext, options.Root, result.GroupID, pending, effectErr, now); err != nil {
			effectErr = errors.Join(effectErr, err)
		}
		result.RuntimeSyncPending = pending
		if logger != nil {
			logWorker(logger, "ERROR", "subscription.effect", "failed", "订阅已持久化，但本地状态副作用失败: %s: %v", groupID, effectErr)
		}
		return result, persistedEffectFailure(result, effectErr)
	}
	if runtimeState == subscription.RuntimeSyncNotRunning {
		if err := subscription.RecordRuntimeSyncNotRunning(localContext, options.Root, result.GroupID, now); err != nil {
			return result, persistedEffectFailure(result, err)
		}
	}
	if runtimeState == subscription.RuntimeSyncApplied {
		if err := subscription.RecordRuntimeSyncSuccess(localContext, options.Root, result.GroupID, now); err != nil {
			result.RuntimeSyncPending = true
			return result, persistedEffectFailure(result, err)
		}
		result.RuntimeSyncPending = false
	}
	return result, nil
}

// NextUpdate 返回下一次自动更新时间；没有自动订阅时返回 0。
func NextUpdate(ctx context.Context, root string, now time.Time) (int64, error) {
	if now.IsZero() {
		now = time.Now()
	}
	return nextUpdate(ctx, root, now.Unix())
}

func nextUpdate(ctx context.Context, root string, now int64) (int64, error) {
	schedule, err := catalog.Schedule(ctx, root, now)
	if err != nil {
		return 0, err
	}
	return schedule.Nearest, nil
}

func applyUpdateEffects(ctx context.Context, options Options, result subscription.Result, groupID string, forceReload bool) (string, bool, error) {
	if options.SyncCatalog == nil {
		return currentRuntimeSyncState(options), false, errors.New("未配置 Catalog 同步回调")
	}
	state, attempted, err := options.SyncCatalog(ctx, groupID, forceReload || result.StructureChanged)
	if err != nil {
		if attempted {
			return subscription.RuntimeSyncFailed, true, err
		}
		return currentRuntimeSyncState(options), attempted, err
	}
	if state == subscription.RuntimeSyncNotRunning {
		return state, attempted, nil
	}
	if state != subscription.RuntimeSyncApplied {
		return currentRuntimeSyncState(options), attempted, fmt.Errorf("Catalog 同步返回无效状态: %s", state)
	}
	if err := workerVerifyRuntime(ctx, options, groupID); err != nil {
		return subscription.RuntimeSyncFailed, true, err
	}
	return subscription.RuntimeSyncApplied, true, nil
}

func currentRuntimeSyncState(options Options) string {
	if workerProcessRunning(options.SingBoxPath) {
		return subscription.RuntimeSyncFailed
	}
	return subscription.RuntimeSyncNotRunning
}

func runtimeSyncFailure(result subscription.Result, cause error) error {
	return &subscription.Error{
		Code:    "subscription.runtime_sync_failed",
		Message: subscription.RuntimeSyncFailureMessage,
		Data: map[string]any{
			"group_id":             result.GroupID,
			"persisted":            result.Persisted,
			"runtime_synced":       false,
			"runtime_sync_state":   subscription.RuntimeSyncFailed,
			"runtime_sync_pending": true,
			"cause":                cause.Error(),
		},
	}
}

func persistedEffectFailure(result subscription.Result, cause error) error {
	return &subscription.Error{
		Code:    "subscription.persisted_effect_failed",
		Message: subscription.PersistedEffectFailureMessage,
		Data: map[string]any{
			"group_id":             result.GroupID,
			"persisted":            result.Persisted,
			"runtime_synced":       result.RuntimeSynced,
			"runtime_sync_state":   result.RuntimeSyncState,
			"runtime_sync_pending": result.RuntimeSyncPending,
			"cause":                cause.Error(),
		},
	}
}

func verifyRuntimeState(ctx context.Context, options Options, groupID string) error {
	runtimeTag, err := catalog.RuntimeTag(ctx, options.Root, groupID)
	if err != nil {
		return err
	}
	document, err := provider.Load(ctx, filepath.Join(options.Root, groupID, "provider.json"))
	if err != nil {
		return fmt.Errorf("读取已持久化 Provider 失败: %w", err)
	}
	expected := make(map[string]struct{})
	for _, node := range provider.Inspect(document) {
		expected[node.Tag] = struct{}{}
	}
	if len(expected) == 0 {
		return fmt.Errorf("运行时 Provider %s 没有可验证节点", runtimeTag)
	}
	client, err := serviceapi.New(options.ServiceAddress, options.ServiceSecret)
	if err != nil {
		return err
	}
	defer client.Close()
	requestContext, cancel := context.WithTimeout(ctx, runtimeVerifyTimeout)
	defer cancel()
	var lastErr error
	for {
		outbounds, requestErr := client.Outbounds(requestContext)
		if requestErr == nil {
			if runtimeProviderMatches(outbounds, runtimeTag, expected) {
				return nil
			}
			lastErr = fmt.Errorf("Service API 中的 Provider %s 节点与持久化内容不一致", runtimeTag)
		} else {
			lastErr = fmt.Errorf("读取 Service API 出站失败: %w", requestErr)
		}
		timer := time.NewTimer(runtimeVerifyInterval)
		select {
		case <-requestContext.Done():
			timer.Stop()
			return lastErr
		case <-timer.C:
		}
	}
}

func runtimeProviderMatches(outbounds []serviceapi.GroupItem, runtimeTag string, expected map[string]struct{}) bool {
	prefix := runtimeTag + "/"
	present := make(map[string]struct{}, len(expected))
	for _, outbound := range outbounds {
		if after, ok := strings.CutPrefix(outbound.Tag, prefix); ok {
			present[after] = struct{}{}
		}
	}
	if len(present) != len(expected) {
		return false
	}
	for tag := range expected {
		if _, exists := present[tag]; !exists {
			return false
		}
	}
	return true
}

func validateOptions(options Options) error {
	for name, value := range map[string]string{"Catalog 根目录": options.Root, "PID 文件": options.PIDFile, "模块配置": options.ModuleConf} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s不能为空", name)
		}
	}
	if options.Now == nil {
		return errors.New("Worker 时钟不能为空")
	}
	return nil
}

func acquirePID(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	lock := path + ".lock"
	if err := os.Mkdir(lock, 0o700); err != nil {
		if os.IsExist(err) {
			owner := readPID(filepath.Join(lock, "pid"))
			if owner > 0 && workerProcessPID(owner) {
				return errors.New("后台 Worker 已在运行")
			}
			_ = os.RemoveAll(lock)
			if err = os.Mkdir(lock, 0o700); err != nil {
				return err
			}
		}
		if !os.IsExist(err) && err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(lock, "pid"), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		_ = os.RemoveAll(lock)
		return err
	}
	if pid := readPID(path); pid > 0 && pid != os.Getpid() && workerProcessPID(pid) {
		_ = os.RemoveAll(lock)
		return fmt.Errorf("后台 Worker 已在运行: %d", pid)
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		_ = os.RemoveAll(lock)
		return err
	}
	return nil
}

func releasePID(path string) {
	if readPID(path) == os.Getpid() && readPID(filepath.Join(path+".lock", "pid")) == os.Getpid() {
		_ = os.Remove(path)
		_ = os.RemoveAll(path + ".lock")
	}
}

func readPID(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var pid int
	_, _ = fmt.Sscanf(string(content), "%d", &pid)
	return pid
}

// ReadStatus 返回 Worker 当前状态，不会启动 Worker。
func ReadStatus(ctx context.Context, options Options) (Status, error) {
	if err := validateOptions(options); err != nil {
		return Status{}, err
	}
	pid := readPID(options.PIDFile)
	if pid <= 0 || !workerProcessPID(pid) {
		return Status{State: "stopped"}, nil
	}
	nearest, err := NextUpdate(ctx, options.Root, options.Now())
	if err != nil {
		return Status{}, err
	}
	return Status{State: "running", PID: pid, Nearest: nearest}, nil
}
