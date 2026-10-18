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
func ReadGroupSummary(ctx context.Context, root, groupID, progressDir string) (GroupSummary, error) {
	if strings.TrimSpace(root) == "" || !isValidGroupID(groupID) {
		return GroupSummary{}, errors.New("Catalog 根目录和分组 ID 无效")
	}
	release, err := acquireCatalogRootAndRecover(ctx, root)
	if err != nil {
		return GroupSummary{}, err
	}
	defer release()
	metadata, err := LoadMetadataLocked(filepath.Join(root, groupID, "meta.json"), groupID)
	if err != nil {
		return GroupSummary{}, err
	}
	summary := summaryForMetadata(metadata, progressDir, metadata.NodeCount)

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

func summaryForMetadata(metadata Metadata, progressDir string, nodeCount int) GroupSummary {
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
		ID: metadata.ID, Name: metadata.Name, ProviderTag: ProviderTag(metadata.ID), Type: metadata.Type,
		NodeCount: nodeCount, Revision: metadata.Revision,
		AutoUpdate: metadata.AutoUpdate, UpdateInterval: metadata.UpdateInterval,
		UpdateViaProxy: metadata.UpdateViaProxy, Usage: metadata.Usage,
		ProfileTitle: metadata.ProfileTitle, ProfileWebPageURL: metadata.ProfileWebPageURL,
		LastAttemptAt: metadata.LastAttemptAt, LastSuccessAt: metadata.LastSuccessAt,
		NextUpdateAt: metadata.NextUpdateAt, LastError: metadata.LastError,
		UpdatedAt: metadata.UpdatedAt, Progress: progress,
	}
}
