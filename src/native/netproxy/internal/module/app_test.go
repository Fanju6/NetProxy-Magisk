package module

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
	"google.golang.org/protobuf/encoding/protowire"
)

const testInboundConfig = `{
  "backend": "ebpf",
  "root_policy": "default",
  "app": {"enabled": false, "mode": "blacklist", "proxy_apps": [], "bypass_apps": []},
  "ebpf": {"type": "ebpf", "tag": "netproxy-in", "local": {"enabled": true}, "shared": {"enabled": false}},
  "tun": {"type": "tun", "tag": "netproxy-in", "interface_name": "netproxy", "address": ["172.19.0.1/30"], "auto_route": true, "auto_redirect": true}
}`

func selectionFixture(t *testing.T) Options {
	t.Helper()
	options := newTestOptions(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(options.ModuleConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("{\"selection\":{\"group_id\":\"default\",\"node_tag\":\"\"},\"wifi\":{\"blacklist\":[\"office\"]}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"NODE", "节点 / 02"} {
		if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{
			GroupDir: filepath.Join(options.CatalogRoot, "default"), GroupID: "default", Name: "本地配置", Type: "local",
			Input: "socks://127.0.0.1:1080#" + url.PathEscape(tag),
		}); err != nil {
			t.Fatal(err)
		}
	}
	return options
}

func TestSelectNodeStoppedKeepsTwoFieldStateAndPublicResult(t *testing.T) {
	options := selectionFixture(t)
	for _, target := range []string{"default/节点 / 02", "auto"} {
		result, err := SelectNode(t.Context(), options, target, "")
		if err != nil {
			t.Fatal(err)
		}
		module, err := moduleconfig.LoadModule(options.ModuleConfig)
		if err != nil || len(module.WiFi.Blacklist) != 1 || module.WiFi.Blacklist[0] != "office" || result["group_id"] != "default" || result["mode"] != module.Mode() {
			t.Fatalf("选择结果不一致: %+v %+v %v", result, module, err)
		}
		if target != "auto" && (module.SelectedNodeTag != "节点 / 02" || result["selected"] != "本地配置/节点 / 02") {
			t.Fatalf("手动选择未保留原始 tag: %+v %+v", result, module)
		}
		if target == "auto" && (module.SelectedNodeTag != "" || result["selected"] != "Auto/本地配置") {
			t.Fatalf("自动选择错误: %+v %+v", result, module)
		}
	}
	before, err := os.ReadFile(options.ModuleConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SelectNode(t.Context(), options, "default/missing", ""); err == nil {
		t.Fatal("不存在的节点选择成功")
	}
	after, err := os.ReadFile(options.ModuleConfig)
	if err != nil || string(before) != string(after) || strings.Contains(string(after), "SELECTOR_MODE") || strings.Contains(string(after), "SELECTED_NODE_REF") {
		t.Fatalf("无效选择改变了配置或写回旧字段: %s %v", after, err)
	}
	for _, path := range []string{options.StateFile, options.WorkerPIDFile, options.RuntimeDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("停止时选节点改变了服务、Worker 或 runtime: %s %v", path, err)
		}
	}
}

func TestSelectNodeWaitingForLifecycleCanCancel(t *testing.T) {
	options := selectionFixture(t)
	lock, err := acquireLifecycleLock(options.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := SelectNode(ctx, options, "default/NODE", ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("等锁时未传播取消: %v", err)
	}
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || module.SelectedNodeTag != "" {
		t.Fatalf("取消时写入了选择: %+v %v", module, err)
	}
}

func TestCatalogRemovalWaitsForLifecycleBeforeCommit(t *testing.T) {
	for _, subscriptionGroup := range []bool{false, true} {
		t.Run(fmt.Sprint(subscriptionGroup), func(t *testing.T) {
			options := selectionFixture(t)
			groupID, tag := "default", "NODE"
			if subscriptionGroup {
				groupID, tag = "subscription", "SUB"
				if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, groupID), GroupID: groupID, Type: "subscription", Input: "socks://127.0.0.1:1080#SUB"}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := SelectNode(t.Context(), options, groupID+"/"+tag, ""); err != nil {
				t.Fatal(err)
			}
			providerPath := filepath.Join(options.CatalogRoot, groupID, "provider.json")
			before, err := os.ReadFile(providerPath)
			if err != nil {
				t.Fatal(err)
			}
			lock, err := acquireLifecycleLock(options.StateFile)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.release()
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()
			if subscriptionGroup {
				err = RemoveSubscription(ctx, options, groupID, "")
			} else {
				_, err = NodeRemove(ctx, options, groupID+"/"+tag)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("等待生命周期锁未传播取消: %v", err)
			}
			after, err := os.ReadFile(providerPath)
			if err != nil || string(before) != string(after) {
				t.Fatalf("等待锁时删除了持久节点: %s %v", after, err)
			}
			module, err := moduleconfig.LoadModule(options.ModuleConfig)
			if err != nil || module.Ref() != groupID+"/"+tag {
				t.Fatalf("取消时改变了保存的选择: %+v %v", module, err)
			}
		})
	}
}

func TestCatalogSyncKeepsChoiceSavedWhileWaitingForConfigLock(t *testing.T) {
	options := selectionFixture(t)
	editor, err := moduleconfig.Lock(t.Context(), options.ModuleConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer editor.Release()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := SyncCatalog(ctx, options, "default", false)
		done <- err
	}()
	cancel()
	if err := editor.UpdateSelection(moduleconfig.Selection{ActiveGroupID: "default", SelectedNodeTag: "NODE"}); err != nil {
		t.Fatal(err)
	}
	if err := editor.Release(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || module.SelectedNodeTag != "NODE" || len(module.WiFi.Blacklist) != 1 || module.WiFi.Blacklist[0] != "office" {
		t.Fatalf("覆盖了并发保存的选择或网络设置: %+v %v", module, err)
	}
}

func TestNodeRemovalRejectsBrokenModuleConfigBeforeCommit(t *testing.T) {
	options := selectionFixture(t)
	providerPath := filepath.Join(options.CatalogRoot, "default", "provider.json")
	before, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("{\"selection\":{\"group_id\":\"default\",\"node_tag\":\" \"}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NodeRemove(t.Context(), options, "default/NODE"); err == nil {
		t.Fatal("模块配置损坏时仍删除了节点")
	}
	after, err := os.ReadFile(providerPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("预检失败后 Provider 被改变: %s %v", after, err)
	}
}

func TestCatalogSyncFinishesSavedChoiceAfterCancellation(t *testing.T) {
	options := selectionFixture(t)
	if _, err := SelectNode(t.Context(), options, "default/NODE", ""); err != nil {
		t.Fatal(err)
	}
	options, saved, release, err := lockCatalogChange(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := catalog.RemoveNode(t.Context(), catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, "default"), GroupID: "default", Tag: "NODE"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	state, attempted, err := syncCatalogChange(ctx, options, saved, "default", false, false)
	if err != nil || attempted || state != subscription.RuntimeSyncNotRunning {
		t.Fatalf("提交后的本地选择未完成收敛: %s %v %v", state, attempted, err)
	}
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || module.ActiveGroupID != "default" || module.SelectedNodeTag != "" {
		t.Fatalf("保留了已删除节点的引用: %+v %v", module, err)
	}
}

func TestWorkerCancellationAfterCommitNormalizesSelection(t *testing.T) {
	options := selectionFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"outbounds":[{"type":"socks","tag":"NEW","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()
	const groupID = "subscription"
	if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{
		Root: options.CatalogRoot, GroupID: groupID, Name: "fixture", Type: "subscription",
		URL: server.URL, UpdateViaProxy: "never",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{
		GroupDir: filepath.Join(options.CatalogRoot, groupID), GroupID: groupID, Type: "subscription",
		Input: "socks://127.0.0.1:1080#OLD",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := SelectNode(t.Context(), options, groupID+"/OLD", ""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	workerOptions := workerOptions(options)
	workerOptions.SyncCatalog = func(ctx context.Context, group string, structural bool) (string, bool, error) {
		cancel()
		return SyncCatalog(ctx, options, group, structural)
	}
	result, err := worker.UpdateGroup(ctx, workerOptions, groupID, time.Now(), nil)
	if err != nil || !result.Persisted || result.RuntimeSynced || result.RuntimeSyncPending || result.RuntimeSyncState != subscription.RuntimeSyncNotRunning {
		t.Fatalf("提交后取消未完成本地状态整理: %+v %v", result, err)
	}
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || module.ActiveGroupID != groupID || module.SelectedNodeTag != "" || len(module.WiFi.Blacklist) != 1 || module.WiFi.Blacklist[0] != "office" {
		t.Fatalf("提交后取消保留了失效选择或覆盖网络设置: %+v %v", module, err)
	}
	metadata, err := catalog.PrivateMetadata(t.Context(), options.CatalogRoot, groupID)
	if err != nil || metadata.RuntimeSyncState != subscription.RuntimeSyncNotRunning || metadata.LastError != "" {
		t.Fatalf("提交后取消未落盘正确的订阅同步状态: %+v %v", metadata, err)
	}
	document, err := catalog.GroupProvider(t.Context(), options.CatalogRoot, groupID)
	if err != nil || len(document.Outbounds) != 1 || document.Outbounds[0].Tag != "NEW" {
		t.Fatalf("取消回滚了已提交 Provider: %+v %v", document, err)
	}
}

func TestCatalogSyncCancelledSkipsRuntime(t *testing.T) {
	options := selectionFixture(t)
	if _, err := SelectNode(t.Context(), options, "default/NODE", ""); err != nil {
		t.Fatal(err)
	}
	options, saved, release, err := lockCatalogChange(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := catalog.RemoveNode(t.Context(), catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, "default"), GroupID: "default", Tag: "NODE"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("取消后仍调用了运行时 API")
		_, _ = w.Write(make([]byte, 5))
	}))
	defer server.Close()
	options.ServiceAddress = server.URL
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, attempted, err := syncCatalogChange(ctx, options, saved, "default", false, true)
	if !errors.Is(err, context.Canceled) || !attempted {
		t.Fatalf("运行时同步未保留原请求取消: %v %v", attempted, err)
	}
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || module.SelectedNodeTag != "" {
		t.Fatalf("运行时取消阻止了持久选择整理: %+v %v", module, err)
	}
}

func TestCatalogSyncCleanupDeadlineIncludesLockWait(t *testing.T) {
	options := selectionFixture(t)
	lock, err := acquireLifecycleLock(options.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	started := time.Now()
	_, attempted, err := SyncCatalog(ctx, options, "default", false)
	if !errors.Is(err, context.DeadlineExceeded) || attempted || time.Since(started) > 7*time.Second {
		t.Fatalf("收尾等待锁没有遵循独立时限: attempted=%v elapsed=%v err=%v", attempted, time.Since(started), err)
	}
}

func TestRemoveSubscriptionRecoversConfigBeforeDeletion(t *testing.T) {
	options := selectionFixture(t)
	isolateConfigApplyHooks(t, false)
	if err := os.MkdirAll(filepath.Dir(options.InboundConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.InboundConfig, []byte(testInboundConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, "subscription"), GroupID: "subscription", Type: "subscription", Input: "socks://127.0.0.1:1080#SUB"}); err != nil {
		t.Fatal(err)
	}
	if _, err := SelectNode(t.Context(), options, "subscription/SUB", ""); err != nil {
		t.Fatal(err)
	}
	transaction, err := beginConfigApply(options, options.ModuleConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.setPhase("reload_started"); err != nil {
		t.Fatal(err)
	}
	configSnapshotRestore = func(snapshot configFileSnapshot) error {
		if _, err := os.Stat(filepath.Join(options.CatalogRoot, "subscription")); err != nil {
			t.Fatalf("删除订阅发生在配置恢复之前: %v", err)
		}
		return restoreConfigSnapshot(snapshot)
	}
	if err := RemoveSubscription(t.Context(), options, "subscription", ""); err != nil {
		t.Fatal(err)
	}
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || module.ActiveGroupID != "default" || module.SelectedNodeTag != "" {
		t.Fatalf("恢复覆盖了删除后的选择: %+v %v", module, err)
	}
	if _, err := os.Stat(configTransactionPath(options)); !os.IsNotExist(err) {
		t.Fatalf("配置恢复未清理 journal: %v", err)
	}
}

func TestSubscriptionPending304ReappliesSavedAutoSelection(t *testing.T) {
	options := selectionFixture(t)
	groupID := "subscription"
	selectCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sub":
			w.Header().Set("ETag", `"fixture"`)
			if r.Header.Get("If-None-Match") != "" {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			_, _ = w.Write([]byte(`{"outbounds":[{"type":"socks","tag":"NODE","server":"127.0.0.1","server_port":1080}]}`))
		case "/daemon.StartedService/SelectOutbound":
			selectCalls++
			body, err := io.ReadAll(r.Body)
			expected := protowire.AppendString(protowire.AppendTag(nil, 1, protowire.BytesType), "Proxy")
			expected = protowire.AppendString(protowire.AppendTag(expected, 2, protowire.BytesType), "Auto/订阅测试")
			if err != nil || len(body) < 5 || !bytes.Equal(body[5:], expected) {
				t.Errorf("重试未应用持久化的同组 Auto: %x %v", body, err)
			}
			if selectCalls == 1 {
				http.Error(w, "selection rejected", http.StatusBadRequest)
				return
			}
			_, _ = w.Write(make([]byte, 5))
		case "/daemon.StartedService/SubscribeOutbounds":
			item := protowire.AppendString(protowire.AppendTag(nil, 1, protowire.BytesType), "订阅测试/NODE")
			payload := protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType), item)
			frame := make([]byte, 5)
			binary.BigEndian.PutUint32(frame[1:], uint32(len(payload)))
			_, _ = w.Write(append(frame, payload...))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	options.ServiceAddress = server.URL
	if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{Root: options.CatalogRoot, GroupID: groupID, Name: "订阅测试", Type: "subscription", URL: server.URL + "/sub", UpdateViaProxy: "never"}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, groupID), GroupID: groupID, Type: "subscription", Input: "socks://127.0.0.1:1080#OLD"}); err != nil {
		t.Fatal(err)
	}
	if _, err := SelectNode(t.Context(), options, groupID+"/OLD", ""); err != nil {
		t.Fatal(err)
	}
	workerOptions := workerOptions(options)
	workerOptions.SyncCatalog = func(ctx context.Context, groupID string, structural bool) (string, bool, error) {
		if structural {
			t.Error("已有 Provider 更新或 pending 304 不应重载核心")
		}
		locked, saved, release, err := lockCatalogChange(ctx, options)
		if err != nil {
			return "", false, err
		}
		defer release()
		return syncCatalogChange(ctx, locked, saved, groupID, structural, true)
	}
	first, err := worker.UpdateGroup(t.Context(), workerOptions, groupID, time.Now(), nil)
	var syncErr *subscription.Error
	if !errors.As(err, &syncErr) || syncErr.Code != "subscription.runtime_sync_failed" || !first.RuntimeSyncPending || first.RuntimeSyncState != subscription.RuntimeSyncFailed || selectCalls != 1 {
		t.Fatalf("选择 API 失败未保留运行时 pending: %+v calls=%d err=%v", first, selectCalls, err)
	}
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || module.ActiveGroupID != groupID || module.SelectedNodeTag != "" {
		t.Fatalf("未持久化同组 Auto: %+v %v", module, err)
	}
	second, err := worker.UpdateGroup(t.Context(), workerOptions, groupID, time.Now(), nil)
	if err != nil || !second.NotModified || !second.RuntimeSynced || second.RuntimeSyncPending || selectCalls != 2 {
		t.Fatalf("304 未重新应用保存的 Auto: %+v calls=%d err=%#v", second, selectCalls, err)
	}
	metadata, err := catalog.PrivateMetadata(t.Context(), options.CatalogRoot, groupID)
	if err != nil || metadata.RuntimeSyncPending || metadata.LastError != "" || metadata.RuntimeSyncState != subscription.RuntimeSyncApplied {
		t.Fatalf("同步成功未清理 pending 和运行时错误: %+v %v", metadata, err)
	}
}

func TestSyncSelectionDoesNotSaveOrReload(t *testing.T) {
	options := selectionFixture(t)
	if err := moduleconfig.UpdateSelection(t.Context(), options.ModuleConfig, moduleconfig.Selection{ActiveGroupID: "default", SelectedNodeTag: "NODE"}); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(options.ModuleConfig, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(options.ModuleConfig)
	if err != nil {
		t.Fatal(err)
	}
	for _, fail := range []bool{false, true} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.URL.Path != "/daemon.StartedService/SelectOutbound" {
				t.Errorf("同步调用了非选择接口: %s", r.URL.Path)
			}
			if fail {
				http.Error(w, "selection rejected", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/grpc-web+proto")
			_, _ = w.Write(make([]byte, 5))
		}))
		options.ServiceAddress = server.URL
		err := syncSelection(t.Context(), options)
		server.Close()
		if (err != nil) != fail || (!fail && calls != 2) || (fail && calls != 1) {
			t.Fatalf("同步调用异常: fail=%v calls=%d err=%v", fail, calls, err)
		}
	}
	after, err := os.ReadFile(options.ModuleConfig)
	info, statErr := os.Stat(options.ModuleConfig)
	if err != nil || statErr != nil || string(before) != string(after) || !info.ModTime().Equal(stamp) {
		t.Fatalf("运行时同步重新保存了选择: %v %v", err, statErr)
	}
	if _, err := os.Stat(options.StateFile); !os.IsNotExist(err) {
		t.Fatalf("API 失败触发了服务操作: %v", err)
	}
}

func TestSyncRuntimeSelectorRetriesOnlyTemporaryFailures(t *testing.T) {
	for _, test := range []struct {
		name       string
		httpStatus int
		grpcStatus int
		retry      bool
		failAt     int32
	}{
		{"http-unavailable", http.StatusServiceUnavailable, 0, true, 1},
		{"proxy-http-unavailable", http.StatusServiceUnavailable, 0, true, 2},
		{"grpc-unavailable", 0, 14, true, 1},
		{"request-timeout", 0, 0, true, 1},
		{"http-unauthorized", http.StatusUnauthorized, 0, false, 1},
		{"http-not-found", http.StatusNotFound, 0, false, 1},
		{"grpc-not-found", 0, 5, false, 1},
		{"grpc-permission-denied", 0, 7, false, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if r.URL.Path != "/daemon.StartedService/SelectOutbound" {
					t.Errorf("重试触发了选择以外的 API: %s", r.URL.Path)
				}
				if calls.Add(1) == test.failAt {
					switch {
					case test.httpStatus != 0:
						http.Error(w, "selection unavailable", test.httpStatus)
					case test.grpcStatus != 0:
						payload := []byte(fmt.Sprintf("grpc-status: %d\r\ngrpc-message: selection unavailable\r\n", test.grpcStatus))
						frame := make([]byte, 5)
						frame[0] = 0x80
						binary.BigEndian.PutUint32(frame[1:], uint32(len(payload)))
						_, _ = w.Write(append(frame, payload...))
					default:
						<-r.Context().Done()
					}
					return
				}
				_, _ = w.Write(make([]byte, 5))
			}))
			defer server.Close()
			options := Options{ServiceAddress: server.URL, RequestTimeout: 3 * time.Second}
			err := syncRuntimeSelector(t.Context(), options, "Select/fixture", "fixture/NODE")
			wantCalls := test.failAt
			if test.retry {
				wantCalls += 2
			}
			if (err == nil) != test.retry || calls.Load() != wantCalls {
				t.Fatalf("临时错误重试或永久错误停止异常: calls=%d err=%v", calls.Load(), err)
			}
		})
	}
}

func TestSyncRuntimeSelectorHonorsTimeoutAndCancellation(t *testing.T) {
	for _, scenario := range []string{"request-deadline", "configured-timeout", "cancel-before-request", "cancel-during-request"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			options := Options{RequestTimeout: 6 * time.Second}
			wantErr := context.DeadlineExceeded
			wantCalls := int32(1)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if scenario == "cancel-during-request" {
					cancel()
				}
				http.Error(w, "temporary failure", http.StatusServiceUnavailable)
			}))
			defer server.Close()
			options.ServiceAddress = server.URL
			switch scenario {
			case "request-deadline":
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			case "configured-timeout":
				options.RequestTimeout = 100 * time.Millisecond
			case "cancel-before-request":
				cancel()
				wantErr, wantCalls = context.Canceled, 0
			case "cancel-during-request":
				wantErr = context.Canceled
			}
			started := time.Now()
			err := syncRuntimeSelector(ctx, options, "Auto/fixture", "")
			if !errors.Is(err, wantErr) || calls.Load() != wantCalls || time.Since(started) > time.Second {
				t.Fatalf("重试未遵循总时限或请求取消: calls=%d elapsed=%v err=%v", calls.Load(), time.Since(started), err)
			}
			if scenario == "configured-timeout" && !strings.Contains(err.Error(), "HTTP 503") {
				t.Fatalf("超时未保留最后一次 API 失败原因: %v", err)
			}
		})
	}
}

func TestNodeRemoveClearsMissingSelectionAndEmptyCatalog(t *testing.T) {
	options := selectionFixture(t)
	if _, err := SelectNode(t.Context(), options, "default/NODE", ""); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"default/NODE", "default/节点 / 02"} {
		if _, err := NodeRemove(t.Context(), options, target); err != nil {
			t.Fatal(err)
		}
		module, err := moduleconfig.LoadModule(options.ModuleConfig)
		if err != nil || module.ActiveGroupID != "default" || module.SelectedNodeTag != "" {
			t.Fatalf("删除节点后没有保留同组自动选择: %+v %v", module, err)
		}
	}
}

func TestRemoveSubscriptionNormalizesLatestSelection(t *testing.T) {
	for _, test := range []struct {
		name        string
		selected    moduleconfig.Selection
		replacement string
		want        moduleconfig.Selection
	}{
		{"inactive", moduleconfig.Selection{ActiveGroupID: "default", SelectedNodeTag: "NODE"}, "replacement", moduleconfig.Selection{ActiveGroupID: "default", SelectedNodeTag: "NODE"}},
		{"automatic-replacement", moduleconfig.Selection{ActiveGroupID: "subscription", SelectedNodeTag: "SUB"}, "", moduleconfig.Selection{ActiveGroupID: "default"}},
		{"specified-replacement", moduleconfig.Selection{ActiveGroupID: "subscription", SelectedNodeTag: "SUB"}, "replacement", moduleconfig.Selection{ActiveGroupID: "replacement"}},
		{"invalid-replacement", moduleconfig.Selection{ActiveGroupID: "subscription", SelectedNodeTag: "SUB"}, "subscription", moduleconfig.Selection{ActiveGroupID: "subscription", SelectedNodeTag: "SUB"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := selectionFixture(t)
			for _, group := range []string{"subscription", "replacement"} {
				if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{Root: options.CatalogRoot, GroupID: group, Name: group, Type: "subscription", URL: "https://example.invalid/sub"}); err != nil {
					t.Fatal(err)
				}
				if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, group), GroupID: group, Type: "subscription", Input: "socks://127.0.0.1:1080#SUB"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := moduleconfig.UpdateSelection(t.Context(), options.ModuleConfig, test.selected); err != nil {
				t.Fatal(err)
			}
			err := RemoveSubscription(t.Context(), options, "subscription", test.replacement)
			if (err != nil) != (test.name == "invalid-replacement") {
				t.Fatalf("删除结果异常: %v", err)
			}
			module, err := moduleconfig.LoadModule(options.ModuleConfig)
			if err != nil || module.Selection != test.want {
				t.Fatalf("删除覆盖了当前选择: %+v %v", module, err)
			}
			_, err = os.Stat(filepath.Join(options.CatalogRoot, "subscription"))
			if test.name == "invalid-replacement" && err != nil || test.name != "invalid-replacement" && !os.IsNotExist(err) {
				t.Fatalf("订阅目录状态异常: %v", err)
			}
		})
	}
}

func TestSelectionTargetCoreAppliesSavedChoice(t *testing.T) {
	core := targetCoreBinary(t)
	options := selectionFixture(t)
	if err := moduleconfig.UpdateSelection(t.Context(), options.ModuleConfig, moduleconfig.Selection{ActiveGroupID: "default", SelectedNodeTag: "节点 / 02"}); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().(*net.TCPAddr)
	_ = listener.Close()
	options.ServiceAddress, options.ServiceSecret = address.String(), "fixture"
	configPath := filepath.Join(options.SingBoxDir, "config.json")
	if err := os.MkdirAll(options.SingBoxDir, 0o700); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf(`{"log":{"disabled":true},"experimental":{"cache_file":{"enabled":true,"path":"cache.db"}},"services":[{"type":"api","listen":"127.0.0.1","listen_port":%d,"secret":"fixture"}]}`, address.Port)
	targetCoreWrite(t, configPath, []byte(content))
	providersPath, outboundsPath := filepath.Join(options.RuntimeDir, "providers.json"), filepath.Join(options.RuntimeDir, "outbounds.json")
	if _, err := catalog.BuildRuntime(t.Context(), catalog.RuntimeOptions{Root: options.CatalogRoot, ModuleConfig: options.ModuleConfig, ProvidersOutput: providersPath, OutboundsOutput: outboundsPath}); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(t.Context(), core, "check", "-c", configPath, "-c", providersPath, "-c", outboundsPath).CombinedOutput(); err != nil {
		t.Fatalf("真实核心 check 失败: %v\n%s", err, output)
	}
	command := exec.CommandContext(t.Context(), core, "run", "-c", configPath, "-c", providersPath, "-c", outboundsPath)
	command.Dir = options.SingBoxDir
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	client, err := serviceapi.New(options.ServiceAddress, options.ServiceSecret)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for {
		if _, err := client.Ready(ctx); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("真实核心 API 未就绪")
		case <-time.After(25 * time.Millisecond):
		}
	}
	if err := client.SelectGroup(ctx, "Select/本地配置", "本地配置/NODE"); err != nil {
		t.Fatal(err)
	}
	syncSaved := func() {
		locked, saved, release, err := lockCatalogChange(ctx, options)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		state, attempted, err := syncCatalogChange(ctx, locked, saved, "default", false, true)
		if err != nil || !attempted || state != subscription.RuntimeSyncApplied {
			t.Fatalf("真实核心统一选择同步失败: %s %v %v", state, attempted, err)
		}
	}
	syncSaved()
	groups, err := client.Groups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	matched := 0
	for _, group := range groups {
		if group.Tag == "Proxy" && group.Selected == "Select/本地配置" || group.Tag == "Select/本地配置" && group.Selected == "本地配置/节点 / 02" {
			matched++
		}
	}
	if matched != 2 {
		t.Fatalf("真实核心没有恢复保存的手动选择: %+v", groups)
	}
	started, err := client.StartedAt(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncRuntimeSelector(ctx, options, "Select/本地配置", "本地配置/missing"); err == nil {
		t.Fatal("真实核心接受了不存在的节点")
	}
	after, err := client.StartedAt(ctx)
	if err != nil || after != started {
		t.Fatalf("失败的选择触发了重载: %+v %+v %v", started, after, err)
	}
	if err := moduleconfig.UpdateSelection(ctx, options.ModuleConfig, moduleconfig.Selection{ActiveGroupID: "default"}); err != nil {
		t.Fatal(err)
	}
	syncSaved()
	groups, err = client.Groups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range groups {
		if group.Tag == "Proxy" && group.Selected == "Auto/本地配置" {
			return
		}
	}
	t.Fatalf("真实核心没有切回同组 Auto: %+v", groups)
}

func TestNodeImportAppendsToDefaultGroup(t *testing.T) {
	root := t.TempDir()
	options := newTestOptions(root)
	if err := os.MkdirAll(filepath.Dir(options.ModuleConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("{\"selection\":{\"group_id\":\"default\",\"node_tag\":\"\"}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InitializeGroup(context.Background(), catalog.GroupOptions{
		Root: options.CatalogRoot, GroupID: "default", Name: "已有本地配置", Type: "local",
	}); err != nil {
		t.Fatalf("initialize default group: %v", err)
	}
	if _, err := catalog.AppendNode(context.Background(), catalog.MutationOptions{
		GroupDir: filepath.Join(options.CatalogRoot, "default"), GroupID: "default",
		Name: "已有本地配置", Type: "local", Input: "socks://existing.example:1080#EXISTING",
	}); err != nil {
		t.Fatalf("append existing node: %v", err)
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
	ids, err := catalog.GroupIDs(context.Background(), options.CatalogRoot, "all")
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
	metadata, err := catalog.LoadMetadata(context.Background(), filepath.Join(options.CatalogRoot, "default", "meta.json"), "default")
	if err != nil {
		t.Fatalf("load default metadata: %v", err)
	}
	if metadata.Name != "已有本地配置" {
		t.Fatalf("default group name changed unexpectedly: %q", metadata.Name)
	}
}

func TestLoadAppPolicyReturnsTypedSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbound.json")
	content := strings.Replace(testInboundConfig, `"enabled": false, "mode": "blacklist", "proxy_apps": [], "bypass_apps": []`, `"enabled": true, "mode": "whitelist", "proxy_apps": ["0:com.example.one","10:com.example.two"], "bypass_apps": ["0:com.example.three"]`, 1)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	policy, err := LoadAppPolicy(path)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Enabled || policy.Mode != "whitelist" || policy.ProxyApps != "0:com.example.one,10:com.example.two" || policy.BypassApps != "0:com.example.three" {
		t.Fatalf("unexpected app policy: %+v", policy)
	}
}

func TestUpdateAllSubscriptionsPreservesStructuredFailure(t *testing.T) {
	root := t.TempDir()
	options := newTestOptions(root)
	if err := os.MkdirAll(filepath.Dir(options.ModuleConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("{\"selection\":{\"group_id\":\"default\"}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InitializeGroup(context.Background(), catalog.GroupOptions{
		Root: options.CatalogRoot, GroupID: "failed-subscription", Name: "failed-subscription",
		Type: "subscription", URL: "http://127.0.0.1:1", AutoUpdate: true,
		UpdateInterval: 900, UpdateViaProxy: "never", Timeout: 1,
	}); err != nil {
		t.Fatal(err)
	}

	summary, err := UpdateAllSubscriptions(context.Background(), options)
	if err == nil {
		t.Fatal("update-all hid the subscription failure")
	}
	if len(summary.Failed) != 1 || summary.Failed[0] != "failed-subscription" {
		t.Fatalf("unexpected update-all summary: %+v", summary)
	}
	var subscriptionErr *subscription.Error
	if !errors.As(err, &subscriptionErr) || subscriptionErr.Code != "subscription.convert_failed" {
		t.Fatalf("update-all did not preserve the structured error: %v", err)
	}
}

func TestEditSubscriptionFailureReportsPersistedSettings(t *testing.T) {
	root := t.TempDir()
	options := newTestOptions(root)
	now := time.Unix(1_700_450_500, 0)
	if err := catalog.InitializeGroup(context.Background(), catalog.GroupOptions{
		Root: options.CatalogRoot, GroupID: "edit-failure", Name: "edit-failure",
		Type: "subscription", URL: "https://example.invalid/sub", AutoUpdate: true,
		UpdateInterval: 900, UpdateViaProxy: "never", Timeout: 1,
	}); err != nil {
		t.Fatal(err)
	}
	badURL := "http://127.0.0.1:1/sub"
	result, err := EditSubscription(context.Background(), options, "edit-failure", subscription.EditOptions{
		URL: &badURL, Now: now,
	})
	if err == nil {
		t.Fatal("failed subscription edit was reported as success")
	}
	if !result.Persisted {
		t.Fatalf("edited settings were reported as not persisted: %+v", result)
	}
	var subscriptionErr *subscription.Error
	if !errors.As(err, &subscriptionErr) || subscriptionErr.Code != "subscription.convert_failed" {
		t.Fatalf("unexpected edit failure: %v", err)
	}
	data, ok := subscriptionErr.Data.(map[string]any)
	if !ok || data["persisted"] != true {
		t.Fatalf("structured error lost persisted=true: %#v", subscriptionErr.Data)
	}
	metadata, err := catalog.LoadMetadata(context.Background(), filepath.Join(options.CatalogRoot, "edit-failure", "meta.json"), "edit-failure")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.URL != badURL {
		t.Fatalf("edited URL was not retained after download failure: %q", metadata.URL)
	}
}

func TestAddSubscriptionCancellationReportsPersistedGroup(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()

	root := t.TempDir()
	options := newTestOptions(root)
	ctx, cancel := context.WithCancel(context.Background())
	type outcome struct {
		result subscription.Result
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := AddSubscription(ctx, SubscriptionOptions{
			Options: options, Name: "cancelled-add", URL: server.URL,
			AutoUpdate: true, UpdateInterval: 900, UpdateViaProxy: "never", Timeout: 60,
		})
		finished <- outcome{result: result, err: err}
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("subscription request did not start")
	}
	result := <-finished
	if result.err == nil || !result.result.Persisted {
		t.Fatalf("cancelled add did not report its persisted group: result=%+v err=%v", result.result, result.err)
	}
	var subscriptionErr *subscription.Error
	if !errors.As(result.err, &subscriptionErr) {
		t.Fatalf("cancelled add lost its structured error: %v", result.err)
	}
	data, ok := subscriptionErr.Data.(map[string]any)
	if !ok || data["persisted"] != true {
		t.Fatalf("cancelled add lost persisted=true: %#v", subscriptionErr.Data)
	}
	groups, err := catalog.GroupIDs(context.Background(), options.CatalogRoot, "subscription")
	if err != nil || len(groups) != 1 {
		t.Fatalf("persisted subscription group = %v, err=%v", groups, err)
	}
}

func TestEditSubscriptionSchedulingOnlyDoesNotReload(t *testing.T) {
	root := t.TempDir()
	options := newTestOptions(root)
	if err := os.MkdirAll(filepath.Dir(options.ModuleConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("{\"selection\":{\"group_id\":\"default\"}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_450_000, 0)
	if err := catalog.InitializeGroup(context.Background(), catalog.GroupOptions{
		Root: options.CatalogRoot, GroupID: "schedule-only", Name: "schedule-only",
		Type: "subscription", URL: "https://example.invalid/sub", AutoUpdate: true,
		UpdateInterval: 900, UpdateViaProxy: "never", Timeout: 60,
	}); err != nil {
		t.Fatal(err)
	}
	interval := int64(1800)
	result, err := EditSubscription(context.Background(), options, "schedule-only", subscription.EditOptions{
		UpdateInterval: &interval, Now: now,
	})
	if err != nil {
		t.Fatalf("编辑调度字段失败: %v", err)
	}
	if !result.Persisted || result.RequiresUpdate || result.RuntimeSyncState != subscription.RuntimeSyncNotRunning {
		t.Fatalf("调度字段编辑结果异常: %+v", result)
	}
	metadata, err := catalog.LoadMetadata(context.Background(), filepath.Join(options.CatalogRoot, "schedule-only", "meta.json"), "schedule-only")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.UpdateInterval != interval || metadata.NextUpdateEpoch == 0 {
		t.Fatalf("调度字段未正确持久化: %+v", metadata)
	}
}

func TestEditSubscriptionHistoryFailureKeepsProviderAndMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"edited-provider","server":"127.0.0.1","server_port":1080}]}`))
	}))
	defer server.Close()

	root := t.TempDir()
	options := newTestOptions(root)
	if err := os.MkdirAll(filepath.Dir(options.ModuleConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("{\"selection\":{\"group_id\":\"default\"}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := catalog.InitializeGroup(context.Background(), catalog.GroupOptions{
		Root: options.CatalogRoot, GroupID: "history-edit", Name: "history-edit",
		Type: "subscription", URL: "https://old.example/sub", AutoUpdate: true,
		UpdateInterval: 900, UpdateViaProxy: "never", Timeout: 60,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(options.CatalogRoot, "history-edit", "history.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_451_000, 0)
	newURL := server.URL
	result, err := EditSubscription(context.Background(), options, "history-edit", subscription.EditOptions{
		URL: &newURL, Now: now,
	})
	if err == nil {
		t.Fatal("历史写入失败时不应返回普通成功")
	}
	var subscriptionErr *subscription.Error
	if !errors.As(err, &subscriptionErr) || subscriptionErr.Code != "subscription.history_write_failed" {
		t.Fatalf("未返回结构化历史错误: %v", err)
	}
	if !result.Persisted {
		t.Fatalf("历史写入失败不应伪装成未保存: %+v", result)
	}
	metadata, err := catalog.LoadMetadata(context.Background(), filepath.Join(options.CatalogRoot, "history-edit", "meta.json"), "history-edit")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.URL != server.URL {
		t.Fatalf("编辑后的 URL 未保留: %q", metadata.URL)
	}
	if _, err := provider.Load(context.Background(), filepath.Join(options.CatalogRoot, "history-edit", "provider.json")); err != nil {
		t.Fatalf("Provider 未保留为完整可读文件: %v", err)
	}
	if !strings.Contains(subscriptionErr.Error(), "订阅历史写入失败") {
		t.Fatalf("历史错误消息不明确: %v", subscriptionErr)
	}
}
