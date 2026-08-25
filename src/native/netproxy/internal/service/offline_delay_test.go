package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"encoding/json/jsontext"
	json "encoding/json/v2"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
)

func TestOfflineDelayConfigIsIsolatedAndUsesProviderSnapshot(t *testing.T) {
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	request, err := resolveDelayRequest(context.Background(), Options{
		CatalogRoot: catalogRoot, ModuleConfig: moduleConfig,
	}, "default/NODE", "")
	if err != nil {
		t.Fatal(err)
	}
	document, err := offlineDelayProvider(context.Background(), catalogRoot, request)
	if err != nil {
		t.Fatal(err)
	}
	sessionDir := t.TempDir()
	configPath, err := writeOfflineDelayConfig(context.Background(), sessionDir, request, document, 19090, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]jsontext.Value
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"inbounds", "experimental", "endpoints"} {
		if _, exists := config[forbidden]; exists {
			t.Fatalf("离线测速配置不应包含 %s: %s", forbidden, content)
		}
	}
	var providers []map[string]any
	if err := json.Unmarshal(config["providers"], &providers); err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0]["type"] != "local" || providers[0]["tag"] != "本地配置" {
		t.Fatalf("临时 Provider 配置异常: %#v", providers)
	}
	if _, exists := providers[0]["health_check"]; exists {
		t.Fatalf("临时 Provider 不应与 Auto 分组重复测速: %#v", providers[0])
	}
	providerPath, _ := providers[0]["path"].(string)
	providerContent, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(providerContent), `"tag": "NODE"`) || strings.Contains(string(providerContent), "DROP") {
		t.Fatalf("单节点测速未生成最小 Provider 快照: %s", providerContent)
	}
	var services []map[string]any
	if err := json.Unmarshal(config["services"], &services); err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 || services[0]["listen"] != "127.0.0.1" || services[0]["secret"] != "test-secret" {
		t.Fatalf("临时 Service API 配置异常: %#v", services)
	}
	var dnsOptions struct {
		Servers []struct {
			Type string `json:"type"`
			Tag  string `json:"tag"`
		} `json:"servers"`
		Final string `json:"final"`
	}
	if err := json.Unmarshal(config["dns"], &dnsOptions); err != nil {
		t.Fatal(err)
	}
	if dnsOptions.Final != "dns-direct" {
		t.Fatalf("临时 DNS 兜底异常: %#v", dnsOptions)
	}
	serverTypes := make(map[string]string, len(dnsOptions.Servers))
	for _, server := range dnsOptions.Servers {
		serverTypes[server.Tag] = server.Type
		if server.Type == "local" {
			t.Fatalf("Android CLI 不应使用依赖本机 resolv.conf 的 local DNS: %#v", dnsOptions.Servers)
		}
	}
	if serverTypes["dns-hosts"] != "hosts" || serverTypes["dns-direct"] != "group" ||
		serverTypes["dns-ali"] != "https" || serverTypes["dns-tencent"] != "https" {
		t.Fatalf("临时直连 DNS 配置不完整: %#v", dnsOptions.Servers)
	}
}

func TestOfflineDelayCancellationStopsProcessAndRemovesSession(t *testing.T) {
	if os.Getenv("NETPROXY_OFFLINE_DELAY_HELPER") == "1" {
		time.Sleep(30 * time.Second)
		return
	}
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	request, err := resolveDelayRequest(context.Background(), Options{
		CatalogRoot: catalogRoot, ModuleConfig: moduleConfig,
	}, "auto", "default")
	if err != nil {
		t.Fatal(err)
	}
	originalCommand := offlineDelayCommand
	offlineDelayCommand = func(ctx context.Context, _, _, workingDir string, output *os.File) *exec.Cmd {
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=TestOfflineDelayCancellationStopsProcessAndRemovesSession")
		command.Dir = workingDir
		command.Env = append(os.Environ(), "NETPROXY_OFFLINE_DELAY_HELPER=1")
		command.Stdout = output
		command.Stderr = output
		return command
	}
	defer func() { offlineDelayCommand = originalCommand }()
	delayDir := filepath.Join(t.TempDir(), "delay")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err = runOfflineDelay(ctx, Options{
		CatalogRoot: catalogRoot, ModuleConfig: moduleConfig, DelayDir: delayDir,
		SingBoxPath: os.Args[0], RequestTimeout: time.Second,
	}, request)
	var structured *Error
	if !errors.As(err, &structured) || structured.Code != "node.delay_timeout" {
		t.Fatalf("取消离线测速未返回超时错误: %v", err)
	}
	entries, err := os.ReadDir(delayDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "session-") {
			t.Fatalf("取消后遗留测速会话目录: %s", entry.Name())
		}
	}
}

func TestOfflineDelayWithRealSingBox(t *testing.T) {
	singBoxPath := os.Getenv("NETPROXY_TEST_SING_BOX")
	if singBoxPath == "" {
		t.Skip("未提供 NETPROXY_TEST_SING_BOX")
	}
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	request, err := resolveDelayRequest(context.Background(), Options{
		CatalogRoot: catalogRoot, ModuleConfig: moduleConfig,
	}, "auto", "default")
	if err != nil {
		t.Fatal(err)
	}
	delayDir := filepath.Join(t.TempDir(), "delay")
	result, err := runOfflineDelay(context.Background(), Options{
		CatalogRoot: catalogRoot, ModuleConfig: moduleConfig, DelayDir: delayDir,
		SingBoxPath: singBoxPath, RequestTimeout: 8 * time.Second,
	}, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Target != "Auto/本地配置" || len(result.Groups) != 1 || len(result.Groups[0].Items) != 1 {
		t.Fatalf("真实 sing-box 离线测速结果异常: %#v", result)
	}
	entries, err := os.ReadDir(delayDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "session-") {
			t.Fatalf("真实离线测速后遗留会话目录: %s", entry.Name())
		}
	}
}

func TestOfflineDelayRejectsConcurrentSession(t *testing.T) {
	catalogRoot, moduleConfig := delayFixtureFiles(t)
	request, err := resolveDelayRequest(context.Background(), Options{
		CatalogRoot: catalogRoot, ModuleConfig: moduleConfig,
	}, "auto", "default")
	if err != nil {
		t.Fatal(err)
	}
	delayDir := filepath.Join(t.TempDir(), "delay")
	lock, err := processlock.TryAcquire(filepath.Join(delayDir, "session.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Release() }()
	_, err = runOfflineDelay(context.Background(), Options{
		CatalogRoot: catalogRoot, ModuleConfig: moduleConfig, DelayDir: delayDir,
		SingBoxPath: os.Args[0], RequestTimeout: time.Second,
	}, request)
	var structured *Error
	if !errors.As(err, &structured) || structured.Code != "node.delay_busy" {
		t.Fatalf("并发测速未返回 busy 错误: %v", err)
	}
}
