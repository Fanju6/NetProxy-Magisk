package httpapi

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/apiconfig"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

const resourceTestModule = `{
  "auto_start": false,
  "selection": {"group_id": "default", "node_tag": ""},
  "wifi": {"enabled": false, "mode": "blacklist", "blacklist": [], "whitelist": [], "proxy_on_non_wifi": true}
}`

const resourceTestInbound = `{
  "backend": "tun", "root_policy": "default",
  "app": {"enabled": false, "mode": "blacklist", "proxy_apps": [], "bypass_apps": []},
  "ebpf": {"type": "ebpf", "tag": "netproxy-in", "local": {"enabled": true}, "shared": {"enabled": false}},
  "tun": {"type": "tun", "tag": "netproxy-in", "interface_name": "netproxy", "address": ["172.19.0.1/30"], "auto_route": true, "auto_redirect": true}
}`

type resourceResponse struct {
	Schema  int            `json:"schema"`
	OK      bool           `json:"ok"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Data    jsontext.Value `json:"data"`
}

func resourceFixture(t testing.TB) *Server {
	t.Helper()
	root := t.TempDir()
	t.Setenv("NETPROXY_MODULE_DIR", root)
	t.Setenv("NETPROXY_DEV_ROOT", filepath.Join(root, "dev"))
	options := module.NewOptions(root)
	options.Telemetry = nil
	options.RequestTimeout = 20 * time.Millisecond
	options.ServiceAddress = "127.0.0.1:1"
	resourceWriteFile(t, options.ModuleConfig, resourceTestModule)
	resourceWriteFile(t, options.InboundConfig, resourceTestInbound)
	resourceWriteFile(t, paths.SingBoxConfig(options.SingBoxDir), `{"experimental":{"clash_api":{"default_mode":"Rule"}},"route":{"rules":[{"clash_mode":"Direct","action":"route","outbound":"direct"}]}}`)
	if err := catalog.EnsureGroup(t.Context(), catalog.GroupOptions{Root: options.CatalogRoot, GroupID: "default", Name: "本地配置", Type: "local"}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{
		GroupDir: filepath.Join(options.CatalogRoot, "default"), GroupID: "default", Type: "local",
		Input: "socks://127.0.0.1:1080#" + url.PathEscape("节点 / 02"),
	}); err != nil {
		t.Fatal(err)
	}
	server, err := New(options, apiconfig.Config{Listen: "127.0.0.1:9091", Secret: strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func resourceWriteFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func resourceJSON(t *testing.T, value any) string {
	t.Helper()
	content, err := json.Marshal(value, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func resourceRequest(t *testing.T, server *Server, method, target, body string) (*httptest.ResponseRecorder, resourceResponse) {
	t.Helper()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	request.Host = server.config.Listen
	request.Header.Set("Authorization", "Bearer "+server.config.Secret)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	var result resourceResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("无效响应 %d: %s: %v", response.Code, response.Body.String(), err)
	}
	if result.Schema != 1 || result.Code == "" || result.Message == "" || result.OK != (response.Code == http.StatusOK) {
		t.Fatalf("响应契约不一致 %d: %+v", response.Code, result)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("响应允许缓存私有数据")
	}
	return response, result
}

func resourceAssertSuccess(t *testing.T, server *Server, method, target, body, code string, data any) {
	t.Helper()
	response, result := resourceRequest(t, server, method, target, body)
	if response.Code != http.StatusOK || result.Code != code {
		t.Fatalf("请求 %s %s 失败 %d: %s", method, target, response.Code, response.Body.String())
	}
	if data != nil {
		if err := json.Unmarshal(result.Data, data); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResourcesReadWhileCoreStopped(t *testing.T) {
	server := resourceFixture(t)
	var status service.Status
	resourceAssertSuccess(t, server, "GET", "/api/v1/service", "", "service.status", &status)
	if status.State != "stopped" || status.PID != nil || status.ActiveBackend != nil || status.ConfiguredBackend != "tun" || status.OutboundMode != "Rule" {
		t.Fatalf("停止状态错误: %+v", status)
	}
	for _, request := range []struct{ path, code string }{
		{"/api/v1/catalog", "catalog.groups"},
		{"/api/v1/nodes?group_id=default", "node.list"},
		{"/api/v1/nodes/snapshot?group_id=default", "node.snapshot"},
		{"/api/v1/selection", "node.current"},
		{"/api/v1/mode", "mode.current"},
		{"/api/v1/subscriptions", "subscription.list"},
		{"/api/v1/configs", "config.list"},
		{"/api/v1/config?target=inbound", "config.read"},
		{"/api/v1/app-policy", "app.list"},
		{"/api/v1/logs?kind=service", "logs.show"},
	} {
		t.Run(request.path, func(t *testing.T) {
			resourceAssertSuccess(t, server, "GET", request.path, "", request.code, nil)
		})
	}
	var groups []catalog.GroupSnapshot
	resourceAssertSuccess(t, server, "GET", "/api/v1/nodes", "", "node.list", &groups)
	if len(groups) != 1 || len(groups[0].Nodes) != 1 || groups[0].Nodes[0].Tag != "节点 / 02" {
		t.Fatalf("停止时持久节点不可读: %+v", groups)
	}
	for _, path := range []string{server.options.StateFile, server.options.WorkerPIDFile, server.options.RuntimeDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("读取启动了服务或 Worker: %s %v", path, err)
		}
	}
}

func TestResourcesNodeTagsPreserveSlashes(t *testing.T) {
	server := resourceFixture(t)
	query := url.Values{"group_id": {"default"}, "node_tag": {"节点 / 02"}}.Encode()
	resourceAssertSuccess(t, server, "GET", "/api/v1/nodes/get?"+query, "", "node.loaded", nil)
	resourceAssertSuccess(t, server, "GET", "/api/v1/nodes/export?"+query, "", "node.exported", nil)
	body := resourceJSON(t, map[string]string{"group_id": "default", "node_tag": "节点 / 02"})
	resourceAssertSuccess(t, server, "PUT", "/api/v1/selection", body, "node.selected", nil)
	config, err := moduleconfig.LoadModule(server.options.ModuleConfig)
	if err != nil || config.SelectedNodeTag != "节点 / 02" {
		t.Fatalf("标签未完整保存: %+v %v", config, err)
	}
	var selection service.Selection
	resourceAssertSuccess(t, server, "GET", "/api/v1/selection", "", "node.current", &selection)
	if selection.SelectedNodeRef != "default/节点 / 02" {
		t.Fatalf("读取丢失标签: %+v", selection)
	}
	edit := resourceJSON(t, map[string]string{"group_id": "default", "node_tag": "节点 / 02", "content": "socks://127.0.0.1:1081#" + url.PathEscape("节点 / 03")})
	resourceAssertSuccess(t, server, "PUT", "/api/v1/nodes", edit, "node.edited", nil)
	query = url.Values{"group_id": {"default"}, "node_tag": {"节点 / 03"}}.Encode()
	resourceAssertSuccess(t, server, "DELETE", "/api/v1/nodes?"+query, "", "node.removed", nil)
	config, err = moduleconfig.LoadModule(server.options.ModuleConfig)
	if err != nil || config.SelectedNodeTag != "" {
		t.Fatalf("删除后未回退 Auto: %+v %v", config, err)
	}
	response, _ := resourceRequest(t, server, "GET", "/api/v1/nodes/default/node/extra", "")
	if response.Code != http.StatusNotFound {
		t.Fatal("接受了多级路径节点标签")
	}
}

func TestResourcesNodeContentAndImportNeverReadPaths(t *testing.T) {
	server := resourceFixture(t)
	path := filepath.Join(t.TempDir(), "nodes.json")
	resourceWriteFile(t, path, "socks://127.0.0.1:1099#PRIVATE_FILE_NODE")
	before, err := os.ReadFile(filepath.Join(server.options.CatalogRoot, "default", "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct {
		method, target string
		body           any
	}{
		{"POST", "/api/v1/nodes", map[string]string{"group_id": "default", "content": path}},
		{"PUT", "/api/v1/nodes", map[string]string{"group_id": "default", "node_tag": "节点 / 02", "content": path}},
		{"POST", "/api/v1/nodes/import", map[string]string{"content": path}},
		{"POST", "/api/v1/nodes/import", map[string]string{"source": path}},
	} {
		response, _ := resourceRequest(t, server, request.method, request.target, resourceJSON(t, request.body))
		if response.Code == http.StatusOK || strings.Contains(response.Body.String(), "PRIVATE_FILE_NODE") {
			t.Fatalf("请求读取了文件路径: %s", response.Body.String())
		}
		after, err := os.ReadFile(filepath.Join(server.options.CatalogRoot, "default", "provider.json"))
		if err != nil || string(before) != string(after) {
			t.Fatalf("拒绝路径时改变了 Provider: %v", err)
		}
	}
	resourceAssertSuccess(t, server, "POST", "/api/v1/nodes", `{"content":"socks://127.0.0.1:1082#HTTP_NODE"}`, "node.added", nil)
	resourceAssertSuccess(t, server, "POST", "/api/v1/nodes/import", `{"content":"socks://127.0.0.1:1083#IMPORTED_NODE"}`, "node.imported", nil)
}

func TestResourcesConfigRevisionAndValidation(t *testing.T) {
	server := resourceFixture(t)
	target := "/api/v1/config?target=module/auto_start"
	var initial map[string]string
	resourceAssertSuccess(t, server, "GET", target, "", "config.read", &initial)
	candidate := map[string]string{"revision": initial["revision"], "content": `{"auto_start":true}`}
	resourceAssertSuccess(t, server, "POST", "/api/v1/config/validate?target=module/auto_start", resourceJSON(t, candidate), "config.validate", nil)
	var afterValidation map[string]string
	resourceAssertSuccess(t, server, "GET", target, "", "config.read", &afterValidation)
	if afterValidation["revision"] != initial["revision"] || afterValidation["content"] != initial["content"] {
		t.Fatal("校验写入了配置")
	}
	var saved map[string]string
	resourceAssertSuccess(t, server, "PUT", target, resourceJSON(t, candidate), "config.apply", &saved)
	if saved["revision"] == initial["revision"] || saved["target"] != "module/auto_start" {
		t.Fatalf("保存未返回实际 revision: %+v", saved)
	}
	response, result := resourceRequest(t, server, "PUT", target, resourceJSON(t, candidate))
	if response.Code != http.StatusConflict || result.Code != "config.conflict" {
		t.Fatalf("冲突契约错误: %s", response.Body.String())
	}
	response, result = resourceRequest(t, server, "POST", "/api/v1/config/validate?target=module/auto_start", resourceJSON(t, candidate))
	if response.Code != http.StatusConflict || result.Code != "config.conflict" {
		t.Fatalf("校验未检测冲突: %s", response.Body.String())
	}
	config, err := moduleconfig.LoadModule(server.options.ModuleConfig)
	if err != nil || !config.AutoStart || config.ActiveGroupID != "default" || config.SelectedNodeTag != "" {
		t.Fatalf("分区保存改变了选择: %+v %v", config, err)
	}
	for _, path := range []string{server.options.StateFile, server.options.WorkerPIDFile,
		filepath.Join(server.options.RuntimeDir, "inbound.json"), filepath.Join(server.options.RuntimeDir, "providers.json"), filepath.Join(server.options.RuntimeDir, "outbounds.json")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("保存启动了停止的服务或 Worker: %s %v", path, err)
		}
	}
}

func TestResourcesConfigRequiresRevisionAndRejectsPaths(t *testing.T) {
	server := resourceFixture(t)
	for _, body := range []string{
		`{"content":"{}"}`, `{"revision":"","content":"{}"}`, `{"revision":"x","content":{}}`,
		`{"revision":"x","content":null}`, `{"revision":"x"}`, `{"revision":"x","content":"{}","source":"/data/secret"}`,
	} {
		for _, request := range []struct{ method, target string }{{"PUT", "/api/v1/config?target=module"}, {"POST", "/api/v1/config/validate?target=module"}} {
			response, result := resourceRequest(t, server, request.method, request.target, body)
			if response.Code != http.StatusBadRequest || result.Code != "usage.invalid" {
				t.Fatalf("接受不安全配置输入: %s", response.Body.String())
			}
		}
	}
	for _, target := range []string{"/etc/passwd", "../../module.json", "singbox/../../module.json", `singbox\config.json`, "runtime/../../config/module.json", "api", "singbox/rules/local/../../secret.json"} {
		response, result := resourceRequest(t, server, "GET", "/api/v1/config?target="+url.QueryEscape(target), "")
		if response.Code != http.StatusBadRequest || result.Code != "usage.invalid" {
			t.Fatalf("配置路径未拒绝 %s: %s", target, response.Body.String())
		}
	}
	resourceWriteFile(t, filepath.Join(server.options.RuntimeDir, "inbound.json"), "{}")
	var snapshot map[string]string
	resourceAssertSuccess(t, server, "GET", "/api/v1/config?target=runtime/inbound.json", "", "config.read", &snapshot)
	response, _ := resourceRequest(t, server, "PUT", "/api/v1/config?target=runtime/inbound.json", resourceJSON(t, map[string]string{"revision": snapshot["revision"], "content": "{}"}))
	if response.Code == http.StatusOK {
		t.Fatal("运行时配置可写")
	}
}

func TestResourcesModeUsesNativeNamesWhileStopped(t *testing.T) {
	server := resourceFixture(t)
	var state service.ModeState
	resourceAssertSuccess(t, server, "PUT", "/api/v1/mode", `{"mode":"Direct"}`, "mode.changed", &state)
	if state.Mode != "Direct" {
		t.Fatalf("默认模式未保存: %+v", state)
	}
	resourceAssertSuccess(t, server, "GET", "/api/v1/mode", "", "mode.current", &state)
	if state.Mode != "Direct" {
		t.Fatalf("停止时默认模式不可读: %+v", state)
	}
	if _, err := os.Stat(server.options.StateFile); !os.IsNotExist(err) {
		t.Fatalf("切换模式启动了服务: %v", err)
	}
}

func TestResourcesReadConfigCanRecoverBrokenEditorContent(t *testing.T) {
	server := resourceFixture(t)
	broken := `{"auto_start":`
	resourceWriteFile(t, server.options.ModuleConfig, broken)
	var document map[string]string
	resourceAssertSuccess(t, server, "GET", "/api/v1/config?target=module", "", "config.read", &document)
	if document["content"] != broken || document["revision"] == "" {
		t.Fatalf("损坏原文不可修复: %+v", document)
	}
	resourceAssertSuccess(t, server, "PUT", "/api/v1/config?target=module", resourceJSON(t, map[string]string{"revision": document["revision"], "content": resourceTestModule}), "config.apply", nil)
	resourceAssertSuccess(t, server, "GET", "/api/v1/service", "", "service.status", nil)
}

func TestResourcesServiceActionsAndUnknownCommands(t *testing.T) {
	server := resourceFixture(t)
	for _, action := range []string{"start", "stop", "restart", "reload", "check"} {
		response, result := resourceRequest(t, server, "POST", "/api/v1/service/"+action, `{"binary":"/bin/sh"}`)
		if response.Code != http.StatusBadRequest || result.Code != "usage.invalid" {
			t.Fatalf("服务操作未校验请求 %s: %s", action, response.Body.String())
		}
	}
	for _, target := range []string{"/api/v1/service/toggle", "/api/v1/service/exec", "/api/v1/exec", "/api/v1/raw", "/api/v1/app-policy"} {
		response, _ := resourceRequest(t, server, "POST", target, `{"command":"id"}`)
		if response.Code != http.StatusNotFound && response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("开放非白名单接口 %s: %s", target, response.Body.String())
		}
	}
	response, result := resourceRequest(t, server, "POST", "/api/v1/service/check", "")
	if response.Code == http.StatusOK || result.Code == "usage.invalid" || result.Code == "api.not_found" {
		t.Fatalf("领域检查错误未向上返回: %s", response.Body.String())
	}
}

func TestResourcesRejectMalformedBodiesAndQueryTargets(t *testing.T) {
	server := resourceFixture(t)
	for _, body := range []string{
		`{"group_id":"default","node_tag":"","extra":1}`,
		`{"group_id":"default","group_id":"other","node_tag":""}`,
		`{"group_id":"default","node_tag":""} {}`,
		`{"group_id":5,"node_tag":""}`,
		"{\"group_id\":\"default\",\"node_tag\":\"\xff\"}",
	} {
		response, result := resourceRequest(t, server, "PUT", "/api/v1/selection", body)
		if response.Code != http.StatusBadRequest || result.Code != "usage.invalid" {
			t.Fatalf("接受非法 JSON: %s", response.Body.String())
		}
	}
	for _, target := range []string{
		"/api/v1/nodes?group_id=..", "/api/v1/nodes?group_id=staging", "/api/v1/nodes?group_id=a%2Fb", "/api/v1/nodes?group_id=a%5Cb",
		"/api/v1/nodes/get?group_id=default", "/api/v1/nodes?source=/data/secret", "/api/v1/logs?kind=/data/secret",
		"/api/v1/logs?lines=0", "/api/v1/logs?lines=2001", "/api/v1/logs?lines=-1", "/api/v1/config?target=module&source=/data/secret",
	} {
		response, result := resourceRequest(t, server, "GET", target, "")
		if response.Code != http.StatusBadRequest || result.Code != "usage.invalid" {
			t.Fatalf("接受非法资源参数 %s: %s", target, response.Body.String())
		}
	}
}

func TestResourcesAcceptedWriteSurvivesRequestCancellation(t *testing.T) {
	server := resourceFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server.route("PUT /api/v1/resource-test-cancel", time.Second, false, func(ctx context.Context, r *http.Request) (result contract.Result, err error) {
		cancel()
		return server.saveSelection(ctx, r)
	})
	request := httptest.NewRequest("PUT", "/api/v1/resource-test-cancel", strings.NewReader(`{"group_id":"default","node_tag":"节点 / 02"}`)).WithContext(ctx)
	request.Host = server.config.Listen
	request.Header.Set("Authorization", "Bearer "+server.config.Secret)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("浏览器取消中断已接受写入: %s", response.Body.String())
	}
	config, err := moduleconfig.LoadModule(server.options.ModuleConfig)
	if err != nil || config.SelectedNodeTag != "节点 / 02" {
		t.Fatalf("写入未完成: %+v %v", config, err)
	}
}

func TestResourcesSubscriptionDownloadBudgetIsBounded(t *testing.T) {
	for _, value := range []struct {
		seconds int64
		want    time.Duration
	}{
		{0, 4 * time.Minute}, {60, 4 * time.Minute}, {120, 6 * time.Minute}, {240, 10 * time.Minute}, {1 << 62, 10 * time.Minute},
	} {
		if got := resourceDownloadBudget(value.seconds); got != value.want {
			t.Fatalf("预算 %d: %s != %s", value.seconds, got, value.want)
		}
	}
}
