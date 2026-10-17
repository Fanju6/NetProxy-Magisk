package httpapi

import (
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
)

func TestUpdateAllPreservesPartialOutcomesOnFailure(t *testing.T) {
	server := resourceFixture(t)
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"outbounds":[{"type":"socks","tag":"fixture","server":"127.0.0.1","server_port":9,"version":"5"}]}`))
	}))
	defer good.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer bad.Close()
	for id, source := range map[string]string{"fixture-good": good.URL, "fixture-bad": bad.URL} {
		if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{Root: server.options.CatalogRoot,
			GroupID: id, Name: id, Type: "subscription", URL: source, AutoUpdate: false,
			UpdateInterval: 900, UpdateViaProxy: "never", Timeout: 2}); err != nil {
			t.Fatal(err)
		}
	}
	response, result := resourceRequest(t, server, "POST", "/api/v1/subscriptions/update-all", "")
	if response.Code != http.StatusInternalServerError || result.OK || result.Code != "subscription.convert_failed" {
		t.Fatalf("wrong failure envelope: status=%d code=%s", response.Code, result.Code)
	}
	var data struct {
		Updated    []string       `json:"updated"`
		Failed     []string       `json:"failed"`
		FirstError map[string]any `json:"first_error"`
	}
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Updated) != 1 || data.Updated[0] != "fixture-good" ||
		len(data.Failed) != 1 || data.Failed[0] != "fixture-bad" || data.FirstError == nil {
		t.Fatalf("lost independent outcomes: %+v", data)
	}
}

func TestUpdateAllPreservesPersistedFailureFacts(t *testing.T) {
	server := resourceFixture(t)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"outbounds":[{"type":"socks","tag":"fixture","server":"127.0.0.1","server_port":9,"version":"5"}]}`))
	}))
	defer source.Close()
	if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{Root: server.options.CatalogRoot,
		GroupID: "fixture-persisted", Name: "Fixture", Type: "subscription", URL: source.URL,
		UpdateInterval: 900, UpdateViaProxy: "never", Timeout: 2}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(server.options.CatalogRoot, "fixture-persisted", "history.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, result := resourceRequest(t, server, "POST", "/api/v1/subscriptions/update-all", "")
	var data struct {
		Persisted  bool     `json:"persisted"`
		Updated    []string `json:"updated"`
		Failed     []string `json:"failed"`
		FirstError struct {
			Persisted bool `json:"persisted"`
		} `json:"first_error"`
	}
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Code != "subscription.history_write_failed" || !data.Persisted || !data.FirstError.Persisted ||
		len(data.Updated) != 0 || len(data.Failed) != 1 || data.Failed[0] != "fixture-persisted" {
		t.Fatalf("lost persisted outcome: code=%s data=%+v", result.Code, data)
	}
}
