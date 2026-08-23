package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
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
	workerProcessRunning   = isProcessRunning
	workerProcessPID       = isWorkerProcessPID
	workerReloadService    = reloadService
	workerVerifyRuntime    = verifyRuntimeState
	workerLoadModule       = moduleconfig.LoadModule
	workerUpdateModule     = moduleconfig.UpdateModule
	workerGroupHasNodes    = catalog.GroupHasNodes
	workerGroupContainsTag = catalog.GroupContainsTag
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
	NativePath     string
	SingBoxPath    string
	ServiceAddress string
	ServiceSecret  string
	ProxyURL       string
	FallbackDirect bool
	// PersistedBeforeUpdate 表示订阅编辑已保存设置，更新失败时不得误报为未保存。
	PersistedBeforeUpdate   bool
	NetworkWatchEnabled     bool
	NetworkEvaluate         func(context.Context, string, string) error
	NetworkEventSource      NetworkEventSource
	NetworkStateReader      NetworkStateReader
	NetworkDebounceInterval time.Duration
	Now                     func() time.Time
	NewTimer                TimerFactory
}

// Summary 是一次调度轮次的结果。
type Summary struct {
	Updated      []string `json:"updated"`
	Failed       []string `json:"failed"`
	Nearest      int64    `json:"nearest"`
	failureKinds []workerFailureKind
}

type workerFailureKind uint8

const (
	workerFailurePermanent workerFailureKind = iota
	workerFailureTransient
)

// Status 描述 Worker 的进程和下一次任务。
type Status struct {
	State   string `json:"state"`
	PID     int    `json:"pid,omitempty"`
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

	networkWatchEnabled := options.NetworkWatchEnabled && options.NetworkEvaluate != nil
	var networkDone chan struct{}
	if networkWatchEnabled {
		networkDone = make(chan struct{})
		go func() {
			defer close(networkDone)
			runNetworkWatcher(ctx, options, logger)
		}()
		logWorker(logger, "INFO", "network.watch", "started", "Android 网络事件监听已启动")
	}
	defer func() {
		if networkDone != nil {
			<-networkDone
		}
	}()

	logWorker(logger, "INFO", "worker.run", "started", "后台 Worker 已启动")
	consecutiveFailures := 0
	retryGroups := make(map[string]struct{})
	for {
		now := options.Now()
		summary, err := RunDue(ctx, options, now, logger)
		if err != nil {
			logWorker(logger, "ERROR", "subscription.schedule", "failed", "读取订阅调度失败: %v", err)
		}
		if err == nil {
			for _, groupID := range summary.Updated {
				delete(retryGroups, groupID)
			}
			for _, groupID := range summary.Failed {
				retryGroups[groupID] = struct{}{}
			}
			updateRetryGroups(ctx, options, now, logger, retryGroups, &summary)
		}
		failure := err != nil || len(summary.Failed) > 0
		var retryDelay time.Duration
		if failure {
			consecutiveFailures++
			retryDelay = workerRetryDelay(consecutiveFailures, summary.failureKind(err))
		} else {
			consecutiveFailures = 0
		}
		var nearest int64
		if failure {
			nearest = now.Unix() + int64(retryDelay/time.Second)
		} else {
			nearest, err = nextUpdate(options.Root, now.Unix())
			if err != nil {
				logWorker(logger, "ERROR", "subscription.schedule", "failed", "计算下一次订阅更新时间失败: %v", err)
				consecutiveFailures++
				retryDelay = workerRetryDelay(consecutiveFailures, classifyWorkerError(err))
				nearest = now.Unix() + int64(retryDelay/time.Second)
				failure = true
			}
		}
		if nearest == 0 && !networkWatchEnabled {
			logWorker(logger, "INFO", "worker.run", "stopped", "没有启用自动更新的订阅，Worker 退出")
			return nil
		}
		if nearest == 0 {
			nearest = now.Unix() + int64((24*time.Hour)/time.Second)
		}
		delay := time.Duration(nearest-now.Unix()) * time.Second
		if failure && retryDelay > 0 {
			delay = retryDelay
		}
		if delay < time.Second {
			delay = time.Second
		}
		timer := newTimer(options, delay)
		if failure {
			select {
			case <-ctx.Done():
				stopTimer(timer)
				logWorker(logger, "INFO", "worker.run", "stopped", "后台 Worker 已停止")
				return nil
			case <-timer.C():
			}
			continue
		}
		select {
		case <-ctx.Done():
			stopTimer(timer)
			logWorker(logger, "INFO", "worker.run", "stopped", "后台 Worker 已停止")
			return nil
		case <-wake:
			stopTimer(timer)
		case <-timer.C():
		}
	}
}

func updateRetryGroups(ctx context.Context, options Options, now time.Time, logger *log.Logger, retryGroups map[string]struct{}, summary *Summary) {
	if summary == nil || len(retryGroups) == 0 {
		return
	}
	attempted := make(map[string]struct{}, len(summary.Updated)+len(summary.Failed))
	for _, groupID := range summary.Updated {
		attempted[groupID] = struct{}{}
	}
	for _, groupID := range summary.Failed {
		attempted[groupID] = struct{}{}
	}
	for groupID := range retryGroups {
		if _, ok := attempted[groupID]; ok {
			continue
		}
		if err := ctx.Err(); err != nil {
			return
		}
		_, updateErr := UpdateGroup(ctx, options, groupID, now, logger)
		if updateErr != nil {
			summary.Failed = append(summary.Failed, groupID)
			summary.failureKinds = append(summary.failureKinds, classifyWorkerError(updateErr))
			if logger != nil {
				logWorker(logger, "WARN", "subscription.update", "failed", "订阅退避重试失败: %s: %v", groupID, updateErr)
			}
			continue
		}
		summary.Updated = append(summary.Updated, groupID)
		logWorker(logger, "INFO", "subscription.update", "success", "订阅退避重试成功: %s", groupID)
		delete(retryGroups, groupID)
	}
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

// RunDue 顺序执行当前已经到期的订阅更新。
func RunDue(ctx context.Context, options Options, now time.Time, logger *log.Logger) (Summary, error) {
	if err := validateOptions(options); err != nil {
		return Summary{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	schedule, err := catalog.Schedule(options.Root, now.Unix())
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
			summary.failureKinds = append(summary.failureKinds, classifyWorkerError(updateErr))
			if logger != nil {
				logWorker(logger, "ERROR", "subscription.update", "failed", "订阅更新失败: %s: %v", groupID, updateErr)
			}
			continue
		}
		summary.Updated = append(summary.Updated, groupID)
		logWorker(logger, "INFO", "subscription.update", "success", "订阅更新完成: %s，节点 %d，运行时状态 %s", groupID, updated.NodeCount, updated.RuntimeSyncState)
	}
	return summary, nil
}

// UpdateGroup 执行单个订阅更新，并统一处理更新后的运行时状态。
func (summary Summary) failureKind(runErr error) workerFailureKind {
	if runErr != nil {
		return classifyWorkerError(runErr)
	}
	for _, kind := range summary.failureKinds {
		if kind == workerFailurePermanent {
			return workerFailurePermanent
		}
	}
	return workerFailureTransient
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
	var subscriptionError *subscription.Error
	if errors.As(err, &subscriptionError) {
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
	metadata, err := catalog.LoadMetadata(filepath.Join(options.Root, groupID, "meta.json"), groupID)
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
	runtimeState, runtimeAttempted, effectErr := applyUpdateEffects(ctx, options, result, groupID, logger, forceReload)
	result.RuntimeSyncState = runtimeState
	result.RuntimeSynced = runtimeState == subscription.RuntimeSyncApplied
	if effectErr != nil {
		if runtimeAttempted {
			if err := subscription.RecordRuntimeSyncFailure(ctx, options.Root, result.GroupID, effectErr, now); err != nil {
				effectErr = errors.Join(effectErr, err)
			}
			result.RuntimeSyncPending = true
			if logger != nil {
				logWorker(logger, "ERROR", "subscription.runtime-sync", "failed", "订阅已持久化，但运行时应用失败: %s: %v", groupID, effectErr)
			}
			return result, runtimeSyncFailure(result, effectErr)
		}
		pending := result.RuntimeSyncPending || runtimeState != subscription.RuntimeSyncNotRunning
		if err := subscription.RecordPersistedEffectFailure(ctx, options.Root, result.GroupID, pending, effectErr, now); err != nil {
			effectErr = errors.Join(effectErr, err)
		}
		result.RuntimeSyncPending = pending
		if logger != nil {
			logWorker(logger, "ERROR", "subscription.effect", "failed", "订阅已持久化，但本地状态副作用失败: %s: %v", groupID, effectErr)
		}
		return result, persistedEffectFailure(result, effectErr)
	}
	if runtimeState == subscription.RuntimeSyncNotRunning {
		if err := subscription.RecordRuntimeSyncNotRunning(ctx, options.Root, result.GroupID, now); err != nil {
			return result, persistedEffectFailure(result, err)
		}
	}
	if runtimeState == subscription.RuntimeSyncApplied {
		if err := subscription.RecordRuntimeSyncSuccess(ctx, options.Root, result.GroupID, now); err != nil {
			result.RuntimeSyncPending = true
			return result, persistedEffectFailure(result, err)
		}
		result.RuntimeSyncPending = false
	}
	return result, nil
}

// NextUpdate 返回下一次自动更新时间；没有自动订阅时返回 0。
func NextUpdate(root string, now time.Time) (int64, error) {
	if now.IsZero() {
		now = time.Now()
	}
	return nextUpdate(root, now.Unix())
}

func nextUpdate(root string, now int64) (int64, error) {
	schedule, err := catalog.Schedule(root, now)
	if err != nil {
		return 0, err
	}
	return schedule.Nearest, nil
}

func applyUpdateEffects(ctx context.Context, options Options, result subscription.Result, groupID string, logger *log.Logger, forceReload bool) (string, bool, error) {
	activated, err := activateGroupIfNeeded(ctx, options, groupID)
	if err != nil {
		return currentRuntimeSyncState(options), false, err
	}
	if err := fallbackMissingNode(ctx, options, groupID, logger); err != nil {
		return currentRuntimeSyncState(options), false, err
	}
	if !workerProcessRunning(options.SingBoxPath) {
		return subscription.RuntimeSyncNotRunning, false, nil
	}
	reloaded := forceReload || result.StructureChanged || activated
	if reloaded {
		if err := workerReloadService(ctx, options); err != nil {
			return subscription.RuntimeSyncFailed, true, err
		}
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
	runtimeTag, err := catalog.RuntimeTag(options.Root, groupID)
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
		if strings.HasPrefix(outbound.Tag, prefix) {
			present[strings.TrimPrefix(outbound.Tag, prefix)] = struct{}{}
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

func activateGroupIfNeeded(ctx context.Context, options Options, groupID string) (bool, error) {
	module, err := workerLoadModule(options.ModuleConf)
	if err != nil {
		return false, err
	}
	active := module.ActiveGroupID
	if active != "" {
		hasNodes, hasErr := workerGroupHasNodes(ctx, options.Root, active)
		if hasErr != nil {
			return false, hasErr
		}
		if hasNodes {
			return false, nil
		}
	}
	hasNodes, err := workerGroupHasNodes(ctx, options.Root, groupID)
	if err != nil || !hasNodes {
		return false, err
	}
	if err := workerUpdateModule(options.ModuleConf, map[string]string{
		"ACTIVE_GROUP_ID":   moduleconfig.Quote(groupID),
		"SELECTOR_MODE":     "urltest",
		"SELECTED_NODE_REF": moduleconfig.Quote(""),
	}); err != nil {
		return false, err
	}
	return true, nil
}

func fallbackMissingNode(ctx context.Context, options Options, groupID string, logger *log.Logger) error {
	module, err := workerLoadModule(options.ModuleConf)
	if err != nil || module.SelectorMode != "manual" {
		return err
	}
	selected := module.SelectedNodeRef
	if selected == "" {
		return err
	}
	selectedGroup, selectedTag, found := strings.Cut(selected, "/")
	if !found || selectedGroup != groupID || selectedTag == "" {
		return nil
	}
	present, err := workerGroupContainsTag(ctx, options.Root, groupID, selectedTag)
	if err != nil || present {
		return err
	}
	if err := workerUpdateModule(options.ModuleConf, map[string]string{
		"SELECTOR_MODE":     "urltest",
		"SELECTED_NODE_REF": moduleconfig.Quote(""),
	}); err != nil {
		return err
	}
	runtimeTag, err := catalog.RuntimeTag(options.Root, groupID)
	if err != nil {
		return err
	}
	if logger != nil {
		logWorker(logger, "WARN", "node.selection", "fallback", "手动节点已从 Provider 移除，回退到 Auto/%s", runtimeTag)
	}
	if !isProcessRunning(options.SingBoxPath) {
		return nil
	}
	client, err := serviceapi.New(options.ServiceAddress, options.ServiceSecret)
	if err != nil {
		return err
	}
	defer client.Close()
	requestContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return client.Select(requestContext, "Proxy", "Auto/"+runtimeTag)
}

func reloadService(ctx context.Context, options Options) error {
	if strings.TrimSpace(options.NativePath) == "" {
		return errors.New("未配置 NetProxy 原生组件路径")
	}
	command := exec.CommandContext(ctx, options.NativePath, reloadServiceArguments(options)...)
	command.Stdout = io.Discard
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if diagnostic := strings.TrimSpace(stderr.String()); diagnostic != "" {
			return fmt.Errorf("服务 reload 失败: %s: %w", diagnostic, err)
		}
		return fmt.Errorf("服务 reload 失败: %w", err)
	}
	return nil
}

func reloadServiceArguments(options Options) []string {
	moduleDir := filepath.Dir(filepath.Dir(options.Root))
	return []string{
		"module", "service", "reload",
		"--module-dir", moduleDir,
		"--catalog-root", options.Root,
		"--module-config", options.ModuleConf,
		"--sing-box", options.SingBoxPath,
		"--address", options.ServiceAddress,
		"--secret", options.ServiceSecret,
	}
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
func ReadStatus(options Options) (Status, error) {
	if err := validateOptions(options); err != nil {
		return Status{}, err
	}
	pid := readPID(options.PIDFile)
	if pid <= 0 || !workerProcessPID(pid) {
		return Status{State: "stopped"}, nil
	}
	nearest, err := NextUpdate(options.Root, options.Now())
	if err != nil {
		return Status{}, err
	}
	return Status{State: "running", PID: pid, Nearest: nearest}, nil
}
