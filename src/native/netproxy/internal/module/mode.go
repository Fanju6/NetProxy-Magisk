package module

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"os"
	"slices"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

// ReadMode 返回主配置默认模式、可选模式和核心实际模式。
func ReadMode(ctx context.Context, options Options) (service.ModeState, error) {
	return service.ReadMode(ctx, networkControlOptions(options))
}

// ApplyMode 原子保存主配置默认模式，再应用当前网络策略；运行时失败不撤销持久化。
func ApplyMode(ctx context.Context, options Options, mode string) (result service.ModeState, err error) {
	persisted := false
	defer func() { logOperation(options, "mode", "mode.apply", "出站模式切换", persisted, err) }()
	options, release, err := lockModeConfig(ctx, options)
	if err != nil {
		return result, err
	}
	defer release()
	path := paths.SingBoxConfig(options.SingBoxDir)
	content, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	modes, err := moduleconfig.ParseModes(content)
	if err != nil {
		return result, err
	}
	if !slices.Contains(modes.Available, mode) {
		return result, &service.Error{Code: "mode.invalid", Message: "配置中不存在出站模式: " + mode, Data: modes}
	}
	result = service.ModeState{Mode: mode, Available: modes.Available}
	if mode != modes.Mode {
		content, err = replaceDefaultMode(content, mode)
		if err != nil {
			return result, err
		}
		updated, err := moduleconfig.ParseModes(content)
		if err != nil {
			return result, err
		}
		result.Available = updated.Available
		if err = writeConfigAtomic(path, content, 0o600); err != nil {
			return result, err
		}
	}
	persisted = true
	if !service.ProcessRunning(options.SingBoxPath) {
		return result, nil
	}
	evaluation, err := applyConfiguredNetwork(ctx, options, reloadNetworkConfig)
	result.RuntimeMode = evaluation.RuntimeMode
	if err != nil {
		return result, &service.Error{
			Code: "mode.runtime_sync_failed", Message: fmt.Sprintf("默认模式已保存，但运行时同步失败: %v", err),
			Data: map[string]any{"mode": mode, "persisted": true, "runtime_synced": false, "desired_mode": evaluation.DesiredMode, "runtime_mode": evaluation.RuntimeMode, "cause": err.Error()},
		}
	}
	return result, nil
}

func replaceDefaultMode(content []byte, mode string) ([]byte, error) {
	root, err := configObject(content)
	if err != nil {
		return nil, err
	}
	object := root
	var parents []map[string]jsontext.Value
	for _, key := range []string{"experimental", "clash_api"} {
		child := make(map[string]jsontext.Value)
		if raw := object[key]; len(raw) > 0 && string(raw) != "null" {
			child, err = configObject(raw)
			if err != nil {
				return nil, err
			}
		}
		parents = append(parents, object)
		object = child
	}
	object["default_mode"], err = json.Marshal(mode)
	if err != nil {
		return nil, err
	}
	for index, key := range []string{"clash_api", "experimental"} {
		parent := parents[len(parents)-1-index]
		parent[key], err = json.Marshal(object, json.Deterministic(true))
		if err != nil {
			return nil, err
		}
		object = parent
	}
	return json.Marshal(root, json.Deterministic(true), jsontext.WithIndent("  "))
}

func lockModeConfig(ctx context.Context, options Options) (Options, func(), error) {
	lock, err := waitLifecycleLock(ctx, options.StateFile)
	if err != nil {
		return options, nil, err
	}
	if err := recoverConfigApply(ctx, options); err != nil {
		lock.release()
		return options, nil, err
	}
	options, release, err := lockConfigFiles(ctx, options, options.ModuleConfig, options.InboundConfig, paths.SingBoxConfig(options.SingBoxDir))
	if err != nil {
		lock.release()
		return options, nil, err
	}
	return options, func() { release(); lock.release() }, nil
}
