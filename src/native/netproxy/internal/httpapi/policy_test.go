package httpapi

import (
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func TestPolicyHTTPStoppedSnapshotIsNativeConfigOnly(t *testing.T) {
	server := resourceFixture(t)
	resourceWriteFile(t, paths.SingBoxConfig(server.options.SingBoxDir), `{"outbounds":[{"type":"selector","tag":"Main / 组","outbounds":["Auto","direct"],"default":"direct"},{"type":"urltest","tag":"Auto","providers":["provider-a"]},{"type":"direct","tag":"direct"}],"route":{"final":"Main / 组"}}`)
	if err := os.Remove(server.options.ModuleConfig); err != nil {
		t.Fatal(err)
	}
	response, envelope := resourceRequest(t, server, "GET", "/api/v1/policy-groups", "")
	var snapshot service.PolicySnapshot
	if err := json.Unmarshal(envelope.Data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || envelope.Code != "policy.groups" || snapshot.Live || snapshot.CoreStartedAtMillis != 0 || snapshot.DefaultOutbound != "Main / 组" || len(snapshot.Groups) != 2 {
		t.Fatalf("wrong snapshot: %s", response.Body.String())
	}
	if !snapshot.Groups[0].Selectable || snapshot.Groups[1].Selectable || len(snapshot.Groups[1].Items) != 0 {
		t.Fatalf("wrong native summary: %+v", snapshot)
	}
	for _, field := range []string{`"selected"`, `"selection"`, `url_test_time`, `url_test_delay`, `runtime_groups`, `provider-a/`} {
		if strings.Contains(string(envelope.Data), field) {
			t.Fatalf("invented runtime state %s: %s", field, envelope.Data)
		}
	}
	for _, key := range []string{`"groups"`, `"live"`, `"core_started_at_millis"`, `"default_outbound"`} {
		if !strings.Contains(string(envelope.Data), key) {
			t.Fatalf("missing response field %s", key)
		}
	}
	for _, path := range []string{server.options.StateFile, server.options.WorkerPIDFile, server.options.RuntimeDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("read created runtime file %s: %v", path, err)
		}
	}
}

func TestPolicyHTTPStoppedOperationsDoNotPersistSelection(t *testing.T) {
	server := resourceFixture(t)
	before, err := os.ReadFile(server.options.ModuleConfig)
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct{ path, body string }{
		{"/api/v1/policy-groups/select", `{"group_tag":"Main / 组","member_tag":"provider-b/same"}`},
		{"/api/v1/policy-groups/delay", `{"group_tag":"Main / 组"}`},
	} {
		response, result := resourceRequest(t, server, "POST", request.path, request.body)
		if response.Code != http.StatusConflict || result.Code != "policy.core_not_ready" {
			t.Fatalf("stopped operation accepted: %s", response.Body.String())
		}
	}
	after, err := os.ReadFile(server.options.ModuleConfig)
	if err != nil || string(before) != string(after) {
		t.Fatalf("selection persisted in module: %s %v", after, err)
	}
	for _, path := range []string{server.options.StateFile, server.options.WorkerPIDFile, server.options.SingBoxPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("operation launched core/worker: %s %v", path, err)
		}
	}
}

func TestPolicyHTTPRejectsUnknownOrMalformedBodies(t *testing.T) {
	server := resourceFixture(t)
	for _, test := range []struct{ path, body string }{
		{"select", `{}`},
		{"select", `{"group_tag":"Main"}`},
		{"select", `{"group_tag":"Main","member_tag":" "}`},
		{"select", `{"group_tag":1,"member_tag":"provider-a/same"}`},
		{"select", `{"group_tag":"Main","group_tag":"Other","member_tag":"provider-a/same"}`},
		{"select", `{"group_tag":"Main","member_tag":"provider-a/same","url":"http://127.0.0.1/"}`},
		{"select", `{"group_tag":"Main","member_tag":"provider-a/same","path":"/data/secret"}`},
		{"select", `{"group_tag":"Main","member_tag":"provider-a/same"} {}`},
		{"select", "{\"group_tag\":\"Main\",\"member_tag\":\"\xff\"}"},
		{"delay", `{"group_tag":""}`},
		{"delay", `{"group_tag":"Main","member_tag":"provider-a/same"}`},
		{"delay", `{"group_tag":"Main","timeout":99999}`},
		{"delay", `{"group_tag":"Main","group_tag":"Other"}`},
	} {
		response, result := resourceRequest(t, server, "POST", "/api/v1/policy-groups/"+test.path, test.body)
		if response.Code != http.StatusBadRequest || result.Code != "usage.invalid" {
			t.Fatalf("invalid body accepted %s: %s", test.body, response.Body.String())
		}
	}
	for _, request := range []struct{ method, path, body string }{
		{"GET", "/api/v1/policy-groups?group_tag=Main", ""},
		{"GET", "/api/v1/policy-groups?path=/data/config", ""},
		{"POST", "/api/v1/policy-groups/select?group_tag=Main", `{"group_tag":"Main","member_tag":"provider-a/same"}`},
		{"POST", "/api/v1/policy-groups/delay?url=http://example.com", `{"group_tag":"Main"}`},
	} {
		response, result := resourceRequest(t, server, request.method, request.path, request.body)
		if response.Code != http.StatusBadRequest || result.Code != "usage.invalid" {
			t.Fatalf("unknown query accepted: %s", response.Body.String())
		}
	}
}

func TestPolicyHTTPAuthAndHEADNeverReachDomain(t *testing.T) {
	server := resourceFixture(t)
	resourceWriteFile(t, paths.SingBoxConfig(server.options.SingBoxDir), `{invalid JSON`)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		request := httptest.NewRequest(method, "/api/v1/policy-groups", nil)
		request.Host = server.config.Listen
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated request accepted: %d", response.Code)
		}
	}
	response, envelope := resourceRequest(t, server, "HEAD", "/api/v1/policy-groups", "")
	if response.Code != http.StatusMethodNotAllowed || envelope.Code == "policy.config_invalid" {
		t.Fatalf("HEAD reached config parser: %s", response.Body.String())
	}
	response, envelope = resourceRequest(t, server, "GET", "/api/v1/policy-groups", "")
	if response.Code != http.StatusUnprocessableEntity || envelope.Code != "policy.config_invalid" {
		t.Fatalf("config error hidden: %s", response.Body.String())
	}
}

func TestPolicyHTTPMapsDomainErrorsAndAcceptedSideEffects(t *testing.T) {
	for _, test := range []struct {
		code   string
		status int
	}{
		{"policy.invalid", 400}, {"policy.core_not_ready", 409}, {"policy.group_not_found", 404},
		{"policy.member_not_found", 404}, {"policy.not_selectable", 422}, {"policy.config_invalid", 422},
		{"policy.api_failed", 502}, {"policy.runtime_sync_failed", 502},
		{"catalog.provider_in_use", 409}, {"catalog.provider_conflict", 409},
		{"catalog.read_only", 422}, {"catalog.invalid", 422},
	} {
		err := &service.Error{Code: test.code, Message: "private exception /data/secret", Data: map[string]any{"accepted": true, "cause": "private cause"}}
		envelope, _ := contract.Failure(t.Context(), err)
		envelope = resourcePublicFailure(envelope)
		if failureStatus(envelope.Code, err) != test.status || envelope.Code != test.code || envelope.Data.(map[string]any)["accepted"] != true {
			t.Fatalf("wrong HTTP error mapping: %+v %v", envelope, err)
		}
		if _, exists := envelope.Data.(map[string]any)["cause"]; exists || strings.Contains(envelope.Message, "private") {
			t.Fatalf("private cause exposed: %+v", envelope)
		}
	}
}

func TestPolicyHTTPRemovesLegacySelectionAndDelayRoutes(t *testing.T) {
	server := resourceFixture(t)
	for _, request := range []struct{ method, path string }{
		{"GET", "/api/v1/selection"}, {"PUT", "/api/v1/selection"}, {"POST", "/api/v1/nodes/delay"},
		{"PUT", "/api/v1/policy-groups/select"}, {"GET", "/api/v1/policy-groups/select"},
	} {
		response, _ := resourceRequest(t, server, request.method, request.path, `{}`)
		if response.Code != http.StatusNotFound && response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("legacy or arbitrary method still exposed: %s %s: %d", request.method, request.path, response.Code)
		}
	}
	response, envelope := resourceRequest(t, server, "DELETE", "/api/v1/subscriptions/default?replacement_group_id=other", "")
	if response.Code != http.StatusBadRequest || envelope.Code != "usage.invalid" {
		t.Fatalf("legacy replacement parameter accepted: %s", response.Body.String())
	}
}
