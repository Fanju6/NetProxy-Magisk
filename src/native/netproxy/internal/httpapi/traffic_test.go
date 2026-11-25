package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
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
	resourceAssertSuccess(t, s, http.MethodPost, trafficRoute+"toggle", "", "traffic.toggle", nil)
	resourceAssertSuccess(t, s, http.MethodPost, trafficRoute+"toggle", "", "traffic.toggle", nil)
	w, result = resourceRequest(t, s, http.MethodGet, trafficRoute+"access", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("仍保留抓包凭据端点: %d %+v", w.Code, result)
	}
	resourceAssertSuccess(t, s, http.MethodDelete, trafficRoute+"list", "", "traffic.clear", nil)
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
