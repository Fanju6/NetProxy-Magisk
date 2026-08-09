package worker

import (
	"context"
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
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

type manualClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*manualTimer
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

func (clock *manualClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *manualClock) NewTimer(_ time.Duration) Timer {
	timer := &manualTimer{channel: make(chan time.Time, 1)}
	clock.mu.Lock()
	clock.timers = append(clock.timers, timer)
	clock.mu.Unlock()
	return timer
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
	metadata.NextUpdateAt = subscription.FormatEpochUTC(metadata.NextUpdateEpoch)
	if err := subscription.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
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
		metadata, err := subscription.LoadMetadata(path, "fixture")
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
			subscription.ScheduleAt(&metadata, now)
		}
		group := filepath.Join(root, entry.id)
		if err := os.MkdirAll(group, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := subscription.SaveMetadataAtomic(filepath.Join(group, "meta.json"), metadata); err != nil {
			t.Fatal(err)
		}
	}
	options := NewOptions(root)
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
	options := NewOptions(root)
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
	options := NewOptions(root)
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
	options := NewOptions(root)
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

	metadata, err := subscription.LoadMetadata(filepath.Join(root, "fixture", "meta.json"), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Revision != 2 {
		t.Fatalf("多轮唤醒只执行了 %d 次更新", metadata.Revision)
	}
	markStaleWorker(t, options.PIDFile)
	metadata.NextUpdateEpoch = clock.Now().Unix() - 1
	metadata.NextUpdateAt = subscription.FormatEpochUTC(metadata.NextUpdateEpoch)
	if err := subscription.SaveMetadataAtomic(filepath.Join(root, "fixture", "meta.json"), metadata); err != nil {
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
	options := NewOptions(root)
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
		metadata.NextUpdateAt = subscription.FormatEpochUTC(metadata.NextUpdateEpoch)
		if err := subscription.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
			t.Fatal(err)
		}
		if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), []byte(`{"outbounds":[]}`+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	options := NewOptions(root)
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
