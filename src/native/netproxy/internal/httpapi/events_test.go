package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/fsnotify/fsnotify"
)

func BenchmarkEventIdentityGate(b *testing.B) {
	server := resourceFixture(b)
	if err := moduleapp.WriteServiceState(server.options.StateFile, "ready", 12, 1, 1, "", moduleapp.ServiceIdentity{Backend: "tun", StartedAtMillis: 1000}); err != nil {
		b.Fatal(err)
	}
	state, err := moduleapp.ReadServiceState(server.options.StateFile)
	if err != nil {
		b.Fatal(err)
	}
	h := &eventHub{server: server}
	h.native.Store(&state)
	h.epoch.Store(1)
	event := eventMessage{topic: topicGroups, epoch: 1}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if !h.validEvent(event) {
			b.Fatal("当前身份被拒绝")
		}
	}
}

func BenchmarkEventGroupsBurst(b *testing.B) {
	for _, coalesced := range []bool{false, true} {
		b.Run(fmt.Sprintf("coalesced=%t", coalesced), func(b *testing.B) {
			server := resourceFixture(b)
			ctx, cancel := context.WithCancel(b.Context())
			defer cancel()
			h := &eventHub{server: server, ctx: ctx, cancel: cancel,
				clients: make(map[*eventSubscriber]struct{}), latest: make(map[string]eventMessage)}
			client := testSubscriber()
			client.topics = topicGroups
			h.clients[client] = struct{}{}
			f := newCoreFeed(h)
			defer f.close()
			f.sync()
			f.refresh(topicGroups)
			for len(client.messages) > 0 {
				<-client.messages
			}
			if err := moduleapp.WriteServiceState(server.options.StateFile, "ready", 12, 1, 1, "", moduleapp.ServiceIdentity{Backend: "tun", StartedAtMillis: 1000}); err != nil {
				b.Fatal(err)
			}
			state, err := h.readNative()
			if err != nil {
				b.Fatal(err)
			}
			f.saved, f.identity = state, 1000
			h.native.Store(&state)
			groups := []serviceapi.Group{{Tag: "Auto/fixture", Items: make([]serviceapi.GroupItem, 128)}}
			for i := range groups[0].Items {
				groups[0].Items[i] = serviceapi.GroupItem{Tag: fmt.Sprintf("fixture/node-%d", i), Type: "socks", URLTestDelay: 100}
			}
			drain := func() {
				for len(client.messages) > 0 {
					<-client.messages
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				for i := range 16 {
					groups[0].Items[0].URLTestDelay = int32(i)
					f.consume(coreEvent{generation: f.generation, groups: &groups})
					if !coalesced {
						f.flushGroups()
						drain()
					}
				}
				if coalesced {
					f.flushGroups()
					drain()
				}
				f.clearGroupWindow()
			}
		})
	}
}

func TestEventStateCacheRejectsReplacementWithSameMetadata(t *testing.T) {
	server := resourceFixture(t)
	path := server.options.StateFile
	if err := moduleapp.WriteServiceState(path, "ready", 12, 1, 1, "", moduleapp.ServiceIdentity{Backend: "tun", StartedAtMillis: 1000}); err != nil {
		t.Fatal(err)
	}
	h := testHub(t)
	h.server = server
	saved, err := h.readNative()
	if err != nil {
		t.Fatal(err)
	}
	h.native.Store(&saved)
	event := eventMessage{topic: topicGroups}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := path + ".replacement"
	resourceWriteFile(t, replacement, strings.Replace(string(content), "1000", "2000", 1))
	if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if h.validEvent(event) {
		t.Fatal("相同 mtime/size 的新 inode 允许旧帧发送")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if h.validEvent(event) {
		t.Fatal("状态文件删除后仍允许旧帧")
	}
	resourceWriteFile(t, path, "{broken")
	if h.validEvent(event) {
		t.Fatal("损坏文件回退了缓存身份")
	}
}

func TestEventStateCacheDoesNotBindOldReadToNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.json")
	if err := moduleapp.WriteServiceState(path, "starting", 12, 1, 0, ""); err != nil {
		t.Fatal(err)
	}
	var cache eventStateCache
	calls := 0
	read := func(path string) (moduleapp.ServiceState, error) {
		calls++
		state, err := moduleapp.ReadServiceState(path)
		if err == nil && calls == 1 {
			err = moduleapp.WriteServiceState(path, "stopped", 0, 0, 0, "")
		}
		return state, err
	}
	if _, err := cache.read(path, read); err == nil {
		t.Fatal("stat-read-stat 替换竞态未被拒绝")
	}
	if state, err := cache.read(path, read); err != nil || state.State != "stopped" {
		t.Fatal("读取竞态后未重新解析最新文件", state, err)
	}
	if _, err := cache.read(path, read); err != nil || calls != 2 {
		t.Fatal("稳定文件未命中缓存", calls, err)
	}
}

func TestEventStateCacheReadRaceWithIdenticalMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.json")
	if err := moduleapp.WriteServiceState(path, "ready", 12, 1, 1, "", moduleapp.ServiceIdentity{Backend: "tun", StartedAtMillis: 1000}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	var cache eventStateCache
	_, err = cache.read(path, func(path string) (moduleapp.ServiceState, error) {
		state, err := moduleapp.ReadServiceState(path)
		if err != nil {
			return state, err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return state, err
		}
		replacement := path + ".replacement"
		resourceWriteFile(t, replacement, strings.Replace(string(content), "1000", "2000", 1))
		if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
			return state, err
		}
		return state, os.Rename(replacement, path)
	})
	if err == nil {
		t.Fatal("相同元数据替换绕过 stat-read-stat 门禁")
	}
}

func TestEventStateCacheIsSharedAcrossConcurrentClients(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.json")
	if err := moduleapp.WriteServiceState(path, "stopped", 0, 0, 0, ""); err != nil {
		t.Fatal(err)
	}
	var cache eventStateCache
	var reads atomic.Int32
	var tasks sync.WaitGroup
	for range 8 {
		tasks.Go(func() {
			for range 20 {
				state, err := cache.read(path, func(path string) (moduleapp.ServiceState, error) {
					reads.Add(1)
					return moduleapp.ReadServiceState(path)
				})
				if err != nil || state.State != "stopped" {
					t.Error("并发门禁读取失败", err)
					return
				}
			}
		})
	}
	tasks.Wait()
	if reads.Load() != 1 {
		t.Fatal("每个消费者重复解析了相同文件", reads.Load())
	}
}

func fixtureCoreFeed(t *testing.T, topics int) (*coreFeed, *eventSubscriber) {
	t.Helper()
	h := testHub(t)
	h.server = resourceFixture(t)
	client := testSubscriber()
	client.topics = topics
	h.clients[client] = struct{}{}
	f := newCoreFeed(h)
	t.Cleanup(f.close)
	f.sync()
	f.refresh(topics)
	for len(client.messages) > 0 {
		<-client.messages
	}
	if err := moduleapp.WriteServiceState(h.server.options.StateFile, "ready", 12, 1, 1, "", moduleapp.ServiceIdentity{Backend: "tun", StartedAtMillis: 1000}); err != nil {
		t.Fatal(err)
	}
	state, err := h.readNative()
	if err != nil {
		t.Fatal(err)
	}
	f.saved, f.identity = state, 1000
	h.native.Store(&state)
	return f, client
}

func TestCoreGroupsFixedWindowPublishesLatestWithoutRPC(t *testing.T) {
	for _, topics := range []int{topicStatus, topicGroups, topicStatus | topicGroups} {
		t.Run(fmt.Sprint(topics), func(t *testing.T) {
			f, client := fixtureCoreFeed(t, topics)
			var calls atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer api.Close()
			f.h.server.options.ServiceAddress = api.URL
			send := func(selected string) {
				groups := []serviceapi.Group{{Tag: "Auto/" + f.snapshot.Selection.ActiveGroupRuntimeTag, Selected: selected}}
				if f.consume(coreEvent{generation: f.generation, groups: &groups}) {
					t.Fatal("相同 core 身份触发重新校准")
				}
			}
			begin := time.Now()
			send("first")
			tick := time.NewTicker(30 * time.Millisecond)
			defer tick.Stop()
			deadline := time.NewTimer(2 * time.Second)
			defer deadline.Stop()
			for {
				select {
				case <-f.groupFlush:
					if time.Since(begin) < 230*time.Millisecond {
						t.Fatal("未遵循 250ms 固定窗口")
					}
					f.flushGroups()
					goto published
				case <-tick.C:
					send("latest")
					if len(client.messages) != 0 {
						t.Fatal("窗口内提前发布 groups/status")
					}
				case <-deadline.C:
					t.Fatal("持续 groups 事件无限延长窗口")
				}
			}
		published:
			want := 1
			if topics == topicStatus|topicGroups {
				want = 2
			}
			if len(client.messages) != want {
				t.Fatal("消费者缺少主题快照", len(client.messages))
			}
			for len(client.messages) > 0 {
				event := <-client.messages
				if !strings.Contains(string(event.content), "latest") || strings.Contains(string(event.content), "first") {
					t.Fatal("合并窗口没有使用最新实际选择", string(event.content))
				}
			}
			if calls.Load() != 0 {
				t.Fatal("groups 事件额外调用了 RPC", calls.Load())
			}
			send("next-window")
			select {
			case <-f.groupFlush:
				f.flushGroups()
			case <-time.After(time.Second):
				t.Fatal("第二个窗口未重新启用")
			}
		})
	}
}

func TestCoreFeedRejectsOldGenerationAndQueuedFramesAfterFailure(t *testing.T) {
	f, client := fixtureCoreFeed(t, topicGroups)
	groups := []serviceapi.Group{{Tag: "old"}}
	f.consume(coreEvent{generation: f.generation - 1, groups: &groups})
	if f.groupFlush != nil {
		t.Fatal("旧 generation 启动了合并窗口")
	}
	f.h.emit(topicGroups, "groups", contract.Success("test.groups", "快照", 1))
	queued := <-client.messages
	generation := f.generation
	f.consume(coreEvent{generation: generation, groups: &groups})
	f.consume(coreEvent{generation: generation, err: io.EOF})
	if f.h.validEvent(queued) || f.groupFlush != nil {
		t.Fatal("流失败后旧队列或窗口仍可发布")
	}
	f.consume(coreEvent{generation: generation, groups: &groups})
	if f.groupsReady {
		t.Fatal("失败后消费了旧流帧")
	}
}

func TestCoreGroupWindowCannotPublishAfterStateReplacement(t *testing.T) {
	f, client := fixtureCoreFeed(t, topicGroups)
	groups := []serviceapi.Group{{Tag: "stale", Selected: "old-node"}}
	f.consume(coreEvent{generation: f.generation, groups: &groups})
	if err := moduleapp.WriteServiceState(f.h.server.options.StateFile, "ready", 12, 1, 1, "", moduleapp.ServiceIdentity{Backend: "tun", StartedAtMillis: 2000}); err != nil {
		t.Fatal(err)
	}
	f.flushGroups()
	if len(client.messages) != 0 {
		t.Fatal("合并窗口发出了替换前的 core 帧")
	}
	f.consume(coreEvent{generation: f.generation, groups: &groups})
	if f.groupFlush != nil || f.identity != 0 {
		t.Fatal("文件替换后未撤销旧流")
	}
}

func TestCoreFeedCancellationUnblocksFullQueueAndAllStreams(t *testing.T) {
	started := make(chan struct{}, 3)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/grpc-web+proto")
		for range 64 {
			if _, err := w.Write([]byte{0, 0, 0, 0, 0}); err != nil {
				return
			}
		}
		w.(http.Flusher).Flush()
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer api.Close()
	h := testHub(t)
	f := newCoreFeed(h)
	defer f.close()
	client, err := serviceapi.New(api.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	f.start(client)
	for range 3 {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("core 流未建立")
		}
	}
	deadline := time.Now().Add(time.Second)
	for len(f.messages) != cap(f.messages) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(f.messages) != 8 {
		t.Fatal("未覆盖满队列背压", len(f.messages))
	}
	done := make(chan struct{})
	go func() { f.close(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("取消未解除队列背压并退出全部流")
	}
}

func TestCoreModeEventsUpdateStatusWithoutGroupsOrRPC(t *testing.T) {
	f, client := fixtureCoreFeed(t, topicStatus)
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer api.Close()
	f.h.server.options.ServiceAddress = api.URL
	for _, mode := range []string{"Direct", "Proxy", ""} {
		if f.consume(coreEvent{generation: f.generation, mode: &mode}) {
			t.Fatal("模式变更触发了整个 status 读取")
		}
		want := mode
		if want == "" {
			want = "unknown"
		}
		select {
		case event := <-client.messages:
			if event.name != "status" || !strings.Contains(string(event.content), "\"outbound_mode\":\""+want+"\"") ||
				!strings.Contains(string(event.content), "\"configured_outbound_mode\":\"Rule\"") {
				t.Fatal("纯 API 模式变更未更新实际模式或覆盖了默认模式", string(event.content))
			}
		default:
			t.Fatal("没有 groups/file 事件时模式更新丢失")
		}
	}
	if calls.Load() != 0 || f.groupFlush != nil {
		t.Fatal("模式事件依赖了额外 RPC 或 groups 窗口")
	}
	old := "Direct"
	f.consume(coreEvent{generation: f.generation - 1, mode: &old})
	if len(client.messages) != 0 {
		t.Fatal("旧 generation 模式帧越过门禁")
	}
	if err := moduleapp.WriteServiceState(f.h.server.options.StateFile, "ready", 12, 1, 1, "", moduleapp.ServiceIdentity{Backend: "tun", StartedAtMillis: 2000}); err != nil {
		t.Fatal(err)
	}
	f.consume(coreEvent{generation: f.generation, mode: &old})
	for len(client.messages) > 0 {
		event := <-client.messages
		if strings.Contains(string(event.content), "\"outbound_mode\":\"Direct\"") {
			t.Fatal("替换文件后的旧模式帧被发送")
		}
	}
}

func TestMissingTopicsRequiresFullInitialStatus(t *testing.T) {
	h := testHub(t)
	client := testSubscriber()
	h.clients[client] = struct{}{}
	h.emit(topicStatus, "traffic", contract.Success("api.runtime_status", "校准", nil))
	if got := h.missingTopics(); got != allTopics {
		t.Fatal("traffic 错误替代了完整 status", got)
	}
	h.emit(topicStatus, "status", contract.Success("service.status", "状态", nil))
	h.emit(topicGroups, "groups", contract.Success("node.snapshot", "节点", nil))
	h.emit(topicSubscriptions, "subscriptions", contract.Success("subscription.list", "订阅", nil))
	if h.missingTopics() != 0 {
		t.Fatal("已存在的快照仍需重新读取")
	}
}

func TestCoreFeedStopsWhenOnlyPersistentSubscribersRemain(t *testing.T) {
	f, client := fixtureCoreFeed(t, topicGroups)
	f.cancel = func() {}
	f.done = make(chan struct{})
	close(f.done)
	client.topics = topicSubscriptions
	f.sync()
	if f.cancel != nil || f.identity != 0 {
		t.Fatal("仅订阅持久主题时仍保留 core 流")
	}
}

func TestStatusSubscriberJoiningGroupsFeedGetsFullSnapshotAndCurrentTraffic(t *testing.T) {
	f, _ := fixtureCoreFeed(t, topicGroups)
	status := serviceapi.Status{Memory: 12345}
	f.consume(coreEvent{generation: f.generation, status: &status})
	client := testSubscriber()
	client.topics = topicStatus
	f.h.clients[client] = struct{}{}
	f.refresh(f.h.missingTopics())
	f.h.replay()
	seen := make(map[string]bool)
	for len(client.messages) > 0 {
		event := <-client.messages
		seen[event.name] = true
		if event.name == "traffic" && (!strings.Contains(string(event.content), "12345") || strings.Contains(string(event.content), "\"available\":false")) {
			t.Fatal("新 status 消费者收到过期的不可用占位", string(event.content))
		}
	}
	if !seen["status"] || !seen["traffic"] || seen["groups"] {
		t.Fatal("动态主题消费者的初始快照不完整或越界", seen)
	}
}

func testHub(t *testing.T) *eventHub {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	return &eventHub{ctx: ctx, cancel: cancel, clients: make(map[*eventSubscriber]struct{}), latest: make(map[string]eventMessage)}
}

func testSubscriber() *eventSubscriber {
	return &eventSubscriber{topics: allTopics, messages: make(chan eventMessage, 8), done: make(chan struct{}), seen: make(map[string]bool)}
}

func TestHubNewClientGetsAllTopicsWithoutReplayingOldClient(t *testing.T) {
	h := testHub(t)
	old := testSubscriber()
	h.clients[old] = struct{}{}
	for _, name := range []string{"status", "groups", "subscriptions"} {
		topic := map[string]int{"status": topicStatus, "groups": topicGroups, "subscriptions": topicSubscriptions}[name]
		h.emit(topic, name, contract.Success("test."+name, "快照", 1))
		<-old.messages
	}
	next := testSubscriber()
	h.clients[next] = struct{}{}
	h.emit(topicSubscriptions, "subscriptions", contract.Success("test.subscriptions", "快照", 2))
	h.replay()
	seen := make(map[string]bool)
	for len(next.messages) > 0 {
		seen[(<-next.messages).name] = true
	}
	if len(seen) != 3 {
		t.Fatal("新客户端缺少未变化主题", seen)
	}
	if len(old.messages) != 1 || (<-old.messages).name != "subscriptions" {
		t.Fatal("向已有客户端重复回放快照")
	}
}

func TestHubClosesSlowClientWithoutGrowingQueue(t *testing.T) {
	h := testHub(t)
	client := testSubscriber()
	client.messages = make(chan eventMessage, 1)
	h.clients[client] = struct{}{}
	h.emit(topicStatus, "status", contract.Success("test.status", "快照", 1))
	h.emit(topicStatus, "status", contract.Success("test.status", "快照", 2))
	select {
	case <-client.done:
	default:
		t.Fatal("慢客户端没有关闭")
	}
	if len(h.clients) != 0 || h.ctx.Err() == nil {
		t.Fatal("无客户端时仍驻留")
	}
}

func TestQueuedRuntimeEventsCannotEscapeIdentityGate(t *testing.T) {
	server := resourceFixture(t)
	h := testHub(t)
	h.server = server
	if err := moduleapp.WriteServiceState(server.options.StateFile, "starting", 12, 1, 0, ""); err != nil {
		t.Fatal(err)
	}
	state, err := moduleapp.ReadServiceState(server.options.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	h.native.Store(&state)
	h.epoch.Store(1)
	event := eventMessage{topic: topicGroups, name: "groups", epoch: 1}
	if !h.validEvent(event) {
		t.Fatal("相同身份被拒绝")
	}
	if err := moduleapp.WriteServiceState(server.options.StateFile, "stopped", 0, 0, 0, ""); err != nil {
		t.Fatal(err)
	}
	if h.validEvent(event) {
		t.Fatal("尚未采样新状态时仍允许旧队列发送")
	}
	h.epoch.Store(2)
	if h.validEvent(event) {
		t.Fatal("旧 epoch 通过队列发送门禁")
	}
	if !h.validEvent(eventMessage{topic: topicSubscriptions, epoch: 1}) {
		t.Fatal("核心切换阻断了持久订阅信息")
	}
}

func TestFileEventsOnlyRefreshAffectedTopics(t *testing.T) {
	server := resourceFixture(t)
	h := &eventHub{server: server}
	options := server.options
	for _, test := range []struct {
		path   string
		op     fsnotify.Op
		topics int
	}{
		{options.ModuleConfig, fsnotify.Write, topicStatus | topicGroups},
		{options.StateFile, fsnotify.Create, topicStatus | topicGroups},
		{options.InboundConfig, fsnotify.Write, topicStatus},
		{paths.SingBoxConfig(options.SingBoxDir), fsnotify.Rename, topicStatus},
		{options.CatalogRoot, fsnotify.Create, allTopics},
		{filepath.Join(options.CatalogRoot, "default"), fsnotify.Remove, allTopics},
		{filepath.Join(options.CatalogRoot, "default", "provider.json"), fsnotify.Write, allTopics},
		{filepath.Join(options.CatalogRoot, "default", "meta.json"), fsnotify.Write, allTopics},
		{filepath.Join(options.CatalogRoot, "default", "history.jsonl"), fsnotify.Write, 0},
		{filepath.Join(options.CatalogRoot, "staging", "provider.json"), fsnotify.Write, 0},
		{filepath.Join(options.ProgressDir, "subscription-test.progress.json"), fsnotify.Write, topicSubscriptions},
		{options.ModuleConfig + ".lock", fsnotify.Create, 0},
		{filepath.Join(filepath.Dir(options.ModuleConfig), ".candidate"), fsnotify.Create, 0},
		{options.ModuleConfig, fsnotify.Chmod, 0},
		{filepath.Join(options.LogDir, "core.log"), fsnotify.Write, 0},
	} {
		if got := h.changedTopics(fsnotify.Event{Name: test.path, Op: test.op}); got != test.topics {
			t.Errorf("%s (%s): got %d, want %d", test.path, test.op, got, test.topics)
		}
	}
}

func runningWatcherHub(t *testing.T) (*eventHub, *fsnotify.Watcher, *eventSubscriber) {
	t.Helper()
	server := resourceFixture(t)
	if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{
		Root: server.options.CatalogRoot, GroupID: "subscription-test", Name: "匿名订阅",
		Type: "subscription", URL: "https://example.test/sub", UpdateViaProxy: "never",
	}); err != nil {
		t.Fatal(err)
	}
	h := testHub(t)
	h.server, h.done = server, make(chan struct{})
	client := testSubscriber()
	client.topics = topicSubscriptions
	h.clients[client] = struct{}{}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer close(h.done)
		defer h.cancel()
		h.runEvents(watcher)
	}()
	t.Cleanup(func() {
		h.cancel()
		select {
		case <-h.done:
		case <-time.After(5 * time.Second):
			t.Error("文件事件处理未退出")
		}
		watcher.Close()
	})
	select {
	case <-client.messages:
	case <-time.After(5 * time.Second):
		t.Fatal("未收到初始订阅快照")
	}
	return h, watcher, client
}

func TestWatcherOverflowRebuildsWatchesAndSnapshots(t *testing.T) {
	h, watcher, client := runningWatcherHub(t)
	for _, directory := range watcher.WatchList() {
		if err := watcher.Remove(directory); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.SetGroupName(t.Context(), h.server.options.CatalogRoot, "subscription-test", "OVERFLOW_RECOVERED", time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case watcher.Errors <- fsnotify.ErrEventOverflow:
	case <-time.After(5 * time.Second):
		t.Fatal("文件监听错误未被消费")
	}
	for _, name := range []string{"OVERFLOW_RECOVERED", "WATCH_RESTORED"} {
		if name == "WATCH_RESTORED" {
			if err := catalog.SetGroupName(t.Context(), h.server.options.CatalogRoot, "subscription-test", name, time.Now()); err != nil {
				t.Fatal(err)
			}
		}
		select {
		case event := <-client.messages:
			if event.name != "subscriptions" || !strings.Contains(string(event.content), name) {
				t.Fatalf("未校准丢失的快照或未恢复监听: %s", event.content)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("事件溢出后没有恢复更新", name)
		}
	}
}

func TestWatcherFailureClosesStreamInsteadOfServingStaleState(t *testing.T) {
	h, watcher, _ := runningWatcherHub(t)
	select {
	case watcher.Errors <- errors.New("watcher failed"):
	case <-time.After(5 * time.Second):
		t.Fatal("文件监听错误未被消费")
	}
	select {
	case <-h.ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("监听失效后连接仍保持，可能静默遗漏更新")
	}
}

func TestFileEventBatchHasFixedRefreshDeadline(t *testing.T) {
	h, watcher, client := runningWatcherHub(t)
	for _, directory := range watcher.WatchList() {
		if err := watcher.Remove(directory); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalog.SetGroupName(t.Context(), h.server.options.CatalogRoot, "subscription-test", "BATCHED", time.Now()); err != nil {
		t.Fatal(err)
	}
	event := fsnotify.Event{Name: filepath.Join(h.server.options.CatalogRoot, "subscription-test", "meta.json"), Op: fsnotify.Write}
	select {
	case watcher.Events <- event:
	case <-time.After(5 * time.Second):
		t.Fatal("文件事件未被消费")
	}
	ctx, cancel := context.WithCancel(t.Context())
	feedDone := make(chan struct{})
	go func() {
		defer close(feedDone)
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				select {
				case watcher.Events <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	t.Cleanup(func() { cancel(); <-feedDone })
	select {
	case <-client.messages:
		t.Fatal("连续文件事件未经过合并窗口")
	case <-time.After(30 * time.Millisecond):
	}
	select {
	case event := <-client.messages:
		if !strings.Contains(string(event.content), "BATCHED") {
			t.Fatalf("合并后没有更新快照: %s", event.content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("持续事件无限推迟了刷新")
	}
}
