package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestStatusBackendRequiresMatchingInstance(t *testing.T) {
	for _, test := range []struct {
		name       string
		pid        int
		state      string
		apiStarted int64
		apiFailure bool
		wantActive bool
	}{
		{"confirmed", 123, "ready", 1700000000123, false, true},
		{"different-instance-same-second", 123, "ready", 1700000000456, false, false},
		{"api-unavailable", 123, "ready", 1700000000123, true, false},
		{"pid-reused", 456, "ready", 1700000000123, false, false},
		{"process-exited", 0, "ready", 1700000000123, false, false},
		{"not-ready", 123, "starting", 1700000000123, false, false},
		{"stopped", 0, "stopped", 1700000000123, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			modulePath := filepath.Join(root, "module.conf")
			if err := os.WriteFile(modulePath, []byte(""), 0o600); err != nil {
				t.Fatal(err)
			}
			modePath := modeConfigFixture(t, root, "Rule")
			withServiceProcess(t, test.pid)
			inboundPath := filepath.Join(root, "inbound.json")
			content := `{"backend":"tun","app":{"enabled":false,"mode":"blacklist","proxy_apps":[],"bypass_apps":[]},"ebpf":{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true}},"tun":{"type":"tun","tag":"netproxy-in","interface_name":"netproxy","address":["172.19.0.1/30"],"auto_route":true,"auto_redirect":true}}`
			if err := os.WriteFile(inboundPath, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			statePath := filepath.Join(root, "service.json")
			state := fmt.Sprintf(`{"state":%q,"pid":123,"started_at":1700000000,"ready_at":1700000000,"active_backend":"ebpf","core_started_at_millis":1700000000123}`, test.state)
			if err := os.WriteFile(statePath, []byte(state), 0o600); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				var payload []byte
				if request.URL.Path == "/daemon.StartedService/GetStartedAt" {
					if test.apiFailure {
						http.Error(writer, "not ready", http.StatusServiceUnavailable)
						return
					}
					payload = protowire.AppendTag(payload, 1, protowire.VarintType)
					payload = protowire.AppendVarint(payload, uint64(test.apiStarted))
				}
				writeServiceAPIFrame(t, writer, payload)
			}))
			defer server.Close()
			status, err := ReadStatus(t.Context(), Options{ModuleConfig: modulePath, SingBoxConfig: modePath, CatalogRoot: filepath.Join(root, "catalog"), StateFile: statePath, InboundConfig: inboundPath, ServiceAddress: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			if status.ConfiguredBackend != "tun" {
				t.Fatalf("configured backend = %q", status.ConfiguredBackend)
			}
			if (status.ActiveBackend != nil) != test.wantActive {
				t.Fatalf("active backend = %v, expected confirmed %v", status.ActiveBackend, test.wantActive)
			}
			if status.ActiveBackend != nil && *status.ActiveBackend != "ebpf" {
				t.Fatalf("active backend incorrectly guessed from configured backend: %s", *status.ActiveBackend)
			}
		})
	}
}
