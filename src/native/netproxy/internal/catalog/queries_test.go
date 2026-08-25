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
	if _, err := ImportGroup(context.Background(), ImportOptions{
		Root: root, GroupID: "local-test", Name: "本地配置",
		Input: "socks://example.com:1080#ZED\nsocks://example.net:1081#ALPHA",
	}); err != nil {
		t.Fatalf("import group: %v", err)
	}
	resolved, err := ResolveGroup(root, "本地配置")
	if err != nil || resolved != "local-test" {
		t.Fatalf("resolve group: %q, %v", resolved, err)
	}
	hasNodes, err := GroupHasNodes(context.Background(), root, resolved)
	if err != nil || !hasNodes {
		t.Fatalf("group has nodes: %v, %v", hasNodes, err)
	}
	first, err := GroupFirstTag(context.Background(), root, resolved)
	if err != nil || first != "ALPHA" {
		t.Fatalf("group first tag: %q, %v", first, err)
	}
	contains, err := GroupContainsTag(context.Background(), root, resolved, "ZED")
	if err != nil || !contains {
		t.Fatalf("group contains tag: %v, %v", contains, err)
	}
	metadata, err := PrivateMetadata(root, resolved)
	if err != nil || metadata.Name != "本地配置" || metadata.Type != "file" {
		t.Fatalf("private metadata: %+v, %v", metadata, err)
	}
	if _, err := FirstNonEmptyGroup(context.Background(), root, "missing"); err != nil {
		t.Fatalf("first nonempty group: %v", err)
	}
}

func TestNewGroupID(t *testing.T) {
	root := t.TempDir()
	subscriptionID, err := NewGroupID(root, "subscription", "")
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

	filePath := filepath.Join("nodes", "My Nodes.yaml")
	fileID, err := NewGroupID(root, "file", filePath)
	if err != nil || fileID != "local-my-nodes" {
		t.Fatalf("file id: %q, %v", fileID, err)
	}
	if err := os.Mkdir(filepath.Join(root, fileID), 0o700); err != nil {
		t.Fatalf("create collision: %v", err)
	}
	secondFileID, err := NewGroupID(root, "file", filePath)
	if err != nil || secondFileID != "local-my-nodes-2" {
		t.Fatalf("collision id: %q, %v", secondFileID, err)
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
			if _, err := PrivateMetadata(root, groupID); err == nil {
				t.Fatal("保留分组 ID 不应读取元数据")
			}
			if err := DeleteGroup(root, groupID); err == nil {
				t.Fatal("保留分组 ID 不应删除目录")
			}
			if _, err := ResolveGroup(root, groupID); err == nil {
				t.Fatal("保留目录不应解析为 Catalog 分组")
			}
		})
	}
}
