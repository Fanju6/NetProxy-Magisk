package service

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
)

type PolicySnapshot struct {
	Groups              []serviceapi.Group `json:"groups"`
	Live                bool               `json:"live"`
	CoreStartedAtMillis int64              `json:"core_started_at_millis"`
}

type PolicyTarget struct {
	CoreStartedAtMillis int64  `json:"core_started_at_millis"`
	GroupTag            string `json:"group_tag"`
}

func ReadPolicyGroups(ctx context.Context, options Options) (PolicySnapshot, error) {
	options = normalizeOptions(options)
	state := readState(options.StateFile)
	if state.State != "ready" {
		return ReadConfiguredPolicyGroups(ctx, options)
	}
	return withPolicyCore(ctx, options, state, readLivePolicy)
}

// ReadConfiguredPolicyGroups reads persisted definitions. Configured members are definitions, never cached runtime selections.
func ReadConfiguredPolicyGroups(ctx context.Context, options Options) (PolicySnapshot, error) {
	result := PolicySnapshot{Groups: []serviceapi.Group{}}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	content, err := os.ReadFile(options.SingBoxConfig)
	if err != nil {
		return result, policyError("config_invalid", "无法读取原生策略组配置")
	}
	var document *struct {
		Outbounds []jsontext.Value `json:"outbounds"`
		Endpoints []jsontext.Value `json:"endpoints"`
		Providers []struct {
			Tag string `json:"tag"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(content, &document); err != nil || document == nil {
		return result, policyError("config_invalid", "无法读取原生策略组配置")
	}
	type definition struct {
		Tag  string `json:"tag"`
		Type string `json:"type"`
	}
	identities := make([]definition, 0, len(document.Outbounds))
	types := make(map[string]string)
	for index, entries := range [][]jsontext.Value{document.Outbounds, document.Endpoints} {
		for _, raw := range entries {
			var item definition
			if err := json.Unmarshal(raw, &item); err != nil {
				return result, policyError("config_invalid", "无法读取原生出站")
			}
			if index == 0 {
				identities = append(identities, item)
			}
			if item.Tag != "" {
				if _, exists := types[item.Tag]; exists {
					return result, policyError("config_invalid", "原生出站标签重复")
				}
				types[item.Tag] = item.Type
			}
		}
	}
	nativeProviders := make([]string, 0, len(document.Providers))
	for _, item := range document.Providers {
		nativeProviders = append(nativeProviders, item.Tag)
	}
	var available []catalog.GroupSnapshot
	var providersByName map[string]*catalog.GroupSnapshot
	var catalogLoaded bool
	loadCatalog := func() error {
		if catalogLoaded || options.CatalogRoot == "" {
			return nil
		}
		catalogLoaded = true
		var err error
		available, err = catalog.Scan(ctx, catalog.ScanOptions{Root: options.CatalogRoot, WithNodes: true})
		if err != nil && !os.IsNotExist(err) {
			return policyError("config_invalid", "无法读取 Catalog 策略组成员")
		}
		providersByName = make(map[string]*catalog.GroupSnapshot, len(available))
		for i := range available {
			entry := &available[i]
			if len(entry.Nodes) == 0 {
				continue
			}
			name := entry.Group.RuntimeTag
			if _, duplicate := providersByName[name]; duplicate {
				providersByName[name] = nil
			} else {
				providersByName[name] = entry
			}
		}
		return nil
	}
	for index, item := range identities {
		switch item.Type {
		case "selector", "urltest", "loadbalance":
		default:
			continue
		}
		if item.Tag == "" {
			return result, policyError("config_invalid", "策略组缺少标签")
		}
		var config struct {
			Outbounds []string `json:"outbounds"`
			Providers []string `json:"providers"`
			UseAll    bool     `json:"use_all_providers"`
			Default   string   `json:"default"`
			Include   string   `json:"include"`
			Exclude   string   `json:"exclude"`
		}
		if err := json.Unmarshal(document.Outbounds[index], &config); err != nil {
			return result, policyError("config_invalid", "无法读取原生策略组成员")
		}
		group := serviceapi.Group{Tag: item.Tag, Type: item.Type, Items: []serviceapi.GroupItem{}}
		if item.Type == "selector" {
			group.ConfiguredDefault = config.Default
		}
		seen := make(map[string]bool)
		add := func(tag, protocol, displayName string) {
			if !seen[tag] {
				group.Items = append(group.Items, serviceapi.GroupItem{Tag: tag, Type: protocol, DisplayName: displayName})
				seen[tag] = true
			}
		}
		for _, member := range config.Outbounds {
			protocol, name := types[member], ""
			if providerTag, nodeTag, qualified := strings.Cut(member, "/"); qualified && protocol == "" {
				if err := loadCatalog(); err != nil {
					return result, err
				}
				if entry := providersByName[providerTag]; entry != nil && !slices.Contains(nativeProviders, providerTag) {
					for _, node := range entry.Nodes {
						if node.Tag == nodeTag {
							protocol, name = node.Protocol, node.Tag
							break
						}
					}
				}
			}
			add(member, protocol, name)
		}
		if config.UseAll || len(config.Providers) != 0 {
			if err := loadCatalog(); err != nil {
				return result, err
			}
		}
		references := config.Providers
		if config.UseAll {
			references = slices.Clone(nativeProviders)
			for _, entry := range available {
				if len(entry.Nodes) != 0 {
					references = append(references, entry.Group.RuntimeTag)
				}
			}
		}
		include, includeErr := regexp.Compile(config.Include)
		exclude, excludeErr := regexp.Compile(config.Exclude)
		if includeErr != nil || excludeErr != nil {
			return result, policyError("config_invalid", "策略组成员过滤规则无效")
		}
		seenProviders := make(map[string]bool)
		for _, reference := range references {
			if seenProviders[reference] {
				continue
			}
			seenProviders[reference] = true
			entry := providersByName[reference]
			if entry == nil || slices.Contains(nativeProviders, reference) {
				group.UnavailableProviders = append(group.UnavailableProviders, reference)
				continue
			}
			for _, node := range entry.Nodes {
				tag := reference + "/" + node.Tag
				if (config.Include != "" && !include.MatchString(tag)) || (config.Exclude != "" && exclude.MatchString(tag)) {
					continue
				}
				add(tag, node.Protocol, node.Tag)
			}
		}
		result.Groups = append(result.Groups, group)
	}
	return result, nil
}

func SelectPolicyGroup(ctx context.Context, options Options, target PolicyTarget, member string) (PolicySnapshot, error) {
	if strings.TrimSpace(member) == "" {
		return PolicySnapshot{}, policyError("invalid", "必须提供成员标签")
	}
	return operatePolicyGroup(ctx, options, target, member)
}

// An accepted URL test is asynchronous; this does not return fresh measurements.
func RequestPolicyDelay(ctx context.Context, options Options, target PolicyTarget) error {
	_, err := operatePolicyGroup(ctx, options, target, "")
	return err
}

func operatePolicyGroup(ctx context.Context, options Options, target PolicyTarget, member string) (PolicySnapshot, error) {
	if strings.TrimSpace(target.GroupTag) == "" || target.CoreStartedAtMillis <= 0 {
		return PolicySnapshot{}, policyError("invalid", "必须提供策略组标签及核心实例标识")
	}
	options = normalizeOptions(options)
	state := readState(options.StateFile)
	if state.State != "ready" || state.CoreStartedAtMillis != target.CoreStartedAtMillis {
		return PolicySnapshot{}, policyError("core_changed", "核心尚未就绪或已变化，请刷新后再试")
	}
	return withPolicyCore(ctx, options, state, func(ctx context.Context, options Options, state stateFile, client *serviceapi.Client) (PolicySnapshot, error) {
		current, err := readLivePolicy(ctx, options, state, client)
		if err != nil {
			return PolicySnapshot{}, err
		}
		index := slices.IndexFunc(current.Groups, func(group serviceapi.Group) bool { return group.Tag == target.GroupTag })
		if index < 0 {
			return PolicySnapshot{}, policyError("group_not_found", "核心中没有这个策略组")
		}
		if member == "" {
			if err := client.URLTest(ctx, target.GroupTag); err != nil {
				return PolicySnapshot{}, policyError("outcome_unknown", "无法确认测速请求是否已接受，请刷新后核对")
			}
			if err := confirmPolicyCore(ctx, options, state, client); err != nil {
				return PolicySnapshot{}, policyError("outcome_unknown", "核心已接受测速请求，但实例状态未能确认")
			}
			return PolicySnapshot{}, nil
		}
		group := current.Groups[index]
		if !group.Selectable {
			return PolicySnapshot{}, policyError("not_selectable", "该策略组不支持手动选择")
		}
		if !slices.ContainsFunc(group.Items, func(item serviceapi.GroupItem) bool { return item.Tag == member }) {
			return PolicySnapshot{}, policyError("member_not_found", "策略组中没有这个成员")
		}
		if group.Selected == member {
			return current, nil
		}
		// One write only; a failed readback must not trigger another write.
		if err := client.Select(ctx, target.GroupTag, member); err != nil {
			return PolicySnapshot{}, policyError("outcome_unknown", "无法确认节点选择结果，请刷新后核对")
		}
		updated, err := readLivePolicy(ctx, options, state, client)
		if err != nil || !slices.ContainsFunc(updated.Groups, func(group serviceapi.Group) bool {
			return group.Tag == target.GroupTag && group.Selected == member
		}) {
			return PolicySnapshot{}, policyError("outcome_unknown", "核心已接受选择，但快照未能确认，请刷新后核对")
		}
		return updated, nil
	})
}

func withPolicyCore(ctx context.Context, options Options, state stateFile, operation func(context.Context, Options, stateFile, *serviceapi.Client) (PolicySnapshot, error)) (PolicySnapshot, error) {
	client, ctx, cancel, err := newClient(ctx, options)
	if err != nil {
		return PolicySnapshot{}, policyError("api_failed", "核心策略组接口不可用")
	}
	defer cancel()
	defer client.Close()
	if err := confirmPolicyCore(ctx, options, state, client); err != nil {
		return PolicySnapshot{}, err
	}
	return operation(ctx, options, state, client)
}

func readLivePolicy(ctx context.Context, options Options, state stateFile, client *serviceapi.Client) (PolicySnapshot, error) {
	groups, err := client.Groups(ctx)
	if err != nil {
		return PolicySnapshot{}, policyError("api_failed", "无法读取核心策略组快照")
	}
	if err := confirmPolicyCore(ctx, options, state, client); err != nil {
		return PolicySnapshot{}, err
	}
	if groups == nil {
		groups = []serviceapi.Group{}
	}
	for index := range groups {
		if groups[index].Items == nil {
			groups[index].Items = []serviceapi.GroupItem{}
		}
	}
	return PolicySnapshot{Groups: groups, Live: true, CoreStartedAtMillis: state.CoreStartedAtMillis}, nil
}

func confirmPolicyCore(ctx context.Context, options Options, state stateFile, client *serviceapi.Client) error {
	current := func() bool {
		value := readState(options.StateFile)
		return value.State == "ready" && value.PID == state.PID && value.CoreStartedAtMillis == state.CoreStartedAtMillis
	}
	if state.PID <= 0 || state.CoreStartedAtMillis <= 0 || !current() || serviceFindProcess(options.SingBoxPath, state.PID) != state.PID {
		return policyError("core_changed", "核心尚未就绪或已变化，请刷新后再试")
	}
	started, err := client.StartedAt(ctx)
	if err != nil {
		return policyError("api_failed", "无法确认核心实例")
	}
	if started.UnixMilli != state.CoreStartedAtMillis || !current() {
		return policyError("core_changed", "核心已变化，请刷新后再试")
	}
	return nil
}

func policyError(code, message string) error { return &Error{Code: "policy." + code, Message: message} }
