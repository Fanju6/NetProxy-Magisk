package catalog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanAndBuildRuntime(t *testing.T) {
	root := t.TempDir()
	writeGroup(t, root, "default", "同名分组", "local", "本地节点")
	writeGroup(t, root, "remote", "同名分组", "subscription", "订阅节点")
	progressDir := filepath.Join(t.TempDir(), "progress")
	if err := os.MkdirAll(progressDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(progressDir, "remote.progress.json"), []byte(`{"stage":"convert","current":1}`), 0o600); err != nil {
		t.Fatal(err)
	}

	groups, err := Scan(context.Background(), ScanOptions{
		Root: root, ActiveGroup: "remote", ProgressDir: progressDir, WithNodes: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || len(groups[0].Nodes) != 1 || !groups[1].Group.Active {
		t.Fatalf("unexpected groups: %#v", groups)
	}
	if groups[0].Group.RuntimeTag != "同名分组 [default]" || groups[1].Group.RuntimeTag != "同名分组 [remote]" {
		t.Fatalf("unexpected runtime tags: %#v", groups)
	}
	if string(groups[1].Group.Progress) != `{"stage":"convert","current":1}` {
		t.Fatalf("unexpected progress: %s", groups[1].Group.Progress)
	}

	providersPath := filepath.Join(root, "runtime", "providers.json")
	outboundsPath := filepath.Join(root, "runtime", "outbounds.json")
	statePath := filepath.Join(root, "runtime", "catalog.state")
	result, err := BuildRuntime(context.Background(), RuntimeOptions{
		Root: root, ProvidersOutput: providersPath, OutboundsOutput: outboundsPath,
		StateOutput: statePath, ActiveGroup: "remote", SelectorMode: "manual",
		SelectedNodeRef: "remote/订阅节点",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.GroupCount != 2 || result.NodeCount != 2 || result.SelectorMode != "manual" {
		t.Fatalf("unexpected runtime result: %#v", result)
	}
	providers := readFile(t, providersPath)
	outbounds := readFile(t, outboundsPath)
	state := readFile(t, statePath)
	for _, expected := range []string{`"tag": "同名分组 [default]"`, `"tag": "同名分组 [remote]"`} {
		if !strings.Contains(providers, expected) {
			t.Fatalf("providers missing %s: %s", expected, providers)
		}
	}
	if !strings.Contains(outbounds, `"default": "Select/同名分组 [remote]"`) {
		t.Fatalf("unexpected outbounds: %s", outbounds)
	}
	if !strings.Contains(state, "selected_node_ref\tremote/订阅节点") {
		t.Fatalf("unexpected state: %s", state)
	}
}

func TestBuildRuntimeFallbackAndEmpty(t *testing.T) {
	root := t.TempDir()
	writeGroup(t, root, "default", "本地配置", "local", "节点")
	result, err := BuildRuntime(context.Background(), RuntimeOptions{
		Root: root, ProvidersOutput: filepath.Join(root, "providers.json"),
		OutboundsOutput: filepath.Join(root, "outbounds.json"),
		ActiveGroup:     "missing", SelectorMode: "manual", SelectedNodeRef: "missing/节点",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ActiveGroup != "default" || result.SelectorMode != "urltest" || result.SelectedNodeRef != "" {
		t.Fatalf("selection was not normalized: %#v", result)
	}

	emptyRoot := t.TempDir()
	if _, err := BuildRuntime(context.Background(), RuntimeOptions{
		Root: emptyRoot, ProvidersOutput: filepath.Join(emptyRoot, "providers.json"),
		OutboundsOutput: filepath.Join(emptyRoot, "outbounds.json"),
	}); err == nil {
		t.Fatal("expected empty Catalog to fail")
	}
	if _, err := BuildRuntime(context.Background(), RuntimeOptions{
		Root: emptyRoot, ProvidersOutput: filepath.Join(emptyRoot, "providers.json"),
		OutboundsOutput: filepath.Join(emptyRoot, "outbounds.json"), AllowEmpty: true,
	}); err != nil {
		t.Fatalf("allow-empty failed: %v", err)
	}
}

func TestSchedule(t *testing.T) {
	root := t.TempDir()
	writeGroup(t, root, "due", "到期订阅", "subscription", "节点一")
	writeGroup(t, root, "future", "未来订阅", "subscription", "节点二")
	writeGroup(t, root, "local", "本地配置", "local", "节点三")
	updateSchedule(t, filepath.Join(root, "due", "meta.json"), true, 100)
	updateSchedule(t, filepath.Join(root, "future", "meta.json"), true, 300)

	result, err := Schedule(root, 200)
	if err != nil {
		t.Fatal(err)
	}
	if result.Nearest != 100 || len(result.Due) != 1 || result.Due[0] != "due" {
		t.Fatalf("unexpected schedule: %#v", result)
	}
	ids, err := GroupIDs(root, "subscription")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "due" || ids[1] != "future" {
		t.Fatalf("unexpected subscription ids: %#v", ids)
	}
}

func writeGroup(t *testing.T, root, id, name, groupType, tag string) {
	t.Helper()
	directory := filepath.Join(root, id)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(map[string]any{
		"id": id, "name": name, "type": groupType, "revision": 1,
		"update_interval": 86400, "update_via_proxy": "auto",
	})
	if err != nil {
		t.Fatal(err)
	}
	providerDocument := `{"outbounds":[{"type":"socks","tag":"` + tag + `","server":"example.com","server_port":1080}]}`
	if err := os.WriteFile(filepath.Join(directory, "meta.json"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "provider.json"), []byte(providerDocument), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func updateSchedule(t *testing.T, path string, enabled bool, epoch int64) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(content, &metadata); err != nil {
		t.Fatal(err)
	}
	metadata["auto_update"] = enabled
	metadata["next_update_epoch"] = epoch
	content, err = json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}
