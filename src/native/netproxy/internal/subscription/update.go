package subscription

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"encoding/json/jsontext"
	json "encoding/json/v2"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/convert"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/fetch"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const (
	defaultInterval = 24 * time.Hour
	minimumInterval = 15 * time.Minute
	maxHistory      = 20
	cancelPoll      = 100 * time.Millisecond
)

const (
	RuntimeSyncApplied            = "applied"
	RuntimeSyncNotRunning         = "not_running"
	RuntimeSyncFailed             = "failed"
	RuntimeSyncFailureMessage     = "订阅已持久化，但运行时应用失败"
	PersistedEffectFailureMessage = "订阅已持久化，但本地状态副作用失败"
)

// UpdateOptions 定义一次订阅更新的运行上下文。
type UpdateOptions struct {
	Root               string
	GroupID            string
	ProgressDir        string
	ProxyURL           string
	UseConfiguredProxy bool
	FallbackDirect     bool
	RuntimeSyncPending bool
	// PersistedBeforeUpdate 表示调用方已先保存编辑设置；更新失败时仍须在错误数据中报告已保存。
	PersistedBeforeUpdate bool
	Now                   time.Time
	snapshot              *catalog.Metadata
}

// Result 是订阅更新对 Shell 暴露的最小结构化结果。
type Result struct {
	GroupID            string `json:"group_id"`
	NodeCount          int    `json:"node_count"`
	Revision           int64  `json:"revision"`
	NotModified        bool   `json:"not_modified"`
	StructureChanged   bool   `json:"structure_changed"`
	UsedProxy          bool   `json:"used_proxy"`
	Persisted          bool   `json:"persisted"`
	RuntimeSynced      bool   `json:"runtime_synced"`
	RuntimeSyncState   string `json:"runtime_sync_state"`
	RuntimeSyncPending bool   `json:"runtime_sync_pending"`
}

// Error 是可以直接映射为 schema=1 错误响应的订阅错误。
type Error struct {
	Code    string
	Message string
	Data    any
}

func (e *Error) Error() string { return e.Message }

// MarkPersistedError 在结构化错误中补充“编辑设置已保存”状态。
func MarkPersistedError(err error) error {
	if err == nil {
		return nil
	}
	var structured *Error
	if !errors.As(err, &structured) {
		return err
	}
	data := map[string]any{"persisted": true}
	switch value := structured.Data.(type) {
	case map[string]any:
		maps.Copy(data, value)
	case nil:
		// 保留仅新增的 persisted 字段。
	default:
		data["cause"] = value
	}
	return &Error{Code: structured.Code, Message: structured.Message, Data: data}
}

// RecordRuntimeSyncFailure 记录运行时应用失败，并保留 HTTP 更新成功时间。
func RecordRuntimeSyncFailure(ctx context.Context, root, groupID string, cause error, now time.Time) error {
	_ = ctx
	message := RuntimeSyncFailureMessage
	if cause != nil {
		message += ": " + cause.Error()
	}
	return updateRuntimeMetadata(root, groupID, true, message, RuntimeSyncFailed, map[string]any{
		"at": formatTime(now), "ok": false, "code": "subscription.runtime_sync_failed",
		"message": RuntimeSyncFailureMessage, "cause": errorString(cause),
	}, now)
}

// RecordPersistedEffectFailure 记录本地状态副作用失败，并保留 HTTP 更新成功时间。
func RecordPersistedEffectFailure(ctx context.Context, root, groupID string, pending bool, cause error, now time.Time) error {
	_ = ctx
	message := PersistedEffectFailureMessage
	if cause != nil {
		message += ": " + cause.Error()
	}
	state := RuntimeSyncNotRunning
	if pending {
		state = RuntimeSyncFailed
	}
	return updateRuntimeMetadata(root, groupID, pending, message, state, map[string]any{
		"at": formatTime(now), "ok": false, "code": "subscription.persisted_effect_failed",
		"message": PersistedEffectFailureMessage, "cause": errorString(cause),
	}, now)
}

// RecordRuntimeSyncSuccess 清理运行时失败状态；仅在此前确有运行时失败时追加成功历史。
func RecordRuntimeSyncSuccess(ctx context.Context, root, groupID string, now time.Time) error {
	_ = ctx
	return updateRuntimeMetadata(root, groupID, false, "", RuntimeSyncApplied, nil, now)
}

// RecordRuntimeSyncNotRunning 记录服务未运行时已确认的持久化状态，并保留未完成的运行时同步错误。
func RecordRuntimeSyncNotRunning(ctx context.Context, root, groupID string, now time.Time) error {
	_ = ctx
	if strings.TrimSpace(root) == "" || !validGroupID(groupID) {
		return errors.New("订阅目录或分组无效")
	}
	releaseGroup, err := catalog.Acquire(root, groupID)
	if err != nil {
		return err
	}
	defer releaseGroup()
	releaseRoot, err := catalog.AcquireRoot(root)
	if err != nil {
		return err
	}
	defer releaseRoot()
	if err := catalog.RecoverLocked(root); err != nil {
		return err
	}
	metaPath := filepath.Join(root, groupID, "meta.json")
	metadata, err := catalog.LoadMetadataLocked(metaPath, groupID)
	if err != nil {
		return err
	}
	if metadata.RuntimeSyncState == RuntimeSyncNotRunning {
		return nil
	}
	metadata.RuntimeSyncState = RuntimeSyncNotRunning
	if !now.IsZero() {
		metadata.UpdatedAt = formatTime(now)
	}
	return catalog.SaveMetadataAtomicLocked(metaPath, metadata)
}

func updateRuntimeMetadata(root, groupID string, pending bool, lastError, state string, history map[string]any, now time.Time) error {
	if strings.TrimSpace(root) == "" || !validGroupID(groupID) {
		return errors.New("订阅目录或分组无效")
	}
	releaseGroup, err := catalog.Acquire(root, groupID)
	if err != nil {
		return err
	}
	defer releaseGroup()
	releaseRoot, err := catalog.AcquireRoot(root)
	if err != nil {
		return err
	}
	defer releaseRoot()
	if err := catalog.RecoverLocked(root); err != nil {
		return err
	}
	metaPath := filepath.Join(root, groupID, "meta.json")
	metadata, err := catalog.LoadMetadataLocked(metaPath, groupID)
	if err != nil {
		return err
	}
	previousMetadata := metadata
	previousPending := metadata.RuntimeSyncPending
	previousError := metadata.LastError
	appendSuccess := history == nil && previousPending && strings.HasPrefix(previousError, RuntimeSyncFailureMessage)
	if metadata.RuntimeSyncPending != pending || metadata.LastError != lastError || metadata.RuntimeSyncState != state {
		metadata.RuntimeSyncPending = pending
		metadata.LastError = lastError
		metadata.RuntimeSyncState = state
		if !now.IsZero() {
			metadata.UpdatedAt = formatTime(now)
		}
		if err := catalog.SaveMetadataAtomicLocked(metaPath, metadata); err != nil {
			return err
		}
	}
	if appendSuccess {
		history = map[string]any{
			"at": formatTime(now), "ok": true, "code": "subscription.runtime_sync_applied",
			"message": "订阅运行时应用成功",
		}
	}
	if history != nil {
		if err := appendHistoryChecked(filepath.Join(root, groupID), history); err != nil {
			if appendSuccess {
				if restoreErr := catalog.SaveMetadataAtomicLocked(metaPath, previousMetadata); restoreErr != nil {
					return errors.Join(err, restoreErr)
				}
			}
			return err
		}
	}
	return nil
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func isRuntimeSyncError(message string) bool {
	message = strings.TrimSpace(message)
	return strings.HasPrefix(message, RuntimeSyncFailureMessage) || strings.HasPrefix(message, PersistedEffectFailureMessage)
}

// RequestCancel 仅为仍持有活动更新锁的进程写入取消标记。
// 空闲任务和残留锁不能污染下一次订阅更新。
func RequestCancel(root, groupID, progressDir string) (bool, error) {
	if strings.TrimSpace(root) == "" || !validGroupID(groupID) {
		return false, &Error{Code: "subscription.invalid_target", Message: "订阅目录或分组无效"}
	}
	if strings.TrimSpace(progressDir) == "" {
		return false, errors.New("订阅进度目录不能为空")
	}
	cancelPath := filepath.Join(progressDir, groupID+".cancel")
	lockPath := filepath.Join(root, "staging", "locks", groupID+".lock")
	active, err := lockActive(lockPath)
	if err != nil {
		return false, err
	}
	if !active || !lockReady(lockPath) {
		_ = os.Remove(cancelPath)
		return false, nil
	}
	if err := os.MkdirAll(progressDir, 0o700); err != nil {
		return false, err
	}
	if err := provider.WriteAtomic(cancelPath, []byte("1\n"), 0o600); err != nil {
		return false, err
	}
	return true, nil
}

// Update 执行一次可回滚的订阅更新。
func Update(ctx context.Context, options UpdateOptions) (Result, error) {
	if strings.TrimSpace(options.Root) == "" || strings.TrimSpace(options.GroupID) == "" {
		return Result{}, &Error{Code: "subscription.invalid_target", Message: "订阅目录或分组为空"}
	}
	if !validGroupID(options.GroupID) {
		return Result{}, &Error{Code: "subscription.invalid_group", Message: "订阅分组 ID 无效"}
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	releaseGroup, err := catalog.Acquire(options.Root, options.GroupID)
	if err != nil {
		return Result{}, &Error{Code: "subscription.busy", Message: "订阅或 Catalog 正在被其他进程使用", Data: err.Error()}
	}
	defer releaseGroup()
	releaseRoot, err := catalog.AcquireRoot(options.Root)
	if err != nil {
		return Result{}, &Error{Code: "subscription.busy", Message: "订阅或 Catalog 正在被其他进程使用", Data: err.Error()}
	}
	rootHeld := true
	defer func() {
		if rootHeld {
			releaseRoot()
		}
	}()
	if err := catalog.RecoverLocked(options.Root); err != nil {
		return Result{}, &Error{Code: "subscription.recovery_failed", Message: "恢复未完成订阅事务失败", Data: err.Error()}
	}
	groupDir := filepath.Join(options.Root, options.GroupID)
	metaPath := filepath.Join(groupDir, "meta.json")
	providerPath := filepath.Join(groupDir, "provider.json")
	metadata, err := catalog.LoadMetadataLocked(metaPath, options.GroupID)
	if err != nil {
		return Result{}, &Error{Code: "subscription.metadata_read_failed", Message: "读取订阅元数据失败", Data: err.Error()}
	}
	if metadata.Type != "subscription" || strings.TrimSpace(metadata.URL) == "" {
		return Result{}, &Error{Code: "subscription.invalid_target", Message: "目标不是有效的 URL 订阅"}
	}
	if options.UseConfiguredProxy && options.ProxyURL == "" {
		switch metadata.UpdateViaProxy {
		case "always", "auto":
			options.ProxyURL = "http://127.0.0.1:7080"
			if metadata.UpdateViaProxy == "auto" {
				options.FallbackDirect = true
			}
		case "never":
			// 明确直连，不设置代理地址。
		}
	}
	initialSnapshot := metadata
	options.snapshot = &initialSnapshot
	releaseRoot()
	rootHeld = false
	stagingDir := filepath.Join(options.Root, "staging")
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		return Result{}, &Error{Code: "subscription.stage_failed", Message: "创建订阅临时目录失败", Data: err.Error()}
	}

	lockDir := filepath.Join(options.Root, "staging", "locks", options.GroupID+".lock")
	updateLock, err := acquireLock(lockDir)
	if err != nil {
		return Result{}, &Error{Code: "subscription.busy", Message: "订阅已经有更新任务正在执行"}
	}
	defer releaseLock(lockDir, updateLock)
	if err := cleanupStaleSubscriptionStages(stagingDir, options.GroupID); err != nil {
		return Result{}, &Error{Code: "subscription.recovery_failed", Message: "恢复订阅临时状态失败", Data: err.Error()}
	}
	if options.ProgressDir != "" {
		if err := os.MkdirAll(options.ProgressDir, 0o700); err != nil {
			return Result{}, &Error{Code: "subscription.stage_failed", Message: "订阅状态目录不可写", Data: err.Error()}
		}
	}
	if err := clearStaleProgress(options.ProgressDir, options.GroupID); err != nil {
		return Result{}, &Error{Code: "subscription.recovery_failed", Message: "清理订阅进度状态失败", Data: err.Error()}
	}

	stageDir, err := os.MkdirTemp(stagingDir, options.GroupID+".")
	if err != nil {
		return Result{}, &Error{Code: "subscription.stage_failed", Message: "创建订阅临时目录失败", Data: err.Error()}
	}
	defer os.RemoveAll(stageDir)
	if err := os.WriteFile(filepath.Join(lockDir, "pid"), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		return Result{}, &Error{Code: "subscription.stage_failed", Message: "订阅状态写入失败", Data: err.Error()}
	}
	if err := os.WriteFile(filepath.Join(lockDir, "stage"), []byte(stageDir+"\n"), 0o600); err != nil {
		return Result{}, &Error{Code: "subscription.stage_failed", Message: "订阅事务状态写入失败", Data: err.Error()}
	}
	updateContext, stopWatching := watchCancellation(ctx, options.ProgressDir, options.GroupID)
	defer stopWatching()
	ctx = updateContext
	if cancelled(ctx, options.ProgressDir, options.GroupID) {
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{}, &Error{Code: "subscription.cancelled", Message: "订阅更新已取消"}
	}

	started := options.Now
	if err := writeProgress(options.ProgressDir, options.GroupID, "download", "正在下载订阅"); err != nil {
		return updateFailure(options, metadata, groupDir, started, fetch.Response{}, "subscription.progress_write_failed", "订阅状态写入失败", err)
	}
	response, usedProxy, fetchErr := fetchSubscription(ctx, metadata, options)
	if fetchErr != nil {
		if cancelled(ctx, options.ProgressDir, options.GroupID) {
			return updateFailure(options, metadata, groupDir, started, response, "subscription.cancelled", "订阅更新已取消", fetchErr)
		}
		if _, ok := errors.AsType[*fetch.RedirectError](fetchErr); ok {
			return updateFailure(options, metadata, groupDir, started, response, "subscription.redirect_rejected", "订阅重定向被拒绝", fetchErr)
		}
		if progressErr, ok := errors.AsType[*progressWriteError](fetchErr); ok {
			return updateFailure(options, metadata, groupDir, started, response, "subscription.progress_write_failed", "订阅状态写入失败", progressErr)
		}
		return updateFailure(options, metadata, groupDir, started, response, "subscription.convert_failed", "订阅下载或转换失败", fetchErr)
	}
	metadata = applyResponseMetadata(metadata, response.Metadata, options.Now)
	metadata.Name = resolveName(metadata)

	if response.Metadata.NotModified {
		return commitNotModified(options, groupDir, metaPath, initialSnapshot, response, usedProxy)
	}

	if err := writeProgress(options.ProgressDir, options.GroupID, "convert", "正在转换订阅节点"); err != nil {
		return updateFailure(options, metadata, groupDir, started, response, "subscription.progress_write_failed", "订阅状态写入失败", err)
	}
	parsed, parseErr := convert.Content(ctx, string(response.Body), metadata.AllowInsecure)
	metadata.LastDiagnostics = append(response.Metadata.Diagnostics, parsed.Diagnostics...)
	if parseErr != nil {
		if cancelled(ctx, options.ProgressDir, options.GroupID) {
			return updateFailure(options, metadata, groupDir, started, response, "subscription.cancelled", "订阅更新已取消", parseErr)
		}
		return updateFailure(options, metadata, groupDir, started, response, "subscription.convert_failed", "订阅下载、转换或校验失败", parseErr)
	}
	filtered, filterErr := provider.Filter(parsed.Document, metadata.Include, metadata.Exclude)
	if filterErr != nil || len(filtered.Outbounds)+len(filtered.Endpoints) == 0 {
		if filterErr == nil {
			filterErr = errors.New("订阅中没有可用节点")
		}
		return updateFailure(options, metadata, groupDir, started, response, "provider.empty", "订阅中没有可用节点", filterErr)
	}

	if err := writeProgress(options.ProgressDir, options.GroupID, "validate", "正在校验节点配置"); err != nil {
		return updateFailure(options, metadata, groupDir, started, response, "subscription.progress_write_failed", "订阅状态写入失败", err)
	}
	stageProvider := filepath.Join(stageDir, "provider.json")
	if err := provider.SaveAtomic(ctx, stageProvider, filtered); err != nil {
		return updateFailure(options, metadata, groupDir, started, response, "provider.invalid", "节点配置校验失败", err)
	}
	if cancelled(ctx, options.ProgressDir, options.GroupID) {
		return updateFailure(options, metadata, groupDir, started, response, "subscription.cancelled", "订阅更新已取消", errors.New("subscription update cancelled"))
	}
	oldDocument, oldErr := provider.LoadAllowEmpty(ctx, providerPath)
	if oldErr != nil && !os.IsNotExist(oldErr) {
		return updateFailure(options, metadata, groupDir, started, response, "provider.read_failed", "读取旧节点配置失败", oldErr)
	}
	oldHasNodes := len(oldDocument.Outbounds)+len(oldDocument.Endpoints) > 0
	newNodeCount := len(filtered.Outbounds) + len(filtered.Endpoints)

	metadata.NodeCount = newNodeCount
	metadata.Revision++
	metadata.LastAttemptAt = formatTime(options.Now)
	metadata.LastSuccessAt = metadata.LastAttemptAt
	if !options.RuntimeSyncPending {
		metadata.RuntimeSyncState = RuntimeSyncNotRunning
	}
	if !metadata.RuntimeSyncPending || !isRuntimeSyncError(metadata.LastError) {
		metadata.LastError = ""
	}
	metadata.RuntimeSyncPending = metadata.RuntimeSyncPending || options.RuntimeSyncPending
	scheduleMetadata(&metadata, options.Now)
	metadata.UpdatedAt = metadata.LastAttemptAt
	metadataPath := filepath.Join(stageDir, "meta.json")
	if err := catalog.SaveMetadataAtomicLocked(metadataPath, metadata); err != nil {
		return updateFailure(options, metadata, groupDir, started, response, "metadata.write_failed", "订阅元数据写入失败", err)
	}

	if err := writeProgress(options.ProgressDir, options.GroupID, "apply", "正在应用订阅更新"); err != nil {
		return updateFailure(options, metadata, groupDir, started, response, "subscription.progress_write_failed", "订阅状态写入失败", err)
	}
	providerContent, err := os.ReadFile(stageProvider)
	if err != nil {
		return updateFailure(options, metadata, groupDir, started, response, "provider.read_failed", "读取临时节点配置失败", err)
	}
	var commitReleaseRoot func()
	commitReleaseRoot, err = catalog.AcquireRoot(options.Root)
	if err != nil {
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{}, &Error{Code: "subscription.busy", Message: "订阅或 Catalog 正在被其他进程使用", Data: err.Error()}
	}
	if err := catalog.RecoverLocked(options.Root); err != nil {
		commitReleaseRoot()
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{}, &Error{Code: "subscription.recovery_failed", Message: "恢复未完成订阅事务失败", Data: err.Error()}
	}
	current, err := catalog.LoadMetadataLocked(filepath.Join(groupDir, "meta.json"), options.GroupID)
	if err != nil {
		commitReleaseRoot()
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{}, &Error{Code: "subscription.metadata_read_failed", Message: "读取订阅元数据失败", Data: err.Error()}
	}
	if !sameUpdateSnapshot(current, initialSnapshot) {
		commitReleaseRoot()
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{}, updateConflict(options.GroupID, initialSnapshot, current)
	}
	if current.RuntimeSyncPending != initialSnapshot.RuntimeSyncPending ||
		current.RuntimeSyncState != initialSnapshot.RuntimeSyncState ||
		current.LastError != initialSnapshot.LastError {
		metadata.RuntimeSyncState = current.RuntimeSyncState
		metadata.LastError = current.LastError
	}
	metadata.RuntimeSyncPending = metadata.RuntimeSyncPending || current.RuntimeSyncPending || options.RuntimeSyncPending
	if err := catalog.SaveMetadataAtomicLocked(metadataPath, metadata); err != nil {
		commitReleaseRoot()
		return updateFailure(options, metadata, groupDir, started, response, "metadata.write_failed", "订阅元数据写入失败", err)
	}
	metadataContent, err := os.ReadFile(metadataPath)
	if err != nil {
		commitReleaseRoot()
		return updateFailure(options, metadata, groupDir, started, response, "metadata.read_failed", "读取临时元数据失败", err)
	}
	if err := catalog.CommitPairLocked(options.Root, groupDir, providerContent, metadataContent); err != nil {
		commitReleaseRoot()
		return updateFailure(options, metadata, groupDir, started, response, "subscription.commit_failed", "订阅 Provider 与元数据提交失败", err)
	}
	commitReleaseRoot()

	if err := appendHistoryChecked(groupDir, map[string]any{
		"at": formatTime(options.Now), "ok": true, "code": "subscription.updated",
		"node_count": metadata.NodeCount, "revision": metadata.Revision,
		"duration_seconds": int64(time.Since(started).Seconds()),
		"diagnostics":      metadata.LastDiagnostics,
	}); err != nil {
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{GroupID: options.GroupID, NodeCount: metadata.NodeCount, Revision: metadata.Revision,
				StructureChanged: oldHasNodes != (metadata.NodeCount > 0), UsedProxy: usedProxy,
				Persisted: true, RuntimeSyncState: RuntimeSyncNotRunning, RuntimeSyncPending: metadata.RuntimeSyncPending},
			&Error{Code: "subscription.history_write_failed", Message: "订阅历史写入失败", Data: map[string]any{
				"persisted": true, "cause": err.Error(),
			}}
	}
	clearProgress(options.ProgressDir, options.GroupID)
	return Result{
		GroupID: options.GroupID, NodeCount: metadata.NodeCount, Revision: metadata.Revision,
		StructureChanged: oldHasNodes != (metadata.NodeCount > 0), UsedProxy: usedProxy,
		Persisted: true, RuntimeSyncState: RuntimeSyncNotRunning, RuntimeSyncPending: metadata.RuntimeSyncPending,
	}, nil
}

func commitNotModified(options UpdateOptions, groupDir, metaPath string, initial catalog.Metadata, response fetch.Response, usedProxy bool) (Result, error) {
	releaseRoot, err := catalog.AcquireRoot(options.Root)
	if err != nil {
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{}, &Error{Code: "subscription.busy", Message: "订阅或 Catalog 正在被其他进程使用", Data: err.Error()}
	}
	rootHeld := true
	defer func() {
		if rootHeld {
			releaseRoot()
		}
	}()
	if err := catalog.RecoverLocked(options.Root); err != nil {
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{}, &Error{Code: "subscription.recovery_failed", Message: "恢复未完成订阅事务失败", Data: err.Error()}
	}
	current, err := catalog.LoadMetadataLocked(metaPath, options.GroupID)
	if err != nil {
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{}, &Error{Code: "subscription.metadata_read_failed", Message: "读取订阅元数据失败", Data: err.Error()}
	}
	if !sameUpdateSnapshot(current, initial) {
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{}, updateConflict(options.GroupID, initial, current)
	}
	metadata := applyResponseMetadata(current, response.Metadata, options.Now)
	metadata.Name = resolveName(metadata)
	preserveRuntimeError := metadata.RuntimeSyncPending && isRuntimeSyncError(metadata.LastError)
	metadata.LastAttemptAt = formatTime(options.Now)
	metadata.LastSuccessAt = metadata.LastAttemptAt
	if !preserveRuntimeError {
		metadata.LastError = ""
	}
	if !options.RuntimeSyncPending {
		metadata.RuntimeSyncState = RuntimeSyncNotRunning
	}
	scheduleMetadata(&metadata, options.Now)
	metadata.UpdatedAt = metadata.LastAttemptAt
	if err := catalog.SaveMetadataAtomicLocked(metaPath, metadata); err != nil {
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{}, &Error{Code: "metadata.commit_failed", Message: "订阅元数据提交失败", Data: err.Error()}
	}
	releaseRoot()
	rootHeld = false
	if err := appendHistoryChecked(groupDir, map[string]any{
		"at": formatTime(options.Now), "ok": true, "code": "subscription.not_modified",
		"node_count": metadata.NodeCount, "revision": metadata.Revision,
	}); err != nil {
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{GroupID: options.GroupID, NodeCount: metadata.NodeCount, Revision: metadata.Revision,
				NotModified: true, UsedProxy: usedProxy, Persisted: true,
				RuntimeSyncState: RuntimeSyncNotRunning, RuntimeSyncPending: metadata.RuntimeSyncPending},
			&Error{Code: "subscription.history_write_failed", Message: "订阅历史写入失败", Data: map[string]any{
				"persisted": true, "cause": err.Error(),
			}}
	}
	clearProgress(options.ProgressDir, options.GroupID)
	return Result{
		GroupID: options.GroupID, NodeCount: metadata.NodeCount, Revision: metadata.Revision,
		NotModified: true, UsedProxy: usedProxy, Persisted: true,
		RuntimeSyncState: RuntimeSyncNotRunning, RuntimeSyncPending: metadata.RuntimeSyncPending,
	}, nil
}

func fetchSubscription(ctx context.Context, metadata catalog.Metadata, options UpdateOptions) (fetch.Response, bool, error) {
	request := fetch.Request{
		URL: metadata.URL, UserAgent: metadata.UserAgent, HWID: metadata.HWID,
		Headers: metadata.CustomHeaders, ETag: metadata.ETag, LastModified: metadata.LastModified,
		ProxyURL: options.ProxyURL, AllowInsecure: metadata.AllowInsecure,
		Timeout: time.Duration(metadata.Timeout) * time.Second,
	}
	response, err := fetch.Subscription(ctx, request)
	if err == nil || !options.FallbackDirect || options.ProxyURL == "" || cancelled(ctx, options.ProgressDir, options.GroupID) {
		return response, options.ProxyURL != "", err
	}
	if _, ok := errors.AsType[*fetch.RedirectError](err); ok {
		return response, options.ProxyURL != "", err
	}
	if err := writeProgress(options.ProgressDir, options.GroupID, "download", "代理下载失败，正在尝试直连"); err != nil {
		return response, false, &progressWriteError{cause: err}
	}
	request.ProxyURL = ""
	response, err = fetch.Subscription(ctx, request)
	return response, false, err
}

type progressWriteError struct {
	cause error
}

func (err *progressWriteError) Error() string {
	return "subscription progress state write failed"
}

func (err *progressWriteError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func updateFailure(options UpdateOptions, metadata catalog.Metadata, groupDir string, started time.Time, response fetch.Response, code, message string, cause error) (Result, error) {
	releaseRoot, err := catalog.AcquireRoot(options.Root)
	if err != nil {
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{}, &Error{Code: "subscription.busy", Message: "订阅或 Catalog 正在被其他进程使用", Data: err.Error()}
	}
	defer releaseRoot()
	if err := catalog.RecoverLocked(options.Root); err != nil {
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{}, &Error{Code: "subscription.recovery_failed", Message: "恢复未完成订阅事务失败", Data: err.Error()}
	}
	expected := metadata
	if options.snapshot != nil {
		expected = *options.snapshot
	}
	current, err := catalog.LoadMetadataLocked(filepath.Join(groupDir, "meta.json"), options.GroupID)
	if err != nil {
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{}, &Error{Code: "subscription.metadata_read_failed", Message: "读取订阅元数据失败", Data: err.Error()}
	}
	if !sameUpdateSnapshot(current, expected) {
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{}, updateConflict(options.GroupID, expected, current)
	}
	return updateFailureLocked(options, current, groupDir, started, response, code, message, cause)
}

func updateFailureLocked(options UpdateOptions, metadata catalog.Metadata, groupDir string, started time.Time, response fetch.Response, code, message string, cause error) (Result, error) {
	now := options.Now
	metadata = applyResponseMetadata(metadata, response.Metadata, now)
	metadata.LastAttemptAt = formatTime(now)
	if !metadata.RuntimeSyncPending || !isRuntimeSyncError(metadata.LastError) {
		metadata.LastError = message
	}
	scheduleMetadata(&metadata, now)
	metadata.UpdatedAt = formatTime(now)
	metadataErr := catalog.SaveMetadataAtomicLocked(filepath.Join(groupDir, "meta.json"), metadata)
	historyErr := appendHistoryChecked(groupDir, map[string]any{
		"at": formatTime(now), "ok": false, "code": code, "message": message,
		"duration_seconds": int64(time.Since(started).Seconds()),
	})
	combined := errors.Join(cause, metadataErr, historyErr)
	clearProgress(options.ProgressDir, options.GroupID)
	failureCode, failureMessage := code, message
	if metadataErr != nil {
		failureCode, failureMessage = "metadata.write_failed", "订阅元数据写入失败"
	} else if historyErr != nil {
		failureCode, failureMessage = "subscription.history_write_failed", "订阅历史写入失败"
	}
	return Result{}, &Error{Code: failureCode, Message: failureMessage, Data: map[string]any{
		"cause": errorString(combined), "status_code": response.Metadata.StatusCode, "original_code": code,
		"persisted": options.PersistedBeforeUpdate,
	}}
}

func sameUpdateSnapshot(left, right catalog.Metadata) bool {
	return left.Schema == right.Schema &&
		left.ID == right.ID &&
		left.Name == right.Name &&
		left.Type == right.Type &&
		left.URL == right.URL &&
		left.UserAgent == right.UserAgent &&
		left.HWID == right.HWID &&
		reflect.DeepEqual(left.CustomHeaders, right.CustomHeaders) &&
		left.AutoUpdate == right.AutoUpdate &&
		left.UpdateInterval == right.UpdateInterval &&
		left.IntervalSource == right.IntervalSource &&
		left.UpdateViaProxy == right.UpdateViaProxy &&
		left.Include == right.Include &&
		left.Exclude == right.Exclude &&
		left.AllowInsecure == right.AllowInsecure &&
		left.Timeout == right.Timeout &&
		left.Revision == right.Revision &&
		left.ETag == right.ETag &&
		left.LastModified == right.LastModified
}

func updateConflict(groupID string, expected, current catalog.Metadata) *Error {
	return &Error{Code: "subscription.conflict", Message: "订阅在更新期间发生变化，已放弃提交", Data: map[string]any{
		"group_id":          groupID,
		"expected_revision": expected.Revision,
		"actual_revision":   current.Revision,
	}}
}

func applyResponseMetadata(metadata catalog.Metadata, response fetch.Metadata, now time.Time) catalog.Metadata {
	if response.StatusCode > 0 {
		metadata.LastStatusCode = response.StatusCode
	}
	if response.ETag != "" {
		metadata.ETag = response.ETag
	}
	if response.LastModified != "" {
		metadata.LastModified = response.LastModified
	}
	if response.ProfileTitle != "" {
		metadata.ProfileTitle = response.ProfileTitle
	}
	if response.ProfileWebPageURL != "" {
		metadata.ProfileWebPageURL = response.ProfileWebPageURL
	}
	if response.ContentDisposition != "" {
		metadata.ContentDisposition = response.ContentDisposition
	}
	if response.FileName != "" {
		metadata.FileName = response.FileName
	}
	if response.Usage != nil {
		if usage, err := json.Marshal(response.Usage, json.Deterministic(true)); err == nil {
			metadata.Usage = usage
		}
	} else if !response.NotModified {
		metadata.Usage = jsontext.Value("null")
	}
	metadata.LastDiagnostics = response.Diagnostics
	if response.UpdateIntervalSeconds != nil && metadata.IntervalSource != "user" && *response.UpdateIntervalSeconds >= int64(minimumInterval/time.Second) {
		metadata.UpdateInterval = *response.UpdateIntervalSeconds
		metadata.IntervalSource = "profile"
	}
	_ = now
	return metadata
}

func resolveName(metadata catalog.Metadata) string {
	if strings.TrimSpace(metadata.Name) != "" && metadata.Name != metadata.ID {
		return strings.TrimSpace(metadata.Name)
	}
	for _, candidate := range []string{metadata.ProfileTitle, metadata.FileName, hostName(metadata.URL), metadata.ID, "订阅"} {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" && !strings.ContainsAny(candidate, "\r\n\t") {
			return candidate
		}
	}
	return "订阅"
}

func hostName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func scheduleMetadata(metadata *catalog.Metadata, now time.Time) {
	if metadata.AutoUpdate {
		catalog.ScheduleAt(metadata, now)
		return
	}
	metadata.NextUpdateEpoch = 0
	metadata.NextUpdateAt = ""
}

func appendHistoryChecked(groupDir string, value map[string]any) error {
	historyPath := filepath.Join(groupDir, "history.jsonl")
	content, err := os.ReadFile(historyPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(content), "\r\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	encoded, err := json.Marshal(value, json.Deterministic(true))
	if err != nil {
		return err
	}
	lines = append(lines, string(encoded))
	if len(lines) > maxHistory {
		lines = lines[len(lines)-maxHistory:]
	}
	return provider.WriteAtomic(historyPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

func writeProgress(dir, groupID, stage, message string) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	payload := map[string]any{
		"schema": 1, "group_id": groupID, "stage": stage, "message": message,
		"updated_at": formatTime(time.Now()),
	}
	content, err := json.Marshal(payload, json.Deterministic(true))
	if err != nil {
		return err
	}
	return provider.WriteAtomic(filepath.Join(dir, groupID+".progress.json"), append(content, '\n'), 0o600)
}

func clearProgress(dir, groupID string) {
	_ = clearStaleProgress(dir, groupID)
}

func clearStaleProgress(progressDir, groupID string) error {
	if progressDir == "" {
		return nil
	}
	var firstErr error
	for _, name := range []string{groupID + ".progress.json", groupID + ".cancel"} {
		if err := os.Remove(filepath.Join(progressDir, name)); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func cleanupStaleSubscriptionStages(stagingDir, groupID string) error {
	entries, err := os.ReadDir(stagingDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	prefix := groupID + "."
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(stagingDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// watchCancellation 将外部取消标记转换为当前更新的 context 取消。
// 取消命令与更新任务是不同进程，不能共享 context，只能通过短周期轮询文件标记通信。
func watchCancellation(parent context.Context, dir, groupID string) (context.Context, func()) {
	if dir == "" {
		return parent, func() {}
	}

	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	cancelPath := filepath.Join(dir, groupID+".cancel")
	go func() {
		defer close(done)
		ticker := time.NewTicker(cancelPoll)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := os.Stat(cancelPath); err == nil {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, func() {
		cancel()
		<-done
	}
}

func cancelled(ctx context.Context, dir, groupID string) bool {
	select {
	case <-ctx.Done():
		return true
	default:
	}
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, groupID+".cancel"))
	return err == nil
}

func acquireLock(path string) (*processlock.Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// OS 锁必须位于可回收的任务目录之外，否则崩溃清理会制造两个独立锁入口。
	guard, err := processlock.TryAcquire(path + ".flock")
	if err != nil {
		return nil, err
	}
	if err := os.RemoveAll(path); err != nil {
		_ = guard.Release()
		return nil, err
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		_ = guard.Release()
		return nil, err
	}
	return guard, nil
}

func releaseLock(path string, guard *processlock.Lock) {
	_ = os.RemoveAll(path)
	_ = guard.Release()
}

func lockActive(path string) (bool, error) {
	guard, err := processlock.TryAcquire(path + ".flock")
	if errors.Is(err, processlock.ErrBusy) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, guard.Release()
}

func lockReady(path string) bool {
	_, err := os.Stat(filepath.Join(path, "pid"))
	return err == nil
}

func validGroupID(value string) bool {
	if value == "" || value == "staging" || strings.Contains(value, "..") {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || (index > 0 && (char == '.' || char == '_' || char == '-')) {
			continue
		}
		return false
	}
	return true
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339) }
