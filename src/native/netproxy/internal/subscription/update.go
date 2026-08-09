package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/convert"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/fetch"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const (
	defaultInterval = 24 * time.Hour
	minimumInterval = 15 * time.Minute
	maxHistory      = 20
)

// UpdateOptions 定义一次订阅更新的运行上下文。
type UpdateOptions struct {
	Root               string
	GroupID            string
	ProgressDir        string
	ProxyURL           string
	UseConfiguredProxy bool
	FallbackDirect     bool
	Now                time.Time
}

// Result 是订阅更新对 Shell 暴露的最小结构化结果。
type Result struct {
	GroupID          string `json:"group_id"`
	NodeCount        int    `json:"node_count"`
	Revision         int64  `json:"revision"`
	NotModified      bool   `json:"not_modified"`
	StructureChanged bool   `json:"structure_changed"`
	UsedProxy        bool   `json:"used_proxy"`
}

// Error 是可以直接映射为 schema=1 错误响应的订阅错误。
type Error struct {
	Code    string
	Message string
	Data    any
}

func (e *Error) Error() string { return e.Message }

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
	release := catalog.Acquire(options.GroupID)
	defer release()
	if err := catalog.Recover(options.Root); err != nil {
		return Result{}, &Error{Code: "subscription.recovery_failed", Message: "鎭㈠鏈畬鎴愯闃呬簨鍔″け璐?", Data: err.Error()}
	}
	groupDir := filepath.Join(options.Root, options.GroupID)
	metaPath := filepath.Join(groupDir, "meta.json")
	providerPath := filepath.Join(groupDir, "provider.json")
	metadata, err := LoadMetadata(metaPath, options.GroupID)
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
	if cancelled(ctx, options.ProgressDir, options.GroupID) {
		return Result{}, &Error{Code: "subscription.cancelled", Message: "订阅更新已取消"}
	}
	stagingDir := filepath.Join(options.Root, "staging")
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		return Result{}, &Error{Code: "subscription.stage_failed", Message: "创建订阅临时目录失败", Data: err.Error()}
	}

	lockDir := filepath.Join(options.Root, "staging", "locks", options.GroupID+".lock")
	if err := acquireLock(lockDir); err != nil {
		return Result{}, &Error{Code: "subscription.busy", Message: "订阅已经有更新任务正在执行"}
	}
	defer os.RemoveAll(lockDir)

	stageDir, err := os.MkdirTemp(stagingDir, options.GroupID+".")
	if err != nil {
		return Result{}, &Error{Code: "subscription.stage_failed", Message: "创建订阅临时目录失败", Data: err.Error()}
	}
	defer os.RemoveAll(stageDir)
	_ = os.WriteFile(filepath.Join(lockDir, "pid"), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600)
	_ = os.WriteFile(filepath.Join(lockDir, "stage"), []byte(stageDir+"\n"), 0o600)
	if options.ProgressDir != "" {
		_ = os.MkdirAll(options.ProgressDir, 0o700)
		_ = os.WriteFile(filepath.Join(options.ProgressDir, options.GroupID+".child.pid"), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600)
		defer os.Remove(filepath.Join(options.ProgressDir, options.GroupID+".child.pid"))
	}

	started := options.Now
	writeProgress(options.ProgressDir, options.GroupID, "download", "正在下载订阅")
	response, usedProxy, fetchErr := fetchSubscription(ctx, metadata, options)
	if fetchErr != nil {
		return updateFailure(options, metadata, groupDir, started, response, "subscription.convert_failed", "订阅下载或转换失败", fetchErr)
	}
	metadata = applyResponseMetadata(metadata, response.Metadata, options.Now)
	metadata.Name = resolveName(metadata)

	if response.Metadata.NotModified {
		metadata.LastAttemptAt = formatTime(options.Now)
		metadata.LastSuccessAt = metadata.LastAttemptAt
		metadata.LastError = ""
		scheduleMetadata(&metadata, options.Now)
		metadata.UpdatedAt = metadata.LastAttemptAt
		if err := SaveMetadataAtomic(metaPath, metadata); err != nil {
			return Result{}, &Error{Code: "metadata.commit_failed", Message: "订阅元数据提交失败", Data: err.Error()}
		}
		appendHistory(groupDir, map[string]any{
			"at": formatTime(options.Now), "ok": true, "code": "subscription.not_modified",
			"node_count": metadata.NodeCount, "revision": metadata.Revision,
		})
		clearProgress(options.ProgressDir, options.GroupID)
		return Result{GroupID: options.GroupID, NodeCount: metadata.NodeCount, Revision: metadata.Revision, NotModified: true, UsedProxy: usedProxy}, nil
	}

	writeProgress(options.ProgressDir, options.GroupID, "convert", "正在转换订阅节点")
	parsed, parseErr := convert.Content(ctx, string(response.Body), metadata.AllowInsecure)
	metadata.LastDiagnostics = append(response.Metadata.Diagnostics, parsed.Diagnostics...)
	if parseErr != nil {
		return updateFailure(options, metadata, groupDir, started, response, "subscription.convert_failed", "订阅下载、转换或校验失败", parseErr)
	}
	filtered, filterErr := provider.Filter(parsed.Document, metadata.Include, metadata.Exclude)
	if filterErr != nil || len(filtered.Outbounds)+len(filtered.Endpoints) == 0 {
		if filterErr == nil {
			filterErr = errors.New("订阅中没有可用节点")
		}
		return updateFailure(options, metadata, groupDir, started, response, "provider.empty", "订阅中没有可用节点", filterErr)
	}

	writeProgress(options.ProgressDir, options.GroupID, "validate", "正在校验节点配置")
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
	metadata.LastError = ""
	scheduleMetadata(&metadata, options.Now)
	metadata.UpdatedAt = metadata.LastAttemptAt
	metadataPath := filepath.Join(stageDir, "meta.json")
	if err := SaveMetadataAtomic(metadataPath, metadata); err != nil {
		return updateFailure(options, metadata, groupDir, started, response, "metadata.write_failed", "订阅元数据写入失败", err)
	}

	writeProgress(options.ProgressDir, options.GroupID, "apply", "正在应用订阅更新")
	providerContent, err := os.ReadFile(stageProvider)
	if err != nil {
		return updateFailure(options, metadata, groupDir, started, response, "provider.read_failed", "读取临时节点配置失败", err)
	}
	metadataContent, err := os.ReadFile(metadataPath)
	if err != nil {
		return updateFailure(options, metadata, groupDir, started, response, "metadata.read_failed", "读取临时元数据失败", err)
	}
	if err := catalog.CommitPair(options.Root, groupDir, providerContent, metadataContent); err != nil {
		return updateFailure(options, metadata, groupDir, started, response, "subscription.commit_failed", "订阅 Provider 与元数据提交失败", err)
	}

	appendHistory(groupDir, map[string]any{
		"at": formatTime(options.Now), "ok": true, "code": "subscription.updated",
		"node_count": metadata.NodeCount, "revision": metadata.Revision,
		"duration_seconds": int64(time.Since(started).Seconds()),
		"diagnostics":      metadata.LastDiagnostics,
	})
	clearProgress(options.ProgressDir, options.GroupID)
	return Result{
		GroupID: options.GroupID, NodeCount: metadata.NodeCount, Revision: metadata.Revision,
		StructureChanged: oldHasNodes != (metadata.NodeCount > 0), UsedProxy: usedProxy,
	}, nil
}

func fetchSubscription(ctx context.Context, metadata Metadata, options UpdateOptions) (fetch.Response, bool, error) {
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
	writeProgress(options.ProgressDir, options.GroupID, "download", "代理下载失败，正在尝试直连")
	request.ProxyURL = ""
	response, err = fetch.Subscription(ctx, request)
	return response, false, err
}

func updateFailure(options UpdateOptions, metadata Metadata, groupDir string, started time.Time, response fetch.Response, code, message string, cause error) (Result, error) {
	now := options.Now
	metadata = applyResponseMetadata(metadata, response.Metadata, now)
	metadata.LastAttemptAt = formatTime(now)
	metadata.LastError = message
	scheduleMetadata(&metadata, now)
	metadata.UpdatedAt = formatTime(now)
	_ = SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata)
	appendHistory(groupDir, map[string]any{
		"at": formatTime(now), "ok": false, "code": code, "message": message,
		"duration_seconds": int64(time.Since(started).Seconds()),
	})
	clearProgress(options.ProgressDir, options.GroupID)
	return Result{}, &Error{Code: code, Message: message, Data: map[string]any{"cause": cause.Error(), "status_code": response.Metadata.StatusCode}}
}

func applyResponseMetadata(metadata Metadata, response fetch.Metadata, now time.Time) Metadata {
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
		if usage, err := json.Marshal(response.Usage); err == nil {
			metadata.Usage = usage
		}
	} else if !response.NotModified {
		metadata.Usage = json.RawMessage("null")
	}
	metadata.LastDiagnostics = response.Diagnostics
	if response.UpdateIntervalSeconds != nil && metadata.IntervalSource != "user" && *response.UpdateIntervalSeconds >= int64(minimumInterval/time.Second) {
		metadata.UpdateInterval = *response.UpdateIntervalSeconds
		metadata.IntervalSource = "profile"
	}
	_ = now
	return metadata
}

func resolveName(metadata Metadata) string {
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

func scheduleMetadata(metadata *Metadata, now time.Time) {
	if metadata.AutoUpdate {
		ScheduleAt(metadata, now)
		return
	}
	metadata.NextUpdateEpoch = 0
	metadata.NextUpdateAt = ""
}

func appendHistory(groupDir string, value map[string]any) {
	historyPath := filepath.Join(groupDir, "history.jsonl")
	content, _ := os.ReadFile(historyPath)
	lines := strings.Split(strings.TrimRight(string(content), "\r\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	lines = append(lines, string(encoded))
	if len(lines) > maxHistory {
		lines = lines[len(lines)-maxHistory:]
	}
	_ = provider.WriteAtomic(historyPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

func writeProgress(dir, groupID, stage, message string) {
	if dir == "" {
		return
	}
	payload := map[string]any{
		"schema": 1, "group_id": groupID, "stage": stage, "message": message,
		"updated_at": formatTime(time.Now()),
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = provider.WriteAtomic(filepath.Join(dir, groupID+".progress.json"), append(content, '\n'), 0o600)
}

func clearProgress(dir, groupID string) {
	if dir == "" {
		return
	}
	_ = os.Remove(filepath.Join(dir, groupID+".progress.json"))
	_ = os.Remove(filepath.Join(dir, groupID+".cancel"))
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

func acquireLock(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := os.Mkdir(path, 0o700); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrExist) {
			return err
		}
		pid := readLockPID(filepath.Join(path, "pid"))
		if pid > 0 && isProcessAlive(pid) {
			return os.ErrExist
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return os.ErrExist
}

func readLockPID(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var pid int
	if _, err := fmt.Sscanf(string(content), "%d", &pid); err != nil {
		return 0
	}
	return pid
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
