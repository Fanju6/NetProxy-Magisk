package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"encoding/json/jsontext"
	json "encoding/json/v2"
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
	result, err := BuildRuntime(context.Background(), RuntimeOptions{
		Root: root, ProvidersOutput: providersPath, OutboundsOutput: outboundsPath,
		ActiveGroup: "remote", SelectorMode: "manual",
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
	state := "selected_node_ref\t" + result.SelectedNodeRef
	for _, expected := range []string{`"tag": "同名分组 [default]"`, `"tag": "同名分组 [remote]"`} {
		if !strings.Contains(providers, expected) {
			t.Fatalf("providers missing %s: %s", expected, providers)
		}
	}
	if !strings.Contains(outbounds, `"default": "Select/同名分组 [remote]"`) {
		t.Fatalf("unexpected outbounds: %s", outbounds)
	}
	assertRuntimeGroupSources(t, outbounds)
	if !strings.Contains(state, "selected_node_ref\tremote/订阅节点") {
		t.Fatalf("unexpected state: %s", state)
	}
}

func assertRuntimeGroupSources(t *testing.T, content string) {
	t.Helper()
	var document struct {
		Outbounds []map[string]jsontext.Value `json:"outbounds"`
	}
	if err := json.Unmarshal([]byte(content), &document); err != nil {
		t.Fatalf("parse runtime outbounds: %v", err)
	}
	for _, outbound := range document.Outbounds {
		var tag string
		if err := json.Unmarshal(outbound["tag"], &tag); err != nil {
			t.Fatalf("parse runtime outbound tag: %v", err)
		}
		_, hasOutbounds := outbound["outbounds"]
		_, hasProviders := outbound["providers"]
		switch {
		case tag == "Proxy":
			if !hasOutbounds || hasProviders {
				t.Fatalf("Proxy should only contain outbounds: %s", content)
			}
		case strings.HasPrefix(tag, "Auto/") || strings.HasPrefix(tag, "Select/"):
			if hasOutbounds || !hasProviders {
				t.Fatalf("provider group %q should only contain providers: %s", tag, content)
			}
		}
	}
}

func TestScanSummaryUsesMetadataWithoutParsingProvider(t *testing.T) {
	root := t.TempDir()
	writeGroup(t, root, "first", "同名分组", "local", "节点一")
	writeGroup(t, root, "second", "同名分组", "subscription", "节点二")
	updateNodeCount(t, filepath.Join(root, "first", "meta.json"), 7)
	updateNodeCount(t, filepath.Join(root, "second", "meta.json"), 9)
	if err := os.WriteFile(filepath.Join(root, "second", "provider.json"), []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}

	groups, err := Scan(context.Background(), ScanOptions{Root: root, WithNodes: false})
	if err != nil {
		t.Fatalf("摘要扫描不应解析 Provider: %v", err)
	}
	if len(groups) != 2 || groups[0].Group.NodeCount != 7 || groups[1].Group.NodeCount != 9 {
		t.Fatalf("摘要未使用 metadata 节点数: %#v", groups)
	}
	if groups[0].Group.RuntimeTag != "同名分组 [first]" || groups[1].Group.RuntimeTag != "同名分组 [second]" {
		t.Fatalf("摘要扫描丢失名称消歧: %#v", groups)
	}
	if _, err := Scan(context.Background(), ScanOptions{Root: root, WithNodes: true}); err == nil {
		t.Fatal("节点详情扫描应拒绝损坏的 Provider")
	}
}

func TestRuntimeTagIgnoresEmptyDuplicateGroup(t *testing.T) {
	root := t.TempDir()
	writeGroup(t, root, "ready", "同名分组", "subscription", "可用节点")
	writeGroup(t, root, "empty", "同名分组", "subscription", "待更新节点")
	if err := os.WriteFile(filepath.Join(root, "empty", "provider.json"), []byte(`{"outbounds":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	updateNodeCount(t, filepath.Join(root, "empty", "meta.json"), 0)

	groups, err := Scan(context.Background(), ScanOptions{Root: root, WithNodes: false})
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range groups {
		if group.Group.RuntimeTag != "同名分组" {
			t.Fatalf("empty duplicate changed summary RuntimeTag: %#v", groups)
		}
	}
	runtimeTag, err := RuntimeTag(root, "ready")
	if err != nil || runtimeTag != "同名分组" {
		t.Fatalf("RuntimeTag = %q, err=%v", runtimeTag, err)
	}

	runtimeDir := t.TempDir()
	providersPath := filepath.Join(runtimeDir, "providers.json")
	result, err := BuildRuntime(context.Background(), RuntimeOptions{
		Root: root, ProvidersOutput: providersPath,
		OutboundsOutput: filepath.Join(runtimeDir, "outbounds.json"), ActiveGroup: "ready",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.GroupCount != 1 || result.ActiveGroupTag != runtimeTag {
		t.Fatalf("runtime result and lookup disagree: result=%+v tag=%q", result, runtimeTag)
	}
	providers := readFile(t, providersPath)
	if !strings.Contains(providers, `"tag": "同名分组"`) || strings.Contains(providers, `"tag": "同名分组 [ready]"`) {
		t.Fatalf("runtime providers used a different tag: %s", providers)
	}

	if err := os.WriteFile(filepath.Join(root, "empty", "provider.json"), []byte(`{"outbounds":[{"type":"socks","tag":"新节点","server":"example.com","server_port":1080}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	updateNodeCount(t, filepath.Join(root, "empty", "meta.json"), 1)
	runtimeTag, err = RuntimeTag(root, "ready")
	if err != nil || runtimeTag != "同名分组 [ready]" {
		t.Fatalf("non-empty duplicate RuntimeTag = %q, err=%v", runtimeTag, err)
	}
	result, err = BuildRuntime(context.Background(), RuntimeOptions{
		Root: root, ProvidersOutput: providersPath,
		OutboundsOutput: filepath.Join(runtimeDir, "outbounds.json"), ActiveGroup: "ready",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.GroupCount != 2 || result.ActiveGroupTag != runtimeTag {
		t.Fatalf("runtime did not adopt duplicate tags after the group gained nodes: result=%+v tag=%q", result, runtimeTag)
	}
}

func BenchmarkScanSummary(b *testing.B) {
	root := b.TempDir()
	for groupIndex := range 40 {
		id := fmt.Sprintf("group-%02d", groupIndex)
		directory := filepath.Join(root, id)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			b.Fatal(err)
		}
		metadata, err := json.Marshal(map[string]any{
			"id": id, "name": id, "type": "subscription", "revision": 1,
			"node_count": 250, "update_interval": 86400, "update_via_proxy": "auto",
		}, json.Deterministic(true))
		if err != nil {
			b.Fatal(err)
		}
		nodes := make([]map[string]any, 250)
		for nodeIndex := range nodes {
			nodes[nodeIndex] = map[string]any{
				"type": "socks", "tag": fmt.Sprintf("node-%03d", nodeIndex),
				"server": "example.com", "server_port": 1080,
			}
		}
		providerDocument, err := json.Marshal(map[string]any{"outbounds": nodes}, json.Deterministic(true))
		if err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "meta.json"), metadata, 0o600); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "provider.json"), providerDocument, 0o600); err != nil {
			b.Fatal(err)
		}
	}

	b.Run("summary", func(b *testing.B) {
		for range b.N {
			if _, err := Scan(context.Background(), ScanOptions{Root: root, WithNodes: false}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("with-nodes", func(b *testing.B) {
		for range b.N {
			if _, err := Scan(context.Background(), ScanOptions{Root: root, WithNodes: true}); err != nil {
				b.Fatal(err)
			}
		}
	})
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

func TestTargetedScanDoesNotParseOtherProviders(t *testing.T) {
	root := t.TempDir()
	writeGroup(t, root, "default", "本地配置", "local", "LOCAL")
	writeGroup(t, root, "remote", "远程订阅", "subscription", "REMOTE")
	if err := os.WriteFile(filepath.Join(root, "remote", "provider.json"), []byte(`{"outbounds":[`), 0o600); err != nil {
		t.Fatal(err)
	}

	groups, err := Scan(context.Background(), ScanOptions{Root: root, GroupID: "default", WithNodes: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Nodes) != 1 || groups[0].Nodes[0].Tag != "LOCAL" {
		t.Fatalf("unexpected targeted scan: %#v", groups)
	}
}

func TestBuildRuntimeUsesMetadataAndOnlyChecksManualTarget(t *testing.T) {
	root := t.TempDir()
	writeGroup(t, root, "default", "本地配置", "local", "LOCAL")
	writeGroup(t, root, "remote", "远程订阅", "subscription", "REMOTE")
	if err := os.WriteFile(filepath.Join(root, "remote", "provider.json"), []byte(`{"outbounds":[`), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := BuildRuntime(context.Background(), RuntimeOptions{
		Root: root, ProvidersOutput: filepath.Join(root, "providers.json"),
		OutboundsOutput: filepath.Join(root, "outbounds.json"), ActiveGroup: "default",
		SelectorMode: "manual", SelectedNodeRef: "default/LOCAL",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SelectedNodeRef != "default/LOCAL" || result.NodeCount != 2 {
		t.Fatalf("unexpected runtime summary: %#v", result)
	}
}

func TestBuildRuntimeRejectsUnknownSelectorMode(t *testing.T) {
	root := t.TempDir()
	writeGroup(t, root, "default", "本地配置", "local", "NODE")
	_, err := BuildRuntime(context.Background(), RuntimeOptions{
		Root: root, ProvidersOutput: filepath.Join(root, "providers.json"),
		OutboundsOutput: filepath.Join(root, "outbounds.json"), SelectorMode: "selector",
	})
	if err == nil || !strings.Contains(err.Error(), "未知节点选择模式") {
		t.Fatalf("未知选择模式未被拒绝: %v", err)
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
		"node_count": 1, "update_interval": 86400, "update_via_proxy": "auto",
	}, json.Deterministic(true))
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
	content, err = json.Marshal(metadata, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func updateNodeCount(t *testing.T, path string, nodeCount int) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(content, &metadata); err != nil {
		t.Fatal(err)
	}
	metadata["node_count"] = nodeCount
	content, err = json.Marshal(metadata, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}
