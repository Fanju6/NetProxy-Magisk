package subscription

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

// EditOptions 描述一次订阅元数据编辑及其必要的重新验证。
type EditOptions struct {
	Root           string
	GroupID        string
	ProgressDir    string
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
	GroupID          string `json:"group_id"`
	NameChanged      bool   `json:"name_changed"`
	RequiresUpdate   bool   `json:"requires_update"`
	NodeCount        int    `json:"node_count"`
	Revision         int64  `json:"revision"`
	StructureChanged bool   `json:"structure_changed"`
	NotModified      bool   `json:"not_modified"`
}

// Edit 更新订阅设置，并在影响节点内容的设置变化时重新验证订阅。
func Edit(ctx context.Context, options EditOptions) (EditResult, error) {
	if strings.TrimSpace(options.Root) == "" || !validGroupID(options.GroupID) {
		return EditResult{}, &Error{Code: "subscription.invalid_target", Message: "订阅目录或分组无效"}
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	metaPath := filepath.Join(options.Root, options.GroupID, "meta.json")
	metadata, err := LoadMetadata(metaPath, options.GroupID)
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
			requiresUpdate = true
		}
	}
	if options.HWID != nil {
		if err := validateEditText(*options.HWID); err != nil {
			return EditResult{}, err
		}
		if metadata.HWID != *options.HWID {
			metadata.HWID = *options.HWID
			requiresUpdate = true
		}
	}
	if options.CustomHeaders != nil {
		metadata.CustomHeaders = cloneHeaders(*options.CustomHeaders)
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
			requiresUpdate = true
		}
	}
	if options.Exclude != nil {
		if err := validateEditText(*options.Exclude); err != nil {
			return EditResult{}, err
		}
		if metadata.Exclude != *options.Exclude {
			metadata.Exclude = *options.Exclude
			requiresUpdate = true
		}
	}
	if options.AllowInsecure != nil {
		if metadata.AllowInsecure != *options.AllowInsecure {
			metadata.AllowInsecure = *options.AllowInsecure
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
			ScheduleAt(&metadata, options.Now)
		} else {
			metadata.NextUpdateEpoch = 0
			metadata.NextUpdateAt = ""
		}
	}
	metadata.UpdatedAt = formatTime(options.Now)
	if err := SaveMetadataAtomic(metaPath, metadata); err != nil {
		return EditResult{}, &Error{Code: "subscription.edit_failed", Message: "保存订阅设置失败", Data: err.Error()}
	}
	if !requiresUpdate {
		return EditResult{GroupID: options.GroupID, NameChanged: nameChanged}, nil
	}
	updated, err := Update(ctx, UpdateOptions{
		Root: options.Root, GroupID: options.GroupID, ProgressDir: options.ProgressDir,
		ProxyURL: options.ProxyURL, UseConfiguredProxy: true,
		FallbackDirect: options.FallbackDirect, Now: options.Now,
	})
	if err != nil {
		_ = SaveMetadataAtomic(metaPath, oldMetadata)
		return EditResult{}, err
	}
	return EditResult{
		GroupID: options.GroupID, NameChanged: nameChanged, RequiresUpdate: true,
		NodeCount: updated.NodeCount, Revision: updated.Revision,
		StructureChanged: updated.StructureChanged, NotModified: updated.NotModified,
	}, nil
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
