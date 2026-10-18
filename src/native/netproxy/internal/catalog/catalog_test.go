package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	json "encoding/json/v2"
)

func TestScanAndBuildRuntime(t *testing.T) {
	root := t.TempDir()
	writeGroup(t, root, "default", "同名资源", "local", "NODE")
	writeGroup(t, root, "remote", "同名资源", "subscription", "NODE")
	groups, err := Scan(t.Context(), ScanOptions{Root: root, WithNodes: true})
	if err != nil || len(groups) != 2 || groups[0].Group.ProviderTag != "np/default" || groups[1].Group.ProviderTag != "np/remote" {
		t.Fatalf("stable provider identities: %+v %v", groups, err)
	}
	output := filepath.Join(t.TempDir(), "providers.json")
	result, err := BuildRuntime(t.Context(), RuntimeOptions{Root: root, ProvidersOutput: output})
	if err != nil || result.GroupCount != 2 || result.NodeCount != 2 {
		t.Fatalf("runtime: %+v %v", result, err)
	}
	content := readFile(t, output)
	if strings.Contains(content, "\"outbounds\"") || !strings.Contains(content, "\"tag\": \"np/default\"") || !strings.Contains(content, "\"tag\": \"np/remote\"") {
		t.Fatalf("must only emit native Providers: %s", content)
	}
	if err := SetGroupName(t.Context(), root, "remote", "Renamed", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildRuntime(t.Context(), RuntimeOptions{Root: root, ProvidersOutput: output}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, output) != content {
		t.Fatal("renaming changed native Provider identity")
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
	if groups[0].Group.ProviderTag != "np/first" || groups[1].Group.ProviderTag != "np/second" {
		t.Fatalf("摘要扫描丢失名称消歧: %#v", groups)
	}
	if _, err := Scan(context.Background(), ScanOptions{Root: root, WithNodes: true}); err == nil {
		t.Fatal("节点详情扫描应拒绝损坏的 Provider")
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

func TestBuildRuntimeEmptyAndConflicts(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(t.TempDir(), "providers.json")
	result, err := BuildRuntime(t.Context(), RuntimeOptions{Root: root, ProvidersOutput: output})
	if err != nil || result.GroupCount != 0 {
		t.Fatalf("empty Catalog must be valid: %+v %v", result, err)
	}
	if !strings.Contains(readFile(t, output), "\"providers\": []") {
		t.Fatal("empty runtime must not invent an outbound")
	}
	writeGroup(t, root, "default", "Empty", "local", "NODE")
	if err := os.WriteFile(filepath.Join(root, "default", "provider.json"), []byte(`{"outbounds":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	updateNodeCount(t, filepath.Join(root, "default", "meta.json"), 0)
	result, err = BuildRuntime(t.Context(), RuntimeOptions{Root: root, ProvidersOutput: output})
	if err != nil || result.GroupCount != 0 {
		t.Fatalf("empty resource loaded: %+v %v", result, err)
	}
	writeGroup(t, root, "remote", "Remote", "subscription", "NODE")
	before := readFile(t, output)
	if _, err := BuildRuntime(t.Context(), RuntimeOptions{Root: root, ProvidersOutput: output, UserProviderTags: []string{"np/remote"}}); err == nil {
		t.Fatal("duplicate Provider tag accepted")
	}
	if readFile(t, output) != before {
		t.Fatal("conflict replaced valid runtime")
	}
	if err := os.WriteFile(filepath.Join(root, "remote", "provider.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildRuntime(t.Context(), RuntimeOptions{Root: root, ProvidersOutput: output}); err == nil {
		t.Fatal("broken Provider accepted")
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

func TestSchedule(t *testing.T) {
	root := t.TempDir()
	writeGroup(t, root, "due", "到期订阅", "subscription", "节点一")
	writeGroup(t, root, "future", "未来订阅", "subscription", "节点二")
	writeGroup(t, root, "local", "本地配置", "local", "节点三")
	updateSchedule(t, filepath.Join(root, "due", "meta.json"), true, 100)
	updateSchedule(t, filepath.Join(root, "future", "meta.json"), true, 300)

	result, err := Schedule(context.Background(), root, 200)
	if err != nil {
		t.Fatal(err)
	}
	if result.Nearest != 100 || len(result.Due) != 1 || result.Due[0] != "due" {
		t.Fatalf("unexpected schedule: %#v", result)
	}
	ids, err := GroupIDs(context.Background(), root, "subscription")
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
