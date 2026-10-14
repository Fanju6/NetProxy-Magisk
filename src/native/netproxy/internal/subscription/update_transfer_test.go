package subscription

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

func transferFixture(t *testing.T, url string) (UpdateOptions, []byte) {
	t.Helper()
	root := t.TempDir()
	groupID := "transfer-fixture"
	groupDir := filepath.Join(root, groupID)
	metadata := catalog.NewMetadata(groupID, "Transfer Fixture", "subscription", url, time.Now())
	metadata.UpdateViaProxy = "never"
	metadata.Timeout = 60
	metadata.Revision = 3
	metadata.NodeCount = 1
	metadata.ETag = `"old-fixture"`
	if err := catalog.SaveMetadataAtomic(t.Context(), filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	oldProvider := []byte(`{"outbounds":[{"type":"socks","tag":"old-fixture","server":"127.0.0.1","server_port":1080}]}` + "\n")
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), oldProvider, 0o600); err != nil {
		t.Fatal(err)
	}
	return UpdateOptions{Root: root, GroupID: groupID, ProgressDir: filepath.Join(root, "progress")}, oldProvider
}

func TestTransferCancellationInterruptsDownloadKeepsProvider(t *testing.T) {
	for _, phase := range []string{"headers", "body", "proxy", "pending-runtime"} {
		t.Run(phase, func(t *testing.T) {
			started := make(chan struct{})
			requestCancelled := make(chan struct{}, 1)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if requests.Add(1) == 1 {
					if phase == "body" {
						_, _ = writer.Write([]byte(`{"outbounds":[`))
						writer.(http.Flusher).Flush()
					}
					close(started)
				}
				<-request.Context().Done()
				select {
				case requestCancelled <- struct{}{}:
				default:
				}
			}))
			defer server.Close()
			transferContext, cancelTransfer := context.WithCancel(t.Context())
			defer cancelTransfer()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			options, oldProvider := transferFixture(t, server.URL)
			options.TransferContext = transferContext
			expectedError := "订阅更新已取消"
			if phase == "pending-runtime" {
				metaPath := filepath.Join(options.Root, options.GroupID, "meta.json")
				metadata, err := catalog.LoadMetadata(t.Context(), metaPath, options.GroupID)
				if err != nil {
					t.Fatal(err)
				}
				expectedError = RuntimeSyncFailureMessage + ": fixture runtime failure"
				metadata.RuntimeSyncPending = true
				metadata.RuntimeSyncState = RuntimeSyncFailed
				metadata.LastError = expectedError
				if err := catalog.SaveMetadataAtomic(t.Context(), metaPath, metadata); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "proxy" {
				options.ProxyURL = server.URL
				options.FallbackDirect = true
			}
			finished := make(chan error, 1)
			go func() {
				_, err := Update(ctx, options)
				finished <- err
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("订阅下载未开始")
			}
			cancelTransfer()
			select {
			case err := <-finished:
				var structured *Error
				if !errors.As(err, &structured) || structured.Code != "subscription.cancelled" {
					t.Fatalf("下载取消未返回取消错误: %v", err)
				}
				data, ok := structured.Data.(map[string]any)
				if !ok || data["original_code"] != "subscription.cancelled" || data["persisted"] != false {
					t.Fatalf("下载取消的原始错误或持久化语义异常: %#v", structured.Data)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("服务生命周期取消未及时中断下载")
			}
			select {
			case <-requestCancelled:
			case <-time.After(3 * time.Second):
				t.Fatal("HTTP 请求未取消")
			}
			if ctx.Err() != nil || requests.Load() != 1 {
				t.Fatalf("取消影响原上下文或触发直连兜底: ctx=%v requests=%d", ctx.Err(), requests.Load())
			}
			groupDir := filepath.Join(options.Root, options.GroupID)
			current, err := os.ReadFile(filepath.Join(groupDir, "provider.json"))
			if err != nil || !bytes.Equal(current, oldProvider) {
				t.Fatalf("停止下载替换了旧 Provider: %s %v", current, err)
			}
			metadata, err := catalog.LoadMetadata(t.Context(), filepath.Join(groupDir, "meta.json"), options.GroupID)
			if err != nil || metadata.Revision != 3 || metadata.NodeCount != 1 || metadata.ETag != `"old-fixture"` || metadata.LastAttemptAt == "" || metadata.LastSuccessAt != "" || metadata.LastError != expectedError {
				t.Fatalf("下载失败未保留版本或完成状态落盘: %+v %v", metadata, err)
			}
			if phase == "pending-runtime" && (!metadata.RuntimeSyncPending || metadata.RuntimeSyncState != RuntimeSyncFailed) {
				t.Fatalf("下载取消覆盖了未完成的运行时同步状态: %+v", metadata)
			}
			history, err := os.ReadFile(filepath.Join(groupDir, "history.jsonl"))
			if err != nil || !strings.Contains(string(history), "subscription.cancelled") || strings.Contains(string(history), "subscription.convert_failed") {
				t.Fatalf("下载失败历史未落盘: %s %v", history, err)
			}
			if _, err := os.Stat(filepath.Join(options.ProgressDir, options.GroupID+".progress.json")); !os.IsNotExist(err) {
				t.Fatalf("下载取消后残留进度: %v", err)
			}
		})
	}
}

func TestTransferAlreadyCancelledDoesNotStartDownload(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	transferContext, cancelTransfer := context.WithCancel(t.Context())
	cancelTransfer()
	_, _, err := fetchSubscription(t.Context(), catalog.Metadata{URL: server.URL}, UpdateOptions{
		TransferContext: transferContext, ProxyURL: server.URL, FallbackDirect: true,
	})
	if !errors.Is(err, context.Canceled) || requests.Load() != 0 {
		t.Fatalf("已停止的服务仍发起下载: err=%v requests=%d", err, requests.Load())
	}
}

func TestTransferCancellationAfterDownloadDoesNotCancelCommit(t *testing.T) {
	for _, scenario := range []string{"content", "explicit-cancel"} {
		t.Run(scenario, func(t *testing.T) {
			started := make(chan struct{})
			releaseBody := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				close(started)
				select {
				case <-releaseBody:
				case <-request.Context().Done():
					return
				}
				_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"new-fixture","server":"127.0.0.1","server_port":1081}]}`))
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			transferContext, cancelTransfer := context.WithCancel(t.Context())
			defer cancelTransfer()
			options, oldProvider := transferFixture(t, server.URL)
			options.TransferContext = transferContext
			type outcome struct {
				result Result
				err    error
			}
			finished := make(chan outcome, 1)
			go func() {
				result, err := Update(ctx, options)
				finished <- outcome{result, err}
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("订阅下载未开始")
			}
			// 下载不持有根锁；暂停提交以确定取消发生在下载完成之后。
			releaseRoot, err := catalog.AcquireRoot(ctx, options.Root)
			if err != nil {
				t.Fatal(err)
			}
			rootHeld := true
			defer func() {
				if rootHeld {
					releaseRoot()
				}
			}()
			close(releaseBody)
			deadline := time.Now().Add(5 * time.Second)
			for {
				ready := false
				entries, err := os.ReadDir(filepath.Join(options.Root, "staging"))
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					if entry.IsDir() && strings.HasPrefix(entry.Name(), options.GroupID+".") {
						_, err := os.Stat(filepath.Join(options.Root, "staging", entry.Name(), "meta.json"))
						ready = err == nil
					}
				}
				if ready {
					break
				}
				if time.Now().After(deadline) {
					releaseRoot()
					rootHeld = false
					select {
					case completed := <-finished:
						t.Fatalf("下载未进入提交阶段: result=%+v err=%#v", completed.result, completed.err)
					case <-time.After(time.Second):
						t.Fatal("下载未进入提交阶段")
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			cancelTransfer()
			if scenario == "explicit-cancel" {
				cancel()
			}
			releaseRoot()
			rootHeld = false
			select {
			case completed := <-finished:
				if scenario == "explicit-cancel" {
					if completed.err == nil || completed.result.Persisted {
						t.Fatalf("原上下文取消未阻止提交: %+v %v", completed.result, completed.err)
					}
				} else if completed.err != nil || !completed.result.Persisted || completed.result.NotModified {
					t.Fatalf("下载后的生命周期取消影响提交: %+v %v", completed.result, completed.err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("提交未结束")
			}
			groupDir := filepath.Join(options.Root, options.GroupID)
			current, err := os.ReadFile(filepath.Join(groupDir, "provider.json"))
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "content" {
				if !strings.Contains(string(current), "new-fixture") {
					t.Fatalf("新 Provider 未提交: %s", current)
				}
			} else if !bytes.Equal(current, oldProvider) {
				t.Fatalf("旧 Provider 意外改变: %s", current)
			}
			if scenario != "explicit-cancel" {
				metadata, err := catalog.LoadMetadata(t.Context(), filepath.Join(groupDir, "meta.json"), options.GroupID)
				if err != nil || metadata.LastSuccessAt == "" {
					t.Fatalf("成功元数据未落盘: %+v %v", metadata, err)
				}
				history, err := os.ReadFile(filepath.Join(groupDir, "history.jsonl"))
				if err != nil || !strings.Contains(string(history), `"ok":true`) {
					t.Fatalf("提交历史未落盘: %s %v", history, err)
				}
			}
		})
	}
}
