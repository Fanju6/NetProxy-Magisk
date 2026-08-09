package module

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

// NetworkEvaluation 描述一次网络事件评估以及实际应用结果。
type NetworkEvaluation struct {
	Enabled     bool   `json:"enabled"`
	NetworkType string `json:"network_type"`
	SSID        string `json:"ssid,omitempty"`
	Target      string `json:"target,omitempty"`
	DesiredMode string `json:"desired_mode,omitempty"`
	RuntimeMode string `json:"runtime_mode,omitempty"`
	Changed     bool   `json:"changed"`
	Reason      string `json:"reason,omitempty"`
}

// EvaluateNetwork 根据 module.conf 的 WiFi 策略评估并应用目标出站模式。
//
// 网络脚本只负责采集 Android 原始事件；SSID 匹配、移动网络策略、模式切换
// 和连接清理全部在这里完成，避免 Shell 与 Go 各自维护一套状态机。
func EvaluateNetwork(ctx context.Context, options Options, networkType, ssid string) (NetworkEvaluation, error) {
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return NetworkEvaluation{}, err
	}
	networkType = strings.TrimSpace(strings.ToLower(networkType))
	if networkType != "wifi" && networkType != "not_wifi" {
		return NetworkEvaluation{}, errors.New("网络类型必须是 wifi 或 not_wifi")
	}
	result := NetworkEvaluation{
		Enabled:     module.WiFiAutoSwitch,
		NetworkType: networkType,
		SSID:        strings.TrimSpace(ssid),
	}
	if !module.WiFiAutoSwitch {
		result.Target = "proxying"
		result.DesiredMode = module.OutboundMode
		result.RuntimeMode, _ = service.ReadRuntimeMode(ctx, networkControlOptions(options))
		if result.RuntimeMode != "" && result.RuntimeMode != modeToRuntime(module.OutboundMode) {
			if err := ApplyRuntimeMode(ctx, options, module.OutboundMode); err != nil {
				return result, err
			}
			_ = service.CloseAllConnections(ctx, networkControlOptions(options))
			result.RuntimeMode = modeToRuntime(module.OutboundMode)
			result.Changed = true
			result.Reason = "已恢复基础出站模式"
		} else {
			result.Reason = "WiFi 自动切换未启用"
		}
		_ = clearWiFiState(options.WiFiStateFile)
		return result, nil
	}
	if networkType == "wifi" && result.SSID == "" {
		result.Reason = "WiFi 已连接但 SSID 尚不可读"
		return result, nil
	}

	target := "proxying"
	if networkType == "wifi" {
		listed := containsSSID(module.WiFiSSIDList, result.SSID)
		if (module.WiFiSSIDMode == "whitelist" && !listed) ||
			(module.WiFiSSIDMode == "blacklist" && listed) {
			target = "bypassed"
		}
	} else if !module.ProxyOnCellular {
		target = "bypassed"
	}
	desiredMode := module.OutboundMode
	if target == "bypassed" {
		desiredMode = "direct"
	}
	result.Target = target
	result.DesiredMode = desiredMode

	previousState := readWiFiState(options.WiFiStateFile)
	if service.ProcessRunning(options.SingBoxPath) {
		result.RuntimeMode, _ = service.ReadRuntimeMode(ctx, networkControlOptions(options))
	}
	if previousState == target && (result.RuntimeMode == "" || result.RuntimeMode == modeToRuntime(desiredMode)) {
		result.Reason = "网络策略未变化"
		return result, nil
	}
	modeChanged := result.RuntimeMode != modeToRuntime(desiredMode)
	if modeChanged {
		if service.ProcessRunning(options.SingBoxPath) {
			if err := ApplyRuntimeMode(ctx, options, desiredMode); err != nil {
				return result, err
			}
		}
		_ = service.CloseAllConnections(ctx, networkControlOptions(options))
		result.RuntimeMode = modeToRuntime(desiredMode)
	}
	if err := writeWiFiState(options.WiFiStateFile, target); err != nil {
		return result, err
	}
	result.Changed = previousState != target || modeChanged
	if target == "bypassed" {
		result.Reason = "已切换为绕过代理"
	} else {
		result.Reason = "已切换为代理模式"
	}
	return result, nil
}

// ApplyRuntimeMode 只切换运行中的 Service API，不修改用户保存的基础模式。
// WiFi 自动切换使用它，避免绕过网络时把用户的 rule/global/AllowAds 覆盖成 direct。
func ApplyRuntimeMode(ctx context.Context, options Options, mode string) error {
	if mode != "rule" && mode != "global" && mode != "direct" && mode != "AllowAds" {
		return errors.New("未知运行时出站模式: " + mode)
	}
	return service.SetMode(ctx, networkControlOptions(options), mode)
}

func networkControlOptions(options Options) service.Options {
	return service.Options{
		ModuleConfig:   options.ModuleConfig,
		CatalogRoot:    options.CatalogRoot,
		StateFile:      options.StateFile,
		ProgressDir:    options.ProgressDir,
		WorkerPIDFile:  options.WorkerPIDFile,
		SingBoxPath:    options.SingBoxPath,
		ServiceAddress: options.ServiceAddress,
		ServiceSecret:  options.ServiceSecret,
	}
}

func containsSSID(list, target string) bool {
	list = strings.ReplaceAll(list, "，", ",")
	for _, value := range strings.Split(list, ",") {
		if strings.TrimSpace(value) == target && target != "" {
			return true
		}
	}
	return false
}

func modeToRuntime(mode string) string {
	return map[string]string{"rule": "Rule", "global": "Global", "direct": "Direct", "AllowAds": "AllowAds"}[mode]
}

func readWiFiState(path string) string {
	if path == "" {
		return ""
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	switch strings.TrimSpace(string(value)) {
	case "proxying", "bypassed":
		return strings.TrimSpace(string(value))
	default:
		return ""
	}
}

func writeWiFiState(path, state string) error {
	if path == "" {
		return nil
	}
	if state != "proxying" && state != "bypassed" {
		return errors.New("无效的 WiFi 代理状态")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".wifi-state-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(state + "\n"); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func clearWiFiState(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
