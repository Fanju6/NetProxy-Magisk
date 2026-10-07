package module

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
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

// EvaluateNetwork 在配置和生命周期锁内应用一次真实网络事件，不修改默认模式。
func EvaluateNetwork(ctx context.Context, options Options, networkType, ssid string) (result NetworkEvaluation, err error) {
	options, release, err := lockModeConfig(ctx, options)
	if err != nil {
		return result, err
	}
	defer release()
	return evaluateNetwork(ctx, options, networkType, ssid, service.ProcessRunning(options.SingBoxPath))
}

// 调用方已持有生命周期与配置锁，启动、重载和手动模式保存显式复用该路径。
func syncConfiguredMode(ctx context.Context, options Options) (NetworkEvaluation, error) {
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return NetworkEvaluation{}, err
	}
	if !module.WiFiAutoSwitch {
		return evaluateNetwork(ctx, options, "not_wifi", "", true)
	}
	reader := options.NetworkStateReader
	if reader == nil {
		reader = worker.ReadNetworkState
	}
	readContext, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	state, err := reader(readContext)
	if err == nil && state.NetworkType == "wifi" && strings.TrimSpace(state.SSID) == "" {
		err = worker.ErrNetworkUnavailable
	}
	if err != nil {
		if !errors.Is(err, worker.ErrNetworkUnavailable) {
			return NetworkEvaluation{}, err
		}
		// 开机无默认路由时先消除缓存模式，网络就绪事件再应用 Wi-Fi 策略。
		logService(options, "INFO", "network.read", "waiting", "网络尚未就绪：等待 Android 网络默认路由")
		modes, loadErr := moduleconfig.LoadModes(paths.SingBoxConfig(options.SingBoxDir))
		if loadErr != nil {
			return NetworkEvaluation{}, loadErr
		}
		return applyNetworkMode(ctx, options, modes, NetworkEvaluation{Enabled: true, DesiredMode: modes.Mode}, true)
	}
	return evaluateNetwork(ctx, options, state.NetworkType, state.SSID, true)
}

func evaluateNetwork(ctx context.Context, options Options, networkType, ssid string, running bool) (result NetworkEvaluation, err error) {
	defer func() {
		if err != nil || result.Changed {
			message := fmt.Sprintf("网络策略应用 (network_type=%s, target=%s, mode=%s)", result.NetworkType, result.Target, result.DesiredMode)
			logOperation(options, "network", "network.policy", message, false, err)
		}
	}()
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return result, err
	}
	modes, err := moduleconfig.LoadModes(paths.SingBoxConfig(options.SingBoxDir))
	if err != nil {
		return result, err
	}
	networkType = strings.TrimSpace(strings.ToLower(networkType))
	if networkType != "wifi" && networkType != "not_wifi" {
		return result, errors.New("网络类型必须是 wifi 或 not_wifi")
	}
	result = NetworkEvaluation{Enabled: module.WiFiAutoSwitch, NetworkType: networkType, SSID: strings.TrimSpace(ssid), Target: "proxying", DesiredMode: modes.Mode}
	if module.WiFiAutoSwitch {
		if networkType == "wifi" && result.SSID == "" {
			result.Target = ""
			result.Reason = "WiFi 已连接但 SSID 尚不可读"
			return result, nil
		}
		listed := containsSSID(module.WiFiSSIDList, result.SSID)
		if networkType == "wifi" && ((module.WiFiSSIDMode == "whitelist" && !listed) || (module.WiFiSSIDMode == "blacklist" && listed)) ||
			networkType == "not_wifi" && !module.ProxyOnCellular {
			result.Target = "bypassed"
			result.DesiredMode = "Direct"
		}
	}
	return applyNetworkMode(ctx, options, modes, result, running)
}

func applyNetworkMode(ctx context.Context, options Options, modes moduleconfig.Modes, result NetworkEvaluation, running bool) (NetworkEvaluation, error) {
	if !slices.Contains(modes.Available, result.DesiredMode) {
		return result, &service.Error{Code: "mode.unavailable", Message: "当前网络策略所需模式不在主配置中: " + result.DesiredMode, Data: modes}
	}
	previous := readWiFiState(options.WiFiStateFile)
	if running {
		current, err := service.ReadRuntimeMode(ctx, networkControlOptions(options))
		if err != nil {
			return result, err
		}
		result.RuntimeMode = current
		if current != result.DesiredMode {
			if err := service.SetMode(ctx, networkControlOptions(options), result.DesiredMode); err != nil {
				return result, err
			}
			result.RuntimeMode = result.DesiredMode
			result.Changed = true
			if err := service.CloseAllConnections(ctx, networkControlOptions(options)); err != nil {
				return result, fmt.Errorf("模式已切换，但关闭旧连接失败: %w", err)
			}
		}
	}
	if !result.Enabled {
		result.Reason = "WiFi 自动切换未启用"
		return result, clearWiFiState(options.WiFiStateFile)
	}
	if result.Target == "" {
		return result, nil
	}
	if err := writeWiFiState(options.WiFiStateFile, result.Target); err != nil {
		return result, err
	}
	result.Changed = result.Changed || previous != result.Target
	result.Reason = "网络策略未变化"
	if result.Changed {
		if result.Target == "bypassed" {
			result.Reason = "已切换为绕过代理"
		} else {
			result.Reason = "已切换为代理模式"
		}
	}
	return result, nil
}

func networkControlOptions(options Options) service.Options {
	return service.Options{
		ModuleConfig:   options.ModuleConfig,
		SingBoxConfig:  paths.SingBoxConfig(options.SingBoxDir),
		InboundConfig:  options.InboundConfig,
		CatalogRoot:    options.CatalogRoot,
		StateFile:      options.StateFile,
		ProgressDir:    options.ProgressDir,
		WorkerPIDFile:  options.WorkerPIDFile,
		SingBoxPath:    options.SingBoxPath,
		ServiceAddress: options.ServiceAddress,
		ServiceSecret:  options.ServiceSecret,
		RequestTimeout: options.RequestTimeout,
	}
}

func containsSSID(list, target string) bool {
	list = strings.ReplaceAll(list, "，", ",")
	for value := range strings.SplitSeq(list, ",") {
		if strings.TrimSpace(value) == target && target != "" {
			return true
		}
	}
	return false
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
