package subscription

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

func TestUpdateAndNotModified(t *testing.T) {
	root := t.TempDir()
	groupID := "fixture"
	groupDir := filepath.Join(root, groupID)
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("If-None-Match") == `"fixture-v1"` {
			writer.WriteHeader(http.StatusNotModified)
			return
		}
		writer.Header().Set("ETag", `"fixture-v1"`)
		writer.Header().Set("Profile-Title", "Fixture Subscription")
		writer.Header().Set("Profile-Update-Interval", "1")
		writer.Header().Set("Subscription-Userinfo", "upload=10; download=20; total=1000; expire=1900000000")
		writer.Header().Set("Content-Disposition", `attachment; filename="fixture.json"`)
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"fixture-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	metadata := Metadata{
		Schema:         1,
		ID:             groupID,
		Name:           groupID,
		Type:           "subscription",
		URL:            server.URL,
		AutoUpdate:     true,
		UpdateInterval: int64((24 * time.Hour) / time.Second),
		Timeout:        5,
		Usage:          json.RawMessage("null"),
	}
	if err := SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), []byte("{\"outbounds\":[]}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	now := time.Unix(1700000000, 0)
	result, err := Update(context.Background(), UpdateOptions{
		Root:        root,
		GroupID:     groupID,
		ProgressDir: filepath.Join(root, "progress"),
		Now:         now,
	})
	if err != nil {
		t.Fatalf("首次更新失败: %v", err)
	}
	if result.NotModified || result.NodeCount != 1 || result.Revision != 1 {
		t.Fatalf("首次更新结果异常: %+v", result)
	}
	updated, err := LoadMetadata(filepath.Join(groupDir, "meta.json"), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Fixture Subscription" || updated.ETag != `"fixture-v1"` || updated.NodeCount != 1 {
		t.Fatalf("响应头元数据没有正确保存: %+v", updated)
	}
	if updated.IntervalSource != "profile" || updated.UpdateInterval != int64(time.Hour/time.Second) {
		t.Fatalf("订阅周期没有正确应用响应头: %+v", updated)
	}
	providerContent, err := os.ReadFile(filepath.Join(groupDir, "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(providerContent), "fixture-node") {
		t.Fatalf("Provider 没有写入节点: %s", providerContent)
	}
	if _, err := os.Stat(filepath.Join(root, "progress", groupID+".progress.json")); !os.IsNotExist(err) {
		t.Fatalf("更新完成后仍残留进度文件: %v", err)
	}
	history, err := os.ReadFile(filepath.Join(groupDir, "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(strings.TrimSpace(string(history)), "\n") + 1; lines != 1 {
		t.Fatalf("更新历史条数异常: %d", lines)
	}

	result, err = Update(context.Background(), UpdateOptions{Root: root, GroupID: groupID, Now: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("304 更新失败: %v", err)
	}
	if !result.NotModified || result.Revision != 1 || result.NodeCount != 1 {
		t.Fatalf("304 更新结果异常: %+v", result)
	}
}

func TestUpdateRejectsEmptyProviderWithoutReplacingPrevious(t *testing.T) {
	root := t.TempDir()
	groupID := "fixture"
	groupDir := filepath.Join(root, groupID)
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[]}`))
	}))
	defer server.Close()

	metadata := Metadata{Schema: 1, ID: groupID, Name: "Fixture", Type: "subscription", URL: server.URL, Timeout: 5}
	if err := SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	oldProvider := []byte("{\"outbounds\":[{\"type\":\"socks\",\"tag\":\"old-node\",\"server\":\"127.0.0.1\",\"server_port\":1080}]}\n")
	providerPath := filepath.Join(groupDir, "provider.json")
	if err := provider.WriteAtomic(providerPath, oldProvider, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Update(context.Background(), UpdateOptions{Root: root, GroupID: groupID, Now: time.Unix(1700000000, 0)})
	if err == nil {
		t.Fatal("空 Provider 应该更新失败")
	}
	current, readErr := os.ReadFile(providerPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(current) != string(oldProvider) {
		t.Fatalf("更新失败后旧 Provider 被覆盖: %s", current)
	}
}

func TestUpdateFallsBackToDirectWhenConfiguredProxyFails(t *testing.T) {
	root := t.TempDir()
	groupID := "fallback"
	groupDir := filepath.Join(root, groupID)
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"direct-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	metadata := Metadata{
		Schema: 1, ID: groupID, Name: "Fallback", Type: "subscription", URL: server.URL,
		Timeout: 5, UpdateViaProxy: "auto",
	}
	if err := SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), []byte(`{"outbounds":[]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Update(context.Background(), UpdateOptions{
		Root: root, GroupID: groupID, ProxyURL: "http://127.0.0.1:1", FallbackDirect: true,
		Now: time.Unix(1700000000, 0),
	})
	if err != nil {
		t.Fatalf("代理失败后直连回退应成功: %v", err)
	}
	if result.UsedProxy {
		t.Fatal("直连回退后不应报告使用代理")
	}
	content, err := os.ReadFile(filepath.Join(groupDir, "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "direct-node") {
		t.Fatalf("直连回退未提交 Provider: %s", content)
	}
}

func TestCancelledUpdateKeepsPreviousProvider(t *testing.T) {
	root := t.TempDir()
	groupID := "cancelled"
	groupDir := filepath.Join(root, groupID)
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"new-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()
	if err := SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), Metadata{
		Schema: 1, ID: groupID, Name: "Cancelled", Type: "subscription", URL: server.URL, Timeout: 5,
	}); err != nil {
		t.Fatal(err)
	}
	oldProvider := []byte(`{"outbounds":[{"type":"socks","tag":"old-node","server":"127.0.0.1","server_port":1080}]}` + "\n")
	providerPath := filepath.Join(groupDir, "provider.json")
	if err := provider.WriteAtomic(providerPath, oldProvider, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Update(ctx, UpdateOptions{Root: root, GroupID: groupID, Now: time.Unix(1700000000, 0)}); err == nil {
		t.Fatal("已取消更新应返回错误")
	}
	current, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(oldProvider) {
		t.Fatalf("取消更新不应替换旧 Provider: %s", current)
	}
}

func TestAcquireLockRemovesStalePID(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, "locks", "group.lock")
	if err := os.MkdirAll(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockPath, "pid"), []byte("2147483647\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := acquireLock(lockPath); err != nil {
		t.Fatalf("stale lock was not reclaimed: %v", err)
	}
	defer os.RemoveAll(lockPath)
	if _, err := os.Stat(filepath.Join(lockPath, "pid")); !os.IsNotExist(err) {
		t.Fatalf("stale lock contents were unexpectedly preserved: %v", err)
	}
}

func TestAcquireLockWithInvalidPIDFile(t *testing.T) {
	for _, content := range []string{"", "not-a-pid\n", "  \n"} {
		name := strings.ReplaceAll(strings.TrimSpace(content), "-", "_")
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			lockPath := filepath.Join(root, "locks", "group.lock")
			if err := os.MkdirAll(lockPath, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(lockPath, "pid"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}

			if err := acquireLock(lockPath); err != nil {
				t.Fatalf("invalid PID lock was not reclaimed: %v", err)
			}
			defer os.RemoveAll(lockPath)
			if _, err := os.Stat(filepath.Join(lockPath, "pid")); !os.IsNotExist(err) {
				t.Fatalf("invalid PID file was unexpectedly preserved: %v", err)
			}
		})
	}
}
