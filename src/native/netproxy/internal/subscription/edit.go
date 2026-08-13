package subscription

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
)

// EditOptions 描述一次订阅元数据编辑及其必要的重新验证。
type EditOptions struct {
	Root           string
	GroupID        string
	ProgressDir    string
	DeferUpdate    bool
	ProxyURL       string
	FallbackDirect bool
	Name           *string
	URL            *string
	UserAgent      *string
	HWID           *string
	CustomHeaders  *map[string]string
	AutoUpdate     *bool
	UpdateInterval *int64
	UpdateViaProxy *string
	Include        *string
	Exclude        *string
	AllowInsecure  *bool
	Timeout        *int64
	Now            time.Time
}

// EditResult 是订阅编辑对 Shell 暴露的最小结果。
type EditResult struct {
	GroupID            string `json:"group_id"`
	NameChanged        bool   `json:"name_changed"`
	RequiresUpdate     bool   `json:"requires_update"`
	NodeCount          int    `json:"node_count"`
	Revision           int64  `json:"revision"`
	StructureChanged   bool   `json:"structure_changed"`
	NotModified        bool   `json:"not_modified"`
	Persisted          bool   `json:"persisted"`
	RuntimeSynced      bool   `json:"runtime_synced"`
	RuntimeSyncState   string `json:"runtime_sync_state"`
	RuntimeSyncPending bool   `json:"runtime_sync_pending"`
}

// editBeforeRestoreHook 仅供同包测试在恢复窗口内模拟其他进程写入。
var editBeforeRestoreHook = func() {}

// Edit 更新订阅设置，并在影响节点内容的设置变化时重新验证订阅。
func Edit(ctx context.Context, options EditOptions) (EditResult, error) {
	if strings.TrimSpace(options.Root) == "" || !validGroupID(options.GroupID) {
		return EditResult{}, &Error{Code: "subscription.invalid_target", Message: "订阅目录或分组无效"}
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	releaseGroup, err := catalog.Acquire(options.Root, options.GroupID)
	if err != nil {
		return EditResult{}, &Error{Code: "subscription.busy", Message: "订阅或 Catalog 正在被其他进程使用", Data: err.Error()}
	}
	releaseRoot, err := catalog.AcquireRoot(options.Root)
	if err != nil {
		releaseGroup()
		return EditResult{}, &Error{Code: "subscription.busy", Message: "订阅或 Catalog 正在被其他进程使用", Data: err.Error()}
	}
	locked := true
	defer func() {
		if locked {
			releaseRoot()
			releaseGroup()
		}
	}()
	if err := catalog.RecoverLocked(options.Root); err != nil {
		return EditResult{}, &Error{Code: "subscription.recovery_failed", Message: "恢复 Catalog 事务失败", Data: err.Error()}
	}
	metaPath := filepath.Join(options.Root, options.GroupID, "meta.json")
	metadata, err := catalog.LoadMetadataLocked(metaPath, options.GroupID)
	if err != nil {
		return EditResult{}, &Error{Code: "subscription.metadata_read_failed", Message: "读取订阅元数据失败", Data: err.Error()}
	}
	if metadata.Type != "subscription" || strings.TrimSpace(metadata.URL) == "" {
		return EditResult{}, &Error{Code: "subscription.read_only", Message: "目标不是 URL 订阅"}
	}
	oldMetadata := metadata
	oldMetadata.CustomHeaders = cloneHeaders(metadata.CustomHeaders)
	nameChanged := false
	requiresUpdate := false

	if options.Name != nil {
		if err := validateEditText(*options.Name); err != nil {
			return EditResult{}, err
		}
		if strings.TrimSpace(*options.Name) == "" {
			return EditResult{}, &Error{Code: "subscription.edit_invalid", Message: "订阅名称不能为空"}
		}
		nameChanged = metadata.Name != *options.Name
		metadata.Name = *options.Name
	}
	if options.URL != nil {
		if err := validateEditText(*options.URL); err != nil {
			return EditResult{}, err
		}
		if strings.TrimSpace(*options.URL) == "" {
			return EditResult{}, &Error{Code: "subscription.edit_invalid", Message: "订阅 URL 不能为空"}
		}
		if metadata.URL != *options.URL {
			metadata.URL = *options.URL
			metadata.ETag = ""
			metadata.LastModified = ""
			requiresUpdate = true
		}
	}
	if options.UserAgent != nil {
		if err := validateEditText(*options.UserAgent); err != nil {
			return EditResult{}, err
		}
		if metadata.UserAgent != *options.UserAgent {
			metadata.UserAgent = *options.UserAgent
			metadata.ETag = ""
			metadata.LastModified = ""
			requiresUpdate = true
		}
	}
	if options.HWID != nil {
		if err := validateEditText(*options.HWID); err != nil {
			return EditResult{}, err
		}
		if metadata.HWID != *options.HWID {
			metadata.HWID = *options.HWID
			metadata.ETag = ""
			metadata.LastModified = ""
			requiresUpdate = true
		}
	}
	if options.CustomHeaders != nil {
		metadata.CustomHeaders = cloneHeaders(*options.CustomHeaders)
		metadata.ETag = ""
		metadata.LastModified = ""
		requiresUpdate = true
	}
	if options.AutoUpdate != nil {
		metadata.AutoUpdate = *options.AutoUpdate
	}
	if options.UpdateInterval != nil {
		if *options.UpdateInterval < int64(minimumInterval/time.Second) {
			return EditResult{}, &Error{Code: "subscription.interval_too_short", Message: "更新周期不能少于 15 分钟"}
		}
		metadata.UpdateInterval = *options.UpdateInterval
		metadata.IntervalSource = "user"
	}
	if options.UpdateViaProxy != nil {
		switch *options.UpdateViaProxy {
		case "auto", "always", "never":
		default:
			return EditResult{}, &Error{Code: "subscription.proxy_mode_invalid", Message: "订阅更新代理模式无效"}
		}
		if metadata.UpdateViaProxy != *options.UpdateViaProxy {
			metadata.UpdateViaProxy = *options.UpdateViaProxy
			requiresUpdate = true
		}
	}
	if options.Include != nil {
		if err := validateEditText(*options.Include); err != nil {
			return EditResult{}, err
		}
		if metadata.Include != *options.Include {
			metadata.Include = *options.Include
			metadata.ETag = ""
			metadata.LastModified = ""
			requiresUpdate = true
		}
	}
	if options.Exclude != nil {
		if err := validateEditText(*options.Exclude); err != nil {
			return EditResult{}, err
		}
		if metadata.Exclude != *options.Exclude {
			metadata.Exclude = *options.Exclude
			metadata.ETag = ""
			metadata.LastModified = ""
			requiresUpdate = true
		}
	}
	if options.AllowInsecure != nil {
		if metadata.AllowInsecure != *options.AllowInsecure {
			metadata.AllowInsecure = *options.AllowInsecure
			metadata.ETag = ""
			metadata.LastModified = ""
			requiresUpdate = true
		}
	}
	if options.Timeout != nil {
		if *options.Timeout <= 0 {
			return EditResult{}, &Error{Code: "subscription.timeout_invalid", Message: "下载超时必须大于 0"}
		}
		if metadata.Timeout != *options.Timeout {
			metadata.Timeout = *options.Timeout
			requiresUpdate = true
		}
	}
	if options.AutoUpdate != nil || options.UpdateInterval != nil {
		if metadata.AutoUpdate {
			catalog.ScheduleAt(&metadata, options.Now)
		} else {
			metadata.NextUpdateEpoch = 0
			metadata.NextUpdateAt = ""
		}
	}
	metadata.UpdatedAt = formatTime(options.Now)
	if err := catalog.SaveMetadataAtomicLocked(metaPath, metadata); err != nil {
		return EditResult{}, &Error{Code: "subscription.edit_failed", Message: "保存订阅设置失败", Data: err.Error()}
	}
	releaseRoot()
	releaseGroup()
	locked = false
	if !requiresUpdate || options.DeferUpdate {
		return EditResult{
			GroupID: options.GroupID, NameChanged: nameChanged, RequiresUpdate: requiresUpdate,
			NodeCount: metadata.NodeCount, Revision: metadata.Revision, Persisted: true,
			RuntimeSynced:    metadata.RuntimeSyncState == RuntimeSyncApplied && !metadata.RuntimeSyncPending,
			RuntimeSyncState: metadata.RuntimeSyncState, RuntimeSyncPending: metadata.RuntimeSyncPending,
		}, nil
	}
	updated, err := Update(ctx, UpdateOptions{
		Root: options.Root, GroupID: options.GroupID, ProgressDir: options.ProgressDir,
		ProxyURL: options.ProxyURL, UseConfiguredProxy: true,
		FallbackDirect: options.FallbackDirect, Now: options.Now,
	})
	if err != nil {
		if updated.Persisted {
			return mergeEditResult(EditResult{GroupID: options.GroupID, NameChanged: nameChanged, RequiresUpdate: requiresUpdate}, updated), err
		}
		editBeforeRestoreHook()
		restoreErr := restoreMetadataIfUnchanged(options.Root, options.GroupID, metaPath, oldMetadata, metadata)
		return EditResult{}, errors.Join(err, restoreErr)
	}
	return mergeEditResult(EditResult{GroupID: options.GroupID, NameChanged: nameChanged, RequiresUpdate: true}, updated), nil
}

func mergeEditResult(edit EditResult, update Result) EditResult {
	edit.NodeCount = update.NodeCount
	edit.Revision = update.Revision
	edit.StructureChanged = update.StructureChanged
	edit.NotModified = update.NotModified
	edit.Persisted = update.Persisted
	edit.RuntimeSynced = update.RuntimeSynced
	edit.RuntimeSyncState = update.RuntimeSyncState
	edit.RuntimeSyncPending = update.RuntimeSyncPending
	return edit
}

func restoreMetadataIfUnchanged(root, groupID, metaPath string, oldMetadata, expected catalog.Metadata) error {
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
	current, err := catalog.LoadMetadataLocked(metaPath, groupID)
	if err != nil {
		return err
	}
	if !sameEditMetadata(current, expected) {
		return fmt.Errorf("订阅元数据已被其他进程更新，跳过旧元数据恢复")
	}
	return catalog.SaveMetadataAtomicLocked(metaPath, oldMetadata)
}

func sameEditMetadata(left, right catalog.Metadata) bool {
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
		left.CreatedAt == right.CreatedAt
}

func validateEditText(value string) error {
	if strings.ContainsAny(value, "\r\n\t") {
		return &Error{Code: "subscription.text_invalid", Message: "订阅设置不能包含制表符或换行"}
	}
	return nil
}

func cloneHeaders(headers map[string]string) map[string]string {
	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}
