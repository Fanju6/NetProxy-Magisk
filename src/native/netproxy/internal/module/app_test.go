package module

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
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

func catalogFixture(t *testing.T) Options {
	t.Helper()
	options := newTestOptions(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(options.ModuleConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("{\"wifi\":{\"blacklist\":[\"office\"]}}"), 0o600); err != nil {
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
	options.SingBoxPath = fakeSingBox(t)
	writeNativeTestConfig(t, options)
	return options
}

func writeNativeTestConfig(t *testing.T, options Options) {
	t.Helper()
	for path, content := range map[string]string{
		options.InboundConfig:                            testInboundConfig,
		filepath.Join(options.SingBoxDir, "config.json"): `{"outbounds":[{"type":"selector","tag":"Proxy","outbounds":["direct","block"]},{"type":"direct","tag":"direct"},{"type":"direct","tag":"block"}],"route":{"final":"Proxy"}}`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPolicySelectionWaitingForLifecycleCanCancel(t *testing.T) {
	options := catalogFixture(t)
	before := targetCoreRead(t, options.ModuleConfig)
	lock, err := acquireLifecycleLock(options.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := SelectPolicyGroup(ctx, options, "Main", "direct"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("policy selection lost lifecycle cancellation: %v", err)
	}
	if after := targetCoreRead(t, options.ModuleConfig); !bytes.Equal(before, after) {
		t.Fatal("policy selection changed module config")
	}
}

func TestCatalogRemovalWaitsForLifecycleBeforeCommit(t *testing.T) {
	for _, subscriptionGroup := range []bool{false, true} {
		t.Run(fmt.Sprint(subscriptionGroup), func(t *testing.T) {
			options := catalogFixture(t)
			groupID, tag := "default", "NODE"
			if subscriptionGroup {
				groupID, tag = "subscription", "SUB"
				if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, groupID), GroupID: groupID, Type: "subscription", Input: "socks://127.0.0.1:1080#SUB"}); err != nil {
					t.Fatal(err)
				}
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
				err = RemoveSubscription(ctx, options, groupID)
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
			if err != nil || len(module.WiFi.Blacklist) != 1 || module.WiFi.Blacklist[0] != "office" {
				t.Fatalf("取消时改变了模块网络设置: %+v %v", module, err)
			}
		})
	}
}

func TestCatalogSyncDoesNotWriteModuleConfig(t *testing.T) {
	options := catalogFixture(t)
	before := targetCoreRead(t, options.ModuleConfig)
	editor, err := moduleconfig.Lock(t.Context(), options.ModuleConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer editor.Release()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	state, attempted, err := SyncCatalog(ctx, options, "default", false)
	if err != nil || attempted || state != subscription.RuntimeSyncNotRunning {
		t.Fatalf("Catalog sync depended on module writer: %s %v %v", state, attempted, err)
	}
	if after := targetCoreRead(t, options.ModuleConfig); !bytes.Equal(before, after) {
		t.Fatal("Catalog sync rewrote module config")
	}
}

func TestCatalogSyncStoppedPreservesModuleAfterCancellation(t *testing.T) {
	options := catalogFixture(t)
	before := targetCoreRead(t, options.ModuleConfig)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	state, attempted, err := SyncCatalog(ctx, options, "default", false)
	if err != nil || attempted || state != subscription.RuntimeSyncNotRunning {
		t.Fatalf("stopped Catalog cleanup lost cancellation isolation: %s %v %v", state, attempted, err)
	}
	if after := targetCoreRead(t, options.ModuleConfig); !bytes.Equal(before, after) {
		t.Fatal("cancelled Catalog sync changed module config")
	}
}

func TestCatalogSyncReloadsWhenProviderWatcherDoesNotConverge(t *testing.T) {
	options := catalogFixture(t)
	if _, err := Prepare(t.Context(), options); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/daemon.StartedService/SubscribeOutbounds" {
			http.NotFound(w, r)
			return
		}
		item := protowire.AppendString(protowire.AppendTag(nil, 1, protowire.BytesType), "unrelated/node")
		payload := protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType), item)
		frame := make([]byte, 5)
		binary.BigEndian.PutUint32(frame[1:], uint32(len(payload)))
		_, _ = w.Write(append(frame, payload...))
	}))
	defer server.Close()
	options.ServiceAddress = server.URL
	isolateConfigApplyHooks(t, true)
	reloads := 0
	configReload = func(context.Context, Options) error { reloads++; return nil }
	state, attempted, err := syncCatalogChange(t.Context(), options, "default", true, false)
	if err != nil || !attempted || state != subscription.RuntimeSyncApplied || reloads != 1 {
		t.Fatalf("unconfirmed Provider update must reload once: state=%s attempted=%v reloads=%d err=%v", state, attempted, reloads, err)
	}
}

func TestWorkerCancellationAfterCommitPreservesProviderAndModule(t *testing.T) {
	options := catalogFixture(t)
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
	if err != nil || len(module.WiFi.Blacklist) != 1 || module.WiFi.Blacklist[0] != "office" {
		t.Fatalf("提交后取消覆盖了模块网络设置: %+v %v", module, err)
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
	options := catalogFixture(t)
	options, release, err := lockCatalogChange(t.Context(), options)
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
	_, attempted, err := syncCatalogChange(ctx, options, "default", true, false)
	if !errors.Is(err, context.Canceled) || attempted {
		t.Fatalf("运行时同步未保留原请求取消: %v %v", attempted, err)
	}
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || len(module.WiFi.Blacklist) != 1 {
		t.Fatalf("运行时取消改变了独立模块设置: %+v %v", module, err)
	}
}

func TestCatalogSyncCleanupDeadlineIncludesLockWait(t *testing.T) {
	options := catalogFixture(t)
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
	options := catalogFixture(t)
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
	if err := RemoveSubscription(t.Context(), options, "subscription"); err != nil {
		t.Fatal(err)
	}
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil || len(module.WiFi.Blacklist) != 1 || module.WiFi.Blacklist[0] != "office" {
		t.Fatalf("恢复与删除订阅改变了模块网络设置: %+v %v", module, err)
	}
	if _, err := os.Stat(configTransactionPath(options)); !os.IsNotExist(err) {
		t.Fatalf("配置恢复未清理 journal: %v", err)
	}
}

func TestSubscriptionPending304RechecksProviderWithoutSelecting(t *testing.T) {
	options := catalogFixture(t)
	groupID := "subscription"
	var selectCalls, verifyCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sub":
			w.Header().Set("ETag", `"fixture"`)
			if r.Header.Get("If-None-Match") != "" {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			_, _ = w.Write([]byte(`{"outbounds":[{"type":"socks","tag":"NODE","server":"127.0.0.1","server_port":1080}]}`))
		case "/daemon.StartedService/SubscribeOutbounds":
			verifyCalls++
			item := protowire.AppendString(protowire.AppendTag(nil, 1, protowire.BytesType), catalog.ProviderTag(groupID)+"/NODE")
			payload := protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType), item)
			frame := make([]byte, 5)
			binary.BigEndian.PutUint32(frame[1:], uint32(len(payload)))
			_, _ = w.Write(append(frame, payload...))
		case "/daemon.StartedService/SelectOutbound":
			selectCalls++
			http.Error(w, "must not select", http.StatusBadRequest)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	options.ServiceAddress = server.URL
	if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{Root: options.CatalogRoot, GroupID: groupID, Name: "Subscription", Type: "subscription", URL: server.URL + "/sub", UpdateViaProxy: "never"}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.AppendNode(t.Context(), catalog.MutationOptions{GroupDir: filepath.Join(options.CatalogRoot, groupID), GroupID: groupID, Type: "subscription", Input: "socks://127.0.0.1:1080#OLD"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(t.Context(), options); err != nil {
		t.Fatal(err)
	}
	before := targetCoreRead(t, options.ModuleConfig)
	workerOpts := workerOptions(options)
	syncAttempts := 0
	workerOpts.SyncCatalog = func(ctx context.Context, id string, _ bool) (string, bool, error) {
		syncAttempts++
		if syncAttempts == 1 {
			return subscription.RuntimeSyncFailed, true, errors.New("runtime temporarily unavailable")
		}
		locked, release, err := lockCatalogChange(ctx, options)
		if err != nil {
			return "", false, err
		}
		defer release()
		return syncCatalogChange(ctx, locked, id, true, false)
	}
	first, err := worker.UpdateGroup(t.Context(), workerOpts, groupID, time.Now(), nil)
	var syncErr *subscription.Error
	if !errors.As(err, &syncErr) || syncErr.Code != "subscription.runtime_sync_failed" || !first.RuntimeSyncPending {
		t.Fatalf("verification failure lost pending: %+v %v", first, err)
	}
	second, err := worker.UpdateGroup(t.Context(), workerOpts, groupID, time.Now(), nil)
	if err != nil || !second.NotModified || !second.RuntimeSynced || second.RuntimeSyncPending || selectCalls != 0 || verifyCalls != 1 || syncAttempts != 2 {
		t.Fatalf("pending 304 did not verify Provider: %+v selects=%d verifies=%d %v", second, selectCalls, verifyCalls, err)
	}
	if after := targetCoreRead(t, options.ModuleConfig); !bytes.Equal(before, after) {
		t.Fatal("Provider synchronization rewrote module config")
	}
}

func TestNodeRemovalLeavesModuleAndEmptyCatalogIndependent(t *testing.T) {
	options := catalogFixture(t)
	before := targetCoreRead(t, options.ModuleConfig)
	for _, reference := range []string{"default/NODE", "default/节点 / 02"} {
		if _, err := NodeRemove(t.Context(), options, reference); err != nil {
			t.Fatal(err)
		}
	}
	document, err := catalog.GroupProvider(t.Context(), options.CatalogRoot, "default")
	if err != nil || len(document.Outbounds) != 0 {
		t.Fatalf("last node removal failed: %+v %v", document, err)
	}
	if after := targetCoreRead(t, options.ModuleConfig); !bytes.Equal(before, after) {
		t.Fatal("node removal rewrote module config")
	}
}

func TestNodeImportAppendsToDefaultGroup(t *testing.T) {
	root := t.TempDir()
	options := newTestOptions(root)
	options.SingBoxPath = fakeSingBox(t)
	writeNativeTestConfig(t, options)
	if err := os.MkdirAll(filepath.Dir(options.ModuleConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("{}"), 0o600); err != nil {
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
	if err := os.WriteFile(options.ModuleConfig, []byte("{}"), 0o600); err != nil {
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
	if err := os.WriteFile(options.ModuleConfig, []byte("{}"), 0o600); err != nil {
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
	writeNativeTestConfig(t, options)
	if err := os.MkdirAll(filepath.Dir(options.ModuleConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ModuleConfig, []byte("{}"), 0o600); err != nil {
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
