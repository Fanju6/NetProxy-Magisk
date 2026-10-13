package worker

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHostKeepsPIDUntilHTTPDrainCompletes(t *testing.T) {
	options := newTestOptions(t.TempDir())
	options.APIConfig = filepath.Join(t.TempDir(), "api.json")
	if err := os.MkdirAll(filepath.Dir(options.PIDFile), 0o700); err != nil {
		t.Fatal(err)
	}
	markStaleWorker(t, options.PIDFile)
	started, draining, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	options.RunServer = func(ctx context.Context, _ <-chan struct{}) error {
		close(started)
		<-ctx.Done()
		close(draining)
		<-release
		return nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunProcess(ctx, options, log.New(io.Discard, "", 0)) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP 组件没有启动")
	}
	cancel()
	<-draining
	if pid := readPID(options.PIDFile); pid != os.Getpid() {
		t.Fatal("收尾前释放了后台身份", pid)
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("后台宿主未结束")
	}
	if _, err := os.Stat(options.PIDFile); !os.IsNotExist(err) {
		t.Fatal("收尾后遗留PID", err)
	}
}

func TestBrokenModuleSuspendsTasksButKeepsHost(t *testing.T) {
	options := newTestOptions(t.TempDir())
	options.APIConfig = filepath.Join(t.TempDir(), "api.json")
	options.ModuleConfig = filepath.Join(t.TempDir(), "module.json")
	if err := os.WriteFile(options.ModuleConfig, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	options.NetworkWatchEnabled = true
	options.NetworkEvaluate = func(context.Context, string, string) error { return nil }
	delays := make(chan time.Duration, 2)
	options.NewTimer = func(delay time.Duration) Timer {
		delays <- delay
		return systemTimer{timer: time.NewTimer(delay)}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	wake := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() { done <- Run(ctx, options, wake, log.New(io.Discard, "", 0)) }()
	select {
	case <-delays:
	case err := <-done:
		t.Fatal("损坏配置关闭了后台", err)
	case <-time.After(3 * time.Second):
		t.Fatal("没有进入有界等待")
	}
	if err := os.WriteFile(options.ModuleConfig, []byte(`{"wifi":{"enabled":false}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	wake <- struct{}{}
	select {
	case delay := <-delays:
		if delay < 23*time.Hour {
			t.Fatal("修复后仍使用错误退避", delay)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("修复未唤醒任务")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
