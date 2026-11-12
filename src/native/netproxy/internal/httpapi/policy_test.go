package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func TestPolicyRoutesWhileCoreStopped(t *testing.T) {
	server := resourceFixture(t)
	resourceWriteFile(t, paths.SingBoxConfig(server.options.SingBoxDir), `{
		"outbounds": [
			{"type":"selector","tag":"Work","outbounds":["direct"],"providers":["external"],"default":"direct"},
			{"type":"direct","tag":"direct"}
		]
	}`)
	var snapshot service.PolicySnapshot
	const route = "/api/v1/policy-groups"
	resourceAssertSuccess(t, server, http.MethodGet, route, "", "policy.groups", &snapshot)
	if snapshot.Live || snapshot.CoreStartedAtMillis != 0 || len(snapshot.Groups) != 1 {
		t.Fatalf("unexpected configured snapshot: %+v", snapshot)
	}
	group := snapshot.Groups[0]
	if group.Tag != "Work" || group.Type != "selector" || group.Selectable || group.Selected != "" || group.ConfiguredDefault != "direct" ||
		len(group.Items) != 1 || group.Items[0].Tag != "direct" || group.Items[0].Type != "direct" ||
		len(group.UnavailableProviders) != 1 || group.UnavailableProviders[0] != "external" {
		t.Fatalf("unexpected configured group: %+v", group)
	}

	for _, test := range []struct {
		method, suffix, body string
		status               int
		code                 string
	}{
		{http.MethodGet, "?private=true", "", http.StatusBadRequest, "usage.invalid"},
		{http.MethodGet, "?source=live", "", http.StatusBadRequest, "usage.invalid"},
		{http.MethodGet, "?source=config&source=config", "", http.StatusBadRequest, "api.query_invalid"},
		{http.MethodPost, "/select", `{"group_tag":"Work","member_tag":"direct"}`, http.StatusBadRequest, "policy.invalid"},
		{http.MethodPost, "/select", `{"group_tag":"Work","member_tag":"direct","core_started_at_millis":1}`, http.StatusConflict, "policy.core_changed"},
		{http.MethodPost, "/delay", `{"group_tag":"Work","core_started_at_millis":1}`, http.StatusConflict, "policy.core_changed"},
		{http.MethodPost, "/delay", `{"group_tag":"Work","core_started_at_millis":1,"member_tag":"direct"}`, http.StatusBadRequest, "usage.invalid"},
	} {
		recorder, result := resourceRequest(t, server, test.method, route+test.suffix, test.body)
		if recorder.Code != test.status || result.Code != test.code {
			t.Errorf("%s %s: got %d %s, want %d %s", test.method, test.suffix, recorder.Code, result.Code, test.status, test.code)
		}
	}
	request := httptest.NewRequest(http.MethodGet, route, nil)
	request.Host = server.config.Listen
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated policy read returned %d", recorder.Code)
	}
}

func TestConfiguredPolicyRouteIgnoresUnavailableReadyCore(t *testing.T) {
	server := resourceFixture(t)
	resourceWriteFile(t, paths.SingBoxConfig(server.options.SingBoxDir), `{
		"outbounds": [
			{"type":"selector","tag":"Disk","outbounds":["direct"],"providers":["np/default"]},
			{"type":"direct","tag":"direct"}
		]
	}`)
	resourceWriteFile(t, server.options.StateFile, `{"state":"ready","pid":1,"core_started_at_millis":1234}`)
	const route = "/api/v1/policy-groups"
	recorder, _ := resourceRequest(t, server, http.MethodGet, route, "")
	if recorder.Code == http.StatusOK {
		t.Fatal("live route unexpectedly found a usable core")
	}
	var snapshot service.PolicySnapshot
	resourceAssertSuccess(t, server, http.MethodGet, route+"?source=config", "", "policy.groups", &snapshot)
	if snapshot.Live || snapshot.CoreStartedAtMillis != 0 || len(snapshot.Groups) != 1 ||
		snapshot.Groups[0].Tag != "Disk" || len(snapshot.Groups[0].Items) != 2 {
		t.Fatalf("configured view did not read the local config and Catalog: %+v", snapshot)
	}
}

func TestConfiguredPolicyExpandsOnlyReferencedCatalogProviders(t *testing.T) {
	server := resourceFixture(t)
	root := server.options.CatalogRoot
	if err := catalog.EnsureGroup(t.Context(), catalog.GroupOptions{
		Root: root, GroupID: "other", Name: "Other", Type: "local",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{
		GroupDir: filepath.Join(root, "other"), GroupID: "other", Type: "local",
		Input: "socks://127.0.0.1:1081#Same",
	}); err != nil {
		t.Fatal(err)
	}
	defaultTag, err := catalog.RuntimeTag(t.Context(), root, "default")
	if err != nil {
		t.Fatal(err)
	}
	config := resourceJSON(t, map[string]any{
		"providers": []map[string]any{{"type": "remote", "tag": "remote", "url": "https://invalid.invalid/never"}},
		"outbounds": []map[string]any{
			{"type": "direct", "tag": "direct"},
			{"type": "selector", "tag": "Work", "outbounds": []string{"direct", "Auto"}, "providers": []string{defaultTag}, "default": "direct"},
			{"type": "urltest", "tag": "Auto", "use_all_providers": true, "providers": []string{"ignored"}, "include": "/Same$"},
		},
	})
	path := paths.SingBoxConfig(server.options.SingBoxDir)
	resourceWriteFile(t, path, config)
	var snapshot service.PolicySnapshot
	resourceAssertSuccess(t, server, http.MethodGet, "/api/v1/policy-groups", "", "policy.groups", &snapshot)
	if snapshot.Live || snapshot.CoreStartedAtMillis != 0 || len(snapshot.Groups) != 2 {
		t.Fatalf("unexpected offline snapshot: %+v", snapshot)
	}
	work, auto := snapshot.Groups[0], snapshot.Groups[1]
	otherTag, err := catalog.RuntimeTag(t.Context(), root, "other")
	if err != nil {
		t.Fatal(err)
	}
	if work.Selectable || work.Selected != "" || work.ConfiguredDefault != "direct" ||
		len(work.Items) != 3 || work.Items[1].Tag != "Auto" || work.Items[2].Tag != defaultTag+"/节点 / 02" {
		t.Fatalf("wrong configured selector: %+v", work)
	}
	if auto.Selected != "" || len(auto.Items) != 1 || auto.Items[0].Tag != otherTag+"/Same" ||
		!slices.Equal(auto.UnavailableProviders, []string{"remote"}) {
		t.Fatalf("wrong filtered URLTest: %+v", auto)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != config {
		t.Fatalf("offline read changed native config: %v", err)
	}
}
