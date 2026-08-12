package module

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

func TestNodeImportAppendsToDefaultGroup(t *testing.T) {
	root := t.TempDir()
	options := NewOptions(root)
	if err := os.MkdirAll(filepath.Dir(options.ModuleConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("ACTIVE_GROUP_ID=default\nSELECTOR_MODE=urltest\nSELECTED_NODE_REF=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ImportGroup(context.Background(), catalog.ImportOptions{
		Root: options.CatalogRoot, GroupID: "default", Name: "已有本地配置", Input: "socks://existing.example:1080#EXISTING",
	}); err != nil {
		t.Fatalf("initialize default group: %v", err)
	}
	input := filepath.Join(root, "selected-nodes.yaml")
	if err := os.WriteFile(input, []byte("socks://one.example:1081#IMPORTED\nsocks://two.example:1082#IMPORTED_TWO\n"), 0o600); err != nil {
		t.Fatalf("write node file: %v", err)
	}

	result, err := NodeImport(context.Background(), options, input, false)
	if err != nil {
		t.Fatalf("import nodes: %v", err)
	}
	if result.GroupID != "default" || result.NodeCount != 3 || result.Revision != 2 {
		t.Fatalf("unexpected import result: %+v", result)
	}
	ids, err := catalog.GroupIDs(options.CatalogRoot, "all")
	if err != nil {
		t.Fatalf("list catalog groups: %v", err)
	}
	if len(ids) != 1 || ids[0] != "default" {
		t.Fatalf("unexpected groups after import: %v", ids)
	}
	document, err := provider.Load(context.Background(), filepath.Join(options.CatalogRoot, "default", "provider.json"))
	if err != nil {
		t.Fatalf("load default provider: %v", err)
	}
	if got := len(provider.Inspect(document)); got != 3 {
		t.Fatalf("default node count = %d, want 3", got)
	}
	metadata, err := catalog.LoadMetadata(filepath.Join(options.CatalogRoot, "default", "meta.json"), "default")
	if err != nil {
		t.Fatalf("load default metadata: %v", err)
	}
	if metadata.Name != "已有本地配置" {
		t.Fatalf("default group name changed unexpectedly: %q", metadata.Name)
	}
}
