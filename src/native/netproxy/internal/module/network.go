package module

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
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
	result, err := configuredNetwork(ctx, options)
	if err != nil {
		return result, err
	}
	modes, err := moduleconfig.LoadModes(paths.SingBoxConfig(options.SingBoxDir))
	if err != nil {
		return result, err
	}
	return applyNetworkMode(ctx, options, modes, result, true)
}

func configuredNetwork(ctx context.Context, options Options) (NetworkEvaluation, error) {
	if options.networkEvaluation != nil {
		return *options.networkEvaluation, nil
	}
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return NetworkEvaluation{}, err
	}
	modes, err := moduleconfig.LoadModes(paths.SingBoxConfig(options.SingBoxDir))
	if err != nil {
		return NetworkEvaluation{}, err
	}
	if !module.WiFi.Enabled {
		return networkPolicy(module, modes.Mode, "not_wifi", ""), nil
	}
	reader := options.NetworkStateReader
	if reader == nil {
		reader = worker.ReadNetworkState
	}
	readContext, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	state, err := reader(readContext)
	if err == nil && state.NetworkType != "wifi" && state.NetworkType != "not_wifi" {
		return NetworkEvaluation{}, errors.New("网络采集返回了未知网络类型")
	}
	if err == nil && state.NetworkType == "wifi" && state.SSID == "" {
		err = worker.ErrNetworkUnavailable
	}
	if err != nil {
		if !errors.Is(err, worker.ErrNetworkUnavailable) {
			return NetworkEvaluation{}, err
		}
		// 开机无默认路由时先消除缓存模式，网络就绪事件再应用 Wi-Fi 策略。
		logService(options, "INFO", "network.read", "waiting", "网络尚未就绪：等待 Android 网络默认路由")
		return NetworkEvaluation{Enabled: true, DesiredMode: modes.Mode}, nil
	}
	return networkPolicy(module, modes.Mode, state.NetworkType, state.SSID), nil
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
	result = networkPolicy(module, modes.Mode, networkType, ssid)
	if result.Target == "" {
		return result, nil
	}
	return applyNetworkPolicy(ctx, options, modes, result, running, reloadNetworkConfig)
}

func networkPolicy(module moduleconfig.ModuleConfig, base, networkType, ssid string) NetworkEvaluation {
	result := NetworkEvaluation{Enabled: module.WiFi.Enabled, NetworkType: networkType, SSID: ssid, Target: "proxying", DesiredMode: base}
	if result.Enabled {
		if networkType == "wifi" && result.SSID == "" {
			result.Target = ""
			result.Reason = "WiFi 已连接但 SSID 尚不可读"
			return result
		}
		if networkType == "wifi" && ((module.WiFi.Mode == "whitelist" && !slices.Contains(module.WiFi.Whitelist, ssid)) || (module.WiFi.Mode == "blacklist" && slices.Contains(module.WiFi.Blacklist, ssid))) ||
			networkType == "not_wifi" && !module.WiFi.ProxyOnNonWiFi {
			result.Target = "bypassed"
			result.DesiredMode = "Direct"
		}
	}
	if result.DesiredMode == "Direct" {
		result.Target = "bypassed"
	}
	return result
}

func applyConfiguredNetwork(ctx context.Context, options Options, reload func(context.Context, Options) error) (NetworkEvaluation, error) {
	result, err := configuredNetwork(ctx, options)
	if err != nil {
		return result, err
	}
	if result.Enabled && result.Target == "" {
		if service.ProcessRunning(options.SingBoxPath) {
			result.RuntimeMode, err = service.ReadRuntimeMode(ctx, networkControlOptions(options))
		}
		return result, err
	}
	modes, err := moduleconfig.LoadModes(paths.SingBoxConfig(options.SingBoxDir))
	if err != nil {
		return result, err
	}
	return applyNetworkPolicy(ctx, options, modes, result, service.ProcessRunning(options.SingBoxPath), reload)
}

func applyNetworkPolicy(ctx context.Context, options Options, modes moduleconfig.Modes, result NetworkEvaluation, running bool, reload func(context.Context, Options) error) (NetworkEvaluation, error) {
	if !slices.Contains(modes.Available, result.DesiredMode) {
		return result, &service.Error{Code: "mode.unavailable", Message: "当前网络策略所需模式不在主配置中: " + result.DesiredMode, Data: modes}
	}
	if result.DesiredMode == "Direct" {
		if err := validateDirectRouting(options); err != nil {
			return result, err
		}
	}
	if running {
		// API 失败不能触发重载；只有已确认的运行时参数变化才允许重载。
		runtimeMode, err := service.ReadRuntimeMode(ctx, networkControlOptions(options))
		if err != nil {
			return result, err
		}
		result.RuntimeMode = runtimeMode
		config, err := inbound.Load(options.InboundConfig)
		if err != nil {
			return result, err
		}
		config, err = config.WithDNSBypass(result.DesiredMode == "Direct")
		if err != nil {
			return result, err
		}
		wanted, err := config.DNSState()
		if err != nil {
			return result, err
		}
		content, err := os.ReadFile(filepath.Join(options.RuntimeDir, "inbound.json"))
		if err != nil {
			return result, err
		}
		current, err := inbound.RuntimeDNSState(content)
		if err != nil {
			return result, err
		}
		if current != wanted {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			options.networkEvaluation = &result
			logService(options, "INFO", "network.dns", "started", "DNS 接管参数变化，重新加载 sing-box")
			// 新事件只能排队，不能取消已经开始的入站重载收尾。
			applyContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*serviceReadyTimeout+2*serviceStopTimeout+10*time.Second)
			defer cancel()
			if err := reload(applyContext, options); err != nil {
				return result, err
			}
			result.Changed = true
			ctx = applyContext
		}
	}
	return applyNetworkMode(ctx, options, modes, result, running)
}

func reloadNetworkConfig(ctx context.Context, options Options) error {
	transaction, err := beginConfigApply(options, options.ModuleConfig)
	if err != nil {
		return err
	}
	transaction.journal.Mode = options.networkEvaluation.RuntimeMode
	if err := transaction.setPhase("reload_started"); err != nil {
		return errors.Join(err, transaction.rollback())
	}
	if err := reloadAppliedConfig(ctx, options); err != nil {
		return rollbackConfigApply(options, transaction, err)
	}
	if err := transaction.commit(); err != nil {
		return rollbackConfigApply(options, transaction, err)
	}
	return nil
}

func validateDirectRouting(options Options) error {
	content, err := os.ReadFile(paths.SingBoxConfig(options.SingBoxDir))
	if err != nil {
		return err
	}
	var document struct {
		Route struct {
			Rules []map[string]jsontext.Value `json:"rules"`
		} `json:"route"`
		Outbounds []struct {
			Type string `json:"type"`
			Tag  string `json:"tag"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(content, &document); err != nil {
		return err
	}
	if len(document.Route.Rules) > 0 {
		rule := document.Route.Rules[0]
		var mode, action, outbound string
		_ = json.Unmarshal(rule["clash_mode"], &mode)
		_ = json.Unmarshal(rule["action"], &action)
		_ = json.Unmarshal(rule["outbound"], &outbound)
		if len(rule) == 3 && mode == "Direct" && action == "route" {
			if outbound == "direct" {
				return nil
			}
			for _, entry := range document.Outbounds {
				if entry.Type == "direct" && entry.Tag == outbound {
					return nil
				}
			}
		}
	}
	return &service.Error{Code: "network.direct_route_required", Message: "直连模式要求 route.rules 首条为 Direct 路由，并指向 direct 类型出站"}
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
	if previous != result.Target {
		if err := writeWiFiState(options.WiFiStateFile, result.Target); err != nil {
			return result, err
		}
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
