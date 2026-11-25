package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQueryNodeLatency(t *testing.T) {
	for _, tc := range []struct {
		tag, provider, prefix, suffix string
	}{
		{"静态/a%2F", "", "/proxies/", "/delay"},
		{"prov/含 空格%2F/a", "prov", "/providers/proxies/", "/healthcheck"},
	} {
		t.Run(tc.tag, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := r.URL.EscapedPath()
				encoded := strings.TrimSuffix(strings.TrimPrefix(path, tc.prefix), tc.suffix)
				if tc.provider != "" {
					prefix, rest, found := strings.Cut(encoded, "/")
					provider, _ := url.PathUnescape(prefix)
					if !found || provider != tc.provider {
						t.Errorf("unexpected provider route: %v", r.URL)
					}
					encoded = rest
				}
				tag, _ := url.PathUnescape(encoded)
				if tag != tc.tag || strings.Contains(encoded, "/") || !strings.HasPrefix(path, tc.prefix) ||
					!strings.HasSuffix(path, tc.suffix) || r.Header.Get("Authorization") != "Bearer fixture" ||
					r.URL.Query().Get("timeout") != "5000" {
					t.Errorf("unexpected request: %v", r.URL)
				}
				fmt.Fprint(w, `{"delay":70}`)
			}))
			defer server.Close()
			config := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(config, fmt.Appendf(nil, `{"experimental":{"clash_api":{"external_controller":%q,"secret":"fixture"}}}`, strings.TrimPrefix(server.URL, "http://")), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := queryNodeLatency(t.Context(), config, NodeLatencyTarget{NodeTag: tc.tag, ProviderTag: tc.provider})
			if err != nil || result.DelayMS != 70 {
				t.Fatalf("unexpected latency result: %+v, %v", result, err)
			}
		})
	}
}
