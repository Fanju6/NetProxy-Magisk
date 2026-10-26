package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func TestCatalogCandidateCollisionHTTPConflictKeepsProvider(t *testing.T) {
	server := resourceFixture(t)
	resourceWriteFile(t, paths.SingBoxConfig(server.options.SingBoxDir), `{"outbounds":[{"type":"direct","tag":"np/default/NEW"}]}`)
	providerPath := filepath.Join(server.options.CatalogRoot, "default", "provider.json")
	metaPath := filepath.Join(server.options.CatalogRoot, "default", "meta.json")
	beforeProvider, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeMeta, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	response, envelope := resourceRequest(t, server, "POST", "/api/v1/nodes", `{"group_id":"default","content":"socks://127.0.0.1:1080#NEW"}`)
	if response.Code != http.StatusConflict || envelope.Code != "catalog.outbound_conflict" || envelope.OK {
		t.Fatalf("candidate conflict lost HTTP status or business code: %s", response.Body.String())
	}
	afterProvider, providerErr := os.ReadFile(providerPath)
	afterMeta, metaErr := os.ReadFile(metaPath)
	if providerErr != nil || metaErr != nil || !bytes.Equal(beforeProvider, afterProvider) || !bytes.Equal(beforeMeta, afterMeta) {
		t.Fatal("rejected HTTP candidate changed Provider or revision")
	}
}

func TestCatalogGroupsHTTPFixedCRUDKeepsProviderIdentity(t *testing.T) {
	server := resourceFixture(t)
	var created map[string]string
	resourceAssertSuccess(t, server, "POST", "/api/v1/catalog", `{"id":"manual","name":"Local / 资源"}`, "catalog.created", &created)
	if len(created) != 1 || created["id"] != "manual" {
		t.Fatalf("unexpected create DTO: %+v", created)
	}
	before, err := catalog.ReadGroupSummary(t.Context(), server.options.CatalogRoot, "manual", server.options.ProgressDir)
	if err != nil {
		t.Fatal(err)
	}
	resourceAssertSuccess(t, server, "PUT", "/api/v1/catalog/manual", `{"name":"Renamed / 资源"}`, "catalog.renamed", nil)
	after, err := catalog.ReadGroupSummary(t.Context(), server.options.CatalogRoot, "manual", server.options.ProgressDir)
	if err != nil || after.Name != "Renamed / 资源" || before.ProviderTag != after.ProviderTag || after.ProviderTag != catalog.ProviderTag("manual") {
		t.Fatalf("rename changed provider identity: %+v -> %+v %v", before, after, err)
	}
	var snapshot struct {
		Groups []catalog.GroupSnapshot `json:"groups"`
	}
	resourceAssertSuccess(t, server, "GET", "/api/v1/catalog", "", "catalog.groups", &snapshot)
	if len(snapshot.Groups) != 2 {
		t.Fatalf("catalog snapshot = %+v", snapshot)
	}
	resourceAssertSuccess(t, server, "DELETE", "/api/v1/catalog/manual", "", "catalog.removed", nil)
	if _, err := os.Stat(filepath.Join(server.options.CatalogRoot, "manual")); !os.IsNotExist(err) {
		t.Fatalf("local group remains: %v", err)
	}
	for _, path := range []string{server.options.StateFile, server.options.WorkerPIDFile, server.options.RuntimeDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("local CRUD started runtime: %s %v", path, err)
		}
	}
}

func TestCatalogGroupsHTTPRejectsUnknownFieldsPrivateAndPaths(t *testing.T) {
	server := resourceFixture(t)
	for _, request := range []struct{ method, target, body string }{
		{"POST", "/api/v1/catalog", `{"id":"manual","name":"Local","url":"https://example.test/sub"}`},
		{"POST", "/api/v1/catalog", `{"id":"manual","name":"Local","type":"subscription"}`},
		{"POST", "/api/v1/catalog", `{"id":"manual","name":"Local","private":true}`},
		{"POST", "/api/v1/catalog", `{"id":"manual","name":"Local","path":"/data/secret"}`},
		{"POST", "/api/v1/catalog", `{"id":"manual","id":"other","name":"Local"}`},
		{"POST", "/api/v1/catalog", `{"id":"../secret","name":"Local"}`},
		{"POST", "/api/v1/catalog", `{"id":"staging","name":"Local"}`},
		{"POST", "/api/v1/catalog", `{"id":"a/b","name":"Local"}`},
		{"POST", "/api/v1/catalog", `{"id":"a\\b","name":"Local"}`},
		{"POST", "/api/v1/catalog?private=true", `{"id":"manual","name":"Local"}`},
		{"PUT", "/api/v1/catalog/default", `{"name":"Local","id":"renamed-id"}`},
		{"PUT", "/api/v1/catalog/default", `{"name":"Local","private":true}`},
		{"PUT", "/api/v1/catalog/default", `{"name":"Local","url":"https://example.test/sub"}`},
		{"PUT", "/api/v1/catalog/default?source=/data/secret", `{"name":"Local"}`},
		{"PUT", "/api/v1/catalog/a%2Fb", `{"name":"Local"}`},
		{"DELETE", "/api/v1/catalog/default", ""},
		{"DELETE", "/api/v1/catalog/manual?private=true", ""},
		{"DELETE", "/api/v1/catalog/manual?replacement_group_id=other", ""},
		{"DELETE", "/api/v1/catalog/manual", `{"path":"/data/secret"}`},
	} {
		response, envelope := resourceRequest(t, server, request.method, request.target, request.body)
		if response.Code != http.StatusBadRequest || envelope.Code != "usage.invalid" {
			t.Fatalf("unsafe catalog request accepted: %s %s: %s", request.method, request.target, response.Body.String())
		}
	}
	if _, err := os.Stat(filepath.Join(server.options.CatalogRoot, "manual")); !os.IsNotExist(err) {
		t.Fatal("rejected request created local group")
	}
}

func TestCatalogGroupsHTTPInvalidNamesAndSubscriptionReadOnly(t *testing.T) {
	server := resourceFixture(t)
	for _, name := range []string{"", " ", "bad\nname", strings.Repeat("x", 257)} {
		for _, method := range []string{"POST", "PUT"} {
			target := "/api/v1/catalog"
			body := map[string]string{"id": "manual", "name": name}
			if method == "PUT" {
				target += "/default"
				delete(body, "id")
			}
			response, envelope := resourceRequest(t, server, method, target, resourceJSON(t, body))
			if response.Code != http.StatusUnprocessableEntity || envelope.Code != "catalog.invalid" {
				t.Fatalf("invalid name accepted: %s", response.Body.String())
			}
		}
	}
	resourceSubscriptionFixture(t, server, "readonly-sub", "https://example.test/sub?token=private-test-token")
	for _, method := range []string{"PUT", "DELETE"} {
		body := ""
		if method == "PUT" {
			body = `{"name":"Wrong endpoint"}`
		}
		response, envelope := resourceRequest(t, server, method, "/api/v1/catalog/readonly-sub", body)
		if response.Code != http.StatusUnprocessableEntity || envelope.Code != "catalog.read_only" || strings.Contains(response.Body.String(), "private-test-token") {
			t.Fatalf("subscription changed through local endpoint: %s", response.Body.String())
		}
	}
	metadata, err := catalog.PrivateMetadata(t.Context(), server.options.CatalogRoot, "readonly-sub")
	if err != nil || !strings.Contains(metadata.URL, "private-test-token") || metadata.Name != "匿名订阅" {
		t.Fatalf("read-only group changed: %+v %v", metadata, err)
	}
}

func TestCatalogGroupsHTTPInUseRejectionAndPublicErrorProjection(t *testing.T) {
	server := resourceFixture(t)
	resourceAssertSuccess(t, server, "POST", "/api/v1/catalog", `{"id":"guarded","name":"Guarded"}`, "catalog.created", nil)
	resourceWriteFile(t, paths.SingBoxConfig(server.options.SingBoxDir), `{"outbounds":[{"type":"selector","tag":"Main","providers":["np/guarded"]}],"route":{"final":"Main"}}`)
	response, envelope := resourceRequest(t, server, "DELETE", "/api/v1/catalog/guarded", "")
	if response.Code != http.StatusConflict || envelope.Code != "catalog.provider_in_use" {
		t.Fatalf("in-use Provider deleted: %s", response.Body.String())
	}
	if _, err := catalog.PrivateMetadata(t.Context(), server.options.CatalogRoot, "guarded"); err != nil {
		t.Fatalf("rejected delete persisted: %v", err)
	}
	for _, code := range []string{"catalog.provider_conflict", "policy.runtime_sync_failed"} {
		response := httptest.NewRecorder()
		server.fail(response, t.Context(), &service.Error{Code: code, Message: "private exception /data/secret",
			Data: map[string]any{"cause": "private-token", "path": "/data/secret", "accepted": true, "core_started_at_millis": int64(123)}})
		if strings.Contains(response.Body.String(), "private") || strings.Contains(response.Body.String(), "/data/") || strings.Contains(response.Body.String(), "cause") || !strings.Contains(response.Body.String(), `"accepted":true`) {
			t.Fatalf("private error exposed or acceptance lost: %s", response.Body.String())
		}
	}
}
