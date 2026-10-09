package module

import (
	"context"
	"encoding/binary"
	json "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestNetworkPolicyIndependentLists(t *testing.T) {
	module := moduleconfig.DefaultModule()
	module.WiFi.Blacklist = []string{"Home, Wi-Fi"}
	module.WiFi.Whitelist = []string{"Office"}
	for _, test := range []struct {
		mode, network, ssid, base, want string
		enabled, nonWiFi                bool
	}{
		{"blacklist", "wifi", "Home, Wi-Fi", "Rule", "Rule", false, false},
		{"whitelist", "wifi", "Home, Wi-Fi", "Rule", "Rule", false, false},
		{"whitelist", "not_wifi", "", "Rule", "Rule", false, false},
		{"blacklist", "wifi", "Home, Wi-Fi", "Rule", "Direct", true, true},
		{"blacklist", "wifi", "home, Wi-Fi", "Rule", "Rule", true, true},
		{"blacklist", "wifi", " Home, Wi-Fi", "Rule", "Rule", true, true},
		{"whitelist", "wifi", "Office", "Rule", "Rule", true, true},
		{"whitelist", "wifi", "Home, Wi-Fi", "Rule", "Direct", true, true},
		{"whitelist", "not_wifi", "", "Rule", "Direct", true, false},
		{"blacklist", "not_wifi", "", "Rule", "Rule", true, true},
		{"whitelist", "wifi", "Office", "Direct", "Direct", false, true},
	} {
		module.WiFi.Enabled = test.enabled
		module.WiFi.Mode = test.mode
		module.WiFi.ProxyOnNonWiFi = test.nonWiFi
		got := networkPolicy(module, test.base, test.network, test.ssid)
		if got.DesiredMode != test.want || got.Enabled != test.enabled {
			t.Fatalf("%+v -> %+v", test, got)
		}
	}
	module.WiFi.Blacklist = nil
	module.WiFi.Whitelist = nil
	module.WiFi.Enabled = true
	module.WiFi.Mode = "blacklist"
	if networkPolicy(module, "Rule", "wifi", "Home").DesiredMode != "Rule" {
		t.Fatal("空黑名单绕过了所有网络")
	}
	module.WiFi.Mode = "whitelist"
	if networkPolicy(module, "Rule", "wifi", "Home").DesiredMode != "Direct" {
		t.Fatal("空白名单未绕过")
	}
	if got := networkPolicy(module, "Rule", "wifi", ""); got.Target != "" {
		t.Fatalf("未知 SSID 参与了匹配: %+v", got)
	}
}

func TestConfiguredNetworkUnknownAndFrozenRecovery(t *testing.T) {
	options := newTestOptions(t.TempDir())
	writeModeConfig(t, options, "Rule")
	targetCoreWrite(t, options.ModuleConfig, []byte("{\"wifi\":{\"enabled\":true,\"mode\":\"blacklist\"}}"))
	reads := 0
	options.NetworkStateReader = func(context.Context) (worker.NetworkState, error) {
		reads++
		return worker.NetworkState{NetworkType: "unknown"}, nil
	}
	if _, err := configuredNetwork(t.Context(), options); err == nil {
		t.Fatal("未知类型被当作非 Wi-Fi")
	}
	options.networkEvaluation = prepareFromConfigJournal(options, configApplyJournal{Mode: "Direct"}).Network
	result, err := configuredNetwork(t.Context(), options)
	if err != nil || result.DesiredMode != "Direct" || result.Target != "bypassed" || reads != 1 {
		t.Fatalf("恢复未复用快照模式: %+v %d %v", result, reads, err)
	}
}

func TestConfiguredNetworkDisabledSkipsReader(t *testing.T) {
	options := newTestOptions(t.TempDir())
	writeModeConfig(t, options, "Rule")
	targetCoreWrite(t, options.ModuleConfig, []byte("{\"wifi\":{\"enabled\":false,\"mode\":\"whitelist\",\"proxy_on_non_wifi\":false}}"))
	options.NetworkStateReader = func(context.Context) (worker.NetworkState, error) {
		t.Fatal("关闭策略时仍读取网络")
		return worker.NetworkState{}, nil
	}
	got, err := configuredNetwork(t.Context(), options)
	if err != nil || got.Enabled || got.DesiredMode != "Rule" {
		t.Fatalf("关闭后未使用默认模式: %+v %v", got, err)
	}
}

func TestNetworkTargetCoreDNSCheck(t *testing.T) {
	core := targetCoreBinary(t)
	for _, backend := range []string{"ebpf", "tun"} {
		for _, mode := range []string{"Rule", "Direct"} {
			t.Run(backend+"/"+mode, func(t *testing.T) {
				config, err := inbound.Parse([]byte(testInboundConfig))
				if err != nil {
					t.Fatal(err)
				}
				config.Backend = backend
				content, err := json.Marshal(config, json.Deterministic(true))
				if err != nil {
					t.Fatal(err)
				}
				options, _ := targetCoreFixture(t, core, content)
				var document map[string]any
				if err := json.Unmarshal([]byte(targetCoreStaticConfig), &document); err != nil {
					t.Fatal(err)
				}
				document["experimental"].(map[string]any)["clash_api"].(map[string]any)["default_mode"] = mode
				route := document["route"].(map[string]any)
				route["rules"] = append([]any{map[string]any{"clash_mode": "Direct", "action": "route", "outbound": "direct"}}, route["rules"].([]any)...)
				content, err = json.Marshal(document, json.Deterministic(true))
				if err != nil {
					t.Fatal(err)
				}
				targetCoreWrite(t, paths.SingBoxConfig(options.SingBoxDir), content)
				prepared, err := Check(t.Context(), options, false)
				if err != nil {
					t.Fatal(err)
				}
				effective, err := config.WithDNSBypass(mode == "Direct")
				if err != nil {
					t.Fatal(err)
				}
				wanted, err := effective.DNSState()
				if err != nil {
					t.Fatal(err)
				}
				got, err := inbound.RuntimeDNSState(targetCoreRead(t, prepared.Inbound))
				if err != nil || got != wanted {
					t.Fatalf("运行时 DNS 不符: %+v %+v %v", got, wanted, err)
				}
				if prepared.Network == nil || prepared.Network.DesiredMode != mode {
					t.Fatalf("未冻结启动模式: %+v", prepared)
				}
			})
		}
	}
}

func TestNetworkDNSReloadOnlyForParameterChanges(t *testing.T) {
	for _, backend := range []string{"ebpf", "tun"} {
		t.Run(backend, func(t *testing.T) {
			options := newTestOptions(t.TempDir())
			writeModeConfig(t, options, "Rule")
			original, err := inbound.Parse([]byte(testInboundConfig))
			if err != nil {
				t.Fatal(err)
			}
			original.Backend = backend
			if err := os.MkdirAll(filepath.Dir(options.InboundConfig), 0700); err != nil {
				t.Fatal(err)
			}
			saved, _ := json.Marshal(original, json.Deterministic(true))
			targetCoreWrite(t, options.InboundConfig, saved)
			writeRuntime := func(c inbound.Config) {
				t.Helper()
				if _, err := inbound.WriteAtomic(t.Context(), filepath.Join(options.RuntimeDir, "inbound.json"), c); err != nil {
					t.Fatal(err)
				}
			}
			writeRuntime(original)
			var actual atomic.Value
			actual.Store("Rule")
			var failAPI atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if failAPI.Load() {
					http.Error(w, "unavailable", 503)
					return
				}
				var payload []byte
				switch r.URL.Path {
				case "/daemon.StartedService/GetClashModeStatus":
					payload = protowire.AppendTag(payload, 2, protowire.BytesType)
					payload = protowire.AppendString(payload, actual.Load().(string))
				case "/daemon.StartedService/SetClashMode":
					body, _ := io.ReadAll(r.Body)
					if len(body) < 6 {
						t.Error("无效模式请求")
						return
					}
					_, _, n := protowire.ConsumeTag(body[5:])
					mode, _ := protowire.ConsumeString(body[5+n:])
					actual.Store(mode)
				}
				header := make([]byte, 5)
				binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
				w.Write(append(header, payload...))
			}))
			defer server.Close()
			options.ServiceAddress = server.URL
			modes, _ := moduleconfig.LoadModes(filepath.Join(options.SingBoxDir, "config.json"))
			reloads := 0
			failReload := false
			var cancelDuringReload context.CancelFunc
			reload := func(ctx context.Context, frozen Options) error {
				reloads++
				if frozen.networkEvaluation == nil {
					t.Fatal("重载没有冻结策略")
				}
				if failReload {
					return errors.New("reload failed")
				}
				if cancelDuringReload != nil {
					cancelDuringReload()
				}
				effective, err := original.WithDNSBypass(frozen.networkEvaluation.DesiredMode == "Direct")
				if err != nil {
					return err
				}
				writeRuntime(effective)
				actual.Store(frozen.networkEvaluation.DesiredMode)
				return ctx.Err()
			}
			plan := NetworkEvaluation{Enabled: true, DesiredMode: "Direct", Target: "bypassed"}
			if _, err := applyNetworkPolicy(t.Context(), options, modes, plan, true, reload); err != nil {
				t.Fatal(err)
			}
			if _, err := applyNetworkPolicy(t.Context(), options, modes, plan, true, reload); err != nil || reloads != 1 {
				t.Fatalf("相同 DNS 重复重载: %d %v", reloads, err)
			}
			plan.DesiredMode = "Rule"
			plan.Target = "proxying"
			if _, err := applyNetworkPolicy(t.Context(), options, modes, plan, true, reload); err != nil || reloads != 2 {
				t.Fatalf("DNS 未恢复: %d %v", reloads, err)
			}
			actual.Store("Direct")
			if _, err := applyNetworkPolicy(t.Context(), options, modes, plan, true, reload); err != nil || actual.Load() != "Rule" || reloads != 2 {
				t.Fatalf("API 模式校准触发了重载: %d %s %v", reloads, actual.Load(), err)
			}
			failAPI.Store(true)
			plan.DesiredMode = "Direct"
			if _, err := applyNetworkPolicy(t.Context(), options, modes, plan, true, reload); err == nil || reloads != 2 {
				t.Fatalf("API 失败重载兜底: %d %v", reloads, err)
			}
			failAPI.Store(false)
			failReload = true
			result, err := applyNetworkPolicy(t.Context(), options, modes, plan, true, reload)
			if err == nil || result.Changed || readWiFiState(options.WiFiStateFile) == "bypassed" {
				t.Fatalf("失败被标记为已绕过: %+v %v", result, err)
			}
			failReload = false
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			cancelDuringReload = cancel
			result, err = applyNetworkPolicy(ctx, options, modes, plan, true, reload)
			if err != nil || !result.Changed || ctx.Err() == nil || actual.Load() != "Direct" {
				t.Fatalf("开始后的取消中断了重载收尾: %+v %v", result, err)
			}
			persisted, _ := os.ReadFile(options.InboundConfig)
			if string(persisted) != string(saved) {
				t.Fatal("改写了保存偏好")
			}
		})
	}
}

func TestEvaluateNetworkPreservesDefaultAndPersistsPolicyState(t *testing.T) {
	root := t.TempDir()
	modulePath := filepath.Join(root, "module.json")
	statePath := filepath.Join(root, "wifi_state")
	content := "{\"auto_start\":true,\"selection\":{\"group_id\":\"default\",\"node_tag\":\"\"},\"wifi\":{\"blacklist\":[\"办公 WiFi\",\"家庭 WiFi\"],\"enabled\":true,\"mode\":\"blacklist\",\"proxy_on_non_wifi\":true}}"
	if err := os.WriteFile(modulePath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	options := newTestOptions(root)
	writeModeConfig(t, options, "Rule")
	options.ModuleConfig = modulePath
	options.WiFiStateFile = statePath
	options.SingBoxPath = filepath.Join(root, "missing-sing-box")

	result, err := EvaluateNetwork(context.Background(), options, "wifi", "家庭 WiFi")
	if err != nil {
		t.Fatal(err)
	}
	if result.Target != "bypassed" || result.DesiredMode != "Direct" || !result.Changed {
		t.Fatalf("黑名单网络评估错误: %+v", result)
	}
	if value, _ := os.ReadFile(statePath); string(value) != "bypassed\n" {
		t.Fatalf("WiFi 状态 = %q", value)
	}
	logContent, err := os.ReadFile(filepath.Join(options.LogDir, "service.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logContent), "network_type=wifi") || !strings.Contains(string(logContent), "target=bypassed") {
		t.Fatalf("网络策略日志缺少网络类型或目标模式: %s", logContent)
	}

	result, err = EvaluateNetwork(context.Background(), options, "wifi", "移动热点")
	if err != nil {
		t.Fatal(err)
	}
	if result.Target != "proxying" || result.DesiredMode != "Rule" {
		t.Fatalf("代理网络评估错误: %+v", result)
	}

	result, err = EvaluateNetwork(context.Background(), options, "wifi", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Target != "" || result.Reason != "WiFi 已连接但 SSID 尚不可读" {
		t.Fatalf("未知 SSID 不应切换: %+v", result)
	}
}

func TestEvaluateNetworkClearsDisabledOverride(t *testing.T) {
	root := t.TempDir()
	modulePath := filepath.Join(root, "module.json")
	statePath := filepath.Join(root, "wifi_state")
	content := "{\"auto_start\":true,\"selection\":{\"group_id\":\"default\",\"node_tag\":\"\"},\"wifi\":{\"enabled\":false,\"mode\":\"whitelist\",\"proxy_on_non_wifi\":true}}"
	if err := os.WriteFile(modulePath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("bypassed\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	options := newTestOptions(root)
	writeModeConfig(t, options, "Rule")
	options.ModuleConfig = modulePath
	options.WiFiStateFile = statePath
	options.SingBoxPath = filepath.Join(root, "missing-sing-box")
	result, err := EvaluateNetwork(context.Background(), options, "not_wifi", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Enabled || result.Changed || result.Reason != "WiFi 自动切换未启用" {
		t.Fatalf("关闭自动切换时不应误报运行时变更: %+v", result)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("WiFi 临时状态未清理: %v", err)
	}
}
