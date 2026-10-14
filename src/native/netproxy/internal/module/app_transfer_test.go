package module

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func TestSubscriptionPathsPassTransferContext(t *testing.T) {
	for _, operation := range []string{"create", "edit", "update", "update-all"} {
		t.Run(operation, func(t *testing.T) {
			started := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				close(started)
				<-request.Context().Done()
			}))
			defer server.Close()
			transferContext, cancelTransfer := context.WithCancel(t.Context())
			defer cancelTransfer()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			options := newTestOptions(t.TempDir())
			options.TransferContext = transferContext
			if workerOptions(options).TransferContext != transferContext {
				t.Fatal("Worker 构造丢失 TransferContext")
			}
			groupID := "transfer-fixture"
			oldProvider := []byte(`{"outbounds":[{"type":"socks","tag":"old-fixture","server":"127.0.0.1","server_port":1080}]}` + "\n")
			if operation != "create" {
				if err := catalog.InitializeGroup(t.Context(), catalog.GroupOptions{
					Root: options.CatalogRoot, GroupID: groupID, Name: "Transfer Fixture",
					Type: "subscription", URL: server.URL, UpdateViaProxy: "never", Timeout: 60,
				}); err != nil {
					t.Fatal(err)
				}
				if err := provider.WriteAtomic(filepath.Join(options.CatalogRoot, groupID, "provider.json"), oldProvider, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			type outcome struct {
				persisted bool
				err       error
			}
			finished := make(chan outcome, 1)
			go func() {
				switch operation {
				case "create":
					result, err := AddSubscription(ctx, SubscriptionOptions{
						Options: options, Name: "Transfer Fixture", URL: server.URL,
						UpdateInterval: 86400, UpdateViaProxy: "never", Timeout: 60,
					})
					finished <- outcome{result.Persisted, err}
				case "edit":
					userAgent := "fixture-agent"
					result, err := EditSubscription(ctx, options, groupID, subscription.EditOptions{UserAgent: &userAgent})
					finished <- outcome{result.Persisted, err}
				case "update":
					result, err := UpdateSubscription(ctx, options, groupID)
					finished <- outcome{result.Persisted, err}
				case "update-all":
					_, err := UpdateAllSubscriptions(ctx, options)
					finished <- outcome{err: err}
				}
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("订阅下载未开始")
			}
			cancelTransfer()
			select {
			case result := <-finished:
				var structured *subscription.Error
				if !errors.As(result.err, &structured) || structured.Code != "subscription.cancelled" {
					t.Fatalf("生命周期取消未传递到下载: %v", result.err)
				}
				persisted := operation == "create" || operation == "edit"
				data, ok := structured.Data.(map[string]any)
				if result.persisted != persisted || !ok || data["persisted"] != persisted {
					t.Fatalf("下载取消改变了设置持久化语义: %+v %#v", result, structured.Data)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("生命周期取消未及时中断下载")
			}
			if ctx.Err() != nil {
				t.Fatalf("生命周期取消影响原事务上下文: %v", ctx.Err())
			}
			if operation == "create" {
				groups, err := catalog.GroupIDs(t.Context(), options.CatalogRoot, "subscription")
				if err != nil || len(groups) != 1 {
					t.Fatalf("已接受的创建丢失持久分组: %v %v", groups, err)
				}
				groupID = groups[0]
			} else {
				current, err := os.ReadFile(filepath.Join(options.CatalogRoot, groupID, "provider.json"))
				if err != nil || !bytes.Equal(current, oldProvider) {
					t.Fatalf("取消下载替换了旧 Provider: %s %v", current, err)
				}
			}
			metadata, err := catalog.LoadMetadata(t.Context(), filepath.Join(options.CatalogRoot, groupID, "meta.json"), groupID)
			if err != nil || metadata.LastAttemptAt == "" {
				t.Fatalf("失败状态未落盘: %+v %v", metadata, err)
			}
			if operation == "edit" && metadata.UserAgent != "fixture-agent" {
				t.Fatalf("已接受的编辑丢失持久设置: %+v", metadata)
			}
		})
	}
}
