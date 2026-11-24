package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

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
	body := `{"rules":[{"phase":"request","pattern":"^http://example[.]org/","script":"return {reject: true}"}]}`
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
