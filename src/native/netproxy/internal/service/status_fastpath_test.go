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

func TestReadStatusDoesNotReadCorruptCatalog(t *testing.T) {
	temp := t.TempDir()
	root := filepath.Join(temp, "catalog")
	writeStatusGroup(t, root, "default", "Local", 1, []byte(`{"outbounds":[`))
	status, err := ReadStatus(t.Context(), Options{
		SingBoxConfig: modeConfigFixture(t, temp, "Rule"),
		ModuleConfig:  filepath.Join(temp, "missing-module.json"),
		CatalogRoot:   root, SingBoxPath: filepath.Join(temp, "missing-sing-box"),
	})
	if err != nil || status.State != "stopped" || status.Error != "" {
		t.Fatalf("Catalog affected service status: %#v, %v", status, err)
	}
}

func TestSnapshotIgnoresUnrequestedProvider(t *testing.T) {
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
	module := filepath.Join(t.TempDir(), "module.json")
	options := Options{CatalogRoot: root, ModuleConfig: module, SingBoxPath: filepath.Join(t.TempDir(), "not-running-singbox")}
	for _, query := range []string{"wanted", "Group wanted"} {
		snapshot, err := ReadSnapshot(t.Context(), options, query)
		if err != nil || len(snapshot.Groups) != 1 || snapshot.Groups[0].Group.ID != "wanted" || len(snapshot.Groups[0].Nodes) != 1 {
			t.Fatalf("targeted snapshot failed: %+v %v", snapshot, err)
		}
	}
	if _, err := ReadSnapshot(t.Context(), options, "unrelated"); err == nil {
		t.Fatal("目标 Provider 损坏未被发现")
	}
}
