package subscription

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"encoding/json/jsontext"
	json "encoding/json/v2"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/processlock"
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

	metadata := catalog.Metadata{
		Schema:         1,
		ID:             groupID,
		Name:           groupID,
		Type:           "subscription",
		URL:            server.URL,
		AutoUpdate:     true,
		UpdateInterval: int64((24 * time.Hour) / time.Second),
		Timeout:        5,
		Usage:          jsontext.Value("null"),
	}
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
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
	updated, err := catalog.LoadMetadata(filepath.Join(groupDir, "meta.json"), groupID)
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

func TestUpdateDoesNotHoldCatalogRootDuringDownload(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_700_700_000, 0)
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"slow-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()
	for _, groupID := range []string{"slow", "other"} {
		groupDir := filepath.Join(root, groupID)
		if err := os.MkdirAll(groupDir, 0o700); err != nil {
			t.Fatal(err)
		}
		metadata := catalog.NewMetadata(groupID, groupID, "subscription", server.URL, now)
		metadata.UpdateViaProxy = "never"
		metadata.Timeout = 5
		if groupID == "other" {
			metadata.URL = "https://other.invalid/sub"
		}
		if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
			t.Fatal(err)
		}
		if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), []byte(`{"outbounds":[]}`+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	updateDone := make(chan error, 1)
	go func() {
		_, err := Update(context.Background(), UpdateOptions{Root: root, GroupID: "slow", Now: now})
		updateDone <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("慢订阅未进入下载阶段")
	}

	readDone := make(chan error, 1)
	go func() {
		_, err := catalog.LoadMetadata(filepath.Join(root, "other", "meta.json"), "other")
		readDone <- err
	}()
	select {
	case err := <-readDone:
		if err != nil {
			t.Fatalf("其他分组读取失败: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("慢订阅下载期间其他分组读取仍被 Catalog 根锁阻塞")
	}
	close(release)
	select {
	case err := <-updateDone:
		if err != nil {
			t.Fatalf("慢订阅更新失败: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("慢订阅更新未结束")
	}
}

func TestUpdateRejectsMetadataChangeBeforeCommit(t *testing.T) {
	root := t.TempDir()
	groupID := "conflict"
	groupDir := filepath.Join(root, groupID)
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_701_000, 0)
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"new-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()
	metadata := catalog.NewMetadata(groupID, groupID, "subscription", server.URL, now)
	metadata.UpdateViaProxy = "never"
	metadata.Timeout = 5
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	oldProvider := []byte(`{"outbounds":[{"type":"socks","tag":"old-node","server":"127.0.0.1","server_port":1080}]}` + "\n")
	providerPath := filepath.Join(groupDir, "provider.json")
	if err := provider.WriteAtomic(providerPath, oldProvider, 0o600); err != nil {
		t.Fatal(err)
	}

	updateDone := make(chan error, 1)
	go func() {
		_, err := Update(context.Background(), UpdateOptions{Root: root, GroupID: groupID, Now: now})
		updateDone <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("冲突测试未进入下载阶段")
	}
	content, err := os.ReadFile(filepath.Join(groupDir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var changed catalog.Metadata
	if err := json.Unmarshal(content, &changed); err != nil {
		t.Fatal(err)
	}
	changed.Revision++
	changed.Include = "changed-during-download"
	changedContent, err := json.Marshal(changed, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.WriteAtomic(filepath.Join(groupDir, "meta.json"), append(changedContent, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case err := <-updateDone:
		var conflictErr *Error
		if !errors.As(err, &conflictErr) || conflictErr.Code != "subscription.conflict" {
			t.Fatalf("元数据竞争未返回结构化冲突: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("冲突更新未结束")
	}
	currentProvider, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(currentProvider) != string(oldProvider) {
		t.Fatalf("冲突更新错误替换了旧 Provider: %s", currentProvider)
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

	metadata := catalog.Metadata{Schema: 1, ID: groupID, Name: "Fixture", Type: "subscription", URL: server.URL, Timeout: 5}
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
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

	metadata := catalog.Metadata{
		Schema: 1, ID: groupID, Name: "Fallback", Type: "subscription", URL: server.URL,
		Timeout: 5, UpdateViaProxy: "auto",
	}
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
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

func TestUpdateRejectsRedirectWithStructuredError(t *testing.T) {
	root := t.TempDir()
	groupID := "redirect"
	groupDir := filepath.Join(root, groupID)
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		targetHits++
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"unexpected-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL, http.StatusFound)
	}))
	defer source.Close()

	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), catalog.Metadata{
		Schema: 1, ID: groupID, Name: "Redirect", Type: "subscription", URL: source.URL,
		AllowInsecure: true, HWID: "hwid-secret", CustomHeaders: map[string]string{"X-Vendor-Token": "token-secret"},
		Timeout: 5,
	}); err != nil {
		t.Fatal(err)
	}
	oldProvider := []byte(`{"outbounds":[{"type":"socks","tag":"old-node","server":"127.0.0.1","server_port":1080}]}` + "\n")
	providerPath := filepath.Join(groupDir, "provider.json")
	if err := provider.WriteAtomic(providerPath, oldProvider, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Update(context.Background(), UpdateOptions{Root: root, GroupID: groupID, Now: time.Unix(1700000000, 0)})
	if err == nil {
		t.Fatal("HTTPS 降级重定向应失败")
	}
	var subscriptionErr *Error
	if !errors.As(err, &subscriptionErr) || subscriptionErr.Code != "subscription.redirect_rejected" {
		t.Fatalf("重定向拒绝未返回结构化错误: %v", err)
	}
	if targetHits != 0 {
		t.Fatalf("被拒绝的重定向目标被访问: %d", targetHits)
	}
	current, readErr := os.ReadFile(providerPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(current) != string(oldProvider) {
		t.Fatalf("重定向失败替换了旧 Provider: %s", current)
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
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), catalog.Metadata{
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

func TestRequestCancelOnlyMarksActiveUpdate(t *testing.T) {
	root := t.TempDir()
	groupID := "request-cancel"
	progressDir := filepath.Join(root, "progress")

	active, err := RequestCancel(root, groupID, progressDir)
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("空闲订阅不应被报告为可取消")
	}
	if _, err := os.Stat(filepath.Join(progressDir, groupID+".cancel")); !os.IsNotExist(err) {
		t.Fatalf("空闲取消不应留下标记文件: %v", err)
	}

	lockDir := filepath.Join(root, "staging", "locks", groupID+".lock")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockDir, "pid"), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	active, err = RequestCancel(root, groupID, progressDir)
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("复用 PID 的残留目录不应被报告为活动订阅")
	}

	updateLock, err := acquireLock(lockDir)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLock(lockDir, updateLock)
	active, err = RequestCancel(root, groupID, progressDir)
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("尚未清理残留状态的订阅任务不应被报告为可取消")
	}
	if err := os.WriteFile(filepath.Join(lockDir, "pid"), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	active, err = RequestCancel(root, groupID, progressDir)
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("活动订阅应被报告为可取消")
	}
	if _, err := os.Stat(filepath.Join(progressDir, groupID+".cancel")); err != nil {
		t.Fatalf("活动取消标记未写入: %v", err)
	}
	releaseLock(lockDir, updateLock)
	if err := clearStaleProgress(progressDir, groupID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(progressDir, groupID+".cancel")); !os.IsNotExist(err) {
		t.Fatalf("残留锁对应的取消标记未清理: %v", err)
	}
}

func TestCancelMarkerInterruptsInFlightUpdate(t *testing.T) {
	root := t.TempDir()
	groupID := "in-flight-cancel"
	groupDir := filepath.Join(root, groupID)
	progressDir := filepath.Join(root, "progress")
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	oldProvider := []byte(`{"outbounds":[{"type":"socks","tag":"old-node","server":"127.0.0.1","server_port":1080}]}` + "\n")
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), catalog.Metadata{
		Schema: 1, ID: groupID, Name: "In-flight Cancel", Type: "subscription", URL: "http://test.invalid", Timeout: 30,
	}); err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join(groupDir, "provider.json")
	if err := provider.WriteAtomic(providerPath, oldProvider, 0o600); err != nil {
		t.Fatal(err)
	}

	requestStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-request.Context().Done()
	}))
	defer server.Close()
	metadata, err := catalog.LoadMetadata(filepath.Join(groupDir, "meta.json"), groupID)
	if err != nil {
		t.Fatal(err)
	}
	metadata.URL = server.URL
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}

	resultCh := make(chan error, 1)
	go func() {
		_, updateErr := Update(context.Background(), UpdateOptions{
			Root: root, GroupID: groupID, ProgressDir: progressDir, Now: time.Unix(1700000000, 0),
		})
		resultCh <- updateErr
	}()
	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("订阅请求未进入等待状态")
	}
	if err := os.MkdirAll(progressDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(progressDir, groupID+".cancel"), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case updateErr := <-resultCh:
		var subscriptionErr *Error
		if !errors.As(updateErr, &subscriptionErr) || subscriptionErr.Code != "subscription.cancelled" {
			t.Fatalf("取消请求未返回结构化取消错误: %v", updateErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("取消请求未能中断进行中的下载")
	}
	current, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(oldProvider) {
		t.Fatalf("取消更新不应替换旧 Provider: %s", current)
	}
	if _, err := os.Stat(filepath.Join(progressDir, groupID+".child.pid")); !os.IsNotExist(err) {
		t.Fatalf("订阅更新不应再生成 child.pid: %v", err)
	}
}

func TestUpdateRejectsUnreadableProgressStateDirectory(t *testing.T) {
	root := t.TempDir()
	groupID := "progress-failure"
	groupDir := filepath.Join(root, groupID)
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[]}`))
	}))
	defer server.Close()
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), catalog.Metadata{
		Schema: 1, ID: groupID, Name: groupID, Type: "subscription", URL: server.URL, Timeout: 5,
	}); err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join(groupDir, "provider.json")
	oldProvider := []byte(`{"outbounds":[{"type":"socks","tag":"old-node","server":"127.0.0.1","server_port":1080}]}` + "\n")
	if err := provider.WriteAtomic(providerPath, oldProvider, 0o600); err != nil {
		t.Fatal(err)
	}
	progressPath := filepath.Join(root, "progress")
	if err := os.WriteFile(progressPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Update(context.Background(), UpdateOptions{Root: root, GroupID: groupID, ProgressDir: progressPath, Now: time.Unix(1700000000, 0)})
	if err == nil {
		t.Fatal("unwritable subscription state directory was reported as success")
	}
	var subscriptionErr *Error
	if !errors.As(err, &subscriptionErr) || subscriptionErr.Code != "subscription.stage_failed" {
		t.Fatalf("unexpected progress state error: %v", err)
	}
	current, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(oldProvider) {
		t.Fatalf("progress state failure replaced the previous Provider: %s", current)
	}
}

func TestUpdateReportsHistoryWriteFailureAfterPersistence(t *testing.T) {
	root := t.TempDir()
	groupID := "history-failure"
	groupDir := filepath.Join(root, groupID)
	progressDir := filepath.Join(root, "progress")
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"persisted-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), catalog.Metadata{
		Schema: 1, ID: groupID, Name: groupID, Type: "subscription", URL: server.URL, Timeout: 5,
	}); err != nil {
		t.Fatal(err)
	}
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), []byte(`{"outbounds":[]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(groupDir, "history.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}

	result, err := Update(context.Background(), UpdateOptions{Root: root, GroupID: groupID, ProgressDir: progressDir, Now: time.Unix(1700000000, 0)})
	if err == nil {
		t.Fatal("history write failure was reported as success")
	}
	var subscriptionErr *Error
	if !errors.As(err, &subscriptionErr) || subscriptionErr.Code != "subscription.history_write_failed" {
		t.Fatalf("unexpected history write error: %v", err)
	}
	if subscriptionErr.Message != "订阅历史写入失败" {
		t.Fatalf("history write error message is not UTF-8 Chinese: %q", subscriptionErr.Message)
	}
	if !result.Persisted {
		t.Fatalf("persisted result was lost: %+v", result)
	}
	metadata, err := catalog.LoadMetadata(filepath.Join(groupDir, "meta.json"), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.NodeCount != 1 {
		t.Fatalf("metadata was not committed before history failure: %+v", metadata)
	}
	content, err := os.ReadFile(filepath.Join(groupDir, "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "persisted-node") {
		t.Fatalf("Provider was not committed before history failure: %s", content)
	}
	for _, name := range []string{groupID + ".progress.json", groupID + ".cancel"} {
		if _, err := os.Stat(filepath.Join(progressDir, name)); !os.IsNotExist(err) {
			t.Fatalf("history failure left progress artifact %s: %v", name, err)
		}
	}
}

func TestUpdateRecoversStaleSubscriptionStateBeforeStarting(t *testing.T) {
	root := t.TempDir()
	groupID := "recover-state"
	groupDir := filepath.Join(root, groupID)
	progressDir := filepath.Join(root, "progress")
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"recovered-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), catalog.Metadata{
		Schema: 1, ID: groupID, Name: groupID, Type: "subscription", URL: server.URL, Timeout: 5,
	}); err != nil {
		t.Fatal(err)
	}
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), []byte(`{"outbounds":[{"type":"socks","tag":"old-node","server":"127.0.0.1","server_port":1080}]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	staleStage := filepath.Join(root, "staging", groupID+".crashed")
	if err := os.MkdirAll(staleStage, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleStage, "provider.json"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	staleLock := filepath.Join(root, "staging", "locks", groupID+".lock")
	if err := os.MkdirAll(staleLock, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleLock, "pid"), []byte("2147483647\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(progressDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{groupID + ".progress.json", groupID + ".cancel"} {
		if err := os.WriteFile(filepath.Join(progressDir, name), []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	result, err := Update(context.Background(), UpdateOptions{
		Root: root, GroupID: groupID, ProgressDir: progressDir, Now: time.Unix(1_700_600_000, 0),
	})
	if err != nil {
		t.Fatalf("stale subscription state blocked recovery: %v", err)
	}
	if !result.Persisted || result.GroupID != groupID {
		t.Fatalf("unexpected recovered update result: %+v", result)
	}
	if _, err := os.Stat(staleStage); !os.IsNotExist(err) {
		t.Fatalf("stale subscription stage remains: %v", err)
	}
	for _, name := range []string{groupID + ".progress.json", groupID + ".cancel"} {
		if _, err := os.Stat(filepath.Join(progressDir, name)); !os.IsNotExist(err) {
			t.Fatalf("stale progress artifact remains (%s): %v", name, err)
		}
	}
	if _, err := os.Stat(staleLock); !os.IsNotExist(err) {
		t.Fatalf("subscription lock remains after recovery: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(groupDir, "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "recovered-node") {
		t.Fatalf("recovered update did not replace Provider: %s", content)
	}
}

func TestUpdatePreservesPendingRuntimeErrorAcrossFailureAnd304(t *testing.T) {
	root := t.TempDir()
	groupID := "pending-recovery"
	groupDir := filepath.Join(root, groupID)
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requestCount++
		if requestCount == 1 {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		writer.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()
	previousSuccess := "2023-11-14T22:13:20Z"
	previousRuntimeError := RuntimeSyncFailureMessage + ": Service API unavailable"
	metadata := catalog.Metadata{
		Schema: 1, ID: groupID, Name: groupID, Type: "subscription", URL: server.URL,
		AutoUpdate: true, UpdateInterval: 900, LastSuccessAt: previousSuccess,
		RuntimeSyncPending: true, RuntimeSyncState: RuntimeSyncFailed,
		LastError: previousRuntimeError, Timeout: 5,
	}
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	oldProvider := []byte(`{"outbounds":[{"type":"socks","tag":"old-node","server":"127.0.0.1","server_port":1080}]}` + "\n")
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), oldProvider, 0o600); err != nil {
		t.Fatal(err)
	}
	firstNow := time.Unix(1_700_610_000, 0)
	_, err := Update(context.Background(), UpdateOptions{Root: root, GroupID: groupID, Now: firstNow})
	if err == nil {
		t.Fatal("network failure unexpectedly succeeded")
	}
	var subscriptionErr *Error
	if !errors.As(err, &subscriptionErr) || subscriptionErr.Code != "subscription.convert_failed" {
		t.Fatalf("network failure returned the wrong code: %v", err)
	}
	metadata, err = catalog.LoadMetadata(filepath.Join(groupDir, "meta.json"), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if !metadata.RuntimeSyncPending || metadata.RuntimeSyncState != RuntimeSyncFailed || metadata.LastError != previousRuntimeError {
		t.Fatalf("network failure replaced pending runtime state: %+v", metadata)
	}
	if metadata.LastSuccessAt != previousSuccess || metadata.NextUpdateEpoch != firstNow.Unix()+900 {
		t.Fatalf("failure changed successful timestamp or schedule: %+v", metadata)
	}

	secondNow := firstNow.Add(15 * time.Minute)
	result, err := Update(context.Background(), UpdateOptions{Root: root, GroupID: groupID, Now: secondNow})
	if err != nil {
		t.Fatalf("304 recovery failed: %v", err)
	}
	if !result.NotModified || !result.Persisted || !result.RuntimeSyncPending {
		t.Fatalf("304 incorrectly cleared pending state: %+v", result)
	}
	metadata, err = catalog.LoadMetadata(filepath.Join(groupDir, "meta.json"), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if !metadata.RuntimeSyncPending || metadata.RuntimeSyncState != RuntimeSyncNotRunning || metadata.LastError != previousRuntimeError || metadata.LastSuccessAt != secondNow.UTC().Format(time.RFC3339) {
		t.Fatalf("304 recovery changed pending metadata: %+v", metadata)
	}
	content, err := os.ReadFile(filepath.Join(groupDir, "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(oldProvider) {
		t.Fatalf("failure recovery replaced the previous Provider: %s", content)
	}

	metadata.LastError = "订阅更新已取消"
	metadata.RuntimeSyncPending = true
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	thirdNow := secondNow.Add(15 * time.Minute)
	if _, err := Update(context.Background(), UpdateOptions{Root: root, GroupID: groupID, Now: thirdNow}); err != nil {
		t.Fatalf("304 did not recover stale cancellation metadata: %v", err)
	}
	metadata, err = catalog.LoadMetadata(filepath.Join(groupDir, "meta.json"), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if !metadata.RuntimeSyncPending || metadata.RuntimeSyncState != RuntimeSyncNotRunning || metadata.LastError != "" {
		t.Fatalf("304 retained a stale cancellation error: %+v", metadata)
	}
}

func TestUpdateFailureReportsHistoryRecoveryErrorCode(t *testing.T) {
	root := t.TempDir()
	groupID := "history-recovery"
	groupDir := filepath.Join(root, groupID)
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	metadata := catalog.Metadata{
		Schema: 1, ID: groupID, Name: groupID, Type: "subscription", URL: server.URL,
		AutoUpdate: true, UpdateInterval: 900, LastSuccessAt: "2023-11-14T22:13:20Z", Timeout: 5,
	}
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	oldProvider := []byte(`{"outbounds":[{"type":"socks","tag":"old-node","server":"127.0.0.1","server_port":1080}]}` + "\n")
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), oldProvider, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(groupDir, "history.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := Update(context.Background(), UpdateOptions{Root: root, GroupID: groupID, Now: time.Unix(1_700_620_000, 0)})
	if err == nil {
		t.Fatal("history recovery failure was reported as success")
	}
	var subscriptionErr *Error
	if !errors.As(err, &subscriptionErr) || subscriptionErr.Code != "subscription.history_write_failed" {
		t.Fatalf("history recovery failure returned the wrong code: %v", err)
	}
	data, ok := subscriptionErr.Data.(map[string]any)
	if !ok || data["original_code"] != "subscription.convert_failed" {
		t.Fatalf("original failure code was not retained: %#v", subscriptionErr.Data)
	}
	metadata, err = catalog.LoadMetadata(filepath.Join(groupDir, "meta.json"), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.LastSuccessAt != "2023-11-14T22:13:20Z" || metadata.NextUpdateEpoch != 1_700_620_900 {
		t.Fatalf("history failure changed recovery metadata: %+v", metadata)
	}
	content, err := os.ReadFile(filepath.Join(groupDir, "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(oldProvider) {
		t.Fatalf("history failure replaced the previous Provider: %s", content)
	}
}

func TestRuntimeSyncSuccessHistoryFailureRestoresPendingMetadata(t *testing.T) {
	root := t.TempDir()
	groupID := "runtime-history-recovery"
	groupDir := filepath.Join(root, groupID)
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	previousError := RuntimeSyncFailureMessage + ": reload failed"
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), catalog.Metadata{
		Schema: 1, ID: groupID, Name: groupID, Type: "subscription",
		RuntimeSyncPending: true, RuntimeSyncState: RuntimeSyncFailed, LastError: previousError,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(groupDir, "history.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}

	err := RecordRuntimeSyncSuccess(context.Background(), root, groupID, time.Unix(1_700_630_000, 0))
	if err == nil {
		t.Fatal("runtime success unexpectedly ignored history failure")
	}
	metadata, loadErr := catalog.LoadMetadata(filepath.Join(groupDir, "meta.json"), groupID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if !metadata.RuntimeSyncPending || metadata.RuntimeSyncState != RuntimeSyncFailed || metadata.LastError != previousError {
		t.Fatalf("history failure did not restore pending runtime metadata: %+v", metadata)
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

	lock, err := acquireLock(lockPath)
	if err != nil {
		t.Fatalf("stale lock was not reclaimed: %v", err)
	}
	defer releaseLock(lockPath, lock)
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

			lock, err := acquireLock(lockPath)
			if err != nil {
				t.Fatalf("invalid PID lock was not reclaimed: %v", err)
			}
			defer releaseLock(lockPath, lock)
			if _, err := os.Stat(filepath.Join(lockPath, "pid")); !os.IsNotExist(err) {
				t.Fatalf("invalid PID file was unexpectedly preserved: %v", err)
			}
		})
	}
}

func TestAcquireLockRecoversReusedPIDAndRejectsConcurrentHolder(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "locks", "group.lock")
	if err := os.MkdirAll(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockPath, "pid"), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}

	first, err := acquireLock(lockPath)
	if err != nil {
		t.Fatalf("复用 PID 的残留订阅锁不应阻塞新任务: %v", err)
	}
	if _, err := acquireLock(lockPath); !errors.Is(err, processlock.ErrBusy) {
		t.Fatalf("活动订阅锁未阻止并发任务: %v", err)
	}
	releaseLock(lockPath, first)

	second, err := acquireLock(lockPath)
	if err != nil {
		t.Fatalf("释放后无法再次获取订阅锁: %v", err)
	}
	releaseLock(lockPath, second)
}
