package httpapi

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func resourceSubscriptionFixture(t *testing.T, server *Server, id, source string) {
	t.Helper()
	if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{
		Root: server.options.CatalogRoot, GroupID: id, Name: "匿名订阅", Type: "subscription", URL: source,
		CustomHeaders: map[string]string{"X-Test-Private": "private-header-value"}, AutoUpdate: false, Timeout: 7, UpdateViaProxy: "never",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestResourcesSubscriptionSummariesPrivateSettingsAndProgress(t *testing.T) {
	server := resourceFixture(t)
	resourceSubscriptionFixture(t, server, "subscription-test", "https://example.test/sub?token=private-test-token")
	var summaries []catalog.GroupSummary
	response, result := resourceRequest(t, server, "GET", "/api/v1/subscriptions", "")
	if response.Code != http.StatusOK || result.Code != "subscription.list" {
		t.Fatalf("订阅列表失败: %s", response.Body.String())
	}
	if err := json.Unmarshal(result.Data, &summaries); err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(summaries, func(group catalog.GroupSummary) bool { return group.ID == "subscription-test" })
	if len(summaries) != 2 || index < 0 || summaries[index].Name != "匿名订阅" {
		t.Fatalf("摘要不一致: %+v", summaries)
	}
	for _, path := range []string{"/api/v1/subscriptions", "/api/v1/subscriptions/subscription-test"} {
		response, _ := resourceRequest(t, server, "GET", path, "")
		if strings.Contains(response.Body.String(), "private-test-token") || strings.Contains(response.Body.String(), "private-header-value") {
			t.Fatalf("普通订阅读取泄露私有设置: %s", response.Body.String())
		}
	}
	var metadata catalog.Metadata
	resourceAssertSuccess(t, server, "GET", "/api/v1/subscriptions/subscription-test?private=true", "", "subscription.show", &metadata)
	if metadata.CustomHeaders["X-Test-Private"] != "private-header-value" || !strings.Contains(metadata.URL, "private-test-token") {
		t.Fatalf("私有读取丢失字段: %+v", metadata)
	}
	resourceAssertSuccess(t, server, "PUT", "/api/v1/subscriptions/subscription-test", `{"auto_update":false,"update_interval":3600}`, "subscription.edited", nil)
	resourceAssertSuccess(t, server, "GET", "/api/v1/subscriptions/subscription-test?private=true", "", "subscription.show", &metadata)
	if metadata.AutoUpdate || metadata.UpdateInterval != 3600 || metadata.Timeout != 7 || metadata.CustomHeaders["X-Test-Private"] != "private-header-value" {
		t.Fatalf("订阅编辑覆盖了未指定字段: %+v", metadata)
	}
	var progress struct {
		ID       string         `json:"id"`
		Progress jsontext.Value `json:"progress"`
	}
	resourceAssertSuccess(t, server, "GET", "/api/v1/subscriptions/subscription-test/progress", "", "subscription.progress", &progress)
	if progress.ID != "subscription-test" || string(progress.Progress) != "null" {
		t.Fatalf("空闲时进度错误: %+v", progress)
	}
	resourceWriteFile(t, filepath.Join(server.options.ProgressDir, "subscription-test.progress.json"), `{"stage":"download","message":"正在下载订阅"}`)
	resourceAssertSuccess(t, server, "GET", "/api/v1/subscriptions/subscription-test/progress", "", "subscription.progress", &progress)
	if !strings.Contains(string(progress.Progress), "download") {
		t.Fatalf("运行时进度丢失: %+v", progress)
	}
	resourceWriteFile(t, filepath.Join(server.options.ProgressDir, "subscription-test.progress.json"), `{"stage":"complete"}`)
	resourceAssertSuccess(t, server, "GET", "/api/v1/subscriptions/subscription-test/progress", "", "subscription.progress", &progress)
	if string(progress.Progress) != "null" {
		t.Fatalf("完成后仍展示旧进度: %+v", progress)
	}
	resourceAssertSuccess(t, server, "GET", "/api/v1/subscriptions/subscription-test/history", "", "subscription.history", nil)
	resourceAssertSuccess(t, server, "POST", "/api/v1/subscriptions/subscription-test/cancel", "", "subscription.cancelled", nil)
	resourceAssertSuccess(t, server, "DELETE", "/api/v1/subscriptions/subscription-test", "", "subscription.removed", nil)
	response, _ = resourceRequest(t, server, "GET", "/api/v1/subscriptions/subscription-test?private=true", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("删除后订阅仍存在: %s", response.Body.String())
	}
}

func TestResourcesSubscriptionAddAndUpdatesPreserveDomainResult(t *testing.T) {
	server := resourceFixture(t)
	var calls atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("User-Agent") != "sing-box" {
			t.Error("默认 User-Agent 改变")
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("socks://127.0.0.1:1084#SUBSCRIBED_NODE\n"))
	}))
	defer source.Close()
	var added subscription.Result
	resourceAssertSuccess(t, server, "POST", "/api/v1/subscriptions", resourceJSON(t, map[string]any{
		"name": "匿名订阅", "url": source.URL, "auto_update": false, "update_via_proxy": "never", "download_timeout": 2,
	}), "subscription.added", &added)
	if added.GroupID == "" || !added.Persisted || added.NodeCount != 1 || added.RuntimeSyncState != subscription.RuntimeSyncNotRunning {
		t.Fatalf("首次更新结果失真: %+v", added)
	}
	var updated subscription.Result
	resourceAssertSuccess(t, server, "POST", "/api/v1/subscriptions/"+added.GroupID+"/update", "", "subscription.updated", &updated)
	if updated.GroupID != added.GroupID || !updated.Persisted || updated.NodeCount != 1 {
		t.Fatalf("单次更新结果错误: %+v", updated)
	}
	resourceAssertSuccess(t, server, "POST", "/api/v1/subscriptions/update-all", "", "subscription.updated_all", nil)
	if calls.Load() != 3 {
		t.Fatalf("更新重复或遗漏: %d", calls.Load())
	}
	var history []jsontext.Value
	resourceAssertSuccess(t, server, "GET", "/api/v1/subscriptions/"+added.GroupID+"/history", "", "subscription.history", &history)
	if len(history) < 3 {
		t.Fatalf("更新历史未复用: %s", history)
	}
	for _, path := range []string{server.options.StateFile, server.options.WorkerPIDFile, server.options.RuntimeDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("停止时订阅操作启动了服务或 Worker: %s %v", path, err)
		}
	}
}

func TestResourcesSubscriptionFailedDownloadReportsPersistedSettings(t *testing.T) {
	server := resourceFixture(t)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer source.Close()
	response, result := resourceRequest(t, server, "POST", "/api/v1/subscriptions", resourceJSON(t, map[string]any{
		"url": source.URL, "name": "匿名失败订阅", "auto_update": false, "update_via_proxy": "never", "download_timeout": 2,
	}))
	if response.Code == http.StatusOK || !strings.HasPrefix(result.Code, "subscription.") {
		t.Fatalf("下载失败未保留领域错误: %s", response.Body.String())
	}
	var data map[string]any
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data["persisted"] != true {
		t.Fatalf("失败隐去了已持久化设置: %s", result.Data)
	}
	var summaries []catalog.GroupSummary
	resourceAssertSuccess(t, server, "GET", "/api/v1/subscriptions", "", "subscription.list", &summaries)
	if len(summaries) != 2 || !slices.ContainsFunc(summaries, func(group catalog.GroupSummary) bool { return group.Type == "subscription" }) {
		t.Fatalf("已接受设置未保存: %+v", summaries)
	}
}

func TestResourcesSubscriptionRejectsPathTargetsAndClientRuntimeOptions(t *testing.T) {
	server := resourceFixture(t)
	resourceSubscriptionFixture(t, server, "subscription-test", "https://example.test/sub")
	for _, body := range []string{
		`{"root":"/data/private"}`, `{"headers_file":"/data/private"}`, `{"progress_dir":"/data/private"}`,
		`{"download_timeout":"60"}`,
	} {
		response, result := resourceRequest(t, server, "PUT", "/api/v1/subscriptions/subscription-test", body)
		if response.Code != http.StatusBadRequest || result.Code != "usage.invalid" {
			t.Fatalf("订阅接受了非法参数: %s", response.Body.String())
		}
	}
	for _, path := range []string{
		"/api/v1/subscriptions/subscription-test?private=1",
		"/api/v1/subscriptions/%2e%2e?private=true",
		"/api/v1/subscriptions/a%2fb?private=true",
		"/api/v1/subscriptions/a%5cb/history",
	} {
		response, result := resourceRequest(t, server, "GET", path, "")
		if response.Code != http.StatusBadRequest || result.Code != "usage.invalid" {
			t.Fatalf("订阅路径未拒绝 %s: %s", path, response.Body.String())
		}
	}
	response, _ := resourceRequest(t, server, "GET", "/api/v1/subscriptions/default?private=true", "")
	if response.Code == http.StatusOK {
		t.Fatal("订阅私有接口接受了本地分组")
	}
}

func TestResourcesLogsReuseRedactionAndStableEntries(t *testing.T) {
	server := resourceFixture(t)
	log := "[2026-10-13 12:00:00] [INFO] [service] [service.start] [success] [-] 服务启动完成\n" +
		"not-a-native-event https://example.test/sub?token=private-log-token\n"
	for _, item := range []struct {
		kind, name string
		entries    int
	}{{"service", "service.log", 1}, {"core", "sing-box.log", 0}} {
		path := filepath.Join(server.options.LogDir, item.name)
		resourceWriteFile(t, path, log)
		var snapshot module.LogSnapshot
		response, result := resourceRequest(t, server, "GET", "/api/v1/logs?kind="+item.kind+"&lines=20", "")
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "private-log-token") {
			t.Fatalf("日志未脱敏: %s", response.Body.String())
		}
		if err := json.Unmarshal(result.Data, &snapshot); err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Entries) != item.entries {
			t.Fatalf("混用了服务与核心解析契约: %+v", snapshot)
		}
		resourceAssertSuccess(t, server, "DELETE", "/api/v1/logs?kind="+item.kind, "", "logs.cleared", nil)
		info, err := os.Stat(path)
		if err != nil || info.Size() != 0 {
			t.Fatalf("日志未原位截断: %v", err)
		}
	}
}

func TestResourcesEBPFDiagnosticRequiresKnownMode(t *testing.T) {
	server := resourceFixture(t)
	for _, body := range []string{`{"mode":"tun"}`, `{"mode":""}`, `{"raw":"true"}`, `{"binary":"/bin/sh"}`} {
		response, result := resourceRequest(t, server, "POST", "/api/v1/diagnostics/ebpf", body)
		if response.Code != http.StatusBadRequest || result.Code != "usage.invalid" {
			t.Fatalf("诊断接受了非法参数: %s", response.Body.String())
		}
	}
	response, result := resourceRequest(t, server, "POST", "/api/v1/diagnostics/ebpf", `{"mode":"configured"}`)
	if response.Code == http.StatusOK || result.Code != "ebpf.status_invalid" {
		t.Fatalf("TUN 下没有执行 eBPF 预检: %s", response.Body.String())
	}
	if strings.Contains(string(result.Data), `"content":"{`) {
		t.Fatalf("普通诊断返回了原始 JSON: %s", result.Data)
	}
}

func TestResourcesImportSharesBoundedRequestSize(t *testing.T) {
	server := resourceFixture(t)
	content := "socks://127.0.0.1:1085#ANONYMOUS_IMPORT"
	resourceAssertSuccess(t, server, "POST", "/api/v1/nodes/import", resourceJSON(t, map[string]string{"content": content}), "node.imported", nil)
	body := resourceJSON(t, map[string]string{"content": strings.Repeat("x", bodyLimit)})
	for _, path := range []string{"/api/v1/nodes", "/api/v1/nodes/import"} {
		response, result := resourceRequest(t, server, "POST", path, body)
		if response.Code != http.StatusRequestEntityTooLarge || result.Code != "api.body_too_large" {
			t.Fatalf("超大正文未被统一拒绝: %s", response.Body.String())
		}
	}
}

func TestSubscriptionValidationUsesUnprocessableStatus(t *testing.T) {
	server := resourceFixture(t)
	resourceSubscriptionFixture(t, server, "subscription-test", "https://example.test/sub")
	for _, test := range []struct{ method, path, body, code string }{
		{"PUT", "/api/v1/subscriptions/default", `{"name":"anonymous"}`, "subscription.read_only"},
		{"POST", "/api/v1/subscriptions", `{"url":"https://example.test/sub","update_interval":1}`, "subscription.interval_too_short"},
		{"POST", "/api/v1/subscriptions", `{"url":"https://example.test/sub","update_via_proxy":"invalid"}`, "subscription.proxy_mode_invalid"},
		{"POST", "/api/v1/subscriptions", `{"url":"https://example.test/sub","download_timeout":0}`, "subscription.timeout_invalid"},
		{"PUT", "/api/v1/subscriptions/subscription-test", `{"download_timeout":0}`, "subscription.timeout_invalid"},
		{"PUT", "/api/v1/subscriptions/subscription-test", `{"download_timeout":-1}`, "subscription.timeout_invalid"},
		{"PUT", "/api/v1/subscriptions/subscription-test", `{"update_interval":899}`, "subscription.interval_too_short"},
		{"PUT", "/api/v1/subscriptions/subscription-test", `{"update_via_proxy":"invalid"}`, "subscription.proxy_mode_invalid"},
	} {
		response, result := resourceRequest(t, server, test.method, test.path, test.body)
		if response.Code != http.StatusUnprocessableEntity || result.Code != test.code {
			t.Fatal("订阅校验错误被误报为内部错误", response.Code, result.Code)
		}
	}
}

func TestResourcesNodeContentNeedsNoTemporaryFiles(t *testing.T) {
	server := resourceFixture(t)
	content := `{"outbounds":[{"type":"socks","tag":"JSON / Node","server":"127.0.0.1","server_port":1086}]}`
	resourceAssertSuccess(t, server, "POST", "/api/v1/nodes", resourceJSON(t, map[string]string{"content": content}), "node.added", nil)
	resourceAssertSuccess(t, server, "PUT", "/api/v1/nodes", resourceJSON(t, map[string]string{"group_id": "default", "node_tag": "JSON / Node", "content": content}), "node.edited", nil)
	resourceRequest(t, server, "POST", "/api/v1/nodes", `{"content":"/data/adb/private-input.json"}`)
	if _, err := os.Stat(paths.New(server.options.ModuleDir).APITempDir()); !os.IsNotExist(err) {
		t.Fatalf("节点内容入口创建了临时目录: %v", err)
	}
}

func TestResourcesReadAndStopDoNotWaitForExpensiveSlots(t *testing.T) {
	server := resourceFixture(t)
	for range cap(server.expensive) {
		server.expensive <- struct{}{}
	}
	resourceAssertSuccess(t, server, "GET", "/api/v1/service", "", "service.status", nil)
	resourceAssertSuccess(t, server, "GET", "/api/v1/nodes", "", "node.list", nil)
	resourceAssertSuccess(t, server, "POST", "/api/v1/service/stop", "", "service.stop", nil)
	resourceAssertSuccess(t, server, "GET", "/api/v1/service", "", "service.status", nil)
	if _, err := os.Stat(server.options.WorkerPIDFile); !os.IsNotExist(err) {
		t.Fatalf("停服启动了 Worker: %v", err)
	}
}

func TestResourcesDelayKeepsSlashedTagAndOfflineDomainError(t *testing.T) {
	server := resourceFixture(t)
	server.options.RequestTimeout = time.Second
	response, result := resourceRequest(t, server, "POST", "/api/v1/nodes/delay", `{"group_id":"default","node_tag":"节点 / 02"}`)
	if response.Code == http.StatusOK || result.Code != "node.delay_offline_failed" {
		t.Fatalf("停止时没有进入领域离线测速: %s", response.Body.String())
	}
	var data map[string]any
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data["target"] != "本地配置/节点 / 02" {
		t.Fatalf("测速引用丢失标签: %s", result.Data)
	}
	for _, path := range []string{server.options.StateFile, server.options.WorkerPIDFile} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("离线测速改写了正式服务状态: %s %v", path, err)
		}
	}
}
