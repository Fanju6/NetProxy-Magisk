package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

const chainNodePrefix = "__netproxy_chain__"

// IsInternalChainNodeTag 判断 Service API 节点标签是否属于运行时代理链内部节点。
func IsInternalChainNodeTag(tag string) bool {
	return strings.HasPrefix(tag, chainNodePrefix+"/")
}

// ValidateProxyReference 校验一个可作为前置或落地代理的稳定节点引用。
func ValidateProxyReference(ctx context.Context, root, reference string) error {
	if strings.TrimSpace(reference) == "" {
		return nil
	}
	release, err := acquireCatalogRootAndRecover(ctx, root)
	if err != nil {
		return err
	}
	defer release()
	return ValidateProxyReferenceLocked(ctx, root, reference)
}

// ValidateProxyReferenceLocked 在调用方已持有 Catalog 根锁时校验代理链节点。
func ValidateProxyReferenceLocked(ctx context.Context, root, reference string) error {
	if strings.TrimSpace(reference) == "" {
		return nil
	}
	document, err := loadProxyReferenceLocked(ctx, root, reference)
	if err != nil {
		return err
	}
	return validateSimpleChainNode(document)
}

// ValidateProxyReferenceTargetsLocked 校验候选 Provider 仍满足其他订阅的代理链引用。
func ValidateProxyReferenceTargetsLocked(ctx context.Context, root, groupID string, candidate provider.Document, excludedOwner string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !isGroupDir(entry) || entry.Name() == excludedOwner {
			continue
		}
		metadataPath := filepath.Join(root, entry.Name(), "meta.json")
		metadata, err := LoadMetadataLocked(metadataPath, entry.Name())
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, usage := range []struct {
			role      string
			reference string
		}{
			{role: "前置代理", reference: metadata.FrontProxy},
			{role: "落地代理", reference: metadata.LandingProxy},
		} {
			if usage.reference == "" {
				continue
			}
			referencedGroup, tag, err := splitProxyReference(usage.reference)
			if err != nil {
				return fmt.Errorf("订阅 %s 的%s引用无效: %w", metadata.Name, usage.role, err)
			}
			if referencedGroup != groupID {
				continue
			}
			selected, found := provider.Select(candidate, tag)
			if !found {
				return fmt.Errorf("订阅 %s 的%s正在使用节点 %s", metadata.Name, usage.role, usage.reference)
			}
			if err := validateSimpleChainNode(selected); err != nil {
				return fmt.Errorf("订阅 %s 的%s节点 %s 不再可用: %w", metadata.Name, usage.role, usage.reference, err)
			}
		}
	}
	return nil
}

// HasProxyChains 返回 Catalog 中是否存在需要重建运行时 Provider 的代理链。
func HasProxyChains(ctx context.Context, root string) (bool, error) {
	release, err := acquireCatalogRootAndRecover(ctx, root)
	if err != nil {
		return false, err
	}
	defer release()
	entries, err := os.ReadDir(root)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !isGroupDir(entry) {
			continue
		}
		metadata, err := LoadMetadataLocked(filepath.Join(root, entry.Name(), "meta.json"), entry.Name())
		if err != nil {
			return false, err
		}
		if metadata.FrontProxy != "" || metadata.LandingProxy != "" {
			return true, nil
		}
	}
	return false, nil
}

func prepareRuntimeProviderCopies(ctx context.Context, runtimeDir string, groups []*loadedGroup) error {
	for _, group := range groups {
		group.RuntimePath = group.ProviderPath
		group.ChainExclude = nil
		if group.Metadata.FrontProxy == "" && group.Metadata.LandingProxy == "" {
			continue
		}
		document, err := buildChainedProvider(ctx, groups, group)
		if err != nil {
			return fmt.Errorf("生成分组 %s 代理链: %w", group.ID, err)
		}
		chainDir := filepath.Join(runtimeDir, "providers")
		path := filepath.Join(chainDir, group.ID+".json")
		if err := provider.SaveAtomic(ctx, path, document); err != nil {
			return fmt.Errorf("写入分组 %s 运行时 Provider: %w", group.ID, err)
		}
		group.RuntimePath = path
		pattern := "^" + regexp.QuoteMeta(group.RuntimeTag+"/"+chainNodePrefix+"/")
		compiled := regexp.MustCompile(pattern)
		exclude := badoption.Regexp(*compiled)
		group.ChainExclude = &exclude
	}
	return nil
}

func buildChainedProvider(ctx context.Context, groups []*loadedGroup, group *loadedGroup) (provider.Document, error) {
	source, err := provider.Load(ctx, group.ProviderPath)
	if err != nil {
		return provider.Document{}, err
	}
	var front, landing provider.Document
	if group.Metadata.FrontProxy != "" {
		front, err = loadRuntimeProxyReference(ctx, groups, group.Metadata.FrontProxy)
		if err != nil {
			return provider.Document{}, fmt.Errorf("前置代理 %s: %w", group.Metadata.FrontProxy, err)
		}
	}
	if group.Metadata.LandingProxy != "" {
		landing, err = loadRuntimeProxyReference(ctx, groups, group.Metadata.LandingProxy)
		if err != nil {
			return provider.Document{}, fmt.Errorf("落地代理 %s: %w", group.Metadata.LandingProxy, err)
		}
	}

	result := provider.Document{}
	frontTag := ""
	if group.Metadata.FrontProxy != "" {
		frontTag = chainNodePrefix + "/front"
		frontCopy, err := cloneChainNode(ctx, front)
		if err != nil {
			return provider.Document{}, err
		}
		if err := rewriteChainNode(&frontCopy, frontTag, ""); err != nil {
			return provider.Document{}, fmt.Errorf("前置代理不可用: %w", err)
		}
		appendChainDocument(&result, frontCopy)
	}

	index := 0
	for _, outbound := range source.Outbounds {
		single := provider.Document{Outbounds: []option.Outbound{outbound}}
		if err := appendRuntimeChainNode(ctx, &result, single, landing, frontTag, index); err != nil {
			return provider.Document{}, fmt.Errorf("节点 %s: %w", outbound.Tag, err)
		}
		index++
	}
	for _, endpoint := range source.Endpoints {
		single := provider.Document{Endpoints: []option.Endpoint{endpoint}}
		if err := appendRuntimeChainNode(ctx, &result, single, landing, frontTag, index); err != nil {
			return provider.Document{}, fmt.Errorf("节点 %s: %w", endpoint.Tag, err)
		}
		index++
	}
	return result, nil
}

func appendRuntimeChainNode(ctx context.Context, target *provider.Document, source, landing provider.Document, frontTag string, index int) error {
	visibleTag, err := chainNodeTag(source)
	if err != nil {
		return err
	}
	base, err := cloneChainNode(ctx, source)
	if err != nil {
		return err
	}
	if len(landing.Outbounds)+len(landing.Endpoints) == 0 {
		if err := rewriteChainNode(&base, visibleTag, frontTag); err != nil {
			return err
		}
		appendChainDocument(target, base)
		return nil
	}

	baseTag := fmt.Sprintf("%s/base/%d", chainNodePrefix, index)
	if err := rewriteChainNode(&base, baseTag, frontTag); err != nil {
		return err
	}
	landingCopy, err := cloneChainNode(ctx, landing)
	if err != nil {
		return err
	}
	if err := rewriteChainNode(&landingCopy, visibleTag, baseTag); err != nil {
		return fmt.Errorf("落地代理不可用: %w", err)
	}
	appendChainDocument(target, base)
	appendChainDocument(target, landingCopy)
	return nil
}

func loadRuntimeProxyReference(ctx context.Context, groups []*loadedGroup, reference string) (provider.Document, error) {
	groupID, tag, err := splitProxyReference(reference)
	if err != nil {
		return provider.Document{}, err
	}
	for _, group := range groups {
		if group.ID != groupID {
			continue
		}
		document, err := provider.Load(ctx, group.ProviderPath)
		if err != nil {
			return provider.Document{}, err
		}
		selected, found := provider.Select(document, tag)
		if !found {
			return provider.Document{}, fmt.Errorf("节点不存在: %s", reference)
		}
		if err := validateSimpleChainNode(selected); err != nil {
			return provider.Document{}, err
		}
		return selected, nil
	}
	return provider.Document{}, fmt.Errorf("分组不存在或没有可用节点: %s", groupID)
}

func loadProxyReferenceLocked(ctx context.Context, root, reference string) (provider.Document, error) {
	groupID, tag, err := splitProxyReference(reference)
	if err != nil {
		return provider.Document{}, err
	}
	document, err := loadGroupProvider(ctx, root, groupID)
	if err != nil {
		return provider.Document{}, err
	}
	selected, found := provider.Select(document, tag)
	if !found {
		return provider.Document{}, fmt.Errorf("节点不存在: %s", reference)
	}
	return selected, nil
}

func splitProxyReference(reference string) (string, string, error) {
	groupID, tag, found := strings.Cut(strings.TrimSpace(reference), "/")
	if !found || !isValidGroupID(groupID) || strings.TrimSpace(tag) == "" {
		return "", "", errors.New("节点引用格式应为 <group-id>/<节点标签>")
	}
	return groupID, tag, nil
}

func cloneChainNode(ctx context.Context, document provider.Document) (provider.Document, error) {
	content, err := provider.Marshal(ctx, document)
	if err != nil {
		return provider.Document{}, err
	}
	return provider.ParseDocument(ctx, content)
}

func validateSimpleChainNode(document provider.Document) error {
	if len(document.Outbounds)+len(document.Endpoints) != 1 {
		return errors.New("代理链节点必须只包含一个出站")
	}
	if len(document.Outbounds) == 1 {
		wrapper, ok := document.Outbounds[0].Options.(option.DialerOptionsWrapper)
		if !ok {
			return fmt.Errorf("协议 %s 不支持代理链拨号", document.Outbounds[0].Type)
		}
		if wrapper.TakeDialerOptions().Detour != "" {
			return errors.New("节点已包含 detour，不能重复作为订阅级代理链跳点")
		}
		return nil
	}
	wrapper, ok := document.Endpoints[0].Options.(option.DialerOptionsWrapper)
	if !ok {
		return fmt.Errorf("协议 %s 不支持代理链拨号", document.Endpoints[0].Type)
	}
	if wrapper.TakeDialerOptions().Detour != "" {
		return errors.New("节点已包含 detour，不能重复作为订阅级代理链跳点")
	}
	return nil
}

func rewriteChainNode(document *provider.Document, tag, detour string) error {
	if len(document.Outbounds)+len(document.Endpoints) != 1 {
		return errors.New("代理链节点必须只包含一个出站")
	}
	if len(document.Outbounds) == 1 {
		wrapper, ok := document.Outbounds[0].Options.(option.DialerOptionsWrapper)
		if !ok {
			return fmt.Errorf("协议 %s 不支持代理链拨号", document.Outbounds[0].Type)
		}
		dialer := wrapper.TakeDialerOptions()
		if dialer.Detour != "" {
			return errors.New("节点已包含 detour，不能重复作为订阅级代理链跳点")
		}
		dialer.Detour = detour
		wrapper.ReplaceDialerOptions(dialer)
		document.Outbounds[0].Tag = tag
		return nil
	}
	wrapper, ok := document.Endpoints[0].Options.(option.DialerOptionsWrapper)
	if !ok {
		return fmt.Errorf("协议 %s 不支持代理链拨号", document.Endpoints[0].Type)
	}
	dialer := wrapper.TakeDialerOptions()
	if dialer.Detour != "" {
		return errors.New("节点已包含 detour，不能重复作为订阅级代理链跳点")
	}
	dialer.Detour = detour
	wrapper.ReplaceDialerOptions(dialer)
	document.Endpoints[0].Tag = tag
	return nil
}

func chainNodeTag(document provider.Document) (string, error) {
	if len(document.Outbounds)+len(document.Endpoints) != 1 {
		return "", errors.New("代理链节点必须只包含一个出站")
	}
	if len(document.Outbounds) == 1 {
		return document.Outbounds[0].Tag, nil
	}
	return document.Endpoints[0].Tag, nil
}

func appendChainDocument(target *provider.Document, source provider.Document) {
	target.Outbounds = append(target.Outbounds, source.Outbounds...)
	target.Endpoints = append(target.Endpoints, source.Endpoints...)
}
