package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func writeCatalogFixture(t *testing.T, root string) {
	t.Helper()
	groupDir := filepath.Join(root, "default")
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := catalog.NewMetadata("default", "本地配置", "local", "", time.Now())
	if err := subscription.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	providerJSON := []byte(`{"outbounds":[{"type":"socks","tag":"NODE","server":"example.com","server_port":1080}]}`)
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), providerJSON, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadStatusWithoutService(t *testing.T) {
	temp := t.TempDir()
	moduleConfig := filepath.Join(temp, "module.conf")
	if err := os.WriteFile(moduleConfig, []byte("OUTBOUND_MODE=global\nSELECTOR_MODE=urltest\nACTIVE_GROUP_ID=default\nSELECTED_NODE_REF=\"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := ReadStatus(context.Background(), Options{
		CatalogRoot:  filepath.Join(temp, "catalog"),
		ModuleConfig: moduleConfig,
		StateFile:    filepath.Join(temp, "service.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "stopped" || status.OutboundMode != "global" || status.ActiveGroupID != "default" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status.PID != nil || status.SubscriptionWorker != "stopped" {
		t.Fatalf("unexpected process state: %#v", status)
	}
	if status.CPUCount < 1 {
		t.Fatalf("invalid CPU count: %d", status.CPUCount)
	}
	if _, err := json.Marshal(status); err != nil {
		t.Fatal(err)
	}
}

func TestProcessMatchingDoesNotMatchControlCommand(t *testing.T) {
	if processMatches(os.Getpid(), "sing-box") {
		t.Fatal("当前 netproxy-native 进程不应被识别为 sing-box")
	}
	if executableMatches("/data/adb/modules/netproxy/bin/sing-box", "/data/adb/modules/netproxy/bin/sing-box") != true {
		t.Fatal("相同的可执行文件路径应匹配")
	}
	if executableMatches("/data/adb/modules/netproxy/bin/netproxy-native", "/data/adb/modules/netproxy/bin/sing-box") {
		t.Fatal("不同的可执行文件不应匹配")
	}
}

func TestReadGroupsUnavailable(t *testing.T) {
	_, err := ReadGroups(context.Background(), Options{
		ServiceAddress: "127.0.0.1:1",
		RequestTimeout: 10,
	})
	if err == nil {
		t.Fatal("expected an unavailable Service API error")
	}
}

func TestReadSelectionAndSnapshotWithoutService(t *testing.T) {
	temp := t.TempDir()
	catalogRoot := filepath.Join(temp, "catalog")
	moduleConfig := filepath.Join(temp, "module.conf")
	writeCatalogFixture(t, catalogRoot)
	if err := os.WriteFile(moduleConfig, []byte("OUTBOUND_MODE=global\nSELECTOR_MODE=urltest\nACTIVE_GROUP_ID=default\nSELECTED_NODE_REF=\"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := Options{CatalogRoot: catalogRoot, ModuleConfig: moduleConfig}
	selection, err := ReadSelection(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Selected != "Auto/本地配置" || selection.ActiveGroupName != "本地配置" || selection.ActiveGroupNodeCount != 1 {
		t.Fatalf("unexpected automatic selection: %#v", selection)
	}
	if err := os.WriteFile(moduleConfig, []byte("OUTBOUND_MODE=global\nSELECTOR_MODE=manual\nACTIVE_GROUP_ID=default\nSELECTED_NODE_REF=\"default/NODE\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	selection, err = ReadSelection(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Selected != "default/NODE" || selection.SelectedNodeRef != "default/NODE" {
		t.Fatalf("unexpected manual selection: %#v", selection)
	}
	snapshot, err := ReadSnapshot(context.Background(), options, "本地配置")
	if err != nil || len(snapshot.Groups) != 1 || snapshot.Selection.Selected != "default/NODE" {
		t.Fatalf("unexpected snapshot: %#v, err=%v", snapshot, err)
	}
}

func TestReadModeAndModeMappingWithoutService(t *testing.T) {
	temp := t.TempDir()
	moduleConfig := filepath.Join(temp, "module.conf")
	if err := os.WriteFile(moduleConfig, []byte("OUTBOUND_MODE=AllowAds\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := ReadMode(context.Background(), Options{ModuleConfig: moduleConfig, ServiceAddress: "127.0.0.1:1"})
	if err != nil || state.Mode != "AllowAds" || len(state.Available) != 4 || state.RuntimeMode != "" {
		t.Fatalf("unexpected mode state: %#v, err=%v", state, err)
	}
	for module, service := range map[string]string{"rule": "Rule", "global": "Global", "direct": "Direct", "AllowAds": "AllowAds"} {
		got, mapErr := moduleModeToServiceMode(module)
		if mapErr != nil || got != service {
			t.Fatalf("mode mapping %s = %q, err=%v", module, got, mapErr)
		}
	}
}

func TestDelayTargetResolution(t *testing.T) {
	temp := t.TempDir()
	catalogRoot := filepath.Join(temp, "catalog")
	moduleConfig := filepath.Join(temp, "module.conf")
	writeCatalogFixture(t, catalogRoot)
	if err := os.WriteFile(moduleConfig, []byte("SELECTOR_MODE=urltest\nACTIVE_GROUP_ID=default\nSELECTED_NODE_REF=\"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := Options{CatalogRoot: catalogRoot, ModuleConfig: moduleConfig}
	target, err := resolveDelayTarget(context.Background(), options, "auto", "本地配置")
	if err != nil || target != "Auto/本地配置" {
		t.Fatalf("automatic target = %q, err=%v", target, err)
	}
	if err := os.WriteFile(moduleConfig, []byte("SELECTOR_MODE=manual\nACTIVE_GROUP_ID=default\nSELECTED_NODE_REF=\"default/NODE\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target, err = resolveDelayTarget(context.Background(), options, "", "")
	if err != nil || target != "本地配置/NODE" {
		t.Fatalf("manual target = %q, err=%v", target, err)
	}
}

func TestDelayAndCloseAllConnectionsUnavailable(t *testing.T) {
	temp := t.TempDir()
	catalogRoot := filepath.Join(temp, "catalog")
	moduleConfig := filepath.Join(temp, "module.conf")
	writeCatalogFixture(t, catalogRoot)
	if err := os.WriteFile(moduleConfig, []byte("SELECTOR_MODE=urltest\nACTIVE_GROUP_ID=default\nSELECTED_NODE_REF=\"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := Options{
		CatalogRoot:    catalogRoot,
		ModuleConfig:   moduleConfig,
		ServiceAddress: "127.0.0.1:1",
		RequestTimeout: 10 * time.Millisecond,
	}
	if _, err := Delay(context.Background(), options, "auto", "default"); err == nil {
		t.Fatal("expected URLTest to fail when Service API is unavailable")
	}
	if err := CloseAllConnections(context.Background(), options); err == nil {
		t.Fatal("expected close-all to fail when Service API is unavailable")
	}
}
