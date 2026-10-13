package httpapi

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/fsnotify/fsnotify"
)

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
	resourceSubscriptionFixture(t, server, "subscription-test", "https://example.test/sub")
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
