package module

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func TestNodeImportAppendsToDefaultGroup(t *testing.T) {
	root := t.TempDir()
	options := newTestOptions(root)
	if err := os.MkdirAll(filepath.Dir(options.ModuleConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("ACTIVE_GROUP_ID=default\nSELECTOR_MODE=urltest\nSELECTED_NODE_REF=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InitializeGroup(context.Background(), catalog.GroupOptions{
		Root: options.CatalogRoot, GroupID: "default", Name: "已有本地配置", Type: "local",
	}); err != nil {
		t.Fatalf("initialize default group: %v", err)
	}
	if _, err := catalog.AppendNode(context.Background(), catalog.MutationOptions{
		GroupDir: filepath.Join(options.CatalogRoot, "default"), GroupID: "default",
		Name: "已有本地配置", Type: "local", Input: "socks://existing.example:1080#EXISTING",
	}); err != nil {
		t.Fatalf("append existing node: %v", err)
	}
	input := filepath.Join(root, "selected-nodes.yaml")
	if err := os.WriteFile(input, []byte("socks://one.example:1081#IMPORTED\nsocks://two.example:1082#IMPORTED_TWO\n"), 0o600); err != nil {
		t.Fatalf("write node file: %v", err)
	}

	result, err := NodeImport(context.Background(), options, input, false)
	if err != nil {
		t.Fatalf("import nodes: %v", err)
	}
	if result.GroupID != "default" || result.NodeCount != 3 || result.Revision != 2 {
		t.Fatalf("unexpected import result: %+v", result)
	}
	ids, err := catalog.GroupIDs(options.CatalogRoot, "all")
	if err != nil {
		t.Fatalf("list catalog groups: %v", err)
	}
	if len(ids) != 1 || ids[0] != "default" {
		t.Fatalf("unexpected groups after import: %v", ids)
	}
	document, err := provider.Load(context.Background(), filepath.Join(options.CatalogRoot, "default", "provider.json"))
	if err != nil {
		t.Fatalf("load default provider: %v", err)
	}
	if got := len(provider.Inspect(document)); got != 3 {
		t.Fatalf("default node count = %d, want 3", got)
	}
	metadata, err := catalog.LoadMetadata(filepath.Join(options.CatalogRoot, "default", "meta.json"), "default")
	if err != nil {
		t.Fatalf("load default metadata: %v", err)
	}
	if metadata.Name != "已有本地配置" {
		t.Fatalf("default group name changed unexpectedly: %q", metadata.Name)
	}
}

func TestLoadAppPolicyReturnsTypedSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ebpf.conf")
	content := "APP_PROXY_ENABLE=1\nAPP_PROXY_MODE=\"whitelist\"\nPROXY_APPS_LIST=\"0:com.example.one,10:com.example.two\"\nBYPASS_APPS_LIST=\"0:com.example.three\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	policy, err := LoadAppPolicy(path)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Enabled || policy.Mode != "whitelist" || policy.ProxyApps != "0:com.example.one,10:com.example.two" || policy.BypassApps != "0:com.example.three" {
		t.Fatalf("unexpected app policy: %+v", policy)
	}
}

func TestUpdateAllSubscriptionsPreservesStructuredFailure(t *testing.T) {
	root := t.TempDir()
	options := newTestOptions(root)
	if err := os.MkdirAll(filepath.Dir(options.ModuleConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("ACTIVE_GROUP_ID=default\nSELECTOR_MODE=urltest\nOUTBOUND_MODE=rule\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InitializeGroup(context.Background(), catalog.GroupOptions{
		Root: options.CatalogRoot, GroupID: "failed-subscription", Name: "failed-subscription",
		Type: "subscription", URL: "http://127.0.0.1:1", AutoUpdate: true,
		UpdateInterval: 900, UpdateViaProxy: "never", Timeout: 1,
	}); err != nil {
		t.Fatal(err)
	}

	summary, err := UpdateAllSubscriptions(context.Background(), options)
	if err == nil {
		t.Fatal("update-all hid the subscription failure")
	}
	if len(summary.Failed) != 1 || summary.Failed[0] != "failed-subscription" {
		t.Fatalf("unexpected update-all summary: %+v", summary)
	}
	var subscriptionErr *subscription.Error
	if !errors.As(err, &subscriptionErr) || subscriptionErr.Code != "subscription.convert_failed" {
		t.Fatalf("update-all did not preserve the structured error: %v", err)
	}
}

func TestEditSubscriptionFailureReportsPersistedSettings(t *testing.T) {
	root := t.TempDir()
	options := newTestOptions(root)
	now := time.Unix(1_700_450_500, 0)
	if err := catalog.InitializeGroup(context.Background(), catalog.GroupOptions{
		Root: options.CatalogRoot, GroupID: "edit-failure", Name: "edit-failure",
		Type: "subscription", URL: "https://example.invalid/sub", AutoUpdate: true,
		UpdateInterval: 900, UpdateViaProxy: "never", Timeout: 1,
	}); err != nil {
		t.Fatal(err)
	}
	badURL := "http://127.0.0.1:1/sub"
	result, err := EditSubscription(context.Background(), options, "edit-failure", subscription.EditOptions{
		URL: &badURL, Now: now,
	})
	if err == nil {
		t.Fatal("failed subscription edit was reported as success")
	}
	if !result.Persisted {
		t.Fatalf("edited settings were reported as not persisted: %+v", result)
	}
	var subscriptionErr *subscription.Error
	if !errors.As(err, &subscriptionErr) || subscriptionErr.Code != "subscription.convert_failed" {
		t.Fatalf("unexpected edit failure: %v", err)
	}
	data, ok := subscriptionErr.Data.(map[string]any)
	if !ok || data["persisted"] != true {
		t.Fatalf("structured error lost persisted=true: %#v", subscriptionErr.Data)
	}
	metadata, err := catalog.LoadMetadata(filepath.Join(options.CatalogRoot, "edit-failure", "meta.json"), "edit-failure")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.URL != badURL {
		t.Fatalf("edited URL was not retained after download failure: %q", metadata.URL)
	}
}

func TestAddSubscriptionCancellationReportsPersistedGroup(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()

	root := t.TempDir()
	options := newTestOptions(root)
	ctx, cancel := context.WithCancel(context.Background())
	type outcome struct {
		result subscription.Result
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := AddSubscription(ctx, SubscriptionOptions{
			Options: options, Name: "cancelled-add", URL: server.URL,
			AutoUpdate: true, UpdateInterval: 900, UpdateViaProxy: "never", Timeout: 60,
		})
		finished <- outcome{result: result, err: err}
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("subscription request did not start")
	}
	result := <-finished
	if result.err == nil || !result.result.Persisted {
		t.Fatalf("cancelled add did not report its persisted group: result=%+v err=%v", result.result, result.err)
	}
	var subscriptionErr *subscription.Error
	if !errors.As(result.err, &subscriptionErr) {
		t.Fatalf("cancelled add lost its structured error: %v", result.err)
	}
	data, ok := subscriptionErr.Data.(map[string]any)
	if !ok || data["persisted"] != true {
		t.Fatalf("cancelled add lost persisted=true: %#v", subscriptionErr.Data)
	}
	groups, err := catalog.GroupIDs(options.CatalogRoot, "subscription")
	if err != nil || len(groups) != 1 {
		t.Fatalf("persisted subscription group = %v, err=%v", groups, err)
	}
}

func TestEditSubscriptionSchedulingOnlyDoesNotReload(t *testing.T) {
	root := t.TempDir()
	options := newTestOptions(root)
	if err := os.MkdirAll(filepath.Dir(options.ModuleConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("ACTIVE_GROUP_ID=default\nSELECTOR_MODE=urltest\nOUTBOUND_MODE=rule\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_450_000, 0)
	if err := catalog.InitializeGroup(context.Background(), catalog.GroupOptions{
		Root: options.CatalogRoot, GroupID: "schedule-only", Name: "schedule-only",
		Type: "subscription", URL: "https://example.invalid/sub", AutoUpdate: true,
		UpdateInterval: 900, UpdateViaProxy: "never", Timeout: 60,
	}); err != nil {
		t.Fatal(err)
	}
	interval := int64(1800)
	result, err := EditSubscription(context.Background(), options, "schedule-only", subscription.EditOptions{
		UpdateInterval: &interval, Now: now,
	})
	if err != nil {
		t.Fatalf("编辑调度字段失败: %v", err)
	}
	if !result.Persisted || result.RequiresUpdate || result.RuntimeSyncState != subscription.RuntimeSyncNotRunning {
		t.Fatalf("调度字段编辑结果异常: %+v", result)
	}
	metadata, err := catalog.LoadMetadata(filepath.Join(options.CatalogRoot, "schedule-only", "meta.json"), "schedule-only")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.UpdateInterval != interval || metadata.NextUpdateEpoch == 0 {
		t.Fatalf("调度字段未正确持久化: %+v", metadata)
	}
}

func TestEditSubscriptionHistoryFailureKeepsProviderAndMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"edited-provider","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	root := t.TempDir()
	options := newTestOptions(root)
	if err := os.MkdirAll(filepath.Dir(options.ModuleConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("ACTIVE_GROUP_ID=default\nSELECTOR_MODE=urltest\nOUTBOUND_MODE=rule\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InitializeGroup(context.Background(), catalog.GroupOptions{
		Root: options.CatalogRoot, GroupID: "history-edit", Name: "history-edit",
		Type: "subscription", URL: "https://old.example/sub", AutoUpdate: true,
		UpdateInterval: 900, UpdateViaProxy: "never", Timeout: 60,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(options.CatalogRoot, "history-edit", "history.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_451_000, 0)
	newURL := server.URL
	result, err := EditSubscription(context.Background(), options, "history-edit", subscription.EditOptions{
		URL: &newURL, Now: now,
	})
	if err == nil {
		t.Fatal("历史写入失败时不应返回普通成功")
	}
	var subscriptionErr *subscription.Error
	if !errors.As(err, &subscriptionErr) || subscriptionErr.Code != "subscription.history_write_failed" {
		t.Fatalf("未返回结构化历史错误: %v", err)
	}
	if !result.Persisted {
		t.Fatalf("历史写入失败不应伪装成未保存: %+v", result)
	}
	metadata, err := catalog.LoadMetadata(filepath.Join(options.CatalogRoot, "history-edit", "meta.json"), "history-edit")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.URL != server.URL {
		t.Fatalf("编辑后的 URL 未保留: %q", metadata.URL)
	}
	if _, err := provider.Load(context.Background(), filepath.Join(options.CatalogRoot, "history-edit", "provider.json")); err != nil {
		t.Fatalf("Provider 未保留为完整可读文件: %v", err)
	}
	if !strings.Contains(subscriptionErr.Error(), "订阅历史写入失败") {
		t.Fatalf("历史错误消息不明确: %v", subscriptionErr)
	}
}
