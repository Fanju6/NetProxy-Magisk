package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/sharelink"
)

// ResolveGroup 按分组 ID 或唯一显示名称解析分组。
func ResolveGroup(root, query string) (string, error) {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(query) == "" {
		return "", errors.New("Catalog 根目录和分组查询不能为空")
	}
	if err := recoverTransactions(root); err != nil {
		return "", err
	}
	if validGroupID.MatchString(query) {
		if _, err := os.Stat(filepath.Join(root, query)); err == nil {
			return query, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	match := ""
	count := 0
	for _, entry := range entries {
		if !isGroupDir(entry) {
			continue
		}
		metadata, err := loadMetadata(filepath.Join(root, entry.Name(), "meta.json"), entry.Name())
		if err != nil {
			return "", fmt.Errorf("读取分组 %s 元数据: %w", entry.Name(), err)
		}
		if metadata.Name != query {
			continue
		}
		match = entry.Name()
		count++
	}
	if count == 1 {
		return match, nil
	}
	if count > 1 {
		return "", fmt.Errorf("分组名称不唯一: %s", query)
	}
	return "", fmt.Errorf("分组不存在: %s", query)
}

// GroupHasNodes 判断分组是否包含节点。
func GroupHasNodes(ctx context.Context, root, groupID string) (bool, error) {
	document, err := loadGroupProvider(ctx, root, groupID)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return len(document.Outbounds)+len(document.Endpoints) > 0, nil
}

// GroupFirstTag 返回分组按标签排序后的第一个节点标签。
func GroupFirstTag(ctx context.Context, root, groupID string) (string, error) {
	document, err := loadGroupProvider(ctx, root, groupID)
	if err != nil {
		return "", err
	}
	nodes := provider.Inspect(document)
	if len(nodes) == 0 {
		return "", nil
	}
	return nodes[0].Tag, nil
}

// GroupContainsTag 判断分组是否包含指定节点标签。
func GroupContainsTag(ctx context.Context, root, groupID, tag string) (bool, error) {
	document, err := loadGroupProvider(ctx, root, groupID)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return ProviderContainsTagDocument(document, tag), nil
}

// GroupNode 返回分组中指定节点的标准 Provider JSON。
func GroupNode(ctx context.Context, root, groupID, tag string) (provider.Document, error) {
	document, err := loadGroupProvider(ctx, root, groupID)
	if err != nil {
		return provider.Document{}, err
	}
	selected, found := provider.Select(document, tag)
	if !found {
		return provider.Document{}, fmt.Errorf("未找到节点标签 %q", tag)
	}
	return selected, nil
}

// ExportGroupNode 将 Catalog 节点导出为分享链接。
func ExportGroupNode(ctx context.Context, root, groupID, tag string) (sharelink.Result, error) {
	document, err := loadGroupProvider(ctx, root, groupID)
	if err != nil {
		return sharelink.Result{}, err
	}
	return sharelink.Export(document, tag)
}

// PrivateMetadata 返回订阅编辑所需的完整元数据。
func PrivateMetadata(root, groupID string) (Metadata, error) {
	if !validGroupID.MatchString(groupID) {
		return Metadata{}, fmt.Errorf("非法分组 ID: %s", groupID)
	}
	if err := recoverTransactions(root); err != nil {
		return Metadata{}, err
	}
	return LoadMetadata(filepath.Join(root, groupID, "meta.json"), groupID)
}

// GroupType 返回分组类型。
func GroupType(root, groupID string) (string, error) {
	metadata, err := PrivateMetadata(root, groupID)
	if err != nil {
		return "", err
	}
	return metadata.Type, nil
}

// FirstNonEmptyGroup 返回第一个有节点的分组 ID，可排除指定分组。
func FirstNonEmptyGroup(ctx context.Context, root, exclude string) (string, error) {
	ids, err := GroupIDs(root, "all")
	if err != nil {
		return "", err
	}
	for _, groupID := range ids {
		if groupID == exclude {
			continue
		}
		hasNodes, err := GroupHasNodes(ctx, root, groupID)
		if err != nil {
			return "", err
		}
		if hasNodes {
			return groupID, nil
		}
	}
	return "", nil
}

// DeleteGroup 删除 Catalog 分组目录。
func DeleteGroup(root, groupID string) error {
	if !validGroupID.MatchString(groupID) || groupID == "default" {
		return fmt.Errorf("不允许删除分组: %s", groupID)
	}
	release := lockGroup(groupID)
	release.Lock()
	defer release.Unlock()
	if err := recoverTransactions(root); err != nil {
		return err
	}
	groupDir := filepath.Join(root, groupID)
	if _, err := os.Stat(groupDir); err != nil {
		return err
	}
	return os.RemoveAll(groupDir)
}

func loadGroupProvider(ctx context.Context, root, groupID string) (provider.Document, error) {
	if strings.TrimSpace(root) == "" || !validGroupID.MatchString(groupID) {
		return provider.Document{}, errors.New("Catalog 分组参数无效")
	}
	if err := recoverTransactions(root); err != nil {
		return provider.Document{}, err
	}
	return provider.LoadAllowEmpty(ctx, filepath.Join(root, groupID, "provider.json"))
}

// ProviderContainsTagDocument 判断已加载 Provider 是否包含标签。
func ProviderContainsTagDocument(document provider.Document, tag string) bool {
	for _, outbound := range document.Outbounds {
		if outbound.Tag == tag {
			return true
		}
	}
	for _, endpoint := range document.Endpoints {
		if endpoint.Tag == tag {
			return true
		}
	}
	return false
}
