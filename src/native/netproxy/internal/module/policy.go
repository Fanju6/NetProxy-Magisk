package module

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"os"
	"slices"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
)

func ReadPolicyGroups(ctx context.Context, options Options) (service.PolicySnapshot, error) {
	return service.ReadPolicyGroups(ctx, networkControlOptions(options))
}

func SelectPolicyGroup(ctx context.Context, options Options, target service.PolicyTarget, member string) (service.PolicySnapshot, error) {
	var snapshot service.PolicySnapshot
	if strings.TrimSpace(target.GroupTag) == "" || strings.TrimSpace(member) == "" || target.CoreStartedAtMillis < 0 {
		return snapshot, policySelectionError("invalid", "必须提供策略组和成员标签")
	}
	path := paths.SingBoxConfig(options.SingBoxDir)
	options, release, err := lockConfigApply(ctx, options, path, false)
	if err != nil {
		return snapshot, err
	}
	defer release()
	state, err := ReadServiceState(options.StateFile)
	if err != nil {
		return snapshot, err
	}
	live := target.CoreStartedAtMillis > 0
	if live && (state.State != "ready" || state.CoreStartedAtMillis != target.CoreStartedAtMillis) ||
		!live && ((state.State != "stopped" && state.State != "failed") || service.ProcessRunning(options.SingBoxPath)) {
		return snapshot, policySelectionError("core_changed", "服务状态已变化，请刷新后再试")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return snapshot, err
	}
	root, err := configObject(content)
	if err != nil {
		return snapshot, err
	}
	updated, err := updateSelectorDefault(root, target.GroupTag, member)
	if err != nil {
		return snapshot, err
	}
	control := networkControlOptions(options)
	if live {
		snapshot, err = service.SelectPolicyGroup(ctx, control, target, member)
	} else {
		snapshot, err = service.ReadConfiguredPolicyGroups(ctx, control)
		if err == nil && !slices.ContainsFunc(snapshot.Groups, func(group serviceapi.Group) bool {
			return group.Tag == target.GroupTag && slices.ContainsFunc(group.Items, func(item serviceapi.GroupItem) bool {
				return item.Tag == member && item.Type != ""
			})
		}) {
			return service.PolicySnapshot{}, policySelectionError("member_not_found", "配置中没有这个可用成员")
		}
		if err == nil {
			err = clearConflictingPolicyCache(root, options.SingBoxDir, target.GroupTag, member)
		}
	}
	if err != nil {
		return service.PolicySnapshot{}, err
	}
	if updated != nil {
		if err := writeConfigAtomic(path, updated, 0o600); err != nil {
			if live {
				return service.PolicySnapshot{}, policySelectionError("persist_failed", "核心已切换，但未能保存默认节点")
			}
			return service.PolicySnapshot{}, err
		}
	}
	for index := range snapshot.Groups {
		if snapshot.Groups[index].Tag == target.GroupTag {
			snapshot.Groups[index].ConfiguredDefault = member
			break
		}
	}
	return snapshot, nil
}

func policySelectionError(code, message string) error {
	return &service.Error{Code: "policy." + code, Message: message}
}

func updateSelectorDefault(root map[string]jsontext.Value, tag, member string) ([]byte, error) {
	var outbounds []jsontext.Value
	if raw := root["outbounds"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &outbounds); err != nil {
			return nil, err
		}
	}
	index := -1
	var selector map[string]jsontext.Value
	for i, raw := range outbounds {
		var identity struct {
			Tag     string `json:"tag"`
			Type    string `json:"type"`
			Default string `json:"default"`
		}
		if err := json.Unmarshal(raw, &identity); err != nil {
			return nil, err
		}
		if identity.Tag != tag {
			continue
		}
		if index >= 0 {
			return nil, policySelectionError("config_invalid", "原生策略组标签重复")
		}
		if identity.Type != "selector" {
			return nil, policySelectionError("not_selectable", "只有 Selector 支持手动选择")
		}
		index = i
		if identity.Default != member {
			var err error
			selector, err = configObject(raw)
			if err != nil {
				return nil, err
			}
		}
	}
	if index < 0 {
		return nil, policySelectionError("group_not_found", "配置中没有这个策略组")
	}
	if selector == nil {
		return nil, nil
	}
	var err error
	if selector["default"], err = json.Marshal(member); err != nil {
		return nil, err
	}
	if outbounds[index], err = json.Marshal(selector, json.Deterministic(true)); err != nil {
		return nil, err
	}
	if root["outbounds"], err = json.Marshal(outbounds); err != nil {
		return nil, err
	}
	return json.Marshal(root, json.Deterministic(true), jsontext.WithIndent("  "))
}

func RequestPolicyDelay(ctx context.Context, options Options, target service.PolicyTarget) error {
	release, err := lockPolicyOperation(ctx, options)
	if err != nil {
		return err
	}
	defer release()
	return service.RequestPolicyDelay(ctx, networkControlOptions(options), target)
}

func TestNodeLatency(ctx context.Context, options Options, target service.NodeLatencyTarget) (service.NodeLatency, error) {
	return service.TestNodeLatency(ctx, networkControlOptions(options), paths.RuntimeConfig(options.RuntimeDir), target)
}

func lockPolicyOperation(ctx context.Context, options Options) (func(), error) {
	lock, err := waitLifecycleLock(ctx, options.StateFile)
	if err != nil {
		return nil, err
	}
	if err := recoverConfigApply(ctx, options); err != nil {
		lock.release()
		return nil, err
	}
	return lock.release, nil
}
