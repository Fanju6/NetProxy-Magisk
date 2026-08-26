package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"encoding/json/jsontext"
	json "encoding/json/v2"
)

// ReadGroupSummary 读取指定分组的持久化摘要，不读取其他分组的 Provider 内容。
// 所有分组的 meta.json 仍需读取，用于保持重复名称的 RuntimeTag 消歧规则。
func ReadGroupSummary(ctx context.Context, root, groupID, progressDir string) (GroupSummary, error) {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(groupID) == "" {
		return GroupSummary{}, errors.New("Catalog 根目录和活动分组不能为空")
	}
	if err := ctx.Err(); err != nil {
		return GroupSummary{}, err
	}
	release, err := acquireCatalogRootAndRecover(root)
	if err != nil {
		return GroupSummary{}, err
	}
	defer release()

	entries, err := os.ReadDir(root)
	if err != nil {
		return GroupSummary{}, err
	}
	metadataByID := make(map[string]Metadata)
	nameCounts := make(map[string]int)
	for _, entry := range entries {
		if !isGroupDir(entry) {
			continue
		}
		metadata, err := LoadMetadataLocked(filepath.Join(root, entry.Name(), "meta.json"), entry.Name())
		if err != nil {
			return GroupSummary{}, fmt.Errorf("读取分组 %s 元数据失败: %w", entry.Name(), err)
		}
		metadataByID[entry.Name()] = metadata
		if metadata.NodeCount > 0 {
			nameCounts[metadata.Name]++
		}
	}
	metadata, ok := metadataByID[groupID]
	if !ok {
		return GroupSummary{}, fmt.Errorf("活动分组不存在: %s", groupID)
	}
	runtimeTag := metadata.Name
	if metadata.NodeCount > 0 && nameCounts[metadata.Name] > 1 {
		runtimeTag = fmt.Sprintf("%s [%s]", metadata.Name, groupID)
	}
	summary := summaryForMetadata(metadata, runtimeTag, groupID, progressDir, metadata.NodeCount)

	providerPath := filepath.Join(root, groupID, "provider.json")
	if info, err := os.Stat(providerPath); err != nil || !info.Mode().IsRegular() {
		summary.NodeCount = 0
		if err == nil {
			err = errors.New("Provider 不是普通文件")
		}
		return summary, fmt.Errorf("读取活动分组 %s Provider 失败: %w", groupID, err)
	}
	return summary, nil
}

func summaryForMetadata(metadata Metadata, runtimeTag, activeGroup, progressDir string, nodeCount int) GroupSummary {
	progress := jsontext.Value("null")
	if progressDir != "" {
		if content, err := os.ReadFile(filepath.Join(progressDir, metadata.ID+".progress.json")); err == nil {
			var state struct {
				Stage string `json:"stage"`
			}
			if json.Unmarshal(content, &state) == nil {
				switch state.Stage {
				case "download", "convert", "validate", "apply":
					progress = content
				}
			}
		}
	}
	return GroupSummary{
		ID: metadata.ID, Name: metadata.Name, RuntimeTag: runtimeTag, Type: metadata.Type,
		Active: metadata.ID == activeGroup, NodeCount: nodeCount, Revision: metadata.Revision,
		AutoUpdate: metadata.AutoUpdate, UpdateInterval: metadata.UpdateInterval,
		UpdateViaProxy: metadata.UpdateViaProxy, Usage: metadata.Usage,
		ProfileTitle: metadata.ProfileTitle, ProfileWebPageURL: metadata.ProfileWebPageURL,
		LastAttemptAt: metadata.LastAttemptAt, LastSuccessAt: metadata.LastSuccessAt,
		NextUpdateAt: metadata.NextUpdateAt, LastError: metadata.LastError,
		UpdatedAt: metadata.UpdatedAt, Progress: progress,
	}
}
