package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"uuid"
)

func TestCatalogGroupQueries(t *testing.T) {
	root := t.TempDir()
	if _, err := importTestGroup(testImportOptions{
		Root: root, GroupID: "local-test", Name: "本地配置",
		Input: "socks://example.com:1080#ZED\nsocks://example.net:1081#ALPHA",
	}); err != nil {
		t.Fatalf("import group: %v", err)
	}
	resolved, err := ResolveGroup(context.Background(), root, "本地配置")
	if err != nil || resolved != "local-test" {
		t.Fatalf("resolve group: %q, %v", resolved, err)
	}
	contains, err := GroupContainsTag(context.Background(), root, resolved, "ZED")
	if err != nil || !contains {
		t.Fatalf("group contains tag: %v, %v", contains, err)
	}
	metadata, err := PrivateMetadata(context.Background(), root, resolved)
	if err != nil || metadata.Name != "本地配置" || metadata.Type != "local" {
		t.Fatalf("private metadata: %+v, %v", metadata, err)
	}
}

func TestNewSubscriptionGroupID(t *testing.T) {
	root := t.TempDir()
	subscriptionID, err := NewSubscriptionGroupID(context.Background(), root)
	if err != nil || !validGroupID.MatchString(subscriptionID) {
		t.Fatalf("subscription id: %q, %v", subscriptionID, err)
	}
	if len(subscriptionID) != 36 {
		t.Fatalf("unexpected subscription id: %q", subscriptionID)
	}
	parsedID, err := uuid.Parse(subscriptionID)
	if err != nil || parsedID[6]>>4 != 4 {
		t.Fatalf("subscription id is not UUIDv4: %q, %v", subscriptionID, err)
	}
}

func TestReservedGroupIDsAreRejectedByDirectOperations(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "staging"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, groupID := range []string{"staging", "nested..group"} {
		t.Run(groupID, func(t *testing.T) {
			if err := InitializeGroup(context.Background(), GroupOptions{Root: root, GroupID: groupID, Type: "local"}); err == nil {
				t.Fatal("保留分组 ID 不应初始化成功")
			}
			if _, err := PrivateMetadata(context.Background(), root, groupID); err == nil {
				t.Fatal("保留分组 ID 不应读取元数据")
			}
			if err := DeleteGroup(context.Background(), root, groupID); err == nil {
				t.Fatal("保留分组 ID 不应删除目录")
			}
			if _, err := ResolveGroup(context.Background(), root, groupID); err == nil {
				t.Fatal("保留目录不应解析为 Catalog 分组")
			}
		})
	}
}
