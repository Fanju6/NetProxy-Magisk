package worker

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

type manualClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*manualTimer
	delays []time.Duration
}

type manualTimer struct {
	mu      sync.Mutex
	channel chan time.Time
	fired   bool
	stopped bool
}

func newManualClock(now time.Time) *manualClock {
	return &manualClock{now: now}
}

func TestReloadServiceArgumentsMatchModuleFlags(t *testing.T) {
	arguments := reloadServiceArguments(Options{
		Root:           "/data/adb/modules/netproxy/data/catalog",
		ModuleConf:     "/data/adb/modules/netproxy/config/module.conf",
		SingBoxPath:    "/data/adb/modules/netproxy/bin/sing-box",
		ServiceAddress: "127.0.0.1:9090",
		ServiceSecret:  "singbox",
	})
	joined := strings.Join(arguments, " ")
	for _, expected := range []string{"module service reload", "--address 127.0.0.1:9090", "--secret singbox"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("reload 参数缺少 %q: %s", expected, joined)
		}
	}
	for _, removed := range []string{"--service-address", "--service-secret"} {
		if strings.Contains(joined, removed) {
			t.Fatalf("reload 仍使用未注册参数 %q: %s", removed, joined)
		}
	}
}

func (clock *manualClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *manualClock) NewTimer(duration time.Duration) Timer {
	timer := &manualTimer{channel: make(chan time.Time, 1)}
	clock.mu.Lock()
	clock.timers = append(clock.timers, timer)
	clock.delays = append(clock.delays, duration)
	clock.mu.Unlock()
	return timer
}

func (clock *manualClock) Delays() []time.Duration {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return append([]time.Duration(nil), clock.delays...)
}

func (clock *manualClock) Advance(duration time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(duration)
	timers := append([]*manualTimer(nil), clock.timers...)
	now := clock.now
	clock.mu.Unlock()
	for _, timer := range timers {
		timer.fire(now)
	}
}

func (timer *manualTimer) C() <-chan time.Time {
	return timer.channel
}

func (timer *manualTimer) Stop() bool {
	timer.mu.Lock()
	defer timer.mu.Unlock()
	if timer.fired {
		return false
	}
	timer.stopped = true
	return true
}

func (timer *manualTimer) fire(now time.Time) {
	timer.mu.Lock()
	if timer.stopped || timer.fired {
		timer.mu.Unlock()
		return
	}
	timer.fired = true
	timer.mu.Unlock()
	timer.channel <- now
}

func prepareWorkerFixture(t *testing.T, serverURL string, now time.Time) (string, string) {
	t.Helper()
	root := t.TempDir()
	groupID := "fixture"
	groupDir := filepath.Join(root, groupID)
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := catalog.NewMetadata(groupID, groupID, "subscription", serverURL, now)
	metadata.AutoUpdate = true
	metadata.UpdateInterval = 900
	metadata.UpdateViaProxy = "never"
	metadata.NextUpdateEpoch = now.Unix() - 1
	metadata.NextUpdateAt = catalog.FormatEpochUTC(metadata.NextUpdateEpoch)
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), []byte(`{"outbounds":[]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	moduleConf := filepath.Join(root, "module.conf")
	content := "ACTIVE_GROUP_ID=\"default\"\nSELECTOR_MODE=urltest\nOUTBOUND_MODE=rule\n"
	if err := os.WriteFile(moduleConf, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, moduleConf
}

func waitRequest(t *testing.T, requests <-chan struct{}) {
	t.Helper()
	select {
	case <-requests:
	case <-time.After(3 * time.Second):
		t.Fatal("订阅更新请求未在限定时间内到达")
	}
}

func waitRevision(t *testing.T, path string, expected int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		metadata, err := catalog.LoadMetadata(path, "fixture")
		if err == nil && metadata.Revision >= expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("订阅 Revision 未达到 %d", expected)
}

func markStaleWorker(t *testing.T, pidFile string) {
	t.Helper()
	if err := os.WriteFile(pidFile, []byte("999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lockDir := pidFile + ".lock"
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockDir, "pid"), []byte("999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func installRuntimeHooks(t *testing.T, running bool, reload func(context.Context, Options) error) {
	t.Helper()
	originalRunning, originalReload, originalVerify := workerProcessRunning, workerReloadService, workerVerifyRuntime
	workerProcessRunning = func(string) bool { return running }
	workerReloadService = reload
	workerVerifyRuntime = func(context.Context, Options, string) error { return nil }
	t.Cleanup(func() {
		workerProcessRunning = originalRunning
		workerReloadService = originalReload
		workerVerifyRuntime = originalVerify
	})
}

func installPersistenceHooks(t *testing.T, updateModule func(string, map[string]string) error, groupHasNodes func(context.Context, string, string) (bool, error)) {
	t.Helper()
	originalUpdateModule, originalGroupHasNodes := workerUpdateModule, workerGroupHasNodes
	if updateModule != nil {
		workerUpdateModule = updateModule
	}
	if groupHasNodes != nil {
		workerGroupHasNodes = groupHasNodes
	}
	t.Cleanup(func() {
		workerUpdateModule = originalUpdateModule
		workerGroupHasNodes = originalGroupHasNodes
	})
}

func historyContains(entries []jsontext.Value, code string) bool {
	for _, entry := range entries {
		if strings.Contains(string(entry), code) {
			return true
		}
	}
	return false
}

func TestUpdateGroupWhenServiceStoppedReportsPersistedNotRunning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"stopped-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	now := time.Unix(1_700_400_000, 0)
	root, moduleConf := prepareWorkerFixture(t, server.URL, now)
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.SingBoxPath = filepath.Join(root, "sing-box")
	reloadCalls := 0
	installRuntimeHooks(t, false, func(context.Context, Options) error {
		reloadCalls++
		return nil
	})

	result, err := UpdateGroup(context.Background(), options, "fixture", now, nil)
	if err != nil {
		t.Fatalf("服务停止时订阅更新失败: %v", err)
	}
	if !result.Persisted || result.RuntimeSynced || result.RuntimeSyncState != subscription.RuntimeSyncNotRunning {
		t.Fatalf("服务停止时结果异常: %+v", result)
	}
	if reloadCalls != 0 {
		t.Fatalf("服务停止时不应调用运行时 reload: %d", reloadCalls)
	}
	if _, err := provider.Load(context.Background(), filepath.Join(root, "fixture", "provider.json")); err != nil {
		t.Fatalf("持久化 Provider 不可读: %v", err)
	}
}

func TestSyncEditedGroupReloadsAfterNameChange(t *testing.T) {
	now := time.Unix(1_700_407_000, 0)
	root, moduleConf := prepareWorkerFixture(t, "https://unused.invalid/sub", now)
	groupDir := filepath.Join(root, "fixture")
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), []byte(`{"outbounds":[{"type":"socks","tag":"edited-node","server":"127.0.0.1","server_port":1080}]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata, err := catalog.LoadMetadata(filepath.Join(groupDir, "meta.json"), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	metadata.Name = "新的运行时名称"
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.SingBoxPath = filepath.Join(root, "sing-box")
	reloadCalls := 0
	installRuntimeHooks(t, true, func(context.Context, Options) error {
		reloadCalls++
		return nil
	})

	result, err := SyncEditedGroup(context.Background(), options, "fixture", now, nil)
	if err != nil {
		t.Fatalf("名称变更运行时同步失败: %v", err)
	}
	if !result.Persisted || !result.RuntimeSynced || result.RuntimeSyncState != subscription.RuntimeSyncApplied {
		t.Fatalf("名称变更运行时结果异常: %+v", result)
	}
	if reloadCalls != 1 {
		t.Fatalf("名称变更应触发一次 reload，实际 %d 次", reloadCalls)
	}
}

func TestSyncEditedGroupWhenServiceStoppedDoesNotReload(t *testing.T) {
	now := time.Unix(1_700_408_000, 0)
	root, moduleConf := prepareWorkerFixture(t, "https://unused.invalid/sub", now)
	groupDir := filepath.Join(root, "fixture")
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), []byte(`{"outbounds":[{"type":"socks","tag":"stopped-edit","server":"127.0.0.1","server_port":1080}]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.SingBoxPath = filepath.Join(root, "sing-box")
	reloadCalls := 0
	installRuntimeHooks(t, false, func(context.Context, Options) error {
		reloadCalls++
		return nil
	})

	result, err := SyncEditedGroup(context.Background(), options, "fixture", now, nil)
	if err != nil {
		t.Fatalf("服务停止时编辑同步失败: %v", err)
	}
	if !result.Persisted || result.RuntimeSynced || result.RuntimeSyncState != subscription.RuntimeSyncNotRunning {
		t.Fatalf("服务停止时编辑结果异常: %+v", result)
	}
	if reloadCalls != 0 {
		t.Fatalf("服务停止时编辑不应 reload，实际 %d 次", reloadCalls)
	}
}

func TestUpdateGroupWhenServiceStoppedReturnsModuleConfigEffectError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"module-write-failure","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	now := time.Unix(1_700_405_000, 0)
	root, moduleConf := prepareWorkerFixture(t, server.URL, now)
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.SingBoxPath = filepath.Join(root, "sing-box")
	installRuntimeHooks(t, false, func(context.Context, Options) error { return nil })
	installPersistenceHooks(t, func(string, map[string]string) error {
		return errors.New("module.conf write failed")
	}, nil)

	result, err := UpdateGroup(context.Background(), options, "fixture", now, nil)
	if err == nil {
		t.Fatal("module.conf 写入失败时不应返回普通成功")
	}
	var effectErr *subscription.Error
	if !errors.As(err, &effectErr) || effectErr.Code != "subscription.persisted_effect_failed" {
		t.Fatalf("module.conf 写入失败未返回结构化副作用错误: %v", err)
	}
	if !result.Persisted || result.RuntimeSynced || result.RuntimeSyncState != subscription.RuntimeSyncNotRunning {
		t.Fatalf("module.conf 写入失败结果状态异常: %+v", result)
	}
	data, ok := effectErr.Data.(map[string]any)
	if !ok || !strings.Contains(fmt.Sprint(data["cause"]), "module.conf write failed") {
		t.Fatalf("module.conf 写入失败原因未保留: %#v", effectErr.Data)
	}
}

func TestUpdateGroupWhenServiceStoppedReturnsCatalogReadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"catalog-read-failure","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	now := time.Unix(1_700_408_000, 0)
	root, moduleConf := prepareWorkerFixture(t, server.URL, now)
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.SingBoxPath = filepath.Join(root, "sing-box")
	installRuntimeHooks(t, false, func(context.Context, Options) error { return nil })
	installPersistenceHooks(t, nil, func(context.Context, string, string) (bool, error) {
		return false, errors.New("Catalog read failed")
	})

	result, err := UpdateGroup(context.Background(), options, "fixture", now, nil)
	if err == nil {
		t.Fatal("Catalog 读取失败时不应返回普通成功")
	}
	var effectErr *subscription.Error
	if !errors.As(err, &effectErr) || effectErr.Code != "subscription.persisted_effect_failed" {
		t.Fatalf("Catalog 读取失败未返回结构化副作用错误: %v", err)
	}
	if !result.Persisted || result.RuntimeSynced || result.RuntimeSyncState != subscription.RuntimeSyncNotRunning {
		t.Fatalf("Catalog 读取失败结果状态异常: %+v", result)
	}
	data, ok := effectErr.Data.(map[string]any)
	if !ok || !strings.Contains(fmt.Sprint(data["cause"]), "Catalog read failed") {
		t.Fatalf("Catalog 读取失败原因未保留: %#v", effectErr.Data)
	}
}

func TestUpdateGroupWhenServiceStoppedEffectFailureStoresMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"effect-failure","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	now := time.Unix(1_700_407_000, 0)
	root, moduleConf := prepareWorkerFixture(t, server.URL, now)
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.SingBoxPath = filepath.Join(root, "sing-box")
	installRuntimeHooks(t, false, func(context.Context, Options) error { return nil })
	installPersistenceHooks(t, func(string, map[string]string) error {
		return errors.New("module.conf write failed")
	}, nil)

	result, err := UpdateGroup(context.Background(), options, "fixture", now, nil)
	if err == nil {
		t.Fatal("本地副作用失败时不应返回普通成功")
	}
	var effectErr *subscription.Error
	if !errors.As(err, &effectErr) || effectErr.Code != "subscription.persisted_effect_failed" {
		t.Fatalf("本地副作用失败未返回结构化错误: %v", err)
	}
	if !result.Persisted || result.RuntimeSyncState != subscription.RuntimeSyncNotRunning {
		t.Fatalf("本地副作用失败结果状态异常: %+v", result)
	}
	metadata, err := catalog.LoadMetadata(filepath.Join(root, "fixture", "meta.json"), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(metadata.LastError, subscription.PersistedEffectFailureMessage) {
		t.Fatalf("本地副作用失败未保存 last_error: %+v", metadata)
	}
	history, err := subscription.LoadHistory(filepath.Join(root, "fixture", "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !historyContains(history, "subscription.persisted_effect_failed") {
		t.Fatalf("本地副作用失败未追加历史: %v", history)
	}
}

func TestUpdateGroupWhenServiceStoppedReturnsCatalogReadErrorWithMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"catalog-read-failure","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	now := time.Unix(1_700_408_000, 0)
	root, moduleConf := prepareWorkerFixture(t, server.URL, now)
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.SingBoxPath = filepath.Join(root, "sing-box")
	installRuntimeHooks(t, false, func(context.Context, Options) error { return nil })
	installPersistenceHooks(t, nil, func(context.Context, string, string) (bool, error) {
		return false, errors.New("Catalog read failed")
	})

	result, err := UpdateGroup(context.Background(), options, "fixture", now, nil)
	if err == nil {
		t.Fatal("Catalog 读取失败时不应返回普通成功")
	}
	var effectErr *subscription.Error
	if !errors.As(err, &effectErr) || effectErr.Code != "subscription.persisted_effect_failed" {
		t.Fatalf("Catalog 读取失败未返回结构化错误: %v", err)
	}
	if !result.Persisted || result.RuntimeSyncState != subscription.RuntimeSyncNotRunning {
		t.Fatalf("Catalog 读取失败结果状态异常: %+v", result)
	}
	metadata, err := catalog.LoadMetadata(filepath.Join(root, "fixture", "meta.json"), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(metadata.LastError, subscription.PersistedEffectFailureMessage) {
		t.Fatalf("Catalog 读取失败未保存 last_error: %+v", metadata)
	}
	history, err := subscription.LoadHistory(filepath.Join(root, "fixture", "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !historyContains(history, "subscription.persisted_effect_failed") {
		t.Fatalf("Catalog 读取失败未追加历史: %v", history)
	}
}

func TestUpdateGroupWhenServiceRunningUsesProviderWatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"applied-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	now := time.Unix(1_700_410_000, 0)
	root, moduleConf := prepareWorkerFixture(t, server.URL, now)
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.SingBoxPath = filepath.Join(root, "sing-box")
	if err := os.WriteFile(moduleConf, []byte("ACTIVE_GROUP_ID=fixture\nSELECTOR_MODE=urltest\nOUTBOUND_MODE=rule\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := provider.WriteAtomic(filepath.Join(root, "fixture", "provider.json"), []byte(`{"outbounds":[{"type":"socks","tag":"old-node","server":"127.0.0.1","server_port":1080}]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reloadCalls := 0
	installRuntimeHooks(t, true, func(context.Context, Options) error {
		reloadCalls++
		return nil
	})

	result, err := UpdateGroup(context.Background(), options, "fixture", now, nil)
	if err != nil {
		t.Fatalf("运行服务热重载成功时订阅更新失败: %v", err)
	}
	if !result.Persisted || !result.RuntimeSynced || result.RuntimeSyncState != subscription.RuntimeSyncApplied {
		t.Fatalf("运行服务 Provider 热更新结果异常: %+v", result)
	}
	if reloadCalls != 0 {
		t.Fatalf("已有 Provider 的普通更新不应 reload，实际 %d 次", reloadCalls)
	}
}

func TestUpdateGroupProviderWatchFailureDoesNotReload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"new-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	now := time.Unix(1_700_415_000, 0)
	root, moduleConf := prepareWorkerFixture(t, server.URL, now)
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.SingBoxPath = filepath.Join(root, "sing-box")
	if err := os.WriteFile(moduleConf, []byte("ACTIVE_GROUP_ID=fixture\nSELECTOR_MODE=urltest\nOUTBOUND_MODE=rule\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := provider.WriteAtomic(filepath.Join(root, "fixture", "provider.json"), []byte(`{"outbounds":[{"type":"socks","tag":"old-node","server":"127.0.0.1","server_port":1080}]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reloadCalls := 0
	installRuntimeHooks(t, true, func(context.Context, Options) error {
		reloadCalls++
		return nil
	})
	workerVerifyRuntime = func(context.Context, Options, string) error {
		return errors.New("Provider watcher did not apply update")
	}

	result, err := UpdateGroup(context.Background(), options, "fixture", now, nil)
	if err == nil {
		t.Fatal("Provider 监听未应用时不应返回成功")
	}
	if result.RuntimeSynced || result.RuntimeSyncState != subscription.RuntimeSyncFailed || !result.RuntimeSyncPending {
		t.Fatalf("Provider 监听失败状态异常: %+v", result)
	}
	if reloadCalls != 0 {
		t.Fatalf("Provider 监听失败不得把普通更新升级为服务 reload: %d", reloadCalls)
	}
}

func TestRuntimeProviderMatchesRequiresExactNodeSet(t *testing.T) {
	expected := map[string]struct{}{"keep": {}}
	if !runtimeProviderMatches([]serviceapi.GroupItem{{Tag: "fixture/keep"}}, "fixture", expected) {
		t.Fatal("完全一致的 Provider 节点未通过验证")
	}
	if runtimeProviderMatches([]serviceapi.GroupItem{{Tag: "fixture/keep"}, {Tag: "fixture/drop"}}, "fixture", expected) {
		t.Fatal("包含旧节点的 Provider 被误判为已应用")
	}
	if runtimeProviderMatches([]serviceapi.GroupItem{{Tag: "other/keep"}}, "fixture", expected) {
		t.Fatal("其他 Provider 的同名节点被误判为已应用")
	}
}

func TestUpdateGroupRuntimeSyncFailureReturnsStructuredErrorAndKeepsProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"failed-reload-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	now := time.Unix(1_700_420_000, 0)
	root, moduleConf := prepareWorkerFixture(t, server.URL, now)
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.SingBoxPath = filepath.Join(root, "sing-box")
	reloadCalls := 0
	installRuntimeHooks(t, true, func(context.Context, Options) error {
		reloadCalls++
		return errors.New("Service API unavailable")
	})

	result, err := UpdateGroup(context.Background(), options, "fixture", now, nil)
	if err == nil {
		t.Fatal("运行时 reload 失败不应返回普通成功")
	}
	var syncErr *subscription.Error
	if !errors.As(err, &syncErr) || syncErr.Code != "subscription.runtime_sync_failed" {
		t.Fatalf("运行时失败未返回结构化错误: %v", err)
	}
	if !result.Persisted || result.RuntimeSynced || result.RuntimeSyncState != subscription.RuntimeSyncFailed {
		t.Fatalf("运行时失败结果异常: %+v", result)
	}
	metadata, err := catalog.LoadMetadata(filepath.Join(root, "fixture", "meta.json"), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !metadata.RuntimeSyncPending || !strings.Contains(metadata.LastError, subscription.RuntimeSyncFailureMessage) {
		t.Fatalf("运行时失败未写入元数据状态: %+v", metadata)
	}
	if metadata.LastSuccessAt != now.UTC().Format(time.RFC3339) {
		t.Fatalf("运行时失败覆盖了 HTTP 更新成功时间: %q", metadata.LastSuccessAt)
	}
	history, err := subscription.LoadHistory(filepath.Join(root, "fixture", "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !historyContains(history, "subscription.runtime_sync_failed") {
		t.Fatalf("运行时失败未追加历史: %v", history)
	}
	data, ok := syncErr.Data.(map[string]any)
	if !ok || data["persisted"] != true || data["runtime_synced"] != false || data["runtime_sync_state"] != subscription.RuntimeSyncFailed {
		t.Fatalf("运行时失败错误数据不完整: %#v", syncErr.Data)
	}
	if !strings.Contains(fmt.Sprint(data["cause"]), "Service API unavailable") {
		t.Fatalf("运行时失败原因未保留: %#v", syncErr.Data)
	}
	if reloadCalls != 1 {
		t.Fatalf("运行时失败不应触发未经授权的额外 reload: %d", reloadCalls)
	}
	if _, err := provider.Load(context.Background(), filepath.Join(root, "fixture", "provider.json")); err != nil {
		t.Fatalf("运行时失败后 Provider 不可读: %v", err)
	}

	runtimeDir := filepath.Join(t.TempDir(), "runtime-after-failure")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	runtime, err := catalog.BuildRuntime(context.Background(), catalog.RuntimeOptions{
		Root: root, ModuleConfig: moduleConf,
		ProvidersOutput: filepath.Join(runtimeDir, "providers.json"),
		OutboundsOutput: filepath.Join(runtimeDir, "outbounds.json"),
	})
	if err != nil {
		t.Fatalf("重启准备阶段无法读取最新 Provider: %v", err)
	}
	if runtime.NodeCount != 1 {
		t.Fatalf("重启准备阶段节点数异常: %+v", runtime)
	}
}

func TestUpdateGroupRuntimeVerificationFailureReturnsStructuredError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"verification-failure","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	now := time.Unix(1_700_425_000, 0)
	root, moduleConf := prepareWorkerFixture(t, server.URL, now)
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.SingBoxPath = filepath.Join(root, "sing-box")
	reloadCalls := 0
	installRuntimeHooks(t, true, func(context.Context, Options) error {
		reloadCalls++
		return nil
	})
	originalVerify := workerVerifyRuntime
	workerVerifyRuntime = func(context.Context, Options, string) error {
		return errors.New("Provider state was not applied")
	}
	t.Cleanup(func() { workerVerifyRuntime = originalVerify })

	result, err := UpdateGroup(context.Background(), options, "fixture", now, nil)
	if err == nil {
		t.Fatal("运行时状态验证失败时不应返回普通成功")
	}
	var syncErr *subscription.Error
	if !errors.As(err, &syncErr) || syncErr.Code != "subscription.runtime_sync_failed" {
		t.Fatalf("运行时状态验证失败未返回结构化错误: %v", err)
	}
	if !result.Persisted || result.RuntimeSynced || result.RuntimeSyncState != subscription.RuntimeSyncFailed || !result.RuntimeSyncPending {
		t.Fatalf("运行时状态验证失败结果异常: %+v", result)
	}
	if reloadCalls != 1 {
		t.Fatalf("运行时状态验证失败应只调用一次 reload，实际 %d 次", reloadCalls)
	}
	if _, err := provider.Load(context.Background(), filepath.Join(root, "fixture", "provider.json")); err != nil {
		t.Fatalf("运行时状态验证失败后 Provider 不可读: %v", err)
	}
	metadata, err := catalog.LoadMetadata(filepath.Join(root, "fixture", "meta.json"), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !metadata.RuntimeSyncPending {
		t.Fatal("运行时状态验证失败后未持久化 pending 标志")
	}
}

func TestUpdateGroup304StoppedAndRunningStates(t *testing.T) {
	newServer := func(t *testing.T) *httptest.Server {
		t.Helper()
		return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Header.Get("If-None-Match") == `"etag-v1"` {
				writer.WriteHeader(http.StatusNotModified)
				return
			}
			writer.Header().Set("ETag", `"etag-v1"`)
			_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"etag-node","server":"127.0.0.1","server_port":1080}]}`))
		}))
	}

	t.Run("stopped", func(t *testing.T) {
		server := newServer(t)
		defer server.Close()
		now := time.Unix(1_700_430_000, 0)
		root, moduleConf := prepareWorkerFixture(t, server.URL, now)
		options := newTestOptions(root)
		options.ModuleConf = moduleConf
		options.SingBoxPath = filepath.Join(root, "sing-box")
		reloadCalls := 0
		installRuntimeHooks(t, false, func(context.Context, Options) error { reloadCalls++; return nil })
		if _, err := UpdateGroup(context.Background(), options, "fixture", now, nil); err != nil {
			t.Fatal(err)
		}
		result, err := UpdateGroup(context.Background(), options, "fixture", now.Add(time.Hour), nil)
		if err != nil {
			t.Fatal(err)
		}
		if !result.NotModified || !result.Persisted || result.RuntimeSynced || result.RuntimeSyncState != subscription.RuntimeSyncNotRunning {
			t.Fatalf("停止服务 304 结果异常: %+v", result)
		}
		if reloadCalls != 0 {
			t.Fatalf("停止服务 304 不应调用 reload: %d", reloadCalls)
		}
	})

	t.Run("running-already-synced", func(t *testing.T) {
		server := newServer(t)
		defer server.Close()
		now := time.Unix(1_700_435_000, 0)
		root, moduleConf := prepareWorkerFixture(t, server.URL, now)
		options := newTestOptions(root)
		options.ModuleConf = moduleConf
		options.SingBoxPath = filepath.Join(root, "sing-box")
		reloadCalls := 0
		installRuntimeHooks(t, true, func(context.Context, Options) error { reloadCalls++; return nil })
		if _, err := UpdateGroup(context.Background(), options, "fixture", now, nil); err != nil {
			t.Fatal(err)
		}
		result, err := UpdateGroup(context.Background(), options, "fixture", now.Add(time.Hour), nil)
		if err != nil {
			t.Fatalf("已同步运行服务 304 更新失败: %v", err)
		}
		if !result.NotModified || !result.Persisted || !result.RuntimeSynced || result.RuntimeSyncState != subscription.RuntimeSyncApplied || result.RuntimeSyncPending {
			t.Fatalf("已同步运行服务 304 结果异常: %+v", result)
		}
		if reloadCalls != 1 {
			t.Fatalf("已同步运行服务 304 不应再次 reload，实际 %d 次", reloadCalls)
		}
	})

	t.Run("running-and-retry-after-failure", func(t *testing.T) {
		server := newServer(t)
		defer server.Close()
		now := time.Unix(1_700_440_000, 0)
		root, moduleConf := prepareWorkerFixture(t, server.URL, now)
		options := newTestOptions(root)
		options.ModuleConf = moduleConf
		options.SingBoxPath = filepath.Join(root, "sing-box")
		reloadCalls := 0
		reloadFailed := true
		installRuntimeHooks(t, true, func(context.Context, Options) error {
			reloadCalls++
			if reloadFailed {
				return errors.New("temporary Service API failure")
			}
			return nil
		})
		if _, err := UpdateGroup(context.Background(), options, "fixture", now, nil); err == nil {
			t.Fatal("首次运行时同步失败应返回错误")
		}
		metadata, err := catalog.LoadMetadata(filepath.Join(root, "fixture", "meta.json"), "fixture")
		if err != nil {
			t.Fatal(err)
		}
		if !metadata.RuntimeSyncPending {
			t.Fatal("首次运行时同步失败后未持久化 pending 标志")
		}
		reloadFailed = false
		result, err := UpdateGroup(context.Background(), options, "fixture", now.Add(time.Hour), nil)
		if err != nil {
			t.Fatalf("304 重试运行时同步失败: %v", err)
		}
		if !result.NotModified || !result.Persisted || !result.RuntimeSynced || result.RuntimeSyncState != subscription.RuntimeSyncApplied {
			t.Fatalf("运行服务 304 重试结果异常: %+v", result)
		}
		if reloadCalls != 1 {
			t.Fatalf("304 可直接验证已生效的 Provider，不应再次 reload，实际 %d 次", reloadCalls)
		}
		metadata, err = catalog.LoadMetadata(filepath.Join(root, "fixture", "meta.json"), "fixture")
		if err != nil {
			t.Fatal(err)
		}
		if metadata.RuntimeSyncPending {
			t.Fatal("304 重试成功后未清除 pending 标志")
		}
		if metadata.LastError != "" {
			t.Fatalf("304 重试成功后未清除运行时错误: %q", metadata.LastError)
		}
		history, err := subscription.LoadHistory(filepath.Join(root, "fixture", "history.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if !historyContains(history, "subscription.runtime_sync_failed") || !historyContains(history, "subscription.runtime_sync_applied") {
			t.Fatalf("304 重试历史不完整: %v", history)
		}
	})
}

func TestNextUpdateUsesNearestEnabledSubscription(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_000, 0)
	for _, entry := range []struct {
		id       string
		interval int64
		auto     bool
	}{
		{id: "first", interval: 900, auto: true},
		{id: "later", interval: 3_600, auto: true},
		{id: "manual", interval: 900, auto: false},
	} {
		metadata := catalog.NewMetadata(entry.id, entry.id, "subscription", "https://example.invalid/"+entry.id, now)
		metadata.AutoUpdate = entry.auto
		metadata.UpdateInterval = entry.interval
		if entry.auto {
			catalog.ScheduleAt(&metadata, now)
		}
		group := filepath.Join(root, entry.id)
		if err := os.MkdirAll(group, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := catalog.SaveMetadataAtomic(filepath.Join(group, "meta.json"), metadata); err != nil {
			t.Fatal(err)
		}
	}
	options := newTestOptions(root)
	options.ModuleConf = filepath.Join(root, "module.conf")
	options.Now = func() time.Time { return now }
	got, err := NextUpdate(root, now)
	if err != nil {
		t.Fatal(err)
	}
	if got != now.Unix()+900 {
		t.Fatalf("nearest = %d, want %d", got, now.Unix()+900)
	}
}

func TestRunExitsWhenNoAutomaticSubscription(t *testing.T) {
	root := t.TempDir()
	moduleConf := filepath.Join(root, "module.conf")
	if err := os.WriteFile(moduleConf, []byte("ACTIVE_GROUP_ID=\"default\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.PIDFile = filepath.Join(t.TempDir(), "worker.pid")
	options.Now = time.Now
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Run(ctx, options, nil, log.New(os.Stderr, "", 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(options.PIDFile); !os.IsNotExist(err) {
		t.Fatalf("PID file still exists: %v", err)
	}
}

func TestRunUsesControllableClockForWakeCancelAndRestart(t *testing.T) {
	requests := make(chan struct{}, 4)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests <- struct{}{}
		writer.Header().Set("ETag", `"worker-fixture"`)
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"worker-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	clock := newManualClock(time.Unix(1_700_000_000, 0))
	root, moduleConf := prepareWorkerFixture(t, server.URL, clock.Now())
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.PIDFile = filepath.Join(t.TempDir(), "worker.pid")
	options.ProgressDir = filepath.Join(t.TempDir(), "progress")
	options.Now = clock.Now
	options.NewTimer = clock.NewTimer
	markStaleWorker(t, options.PIDFile)

	ctx, cancel := context.WithCancel(context.Background())
	wake := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() { done <- Run(ctx, options, wake, log.New(io.Discard, "", 0)) }()
	waitRequest(t, requests)
	clock.Advance(15 * time.Minute)
	wake <- struct{}{}
	waitRequest(t, requests)
	waitRevision(t, filepath.Join(root, "fixture", "meta.json"), 2)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("取消 Worker 失败: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("取消 Worker 超时")
	}
	if _, err := os.Stat(options.PIDFile); !os.IsNotExist(err) {
		t.Fatalf("取消后 PID 文件仍存在: %v", err)
	}
}

func TestRunWakeProcessesMultipleRoundsAndRestartsFromStalePID(t *testing.T) {
	requests := make(chan struct{}, 4)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests <- struct{}{}
		writer.Header().Set("ETag", `"worker-round"`)
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"worker-round","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	clock := newManualClock(time.Unix(1_700_100_000, 0))
	root, moduleConf := prepareWorkerFixture(t, server.URL, clock.Now())
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.PIDFile = filepath.Join(t.TempDir(), "worker.pid")
	options.ProgressDir = filepath.Join(t.TempDir(), "progress")
	options.Now = clock.Now
	options.NewTimer = clock.NewTimer
	markStaleWorker(t, options.PIDFile)
	wake := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, options, wake, log.New(io.Discard, "", 0)) }()
	waitRequest(t, requests)
	clock.Advance(15 * time.Minute)
	wake <- struct{}{}
	waitRequest(t, requests)
	waitRevision(t, filepath.Join(root, "fixture", "meta.json"), 2)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("多轮 Worker 退出失败: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("多轮 Worker 取消超时")
	}

	metadata, err := catalog.LoadMetadata(filepath.Join(root, "fixture", "meta.json"), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Revision != 2 {
		t.Fatalf("多轮唤醒只执行了 %d 次更新", metadata.Revision)
	}
	markStaleWorker(t, options.PIDFile)
	metadata.NextUpdateEpoch = clock.Now().Unix() - 1
	metadata.NextUpdateAt = catalog.FormatEpochUTC(metadata.NextUpdateEpoch)
	if err := catalog.SaveMetadataAtomic(filepath.Join(root, "fixture", "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}

	ctx, cancel = context.WithCancel(context.Background())
	done = make(chan error, 1)
	go func() { done <- Run(ctx, options, make(chan struct{}, 1), log.New(io.Discard, "", 0)) }()
	waitRequest(t, requests)
	waitRevision(t, filepath.Join(root, "fixture", "meta.json"), 3)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("重启恢复 Worker 退出失败: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("重启恢复 Worker 取消超时")
	}
}

func TestConcurrentSubscriptionUpdateSerializesPerGroup(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		once.Do(func() { close(started) })
		<-release
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"locked-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	now := time.Unix(1_700_200_000, 0)
	root, moduleConf := prepareWorkerFixture(t, server.URL, now)
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.PIDFile = filepath.Join(root, "worker.pid")
	options.Now = func() time.Time { return now }
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() {
		_, err := UpdateGroup(context.Background(), options, "fixture", now, nil)
		firstDone <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("首个订阅更新未进入下载阶段")
	}
	go func() {
		_, err := UpdateGroup(context.Background(), options, "fixture", now, nil)
		secondDone <- err
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("第二个订阅更新未等待分组锁: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	for name, done := range map[string]chan error{"首个": firstDone, "第二个": secondDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s订阅更新失败: %v", name, err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("%s订阅更新未结束", name)
		}
	}
}

func TestRunDueContinuesAfterOneSubscriptionFails(t *testing.T) {
	now := time.Unix(1_700_300_000, 0)
	goodServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"good-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer goodServer.Close()

	root := t.TempDir()
	moduleConf := filepath.Join(root, "module.conf")
	if err := os.WriteFile(moduleConf, []byte("ACTIVE_GROUP_ID=\"good\"\nSELECTOR_MODE=urltest\nOUTBOUND_MODE=rule\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		id  string
		url string
	}{
		{id: "good", url: goodServer.URL},
		{id: "bad", url: "http://127.0.0.1:1"},
	} {
		groupDir := filepath.Join(root, item.id)
		if err := os.MkdirAll(groupDir, 0o700); err != nil {
			t.Fatal(err)
		}
		metadata := catalog.NewMetadata(item.id, item.id, "subscription", item.url, now)
		metadata.AutoUpdate = true
		metadata.UpdateInterval = 900
		metadata.UpdateViaProxy = "never"
		metadata.NextUpdateEpoch = now.Unix() - 1
		metadata.NextUpdateAt = catalog.FormatEpochUTC(metadata.NextUpdateEpoch)
		if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
			t.Fatal(err)
		}
		if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), []byte(`{"outbounds":[]}`+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.Now = func() time.Time { return now }
	summary, err := RunDue(context.Background(), options, now, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("批量更新不应因单项失败而中断: %v", err)
	}
	if len(summary.Updated) != 1 || summary.Updated[0] != "good" || len(summary.Failed) != 1 || summary.Failed[0] != "bad" {
		t.Fatalf("批量更新摘要异常: %+v", summary)
	}
	content, err := os.ReadFile(filepath.Join(root, "good", "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "good-node") {
		t.Fatalf("成功订阅未提交 Provider: %s", content)
	}
}

func TestWorkerRetryDelayClassifiesAndCapsFailures(t *testing.T) {
	transient := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute}
	for attempt, want := range transient {
		if got := workerRetryDelay(attempt+1, workerFailureTransient); got != want {
			t.Fatalf("transient retry %d = %s, want %s", attempt+1, got, want)
		}
	}
	permanent := []time.Duration{15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 4 * time.Hour, 6 * time.Hour, 6 * time.Hour}
	for attempt, want := range permanent {
		if got := workerRetryDelay(attempt+1, workerFailurePermanent); got != want {
			t.Fatalf("permanent retry %d = %s, want %s", attempt+1, got, want)
		}
	}
	if got := workerRetryDelay(20, workerFailureTransient); got != workerTransientRetryMax {
		t.Fatalf("transient retry cap = %s, want %s", got, workerTransientRetryMax)
	}

	if got := classifyWorkerError(&subscription.Error{Code: "subscription.runtime_sync_failed"}); got != workerFailureTransient {
		t.Fatalf("runtime sync failure classified as %d", got)
	}
	if got := classifyWorkerError(&subscription.Error{Code: "provider.invalid"}); got != workerFailurePermanent {
		t.Fatalf("provider validation failure classified as %d", got)
	}
	if got := classifyWorkerError(&subscription.Error{Code: "subscription.conflict"}); got != workerFailurePermanent {
		t.Fatalf("subscription conflict classified as %d", got)
	}
}

func TestWorkerRunBacksOffScheduleFailures(t *testing.T) {
	clock := newManualClock(time.Unix(1_700_400_000, 0))
	options := newTestOptions(filepath.Join(t.TempDir(), "missing-catalog"))
	options.ModuleConf = filepath.Join(t.TempDir(), "module.conf")
	options.PIDFile = filepath.Join(t.TempDir(), "worker.pid")
	options.Now = clock.Now
	options.NewTimer = clock.NewTimer

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, options, make(chan struct{}), log.New(io.Discard, "", 0)) }()

	waitTimerDelay(t, clock, 15*time.Minute, 0)
	clock.Advance(15 * time.Minute)
	waitTimerDelay(t, clock, 30*time.Minute, 1)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Worker exited with error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Worker did not stop after cancellation")
	}
}

func TestWorkerRetrySuccessRestoresNormalSchedule(t *testing.T) {
	requests := make(chan struct{}, 4)
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		attempt++
		requests <- struct{}{}
		if attempt == 1 {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"retry-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	clock := newManualClock(time.Unix(1_700_500_000, 0))
	root, moduleConf := prepareWorkerFixture(t, server.URL, clock.Now())
	options := newTestOptions(root)
	options.ModuleConf = moduleConf
	options.PIDFile = filepath.Join(t.TempDir(), "worker.pid")
	options.ProgressDir = filepath.Join(t.TempDir(), "progress")
	options.Now = clock.Now
	options.NewTimer = clock.NewTimer

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, options, make(chan struct{}), log.New(io.Discard, "", 0)) }()
	waitRequest(t, requests)
	waitTimerDelay(t, clock, time.Minute, 0)
	clock.Advance(time.Minute)
	waitRequest(t, requests)
	waitTimerDelay(t, clock, 15*time.Minute, 1)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Worker exited with error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Worker did not stop after retry success")
	}

	metadata, err := catalog.LoadMetadata(filepath.Join(root, "fixture", "meta.json"), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.LastError != "" || metadata.RuntimeSyncPending {
		t.Fatalf("retry success left failure state: %+v", metadata)
	}
}

func waitTimerDelay(t *testing.T, clock *manualClock, want time.Duration, index int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		delays := clock.Delays()
		if len(delays) > index {
			if delays[index] != want {
				t.Fatalf("timer %d = %s, want %s", index, delays[index], want)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timer %d was not created", index)
}

func TestWorkerPIDLockHandlesStaleReuseAndForeignRelease(t *testing.T) {
	original := workerProcessPID
	defer func() { workerProcessPID = original }()
	livePID := 12345
	workerProcessPID = func(pid int) bool { return pid == livePID }

	path := filepath.Join(t.TempDir(), "worker.pid")
	lock := path + ".lock"
	if err := os.MkdirAll(lock, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lock, "pid"), []byte("12345\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := acquirePID(path); err == nil {
		t.Fatal("live Worker lock was not rejected")
	}

	workerProcessPID = func(int) bool { return false }
	if err := acquirePID(path); err != nil {
		t.Fatalf("stale Worker lock was not recovered: %v", err)
	}
	releasePID(path)
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatalf("recovered lock remains: %v", err)
	}

	for _, pidContent := range []string{"not-a-pid\n", "12345\n"} {
		if err := os.MkdirAll(lock, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(lock, "pid"), []byte(pidContent), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := acquirePID(path); err != nil {
			t.Fatalf("PID lock %q was not recovered: %v", pidContent, err)
		}
		releasePID(path)
	}

	if err := acquirePID(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lock, "pid"), []byte("54321\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	releasePID(path)
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("release removed a foreign lock: %v", err)
	}
	_ = os.RemoveAll(lock)
	_ = os.Remove(path)
}

func TestWorkerStartRequiresPIDState(t *testing.T) {
	ctx := t.Context()
	err := waitForWorkerPID(ctx, filepath.Join(t.TempDir(), "worker.pid"), os.Getpid(), 20*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "PID") {
		t.Fatalf("missing PID state was not reported: %v", err)
	}
}
