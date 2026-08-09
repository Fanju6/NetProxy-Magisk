package module

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestContainsSSID(t *testing.T) {
	if !containsSSID("办公 WiFi，家庭 WiFi", "家庭 WiFi") {
		t.Fatal("全角逗号分隔的 SSID 应该可以命中")
	}
	if containsSSID("办公 WiFi,家庭 WiFi", "访客 WiFi") {
		t.Fatal("不存在的 SSID 不应该命中")
	}
}

func TestEvaluateNetworkPersistsModeAndState(t *testing.T) {
	root := t.TempDir()
	modulePath := filepath.Join(root, "module.conf")
	statePath := filepath.Join(root, "wifi_state")
	content := `AUTO_START=1
OUTBOUND_MODE=rule
SELECTOR_MODE=urltest
ACTIVE_GROUP_ID=default
SELECTED_NODE_REF=""
WIFI_AUTO_SWITCH=1
WIFI_SSID_MODE=blacklist
WIFI_SSID_LIST="办公 WiFi，家庭 WiFi"
PROXY_ON_CELLULAR=1
`
	if err := os.WriteFile(modulePath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	options := NewOptions(root)
	options.ModuleConfig = modulePath
	options.WiFiStateFile = statePath
	options.SingBoxPath = filepath.Join(root, "missing-sing-box")

	result, err := EvaluateNetwork(context.Background(), options, "wifi", "家庭 WiFi")
	if err != nil {
		t.Fatal(err)
	}
	if result.Target != "bypassed" || result.DesiredMode != "direct" || !result.Changed {
		t.Fatalf("黑名单网络评估错误: %+v", result)
	}
	if value, _ := os.ReadFile(statePath); string(value) != "bypassed\n" {
		t.Fatalf("WiFi 状态 = %q", value)
	}

	result, err = EvaluateNetwork(context.Background(), options, "wifi", "移动热点")
	if err != nil {
		t.Fatal(err)
	}
	if result.Target != "proxying" || result.DesiredMode != "rule" {
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
	modulePath := filepath.Join(root, "module.conf")
	statePath := filepath.Join(root, "wifi_state")
	content := `AUTO_START=1
OUTBOUND_MODE=rule
SELECTOR_MODE=urltest
ACTIVE_GROUP_ID=default
SELECTED_NODE_REF=""
WIFI_AUTO_SWITCH=0
WIFI_SSID_MODE=blacklist
WIFI_SSID_LIST=""
PROXY_ON_CELLULAR=1
`
	if err := os.WriteFile(modulePath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("bypassed\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	options := NewOptions(root)
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
