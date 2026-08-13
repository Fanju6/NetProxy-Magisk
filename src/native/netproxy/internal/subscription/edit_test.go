package subscription

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

func TestEditUpdatesSchedulingWithoutDownloading(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_700_000_000, 0)
	metadata := catalog.NewMetadata("sub-test", "测试订阅", "subscription", "https://example.test/sub", now)
	metadata.AutoUpdate = true
	metadata.UpdateInterval = 900
	catalog.ScheduleAt(&metadata, now)
	groupDir := filepath.Join(root, metadata.ID)
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	disabled := false
	result, err := Edit(context.Background(), EditOptions{
		Root: root, GroupID: metadata.ID, AutoUpdate: &disabled, Now: now,
	})
	if err != nil {
		t.Fatalf("disable auto update: %v", err)
	}
	if result.RequiresUpdate {
		t.Fatal("auto update toggle unexpectedly downloaded subscription")
	}
	updated, err := catalog.LoadMetadata(filepath.Join(groupDir, "meta.json"), metadata.ID)
	if err != nil {
		t.Fatalf("load edited metadata: %v", err)
	}
	if updated.AutoUpdate || updated.NextUpdateEpoch != 0 || updated.NextUpdateAt != "" {
		t.Fatalf("schedule was not cleared: %+v", updated)
	}
}

func TestEditKeepsMetadataAfterPersistedUpdateHistoryFailure(t *testing.T) {
	root, groupID, groupDir, server := newEditableSubscription(t)
	if err := os.Mkdir(filepath.Join(groupDir, "history.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}

	name := "Edited"
	url := server.URL + "?edited=1"
	_, err := Edit(context.Background(), EditOptions{
		Root: root, GroupID: groupID, Name: &name, URL: &url, Now: time.Unix(1_700_000_000, 0),
	})
	if err == nil {
		t.Fatal("history write failure was reported as success")
	}
	var subscriptionErr *Error
	if !errors.As(err, &subscriptionErr) || subscriptionErr.Code != "subscription.history_write_failed" {
		t.Fatalf("unexpected history failure: %v", err)
	}

	metadata, err := catalog.LoadMetadata(filepath.Join(groupDir, "meta.json"), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Name != name || metadata.NodeCount != 1 || metadata.Revision != 1 {
		t.Fatalf("persisted edit metadata was restored unexpectedly: %+v", metadata)
	}
	content, err := os.ReadFile(filepath.Join(groupDir, "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "new-node") {
		t.Fatalf("persisted edit Provider was restored unexpectedly: %s", content)
	}
}

func TestEditFilterChangeInvalidatesConditionalValidators(t *testing.T) {
	root := t.TempDir()
	groupID := "filter-edit"
	groupDir := filepath.Join(root, groupID)
	var conditionalHeader string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		conditionalHeader = request.Header.Get("If-None-Match")
		if conditionalHeader == `"fixture-v1"` {
			writer.WriteHeader(http.StatusNotModified)
			return
		}
		writer.Header().Set("ETag", `"fixture-v1"`)
		_, _ = writer.Write([]byte(`{"outbounds":[
			{"type":"socks","tag":"keep","server":"127.0.0.1","server_port":1080},
			{"type":"socks","tag":"drop","server":"127.0.0.1","server_port":1081}
		]}`))
	}))
	defer server.Close()

	metadata := catalog.NewMetadata(groupID, "Filter", "subscription", server.URL, time.Unix(1_700_000_000, 0))
	metadata.ETag = `"fixture-v1"`
	metadata.LastModified = "Wed, 01 Nov 2023 00:00:00 GMT"
	metadata.Timeout = 5
	metadata.UpdateViaProxy = "never"
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), []byte(`{"outbounds":[
		{"type":"socks","tag":"keep","server":"127.0.0.1","server_port":1080},
		{"type":"socks","tag":"drop","server":"127.0.0.1","server_port":1081}
	]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	include := "^keep$"
	result, err := Edit(context.Background(), EditOptions{
		Root: root, GroupID: groupID, Include: &include, Now: time.Unix(1_700_000_001, 0),
	})
	if err != nil {
		t.Fatalf("filter edit failed: %v", err)
	}
	if result.NotModified {
		t.Fatalf("filter edit unexpectedly used 304: %+v", result)
	}
	if conditionalHeader != "" {
		t.Fatalf("filter edit sent stale conditional validator: %q", conditionalHeader)
	}

	document, err := provider.Load(context.Background(), filepath.Join(groupDir, "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Outbounds) != 1 || document.Outbounds[0].Tag != "keep" {
		t.Fatalf("filter edit did not rebuild Provider: %+v", provider.Inspect(document))
	}
}

func TestEditRestoresMetadataBeforeUpdateCommit(t *testing.T) {
	root, groupID, groupDir, _ := newEditableSubscription(t)
	progressPath := filepath.Join(root, "progress")
	if err := os.WriteFile(progressPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	name := "Edited"
	url := "https://example.test/edited"
	_, err := Edit(context.Background(), EditOptions{
		Root: root, GroupID: groupID, ProgressDir: progressPath, Name: &name, URL: &url,
		Now: time.Unix(1_700_000_000, 0),
	})
	if err == nil {
		t.Fatal("pre-commit failure was reported as success")
	}

	metadata, err := catalog.LoadMetadata(filepath.Join(groupDir, "meta.json"), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Name != "Original" || metadata.NodeCount != 0 || metadata.Revision != 0 {
		t.Fatalf("pre-commit failure did not restore metadata: %+v", metadata)
	}
	content, err := os.ReadFile(filepath.Join(groupDir, "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "old-node") || strings.Contains(string(content), "new-node") {
		t.Fatalf("pre-commit failure changed Provider: %s", content)
	}
}

func TestEditDoesNotOverwriteConcurrentMetadataWriteDuringRestore(t *testing.T) {
	root, groupID, groupDir, _ := newEditableSubscription(t)
	progressPath := filepath.Join(root, "progress")
	if err := os.WriteFile(progressPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	var concurrentErr error
	previousHook := editBeforeRestoreHook
	editBeforeRestoreHook = func() {
		_, concurrentErr = catalog.AppendNode(context.Background(), catalog.MutationOptions{
			GroupDir: groupDir, GroupID: groupID, Type: "subscription",
			Input: "socks://127.0.0.1:1081#concurrent-node", Now: time.Unix(1_700_000_001, 0),
		})
	}
	defer func() { editBeforeRestoreHook = previousHook }()

	name := "Edited"
	url := "https://example.test/edited"
	_, err := Edit(context.Background(), EditOptions{
		Root: root, GroupID: groupID, ProgressDir: progressPath, Name: &name, URL: &url,
		Now: time.Unix(1_700_000_000, 0),
	})
	if err == nil {
		t.Fatal("pre-commit failure was reported as success")
	}
	if concurrentErr != nil {
		t.Fatalf("concurrent Catalog write failed: %v", concurrentErr)
	}

	metadata, err := catalog.LoadMetadata(filepath.Join(groupDir, "meta.json"), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Revision != 1 || metadata.Name != name || metadata.NodeCount != 2 {
		t.Fatalf("concurrent metadata was overwritten: %+v", metadata)
	}
	content, err := os.ReadFile(filepath.Join(groupDir, "provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "concurrent-node") {
		t.Fatalf("concurrent Provider was overwritten: %s", content)
	}
}

func newEditableSubscription(t *testing.T) (root, groupID, groupDir string, server *httptest.Server) {
	t.Helper()
	root = t.TempDir()
	groupID = "editable"
	groupDir = filepath.Join(root, groupID)
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"socks","tag":"new-node","server":"127.0.0.1","server_port":1080}]}`))
	}))
	t.Cleanup(server.Close)
	metadata := catalog.NewMetadata(groupID, "Original", "subscription", server.URL, time.Unix(1_700_000_000, 0))
	metadata.Timeout = 5
	metadata.UpdateViaProxy = "never"
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), []byte(`{"outbounds":[{"type":"socks","tag":"old-node","server":"127.0.0.1","server_port":1080}]}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, groupID, groupDir, server
}
