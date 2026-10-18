package service

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"os"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/sagernet/sing-box/option"
)

type PolicySnapshot struct {
	Groups              []serviceapi.Group `json:"groups"`
	Live                bool               `json:"live"`
	CoreStartedAtMillis int64              `json:"core_started_at_millis"`
	DefaultOutbound     string             `json:"default_outbound"`
}

func ReadConfiguredPolicyGroups(path string) (PolicySnapshot, error) {
	result := PolicySnapshot{Groups: []serviceapi.Group{}}
	content, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	var document *struct {
		Outbounds []jsontext.Value `json:"outbounds"`
		Route     *struct {
			Final string `json:"final"`
		} `json:"route"`
	}
	if err := json.Unmarshal(content, &document); err != nil || document == nil {
		return result, policyError("config_invalid", "无法读取原生策略组配置")
	}
	if document.Route != nil {
		result.DefaultOutbound = document.Route.Final
	}
	type outboundIdentity struct {
		Tag  string `json:"tag"`
		Type string `json:"type"`
	}
	identities := make([]outboundIdentity, len(document.Outbounds))
	types := make(map[string]string, len(document.Outbounds))
	for index, raw := range document.Outbounds {
		var identity *outboundIdentity
		if err := json.Unmarshal(raw, &identity); err != nil || identity == nil {
			return result, policyError("config_invalid", "无法读取原生出站摘要")
		}
		identities[index] = *identity
		if identity.Tag != "" {
			if _, exists := types[identity.Tag]; exists {
				return result, policyError("config_invalid", "原生出站标签重复")
			}
			types[identity.Tag] = identity.Type
		}
	}
	for index, identity := range identities {
		var common option.GroupCommonOption
		switch identity.Type {
		case "selector":
			var native option.SelectorOutboundOptions
			if err := json.Unmarshal(document.Outbounds[index], &native); err != nil {
				return result, policyError("config_invalid", "无法读取原生 selector 配置")
			}
			common = native.GroupCommonOption
		case "urltest":
			var native option.URLTestOutboundOptions
			if err := json.Unmarshal(document.Outbounds[index], &native); err != nil {
				return result, policyError("config_invalid", "无法读取原生 urltest 配置")
			}
			common = native.GroupCommonOption
		default:
			continue
		}
		if strings.TrimSpace(identity.Tag) == "" {
			return result, policyError("config_invalid", "原生策略组标签不能为空")
		}
		group := serviceapi.Group{Tag: identity.Tag, Type: identity.Type,
			Selectable: identity.Type == "selector", Items: []serviceapi.GroupItem{}}
		// Provider 成员由核心展开；停止态不读取缓存或猜测实时成员与选择。
		for _, tag := range common.Outbounds {
			group.Items = append(group.Items, serviceapi.GroupItem{Tag: tag, Type: types[tag]})
		}
		result.Groups = append(result.Groups, group)
	}
	return result, nil
}

func ReadPolicyGroups(ctx context.Context, options Options) (PolicySnapshot, error) {
	options = normalizeOptions(options)
	result := PolicySnapshot{Groups: []serviceapi.Group{}}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	state := readState(options.StateFile)
	if state.State != "ready" {
		return ReadConfiguredPolicyGroups(options.SingBoxConfig)
	}
	defaultOutbound, err := ReadDefaultOutbound(options.SingBoxConfig)
	if err != nil {
		return result, err
	}
	client, requestCtx, cancel, err := newClient(ctx, options)
	if err != nil {
		return result, policyError("api_failed", "策略组接口不可用")
	}
	defer cancel()
	defer client.Close()
	if err := policyIdentity(requestCtx, options, state, client); err != nil {
		return result, err
	}
	return readLivePolicy(requestCtx, options, state, client, defaultOutbound)
}

func SelectPolicyGroup(ctx context.Context, options Options, groupTag, memberTag string) (PolicySnapshot, error) {
	return operatePolicyGroup(ctx, options, groupTag, memberTag, false)
}

func DelayPolicyGroup(ctx context.Context, options Options, groupTag string) (PolicySnapshot, error) {
	return operatePolicyGroup(ctx, options, groupTag, "", true)
}

func operatePolicyGroup(ctx context.Context, options Options, groupTag, memberTag string, delay bool) (PolicySnapshot, error) {
	result := PolicySnapshot{Groups: []serviceapi.Group{}}
	if strings.TrimSpace(groupTag) == "" || !delay && strings.TrimSpace(memberTag) == "" {
		return result, policyError("invalid", "必须提供策略组和成员标签")
	}
	options = normalizeOptions(options)
	state := readState(options.StateFile)
	if state.State != "ready" {
		return result, policyError("core_not_ready", "sing-box 尚未就绪")
	}
	defaultOutbound, err := ReadDefaultOutbound(options.SingBoxConfig)
	if err != nil {
		return result, err
	}
	client, requestCtx, cancel, err := newClient(ctx, options)
	if err != nil {
		return result, policyError("api_failed", "策略组接口不可用")
	}
	defer cancel()
	defer client.Close()
	if err := policyIdentity(requestCtx, options, state, client); err != nil {
		return result, err
	}
	result, err = readLivePolicy(requestCtx, options, state, client, defaultOutbound)
	if err != nil {
		return result, err
	}
	var target *serviceapi.Group
	for index := range result.Groups {
		if result.Groups[index].Tag == groupTag {
			target = &result.Groups[index]
			break
		}
	}
	if target == nil {
		return result, policyError("group_not_found", "未找到原生策略组")
	}
	if !delay {
		if !target.Selectable {
			return result, policyError("not_selectable", "该策略组不支持手动选择")
		}
		found := false
		for _, item := range target.Items {
			if item.Tag == memberTag {
				found = true
				break
			}
		}
		if !found {
			return result, policyError("member_not_found", "该策略组中不存在指定成员")
		}
		err = client.Select(requestCtx, groupTag, memberTag)
	} else {
		err = client.URLTest(requestCtx, groupTag)
	}
	if err != nil {
		if requestCtx.Err() != nil {
			return result, requestCtx.Err()
		}
		return result, policyError("runtime_sync_failed", "策略组操作未能确认，请刷新后核对")
	}
	// 核心拥有 cache_file，不能写 module selection，也不能在回读失败后重发操作。
	updated, err := readLivePolicy(requestCtx, options, state, client, defaultOutbound)
	if err == nil && !delay {
		confirmed := false
		for _, group := range updated.Groups {
			if group.Tag == groupTag && group.Selected == memberTag {
				confirmed = true
				break
			}
		}
		if !confirmed {
			err = policyError("runtime_sync_failed", "核心未确认策略组选择，请刷新后核对")
		}
	}
	if err != nil {
		return updated, &Error{Code: "policy.runtime_sync_failed", Message: "核心已接受操作，但快照未能确认，请刷新后核对",
			Data: map[string]any{"accepted": true, "core_started_at_millis": state.CoreStartedAtMillis}}
	}
	return updated, nil
}

func readLivePolicy(ctx context.Context, options Options, state stateFile, client *serviceapi.Client, defaultOutbound string) (PolicySnapshot, error) {
	result := PolicySnapshot{Groups: []serviceapi.Group{}, DefaultOutbound: defaultOutbound}
	groups, err := client.Groups(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, policyError("api_failed", "无法读取核心策略组快照")
	}
	if err := policyIdentity(ctx, options, state, client); err != nil {
		return result, err
	}
	if groups != nil {
		result.Groups = groups
	}
	result.Live = true
	result.CoreStartedAtMillis = state.CoreStartedAtMillis
	return result, nil
}

func policyIdentity(ctx context.Context, options Options, expected stateFile, client *serviceapi.Client) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current := readState(options.StateFile)
	if expected.State != "ready" || expected.PID <= 0 || expected.CoreStartedAtMillis <= 0 ||
		current.State != "ready" || current.PID != expected.PID || current.CoreStartedAtMillis != expected.CoreStartedAtMillis ||
		serviceFindProcess(options.SingBoxPath, expected.PID) != expected.PID {
		return policyError("core_not_ready", "sing-box 运行身份尚未确认")
	}
	started, err := client.StartedAt(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return policyError("api_failed", "无法确认核心运行身份")
	}
	if started.UnixMilli != expected.CoreStartedAtMillis {
		return policyError("core_not_ready", "sing-box 运行身份已变化")
	}
	current = readState(options.StateFile)
	if current.State != "ready" || current.PID != expected.PID || current.CoreStartedAtMillis != expected.CoreStartedAtMillis {
		return policyError("core_not_ready", "sing-box 运行身份已变化")
	}
	return nil
}

func policyError(code, message string) error {
	return &Error{Code: "policy." + code, Message: message}
}
