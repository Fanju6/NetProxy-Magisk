package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/traffic"
)

func TestTrafficAppsContract(t *testing.T) {
	s := resourceFixture(t)
	target := trafficRoute + "apps"
	r := httptest.NewRequest(http.MethodPut, target, strings.NewReader(`{"apps":[]}`))
	r.Host = s.config.Listen
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("未认证请求获得 %d", w.Code)
	}
	var policy traffic.AppsPolicy
	resourceAssertSuccess(t, s, http.MethodGet, target, "", "traffic.apps", &policy)
	if policy.Apps == nil || len(policy.Apps) != 0 {
		t.Fatalf("默认名单不为空: %+v", policy)
	}
	if policy.DisableQUIC == nil || !*policy.DisableQUIC {
		t.Fatalf("默认未禁用 QUIC: %+v", policy)
	}
	resourceAssertSuccess(t, s, http.MethodPut, target, `{"apps":[],"disable_quic":false}`, "traffic.apps_saved", &policy)
	resourceAssertSuccess(t, s, http.MethodPut, target, `{"apps":["10:app.example"]}`, "traffic.apps_saved", &policy)
	invalid, result := resourceRequest(t, s, http.MethodPut, target, `{"apps":["app.invalid"]}`)
	if invalid.Code != http.StatusUnprocessableEntity || result.Code != "traffic.apps_failed" {
		t.Fatalf("无效名单响应: %d %+v", invalid.Code, result)
	}
	resourceAssertSuccess(t, s, http.MethodGet, target, "", "traffic.apps", &policy)
	if len(policy.Apps) != 1 || policy.Apps[0] != "10:app.example" {
		t.Fatalf("未保留原始名单: %+v", policy)
	}
	if policy.DisableQUIC == nil || *policy.DisableQUIC {
		t.Fatalf("保存名单覆盖了 QUIC 偏好: %+v", policy)
	}
	resourceAssertSuccess(t, s, http.MethodPut, target, `{"mode":"blacklist","blacklist":["0:app.example"],"whitelist":["10:app.example"]}`, "traffic.apps_saved", &policy)
	resourceAssertSuccess(t, s, http.MethodPut, target, `{"apps":[],"disable_quic":true}`, "traffic.apps_saved", &policy)
	if policy.Mode != "blacklist" || len(policy.Blacklist) != 1 || len(policy.Whitelist) != 1 || !*policy.DisableQUIC {
		t.Fatalf("旧版客户端修改 QUIC 时覆盖了模式或名单: %+v", policy)
	}
}

func TestTrafficManagementRequiresAuthAndValidatesRules(t *testing.T) {
	s := resourceFixture(t)
	target := trafficRoute + "rules"
	r := httptest.NewRequest(http.MethodPut, target, strings.NewReader(`{"rules":[]}`))
	r.Host = s.config.Listen
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("未认证请求获得 %d", w.Code)
	}
	resourceAssertSuccess(t, s, http.MethodGet, target, "", "traffic.rules", nil)
	body := `{"rules":[{"domains":["example.org"],"phase":"request","pattern":"^http://example[.]org/","script":"return {reject: true}"}]}`
	resourceAssertSuccess(t, s, http.MethodPut, target, body, "traffic.rules_saved", nil)
	w, result := resourceRequest(t, s, http.MethodPut, target, `{"rules":[{"phase":"bad","pattern":".","script":"return {}"}]}`)
	if w.Code != http.StatusUnprocessableEntity || result.Code != "traffic.rules_failed" {
		t.Fatalf("无效规则响应: %d %+v", w.Code, result)
	}
	path := paths.New(s.options.ModuleDir).TrafficRules()
	saved, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(saved), "example") {
		t.Fatalf("无效规则覆盖已有配置: %q %v", saved, err)
	}
	var rules trafficRules
	resourceAssertSuccess(t, s, http.MethodGet, target, "", "traffic.rules", &rules)
	if len(rules.Rules) != 1 || rules.Rules[0].Enabled == nil || !*rules.Rules[0].Enabled || rules.Rules[0].Name != "" {
		t.Fatalf("旧规则未默认启用: %+v", rules.Rules)
	}
	disabled := strings.Replace(body, `"phase":"request"`, `"phase":"request","enabled":false,"name":"  请求标题  "`, 1)
	resourceAssertSuccess(t, s, http.MethodPut, target, disabled, "traffic.rules_saved", nil)
	resourceAssertSuccess(t, s, http.MethodGet, target, "", "traffic.rules", &rules)
	if len(rules.Rules) != 1 || rules.Rules[0].Enabled == nil || *rules.Rules[0].Enabled {
		t.Fatalf("未保留规则禁用状态: %+v", rules.Rules)
	}
	if rules.Rules[0].Name != "请求标题" {
		t.Fatalf("未保留规范化的规则标题: %+v", rules.Rules)
	}
	overlong := strings.Replace(disabled, "请求标题", strings.Repeat("字", 65), 1)
	w, result = resourceRequest(t, s, http.MethodPut, target, overlong)
	if w.Code != http.StatusUnprocessableEntity || result.Code != "traffic.rules_failed" {
		t.Fatalf("超长标题响应: %d %+v", w.Code, result)
	}
	resourceAssertSuccess(t, s, http.MethodPost, trafficRoute+"toggle", "", "traffic.toggle", nil)
	resourceAssertSuccess(t, s, http.MethodPost, trafficRoute+"toggle", "", "traffic.toggle", nil)
	w, result = resourceRequest(t, s, http.MethodGet, trafficRoute+"access", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("仍保留抓包凭据端点: %d %+v", w.Code, result)
	}
	resourceAssertSuccess(t, s, http.MethodDelete, trafficRoute+"list", "", "traffic.clear", nil)
}

func TestTrafficBatchContract(t *testing.T) {
	s := resourceFixture(t)
	var batches []traffic.Batch
	var entries []traffic.Entry
	resourceAssertSuccess(t, s, http.MethodPost, trafficRoute+"start", "", "traffic.start", nil)
	resourceAssertSuccess(t, s, http.MethodGet, trafficRoute+"batches", "", "traffic.batches", &batches)
	if len(batches) != 1 || !batches[0].Running {
		t.Fatalf("unexpected running batches: %+v", batches)
	}
	id := batches[0].ID
	target := trafficRoute + "batches?id=" + id
	resourceAssertSuccess(t, s, http.MethodGet, trafficRoute+"list?batch="+id, "", "traffic.list", &entries)
	var page traffic.HistoryPage
	resourceAssertSuccess(t, s, http.MethodGet, trafficRoute+"page?batch="+id+"&limit=20&since=1", "", "traffic.page", &page)
	if page.Batch != id || page.Entries == nil {
		t.Fatalf("invalid initial page: %+v", page)
	}
	w, result := resourceRequest(t, s, http.MethodGet, trafficRoute+"page?before=1&since=1", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("mutually exclusive page cursors: %d, %s", w.Code, result.Code)
	}
	w, result = resourceRequest(t, s, http.MethodDelete, target, "")
	if w.Code != http.StatusConflict || result.Code != "traffic.batch_running" {
		t.Fatalf("delete running batch: %d, %s", w.Code, result.Code)
	}
	resourceAssertSuccess(t, s, http.MethodPost, trafficRoute+"stop", "", "traffic.stop", nil)
	resourceAssertSuccess(t, s, http.MethodDelete, target, "", "traffic.batch_deleted", nil)
	w, result = resourceRequest(t, s, http.MethodGet, trafficRoute+"list?batch="+id, "")
	if w.Code != http.StatusNotFound || result.Code != "traffic.batch_not_found" {
		t.Fatalf("read deleted batch: %d, %s", w.Code, result.Code)
	}
}

func TestTrafficMITMManagementNeverExposesPrivateKey(t *testing.T) {
	s := resourceFixture(t)
	var policy struct {
		Enabled bool `json:"enabled"`
	}
	resourceAssertSuccess(t, s, http.MethodGet, trafficRoute+"mitm", "", "traffic.mitm", &policy)
	if policy.Enabled {
		t.Fatal("HTTPS 解密默认开启")
	}
	w, _ := resourceRequest(t, s, http.MethodPost, trafficRoute+"ca-init", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "BEGIN CERTIFICATE") || strings.Contains(w.Body.String(), "PRIVATE KEY") {
		t.Fatalf("CA 初始化响应无效: %d", w.Code)
	}
	resourceAssertSuccess(t, s, http.MethodGet, trafficRoute+"ca", "", "traffic.ca", nil)
	resourceAssertSuccess(t, s, http.MethodPut, trafficRoute+"mitm", `{"enabled":true,"hosts":["*.example.org"],"system_trust":false}`, "traffic.mitm_saved", &policy)
	if !policy.Enabled {
		t.Fatal("HTTPS 解密未按显式请求开启")
	}
	w, result := resourceRequest(t, s, http.MethodDelete, trafficRoute+"ca/trust", "")
	if w.Code != http.StatusConflict || result.Code != "traffic.user_trust_in_use" {
		t.Fatalf("解密开启时应拒绝移除个人 CA: %d %+v", w.Code, result)
	}
	resourceAssertSuccess(t, s, http.MethodPut, trafficRoute+"mitm", `{"enabled":false,"system_trust":true}`, "traffic.mitm_saved", nil)
	resourceAssertSuccess(t, s, http.MethodDelete, trafficRoute+"ca/trust", "", "traffic.user_trust_removed", nil)
	var saved traffic.MITMPolicy
	resourceAssertSuccess(t, s, http.MethodGet, trafficRoute+"mitm", "", "traffic.mitm", &saved)
	if saved.Enabled || !saved.SystemTrust {
		t.Fatalf("解密关闭和显式移除个人 CA 不应关闭系统信任: %+v", saved)
	}
}

func TestTrafficPinnedListAndClear(t *testing.T) {
	s := resourceFixture(t)
	var entries []traffic.PinnedBypass
	resourceAssertSuccess(t, s, http.MethodGet, trafficRoute+"pinned", "", "traffic.pinned", &entries)
	if entries == nil || len(entries) != 0 {
		t.Fatalf("预期空跳过列表: %+v", entries)
	}
	resourceAssertSuccess(t, s, http.MethodDelete, trafficRoute+"pinned", "", "traffic.pinned_cleared", &entries)
}

func TestTrafficDetailRequiresAuthenticationAndValidID(t *testing.T) {
	s := resourceFixture(t)
	target := trafficRoute + "detail"
	r := httptest.NewRequest(http.MethodGet, target, nil)
	r.Host = s.config.Listen
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated detail request: %d", w.Code)
	}
	w, _ = resourceRequest(t, s, http.MethodGet, target+"?id=bad", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid detail request: %d", w.Code)
	}
	w, result := resourceRequest(t, s, http.MethodGet, target+"?id="+strings.Repeat("A", 26), "")
	if w.Code != http.StatusNotFound || result.Code != "traffic.record_not_found" {
		t.Fatalf("missing record: %d %s", w.Code, result.Code)
	}
}

func TestTrafficPreviewRequiresAuthentication(t *testing.T) {
	s := resourceFixture(t)
	target := trafficRoute + "preview"
	r := httptest.NewRequest(http.MethodGet, target, nil)
	r.Host = s.config.Listen
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated preview request: %d", w.Code)
	}
	var preview traffic.Preview
	resourceAssertSuccess(t, s, http.MethodGet, target, "", "traffic.preview", &preview)
	if preview.Running || preview.Count != 0 || preview.Entries == nil || len(preview.Entries) != 0 {
		t.Fatalf("stopped preview: %+v", preview)
	}
}

func TestTrafficReplayRejectsUnauthorizedInvalidAndMissingRecords(t *testing.T) {
	s := resourceFixture(t)
	target := trafficRoute + "replay"
	r := httptest.NewRequest(http.MethodPost, target, nil)
	r.Host = s.config.Listen
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated replay: %d", w.Code)
	}
	w, result := resourceRequest(t, s, http.MethodPost, target, `{"id":"bad"}`)
	if w.Code != http.StatusBadRequest || result.Code != "traffic.replay_invalid" {
		t.Fatalf("invalid replay: %d %s", w.Code, result.Code)
	}
	w, result = resourceRequest(t, s, http.MethodPost, target, `{"id":"`+strings.Repeat("A", 26)+`"}`)
	if w.Code != http.StatusNotFound || result.Code != "traffic.record_not_found" {
		t.Fatalf("missing replay record: %d %s", w.Code, result.Code)
	}
}

func TestTrafficBreakpointContract(t *testing.T) {
	s := resourceFixture(t)
	config := trafficRoute + "breakpoints"
	w, result := resourceRequest(t, s, http.MethodPut, config, `{"rules":[{"domains":[],"pattern":"/","phase":"both"}]}`)
	if w.Code != http.StatusUnprocessableEntity || result.Code != "traffic.breakpoints_invalid" {
		t.Fatalf("invalid breakpoint rules: %d %s", w.Code, result.Code)
	}
	w, result = resourceRequest(t, s, http.MethodPut, config, `{"rules":[{"domains":["example.test"],"pattern":"/","phase":"both"}]}`)
	if w.Code != http.StatusOK || result.Code != "traffic.breakpoints_saved" || !strings.Contains(w.Body.String(), `"timeout_seconds":60`) {
		t.Fatalf("save breakpoint rules: %d %s", w.Code, result.Code)
	}
	w, result = resourceRequest(t, s, http.MethodGet, trafficRoute+"paused", "")
	if w.Code != http.StatusOK || result.Code != "traffic.paused" || !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Fatalf("list paused requests: %d %s", w.Code, result.Code)
	}
	w, result = resourceRequest(t, s, http.MethodPost, trafficRoute+"resume?id="+strings.Repeat("A", 26), `{}`)
	if w.Code != http.StatusNotFound || result.Code != "traffic.breakpoint_gone" {
		t.Fatalf("resume missing pause: %d %s", w.Code, result.Code)
	}
}

func TestTrafficDomainQueries(t *testing.T) {
	s := resourceFixture(t)
	id, batch := strings.Repeat("A", 26), strings.Repeat("B", 26)
	dir := filepath.Join(paths.New(s.options.ModuleDir).TrafficHistory(), batch)
	resourceWriteFile(t, filepath.Join(dir, "batch.json"), `{"id":"`+batch+`","started_at":"2026-01-01T00:00:00Z"}`)
	resourceWriteFile(t, filepath.Join(dir, "0000000000000001-"+id+".json"), `{"id":"`+id+`","host":"example.test:8080"}`)
	s.traffic = traffic.New(paths.New(s.options.ModuleDir))
	var domains []traffic.CaptureDomain
	resourceAssertSuccess(t, s, http.MethodGet, trafficRoute+"domains?batch="+batch, "", "traffic.domains", &domains)
	if len(domains) != 1 || domains[0].Host != "example.test" || domains[0].Count != 1 {
		t.Fatalf("domains: %+v", domains)
	}
	var page traffic.HistoryPage
	resourceAssertSuccess(t, s, http.MethodGet, trafficRoute+"page?host=example.test", "", "traffic.page", &page)
	if len(page.Entries) != 1 || page.Entries[0].ID != id {
		t.Fatalf("filtered page: %+v", page)
	}
}

func TestTrafficBodyAuthenticationAndRanges(t *testing.T) {
	s := resourceFixture(t)
	id, batch := strings.Repeat("A", 26), strings.Repeat("B", 26)
	dir := filepath.Join(paths.New(s.options.ModuleDir).TrafficHistory(), batch)
	resourceWriteFile(t, filepath.Join(dir, "batch.json"), `{"id":"`+batch+`","started_at":"2026-01-01T00:00:00Z"}`)
	resourceWriteFile(t, filepath.Join(dir, "0000000000000001-"+id+".json"),
		`{"id":"`+id+`","http":{"response":{"body":{"format":"binary","blob":true,"captured_size":6}}}}`)
	resourceWriteFile(t, filepath.Join(dir, id+".response.blob"), "abcdef")
	s.traffic = traffic.New(paths.New(s.options.ModuleDir))
	fetch := func(auth bool, rangeHeader string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, trafficRoute+"body?id="+id+"&side=response", nil)
		r.Host = s.config.Listen
		if auth {
			r.Header.Set("Authorization", "Bearer "+s.config.Secret)
		}
		if rangeHeader != "" {
			r.Header.Set("Range", rangeHeader)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := fetch(false, "bytes=2-4"); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status: %d", w.Code)
	}
	if w := fetch(true, "bytes=2-4"); w.Code != http.StatusPartialContent || w.Body.String() != "cde" ||
		w.Header().Get("Content-Range") != "bytes 2-4/6" {
		t.Fatalf("partial content: %d %q %s", w.Code, w.Body, w.Header())
	}
	if w := fetch(true, "bytes=6-"); w.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("out-of-range status: %d", w.Code)
	}
	if w := fetch(true, ""); w.Code != http.StatusOK || w.Body.String() != "abcdef" {
		t.Fatalf("whole body: %d %q", w.Code, w.Body)
	}
}
