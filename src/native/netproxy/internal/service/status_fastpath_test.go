package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

func writeStatusGroup(t *testing.T, root, groupID, name string, nodeCount int, providerContent []byte) {
	t.Helper()
	groupDir := filepath.Join(root, groupID)
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := catalog.NewMetadata(groupID, name, "local", "", time.Now())
	metadata.NodeCount = nodeCount
	if err := catalog.SaveMetadataAtomic(context.Background(), filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), providerContent, 0o600); err != nil {
		t.Fatal(err)
	}
}

func statusFixtureProvider(tag string) []byte {
	return []byte(`{"outbounds":[{"type":"socks","tag":"` + tag + `","server":"example.com","server_port":1080}]}`)
}

func TestReadGroupSummaryUsesNameAsRuntimeTag(t *testing.T) {
	temp := t.TempDir()
	catalogRoot := filepath.Join(temp, "catalog")
	if err := os.MkdirAll(catalogRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeStatusGroup(t, catalogRoot, "default", "同名分组", 1, statusFixtureProvider("LOCAL"))
	writeStatusGroup(t, catalogRoot, "remote", "同名分组", 1, statusFixtureProvider("REMOTE"))

	summary, err := catalog.ReadGroupSummary(context.Background(), catalogRoot, "default", "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.RuntimeTag != "同名分组" || summary.NodeCount != 1 {
		t.Fatalf("RuntimeTag 或摘要节点数错误: %#v", summary)
	}
}

func TestReadNodesIgnoresUnrequestedProvider(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"wanted", "unrelated"} {
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		metadata := catalog.NewMetadata(id, "Group "+id, "local", "", time.Now())
		metadata.NodeCount = 1
		if err := catalog.SaveMetadataAtomic(t.Context(), filepath.Join(dir, "meta.json"), metadata); err != nil {
			t.Fatal(err)
		}
		document := []byte(`{"outbounds":[{"type":"socks","tag":"node","server":"127.0.0.1","server_port":1080}]}`)
		if err := provider.WriteAtomic(filepath.Join(dir, "provider.json"), document, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated", "provider.json"), []byte(`{"outbounds":[`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Scan(t.Context(), catalog.ScanOptions{Root: root, WithNodes: true, GroupID: "wanted"}); err != nil {
		t.Fatalf("targeted catalog scan failed: %v", err)
	}
	options := Options{CatalogRoot: root}
	for _, query := range []string{"wanted", "Group wanted"} {
		groups, err := ReadNodes(t.Context(), options, query)
		if err != nil || len(groups) != 1 || groups[0].Group.ID != "wanted" || len(groups[0].Nodes) != 1 {
			t.Fatalf("目标分组读取错误: %+v %v", groups, err)
		}
	}
	if _, err := ReadNodes(t.Context(), options, "unrelated"); err == nil {
		t.Fatal("目标 Provider 损坏未被发现")
	}
}
