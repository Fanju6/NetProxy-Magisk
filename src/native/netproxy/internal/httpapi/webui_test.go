package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebUIPublicAssetsAndPrivateAPIRemainSeparated(t *testing.T) {
	server := resourceFixture(t)
	root := filepath.Join(server.options.ModuleDir, "webroot", "netproxy", "control")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>NetProxy</h1>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app.js"), []byte("console.log('panel')"), 0644); err != nil {
		t.Fatal(err)
	}

	check := func(method, target, host, origin, auth string, code int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, target, nil)
		req.Host = host
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		if w.Code != code {
			t.Fatalf("%s %s: got %d, want %d: %s", method, target, w.Code, code, w.Body.String())
		}
		return w
	}

	for _, p := range []string{"/ui/", "/ui/app.js"} {
		w := check("GET", p, server.config.Listen, "", "", http.StatusOK)
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("WebUI 静态文件被缓存")
		}
		if !strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src 'self'") {
			t.Fatal("未限制 WebUI 脚本来源")
		}
		if w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("未限制 Referrer")
		}
	}
	check("GET", "/ui", server.config.Listen, "", "", http.StatusPermanentRedirect)
	check("GET", "/ui/", "localhost:9091", "", "", http.StatusForbidden)
	check("GET", "/ui/", server.config.Listen, "https://evil.example", "", http.StatusForbidden)
	check("GET", "/ui/?secret=redacted", server.config.Listen, "", "", http.StatusBadRequest)
	check("POST", "/ui/", server.config.Listen, "", "", http.StatusUnauthorized)
	check("GET", "/api/v1/info", server.config.Listen, "", "", http.StatusUnauthorized)
	check("GET", "/api/v1/events", server.config.Listen, "", "", http.StatusUnauthorized)
	check("GET", "/api/v1/info", server.config.Listen, "https://evil.example", "Bearer "+server.config.Secret, http.StatusForbidden)
	check("HEAD", "/api/v1/info", server.config.Listen, "", "Bearer "+server.config.Secret, http.StatusMethodNotAllowed)
}
