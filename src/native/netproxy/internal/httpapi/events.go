package httpapi

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/logfile"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"github.com/fsnotify/fsnotify"
)

const (
	topicStatus = 1 << iota
	topicGroups
	topicSubscriptions
	allTopics = topicStatus | topicGroups | topicSubscriptions
)

type eventMessage struct {
	topic   int
	name    string
	content []byte
	epoch   uint64
}

type eventSubscriber struct {
	topics   int
	messages chan eventMessage
	done     chan struct{}
	seen     map[string]bool
}

type coreEvent struct {
	generation uint64
	status     *serviceapi.Status
	groups     *[]serviceapi.Group
	mode       *string
	err        error
}

type eventHub struct {
	server     *Server
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	clients    map[*eventSubscriber]struct{}
	latest     map[string]eventMessage
	wake       chan struct{}
	done       chan struct{}
	epoch      atomic.Uint64
	native     atomic.Pointer[moduleapp.ServiceState]
	stateCache eventStateCache
}

func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	topics := allTopics
	if value := r.URL.Query().Get("topics"); value != "" {
		topics = 0
		for name := range strings.SplitSeq(value, ",") {
			switch name {
			case "status":
				topics |= topicStatus
			case "groups":
				topics |= topicGroups
			case "subscriptions":
				topics |= topicSubscriptions
			default:
				s.fail(w, r.Context(), invalid("事件主题只能为 status、groups、subscriptions"))
				return
			}
		}
	}
	if _, ok := w.(http.Flusher); !ok {
		s.fail(w, r.Context(), invalid("此连接不支持实时输出"))
		return
	}
	s.eventMu.Lock()
	if s.stopping.Load() {
		s.eventMu.Unlock()
		s.fail(w, r.Context(), busy())
		return
	}
	h := s.events
	if h != nil {
		h.mu.Lock()
		if h.ctx.Err() != nil {
			h.mu.Unlock()
			<-h.done
			h = nil
		}
	}
	if h == nil {
		ctx, cancel := context.WithCancel(s.lifetime)
		h = &eventHub{server: s, ctx: ctx, cancel: cancel, clients: make(map[*eventSubscriber]struct{}),
			latest: make(map[string]eventMessage), wake: make(chan struct{}, 1), done: make(chan struct{})}
		s.events = h
		h.mu.Lock()
	}
	if len(h.clients) >= 8 {
		h.mu.Unlock()
		s.eventMu.Unlock()
		s.fail(w, r.Context(), busy())
		return
	}
	first := len(h.clients) == 0
	client := &eventSubscriber{topics: topics, messages: make(chan eventMessage, 8), done: make(chan struct{}), seen: make(map[string]bool)}
	h.clients[client] = struct{}{}
	h.mu.Unlock()
	s.eventMu.Unlock()
	if first {
		go h.run()
	}
	select {
	case h.wake <- struct{}{}:
	default:
	}
	defer func() {
		h.mu.Lock()
		delete(h.clients, client)
		if len(h.clients) == 0 {
			h.cancel()
		}
		h.mu.Unlock()
		select {
		case h.wake <- struct{}{}:
		default:
		}
	}()
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		return
	}
	if err := controller.Flush(); err != nil {
		return
	}
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.ctx.Done():
			return
		case <-client.done:
			return
		case event := <-client.messages:
			if !h.validEvent(event) {
				continue
			}
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.name, event.content); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		case <-heartbeat.C:
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}

func (h *eventHub) interests() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	topics := 0
	for client := range h.clients {
		topics |= client.topics
	}
	return topics
}

func (h *eventHub) missingTopics() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	topics := 0
	for client := range h.clients {
		topics |= client.topics
	}
	for _, event := range h.latest {
		topics &^= event.topic
	}
	// traffic 不是完整 status 快照，不能代替新消费者的初始 status。
	if _, exists := h.latest["status"]; !exists {
		for client := range h.clients {
			topics |= client.topics & topicStatus
		}
	}
	return topics
}

func (h *eventHub) emit(topic int, name string, response contract.Result) {
	h.emitAt(topic, name, response, h.epoch.Load())
}

func (h *eventHub) validEvent(event eventMessage) bool {
	if event.topic == topicSubscriptions {
		return true
	}
	if event.epoch != h.epoch.Load() {
		return false
	}
	saved := h.native.Load()
	if saved == nil {
		return true
	}
	state, err := h.readNative()
	return err == nil && state == *saved && event.epoch == h.epoch.Load()
}

func (h *eventHub) emitAt(topic int, name string, response contract.Result, epoch uint64) {
	content, err := json.Marshal(response, json.Deterministic(true))
	if err != nil || len(content) > 4<<20 {
		content, _ = json.Marshal(contract.Result{Schema: 1, Code: "api.event_too_large", Message: "事件过大，请通过列表接口读取"}, json.Deterministic(true))
	}
	event := eventMessage{topic: topic, name: name, content: content, epoch: epoch}
	if !h.validEvent(event) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	previous, exists := h.latest[name]
	h.latest[name] = event
	for client := range h.clients {
		if client.topics&topic == 0 {
			continue
		}
		if exists && previous.epoch == epoch && bytes.Equal(previous.content, content) && client.seen[name] {
			continue
		}
		select {
		case client.messages <- event:
			client.seen[name] = true
		default:
			delete(h.clients, client)
			close(client.done)
		}
	}
	if len(h.clients) == 0 {
		h.cancel()
	}
}

func (h *eventHub) replay() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		for _, event := range h.latest {
			if client.topics&event.topic != 0 && !client.seen[event.name] {
				select {
				case client.messages <- event:
					client.seen[event.name] = true
				default:
				}
			}
		}
	}
}

func (h *eventHub) run() {
	defer close(h.done)
	defer h.cancel()
	watcher, err := fsnotify.NewWatcher()
	if err == nil {
		defer watcher.Close()
		err = h.runEvents(watcher)
	}
	if err != nil && h.ctx.Err() == nil {
		_ = logfile.AppendEntry(h.server.options.WorkerLogFile, logfile.Entry{Level: "ERROR", Component: "api", Event: "api.events", Result: "failed", ErrorCode: "api.events_unavailable", Message: "文件事件监听失败: " + err.Error()})
	}
}

func (h *eventHub) runEvents(watcher *fsnotify.Watcher) error {
	if err := h.watchDirectories(watcher); err != nil {
		return err
	}
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	var dirty int
	var reconcile bool
	var flush <-chan time.Time
	queue := func(topics int) {
		dirty |= topics & h.interests()
		if (dirty != 0 || reconcile) && flush == nil {
			// 固定窗口合并连续事件，不能让持续写入无限推迟刷新。
			timer.Reset(100 * time.Millisecond)
			flush = timer.C
		}
	}
	feed := newCoreFeed(h)
	defer feed.close()
	feed.sync()
	feed.refresh(h.interests())
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-h.ctx.Done():
			return nil
		case <-h.wake:
			if feed.sync() {
				feed.refresh(h.interests() & (topicStatus | topicGroups))
			}
			feed.refresh(h.missingTopics())
			h.replay()
		case event, ok := <-watcher.Events:
			if !ok {
				return errors.New("文件监听已关闭")
			}
			topics := h.changedTopics(event)
			if topics == 0 {
				continue
			}
			reconcile = reconcile || event.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0
			queue(topics)
		case err, ok := <-watcher.Errors:
			if !ok {
				return errors.New("文件监听已关闭")
			}
			if !errors.Is(err, fsnotify.ErrEventOverflow) {
				return err
			}
			reconcile = true
			queue(allTopics)
		case <-flush:
			if reconcile {
				if err := h.watchDirectories(watcher); err != nil {
					return err
				}
				reconcile = false
			}
			topics := dirty
			dirty, flush = 0, nil
			if feed.sync() {
				topics |= topicStatus | topicGroups
			}
			feed.refresh(topics & h.interests())
		case <-tick.C:
			if feed.sync() {
				feed.refresh(h.interests() & (topicStatus | topicGroups))
			}
		case <-feed.groupFlush:
			feed.flushGroups()
		case event := <-feed.messages:
			if feed.consume(event) {
				feed.refresh(h.interests() & (topicStatus | topicGroups))
			}
		}
	}
}

func (h *eventHub) changedTopics(event fsnotify.Event) int {
	name := filepath.Base(event.Name)
	if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".lock") {
		return 0
	}
	path := filepath.Clean(event.Name)
	options := h.server.options
	switch path {
	case options.StateFile, filepath.Dir(options.StateFile):
		return topicStatus | topicGroups
	case options.ModuleConfig, options.InboundConfig, filepath.Dir(options.ModuleConfig), filepath.Dir(options.InboundConfig):
		return topicStatus
	case options.SingBoxDir, paths.SingBoxConfig(options.SingBoxDir):
		return topicStatus | topicGroups
	case options.ProgressDir:
		return topicSubscriptions
	case options.CatalogRoot, filepath.Dir(options.CatalogRoot):
		return topicSubscriptions
	}
	if filepath.Dir(path) == options.ProgressDir {
		return topicSubscriptions
	}
	if relative, err := filepath.Rel(options.CatalogRoot, path); err == nil && filepath.IsLocal(relative) {
		group, file, _ := strings.Cut(filepath.ToSlash(relative), "/")
		if group != "staging" && !strings.HasPrefix(group, ".") && (file == "" || file == "meta.json" || file == "provider.json") {
			return topicSubscriptions
		}
	}
	return 0
}

func (h *eventHub) watchDirectories(watcher *fsnotify.Watcher) error {
	options := h.server.options
	directories := []string{options.ModuleDir, filepath.Dir(options.ModuleConfig), filepath.Dir(options.InboundConfig),
		options.SingBoxDir, filepath.Dir(options.StateFile), filepath.Dir(options.CatalogRoot), options.CatalogRoot, options.ProgressDir}
	if entries, err := os.ReadDir(options.CatalogRoot); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && entry.Name() != "staging" && !strings.HasPrefix(entry.Name(), ".") {
				directories = append(directories, filepath.Join(options.CatalogRoot, entry.Name()))
			}
		}
	}
	known := make(map[string]bool)
	for _, path := range watcher.WatchList() {
		known[path] = true
	}
	for _, directory := range directories {
		if known[directory] {
			continue
		}
		info, err := os.Lstat(directory)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			if err := watcher.Add(directory); err != nil {
				return err
			}
		}
	}
	return nil
}
