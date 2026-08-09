package catalog

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/convert"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

func TestCatalogNodeMutationsCommitPair(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_700_000_000, 0)
	result, err := ImportGroup(context.Background(), ImportOptions{
		Root: root, GroupID: "local-test", Name: "本地配置",
		Input: "socks://example.com:1080#FIRST", Now: now,
	})
	if err != nil {
		t.Fatalf("import group: %v", err)
	}
	if result.NodeCount != 1 || result.Revision != 1 || !result.StructureChanged {
		t.Fatalf("unexpected import result: %+v", result)
	}

	groupDir := filepath.Join(root, "local-test")
	result, err = AppendNode(context.Background(), MutationOptions{
		GroupDir: groupDir, GroupID: "local-test", Input: "socks://example.net:1081#SECOND", Now: now,
	})
	if err != nil {
		t.Fatalf("append node: %v", err)
	}
	if result.NodeCount != 2 || result.Revision != 2 {
		t.Fatalf("unexpected append result: %+v", result)
	}

	result, err = EditNode(context.Background(), MutationOptions{
		GroupDir: groupDir, GroupID: "local-test", Tag: "FIRST",
		Input: "socks://edited.example:1082#EDITED", Now: now,
	})
	if err != nil {
		t.Fatalf("edit node: %v", err)
	}
	if result.NodeCount != 2 || result.Revision != 3 {
		t.Fatalf("unexpected edit result: %+v", result)
	}

	result, err = RemoveNode(context.Background(), MutationOptions{
		GroupDir: groupDir, GroupID: "local-test", Tag: "SECOND", Now: now,
	})
	if err != nil {
		t.Fatalf("remove node: %v", err)
	}
	if result.NodeCount != 1 || result.Revision != 4 {
		t.Fatalf("unexpected remove result: %+v", result)
	}

	document, err := provider.Load(context.Background(), filepath.Join(groupDir, "provider.json"))
	if err != nil {
		t.Fatalf("load provider: %v", err)
	}
	nodes := provider.Inspect(document)
	if len(nodes) != 1 || nodes[0].Tag != "EDITED" {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
	metadata, err := LoadMetadata(filepath.Join(groupDir, "meta.json"), "local-test")
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if metadata.NodeCount != 1 || metadata.Revision != 4 || metadata.Name != "本地配置" {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
	for _, name := range []string{"provider.json.bak", "meta.json.bak"} {
		if _, err := os.Stat(filepath.Join(groupDir, name)); !os.IsNotExist(err) {
			t.Fatalf("backup file was not removed: %s", name)
		}
	}
}

func TestCatalogNodeMutationRejectsMissingTag(t *testing.T) {
	root := t.TempDir()
	_, err := ImportGroup(context.Background(), ImportOptions{
		Root: root, GroupID: "local-test", Input: "socks://example.com:1080#FIRST",
	})
	if err != nil {
		t.Fatalf("import group: %v", err)
	}
	_, err = RemoveNode(context.Background(), MutationOptions{
		GroupDir: filepath.Join(root, "local-test"), GroupID: "local-test", Tag: "MISSING",
	})
	if err == nil {
		t.Fatal("remove of missing tag unexpectedly succeeded")
	}
}

func TestCatalogNodeMutationsSerializePerGroup(t *testing.T) {
	root := t.TempDir()
	if _, err := ImportGroup(context.Background(), ImportOptions{
		Root: root, GroupID: "concurrent", Name: "Concurrent", Input: "socks://example.com:1080#BASE",
	}); err != nil {
		t.Fatalf("import group: %v", err)
	}

	var wait sync.WaitGroup
	errorsCh := make(chan error, 2)
	for _, input := range []string{
		"socks://one.example:1081#ONE",
		"socks://two.example:1082#TWO",
	} {
		wait.Add(1)
		go func(input string) {
			defer wait.Done()
			_, err := AppendNode(context.Background(), MutationOptions{
				GroupDir: filepath.Join(root, "concurrent"),
				GroupID:  "concurrent",
				Input:    input,
			})
			errorsCh <- err
		}(input)
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent append: %v", err)
		}
	}

	document, err := provider.Load(context.Background(), filepath.Join(root, "concurrent", "provider.json"))
	if err != nil {
		t.Fatalf("load provider: %v", err)
	}
	if got := len(provider.Inspect(document)); got != 3 {
		t.Fatalf("concurrent append lost nodes: got %d", got)
	}
	metadata, err := LoadMetadata(filepath.Join(root, "concurrent", "meta.json"), "concurrent")
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if metadata.NodeCount != 3 || metadata.Revision != 3 {
		t.Fatalf("provider and metadata diverged: nodes=%d revision=%d", metadata.NodeCount, metadata.Revision)
	}
}

func TestConcurrentNodeEditsWithSubscriptionUpdate(t *testing.T) {
	root := t.TempDir()
	groupID := "concurrent-update"
	now := time.Unix(1_700_000_000, 0)
	if _, err := ImportGroup(context.Background(), ImportOptions{
		Root: root, GroupID: groupID, Name: "并发测试", Input: "socks://base.example:1080#BASE", Now: now,
	}); err != nil {
		t.Fatalf("import group: %v", err)
	}

	parsed, err := convert.Input(context.Background(), "socks://example:2080#SUB", false)
	if err != nil {
		t.Fatalf("convert subscription node: %v", err)
	}
	metadata := NewMetadata(groupID, "订阅更新", "subscription", "https://example.test/sub", now)
	metadata.NodeCount = 1
	metadata.Revision = 10
	metadata.UpdatedAt = FormatEpochUTC(now.Unix())
	groupDir := filepath.Join(root, groupID)

	start := make(chan struct{})
	errorsCh := make(chan error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		_, appendErr := AppendNode(context.Background(), MutationOptions{
			GroupDir: groupDir,
			GroupID:  groupID,
			Input:    "socks://append.example:2081#APPEND",
			Now:      now,
		})
		errorsCh <- appendErr
	}()
	go func() {
		defer wait.Done()
		<-start
		// 订阅更新在提交阶段持有同一个分组锁，不能和节点编辑交错读写。
		release := Acquire(groupID)
		defer release()
		errorsCh <- commitPair(context.Background(), groupDir, parsed.Document, metadata)
	}()
	close(start)
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent catalog write: %v", err)
		}
	}

	document, err := provider.Load(context.Background(), filepath.Join(groupDir, "provider.json"))
	if err != nil {
		t.Fatalf("load final provider: %v", err)
	}
	finalMetadata, err := LoadMetadata(filepath.Join(groupDir, "meta.json"), groupID)
	if err != nil {
		t.Fatalf("load final metadata: %v", err)
	}
	nodes := provider.Inspect(document)
	if finalMetadata.NodeCount != len(nodes) {
		t.Fatalf("provider and metadata node count diverged: nodes=%d metadata=%d", len(nodes), finalMetadata.NodeCount)
	}
	if !providerHasTag(nodes, "SUB") {
		t.Fatalf("subscription update was lost: nodes=%+v", nodes)
	}
	// 订阅先提交时，节点追加会把 revision 从 10 提升到 11；反之保持 10。
	if (len(nodes) == 1 && finalMetadata.Revision != 10) ||
		(len(nodes) == 2 && finalMetadata.Revision != 11) {
		t.Fatalf("provider and metadata revision diverged: nodes=%d revision=%d", len(nodes), finalMetadata.Revision)
	}
}

func providerHasTag(nodes []provider.NodeSummary, tag string) bool {
	for _, node := range nodes {
		if node.Tag == tag {
			return true
		}
	}
	return false
}

func TestCatalogGroupInitialization(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_700_000_000, 0)
	if err := InitializeGroup(context.Background(), GroupOptions{
		Root: root, GroupID: "subscription-test", Name: "测试订阅", Type: "subscription",
		URL: "https://example.com/sub", UserAgent: "sing-box", HWID: "device",
		CustomHeaders: map[string]string{"X-Test": "value"}, AutoUpdate: true,
		UpdateInterval: 900, IntervalSource: "user", UpdateViaProxy: "auto",
		Timeout: 30, Now: now,
	}); err != nil {
		t.Fatalf("initialize group: %v", err)
	}
	groupDir := filepath.Join(root, "subscription-test")
	metadata, err := LoadMetadata(filepath.Join(groupDir, "meta.json"), "subscription-test")
	if err != nil {
		t.Fatalf("load initialized metadata: %v", err)
	}
	if metadata.Type != "subscription" || metadata.URL != "https://example.com/sub" ||
		metadata.CustomHeaders["X-Test"] != "value" || metadata.NextUpdateEpoch != now.Unix()+900 {
		t.Fatalf("unexpected initialized metadata: %+v", metadata)
	}
	document, err := provider.LoadAllowEmpty(context.Background(), filepath.Join(groupDir, "provider.json"))
	if err != nil || len(document.Outbounds)+len(document.Endpoints) != 0 {
		t.Fatalf("unexpected initialized provider: %v, %+v", err, document)
	}

	if err := EnsureGroup(context.Background(), GroupOptions{
		Root: root, GroupID: "default", Name: "本地配置", Type: "local", Now: now,
	}); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	if err := EnsureGroup(context.Background(), GroupOptions{
		Root: root, GroupID: "default", Name: "本地配置", Type: "local", Now: now,
	}); err != nil {
		t.Fatalf("ensure existing group: %v", err)
	}
	if err := SetGroupName(context.Background(), root, "subscription-test", "更新后的订阅", now); err != nil {
		t.Fatalf("set group name: %v", err)
	}
	metadata, err = LoadMetadata(filepath.Join(groupDir, "meta.json"), "subscription-test")
	if err != nil || metadata.Name != "更新后的订阅" {
		t.Fatalf("unexpected renamed metadata: %v, %+v", err, metadata)
	}
}
