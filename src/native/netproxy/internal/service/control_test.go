package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	json "encoding/json/v2"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"google.golang.org/protobuf/encoding/protowire"
)

func writeCatalogFixture(t *testing.T, root string) {
	t.Helper()
	groupDir := filepath.Join(root, "default")
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := catalog.NewMetadata("default", "本地配置", "local", "", time.Now())
	if err := catalog.SaveMetadataAtomic(filepath.Join(groupDir, "meta.json"), metadata); err != nil {
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
	if status.State != "stopped" || status.OutboundMode != "global" ||
		status.ConfiguredOutboundMode != "global" || status.ActiveGroupID != "default" {
		t.Fatalf("停止服务时应回退到配置模式: %#v", status)
	}
	if status.PID != nil || status.WorkerState != "stopped" {
		t.Fatalf("unexpected process state: %#v", status)
	}
	if status.CPUCount < 1 {
		t.Fatalf("invalid CPU count: %d", status.CPUCount)
	}
	encoded, err := json.Marshal(status, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("subscription_worker")) {
		t.Fatalf("status 不应继续输出旧 Worker 字段: %s", encoded)
	}
	if !bytes.Contains(encoded, []byte(`"worker_state"`)) || !bytes.Contains(encoded, []byte(`"worker_pid"`)) {
		t.Fatalf("status 缺少后台 Worker 字段: %s", encoded)
	}
}

func writeServiceAPIFrame(t *testing.T, writer io.Writer, payload []byte) {
	t.Helper()
	header := make([]byte, 5)
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := writer.Write(header); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
}

func serviceStatusFixture(t *testing.T, mode string, modeAvailable bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload []byte
		switch request.URL.Path {
		case "/daemon.StartedService/GetClashModeStatus":
			if modeAvailable {
				payload = protowire.AppendTag(payload, 2, protowire.BytesType)
				payload = protowire.AppendBytes(payload, []byte(mode))
			}
		case "/daemon.StartedService/SubscribeStatus", "/daemon.StartedService/SubscribeGroups":
		default:
			http.NotFound(writer, request)
			return
		}
		writeServiceAPIFrame(t, writer, payload)
	}))
}

func withServiceProcess(t *testing.T, pid int) {
	t.Helper()
	original := serviceFindProcess
	serviceFindProcess = func(string, int) int { return pid }
	t.Cleanup(func() { serviceFindProcess = original })
}

func writeReadyServiceState(t *testing.T, path string, pid int) {
	t.Helper()
	content := []byte(`{"state":"ready","pid":` + fmt.Sprint(pid) + `,"started_at":1700000000,"ready_at":1700000005,"error":""}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadStatusUsesActualServiceAPIMode(t *testing.T) {
	temp := t.TempDir()
	moduleConfig := filepath.Join(temp, "module.conf")
	stateFile := filepath.Join(temp, "service.json")
	if err := os.WriteFile(moduleConfig, []byte("OUTBOUND_MODE=rule\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeReadyServiceState(t, stateFile, 123)
	server := serviceStatusFixture(t, "Global", true)
	defer server.Close()
	withServiceProcess(t, 123)

	status, err := ReadStatus(context.Background(), Options{
		ModuleConfig: moduleConfig, StateFile: stateFile, ServiceAddress: server.URL,
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.OutboundMode != "global" || status.ConfiguredOutboundMode != "rule" {
		t.Fatalf("Service API 实际模式未正确映射: %#v", status)
	}
}

func TestReadStatusServiceAPIFailureDoesNotUseConfiguredMode(t *testing.T) {
	temp := t.TempDir()
	moduleConfig := filepath.Join(temp, "module.conf")
	stateFile := filepath.Join(temp, "service.json")
	if err := os.WriteFile(moduleConfig, []byte("OUTBOUND_MODE=AllowAds\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeReadyServiceState(t, stateFile, 123)
	withServiceProcess(t, 123)

	status, err := ReadStatus(context.Background(), Options{
		ModuleConfig: moduleConfig, StateFile: stateFile, ServiceAddress: "127.0.0.1:1",
		RequestTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.OutboundMode != unknownOutboundMode || status.ConfiguredOutboundMode != "AllowAds" {
		t.Fatalf("Service API 不可用时错误回退到配置模式: %#v", status)
	}
}

func TestReadStatusEmptyModeAndRecovery(t *testing.T) {
	temp := t.TempDir()
	moduleConfig := filepath.Join(temp, "module.conf")
	stateFile := filepath.Join(temp, "service.json")
	if err := os.WriteFile(moduleConfig, []byte("OUTBOUND_MODE=direct\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeReadyServiceState(t, stateFile, 123)
	mode := ""
	server := serviceStatusFixture(t, mode, true)
	defer server.Close()
	withServiceProcess(t, 123)

	options := Options{ModuleConfig: moduleConfig, StateFile: stateFile, ServiceAddress: server.URL, RequestTimeout: time.Second}
	status, err := ReadStatus(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if status.OutboundMode != unknownOutboundMode || status.ConfiguredOutboundMode != "direct" {
		t.Fatalf("空 Service API 模式未返回 unknown: %#v", status)
	}

	server.Close()
	server = serviceStatusFixture(t, "Rule", true)
	defer server.Close()
	options.ServiceAddress = server.URL
	status, err = ReadStatus(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if status.OutboundMode != "rule" || status.ConfiguredOutboundMode != "direct" {
		t.Fatalf("Service API 恢复后未返回实际模式: %#v", status)
	}
}

func TestReadStatusOldSnapshotDoesNotClaimConfiguredMode(t *testing.T) {
	temp := t.TempDir()
	moduleConfig := filepath.Join(temp, "module.conf")
	stateFile := filepath.Join(temp, "service.json")
	if err := os.WriteFile(moduleConfig, []byte("OUTBOUND_MODE=global\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeReadyServiceState(t, stateFile, 123)
	withServiceProcess(t, 0)

	status, err := ReadStatus(context.Background(), Options{
		ModuleConfig: moduleConfig, StateFile: stateFile, ServiceAddress: "127.0.0.1:1",
		RequestTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "failed" || status.OutboundMode != unknownOutboundMode || status.ConfiguredOutboundMode != "global" {
		t.Fatalf("旧服务快照错误声明了配置模式: %#v", status)
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
	if executableMatches("/data/adb/modules/other/bin/sing-box", "/data/adb/modules/netproxy/bin/sing-box") {
		t.Fatal("不同目录中的同名 sing-box 不应匹配")
	}
	if !executableMatches("/data/adb/modules/netproxy/bin/sing-box (deleted)", "/data/adb/modules/netproxy/bin/sing-box") {
		t.Fatal("已删除但路径相同的进程应保持可识别")
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
	request, err := resolveDelayRequest(context.Background(), options, "auto", "本地配置")
	if err != nil || request.Target != "Auto/本地配置" {
		t.Fatalf("automatic target = %q, err=%v", request.Target, err)
	}
	if err := os.WriteFile(moduleConfig, []byte("SELECTOR_MODE=manual\nACTIVE_GROUP_ID=default\nSELECTED_NODE_REF=\"default/NODE\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request, err = resolveDelayRequest(context.Background(), options, "", "")
	if err != nil || request.Target != "本地配置/NODE" {
		t.Fatalf("manual target = %q, err=%v", request.Target, err)
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
