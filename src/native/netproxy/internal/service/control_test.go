package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
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
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
	"google.golang.org/protobuf/encoding/protowire"
)

func modeConfigFixture(t *testing.T, root, mode string) string {
	t.Helper()
	path := filepath.Join(root, "config.json")
	content := fmt.Sprintf(`{"experimental":{"clash_api":{"default_mode":%q}},"route":{"rules":[{"clash_mode":["Proxy","Direct","RuleAllowAds"]}]}}`, mode)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeCatalogFixture(t *testing.T, root string) {
	t.Helper()
	groupDir := filepath.Join(root, "default")
	if err := os.MkdirAll(groupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := catalog.NewMetadata("default", "本地配置", "local", "", time.Now())
	metadata.NodeCount = 1
	if err := catalog.SaveMetadataAtomic(context.Background(), filepath.Join(groupDir, "meta.json"), metadata); err != nil {
		t.Fatal(err)
	}
	providerJSON := []byte(`{"outbounds":[{"type":"socks","tag":"NODE","server":"example.com","server_port":1080}]}`)
	if err := provider.WriteAtomic(filepath.Join(groupDir, "provider.json"), providerJSON, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadStatusWithoutService(t *testing.T) {
	temp := t.TempDir()
	moduleConfig := filepath.Join(temp, "module.json")
	if err := os.WriteFile(moduleConfig, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := ReadStatus(context.Background(), Options{SingBoxConfig: modeConfigFixture(t, temp, "Proxy"),
		CatalogRoot:  filepath.Join(temp, "catalog"),
		ModuleConfig: moduleConfig,
		StateFile:    filepath.Join(temp, "service.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "stopped" || status.OutboundMode != "Proxy" ||
		status.ConfiguredOutboundMode != "Proxy" || status.DefaultOutbound != "" {
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
	for _, field := range []string{"subscription_worker", "selector_mode", "active_group_id", "active_group_name", "active_group_runtime_tag", "active_group_node_count", "selected_node_ref"} {
		if bytes.Contains(encoded, []byte(field)) {
			t.Fatalf("status contains removed field %s: %s", field, encoded)
		}
	}
	if !bytes.Contains(encoded, []byte(`"worker_state"`)) || !bytes.Contains(encoded, []byte(`"worker_pid"`)) {
		t.Fatalf("status 缺少后台 Worker 字段: %s", encoded)
	}
}

func TestReadStatusDoesNotWaitForCatalog(t *testing.T) {
	temp := t.TempDir()
	root := filepath.Join(temp, "catalog")
	release, err := catalog.AcquireRoot(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	status, err := ReadStatus(ctx, Options{
		SingBoxConfig: modeConfigFixture(t, temp, "Rule"),
		CatalogRoot:   root, ModuleConfig: filepath.Join(temp, "missing-module.json"),
		StateFile:     filepath.Join(temp, "service.json"),
		WorkerPIDFile: filepath.Join(temp, "worker.pid"),
	})
	if err != nil || status.State != "stopped" || status.Error != "" {
		t.Fatalf("Catalog lock or missing module affected status: %#v, %v", status, err)
	}
	cancel()
	if _, err := ReadStatus(ctx, Options{SingBoxConfig: filepath.Join(temp, "config.json")}); !errors.Is(err, context.Canceled) {
		t.Fatalf("status lost cancellation: %v", err)
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
	moduleConfig := filepath.Join(temp, "module.json")
	stateFile := filepath.Join(temp, "service.json")
	if err := os.WriteFile(moduleConfig, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeReadyServiceState(t, stateFile, 123)
	server := serviceStatusFixture(t, "Proxy", true)
	defer server.Close()
	withServiceProcess(t, 123)

	status, err := ReadStatus(context.Background(), Options{SingBoxConfig: modeConfigFixture(t, temp, "Rule"),
		ModuleConfig: moduleConfig, StateFile: stateFile, ServiceAddress: server.URL,
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.OutboundMode != "Proxy" || status.ConfiguredOutboundMode != "Rule" {
		t.Fatalf("Service API 实际模式未正确映射: %#v", status)
	}
}

func TestReadStatusFetchesIndependentServiceAPISnapshotsConcurrently(t *testing.T) {
	temp := t.TempDir()
	catalogRoot := filepath.Join(temp, "catalog")
	moduleConfig := filepath.Join(temp, "module.json")
	stateFile := filepath.Join(temp, "service.json")
	writeCatalogFixture(t, catalogRoot)
	if err := os.WriteFile(moduleConfig, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeReadyServiceState(t, stateFile, 123)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		time.Sleep(100 * time.Millisecond)
		var payload []byte
		if request.URL.Path == "/daemon.StartedService/GetClashModeStatus" {
			payload = protowire.AppendTag(payload, 2, protowire.BytesType)
			payload = protowire.AppendBytes(payload, []byte("Rule"))
		}
		writeServiceAPIFrame(t, writer, payload)
	}))
	defer server.Close()
	withServiceProcess(t, 123)

	started := time.Now()
	status, err := ReadStatus(context.Background(), Options{SingBoxConfig: modeConfigFixture(t, temp, "Rule"),
		CatalogRoot: catalogRoot, ModuleConfig: moduleConfig, StateFile: stateFile,
		ServiceAddress: server.URL, RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed >= 250*time.Millisecond {
		t.Fatalf("Service API 独立快照仍在串行读取: %s", elapsed)
	}
	if status.OutboundMode != "Rule" {
		t.Fatalf("并发快照丢失模式: %#v", status)
	}
}

func TestReadStatusServiceAPIFailureDoesNotUseConfiguredMode(t *testing.T) {
	temp := t.TempDir()
	moduleConfig := filepath.Join(temp, "module.json")
	stateFile := filepath.Join(temp, "service.json")
	if err := os.WriteFile(moduleConfig, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeReadyServiceState(t, stateFile, 123)
	withServiceProcess(t, 123)

	status, err := ReadStatus(context.Background(), Options{SingBoxConfig: modeConfigFixture(t, temp, "RuleAllowAds"),
		ModuleConfig: moduleConfig, StateFile: stateFile, ServiceAddress: "127.0.0.1:1",
		RequestTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.OutboundMode != unknownOutboundMode || status.ConfiguredOutboundMode != "RuleAllowAds" {
		t.Fatalf("Service API 不可用时错误回退到配置模式: %#v", status)
	}
}

func TestReadStatusEmptyModeAndRecovery(t *testing.T) {
	temp := t.TempDir()
	moduleConfig := filepath.Join(temp, "module.json")
	stateFile := filepath.Join(temp, "service.json")
	if err := os.WriteFile(moduleConfig, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeReadyServiceState(t, stateFile, 123)
	mode := ""
	server := serviceStatusFixture(t, mode, true)
	defer server.Close()
	withServiceProcess(t, 123)

	options := Options{SingBoxConfig: modeConfigFixture(t, temp, "Direct"), ModuleConfig: moduleConfig, StateFile: stateFile, ServiceAddress: server.URL, RequestTimeout: time.Second}
	status, err := ReadStatus(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if status.OutboundMode != unknownOutboundMode || status.ConfiguredOutboundMode != "Direct" {
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
	if status.OutboundMode != "Rule" || status.ConfiguredOutboundMode != "Direct" {
		t.Fatalf("Service API 恢复后未返回实际模式: %#v", status)
	}
}

func TestReadStatusOldSnapshotDoesNotClaimConfiguredMode(t *testing.T) {
	temp := t.TempDir()
	moduleConfig := filepath.Join(temp, "module.json")
	stateFile := filepath.Join(temp, "service.json")
	if err := os.WriteFile(moduleConfig, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeReadyServiceState(t, stateFile, 123)
	withServiceProcess(t, 0)

	status, err := ReadStatus(context.Background(), Options{SingBoxConfig: modeConfigFixture(t, temp, "Proxy"),
		ModuleConfig: moduleConfig, StateFile: stateFile, ServiceAddress: "127.0.0.1:1",
		RequestTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "failed" || status.OutboundMode != unknownOutboundMode || status.ConfiguredOutboundMode != "Proxy" {
		t.Fatalf("旧服务快照错误声明了配置模式: %#v", status)
	}
}

func TestProcessMatchingDoesNotMatchControlCommand(t *testing.T) {
	if processMatches(os.Getpid(), "sing-box") {
		t.Fatal("当前 netproxyctl 进程不应被识别为 sing-box")
	}
	if executableMatches("/data/adb/modules/netproxy/bin/sing-box", "/data/adb/modules/netproxy/bin/sing-box") != true {
		t.Fatal("相同的可执行文件路径应匹配")
	}
	if executableMatches("/data/adb/modules/netproxy/bin/netproxyctl", "/data/adb/modules/netproxy/bin/sing-box") {
		t.Fatal("不同的可执行文件不应匹配")
	}
	if executableMatches("/data/adb/modules/other/bin/sing-box", "/data/adb/modules/netproxy/bin/sing-box") {
		t.Fatal("不同目录中的同名 sing-box 不应匹配")
	}
	if !executableMatches("/data/adb/modules/netproxy/bin/sing-box (deleted)", "/data/adb/modules/netproxy/bin/sing-box") {
		t.Fatal("已删除但路径相同的进程应保持可识别")
	}
}

func TestReadSnapshotWithoutModuleOrService(t *testing.T) {
	temp := t.TempDir()
	catalogRoot := filepath.Join(temp, "catalog")
	writeCatalogFixture(t, catalogRoot)
	options := Options{CatalogRoot: catalogRoot, ModuleConfig: filepath.Join(temp, "missing-module.json"), ServiceAddress: "127.0.0.1:1"}
	snapshot, err := ReadSnapshot(t.Context(), options, "本地配置")
	if err != nil || len(snapshot.Groups) != 1 || snapshot.Groups[0].Group.ID != "default" || len(snapshot.Groups[0].Nodes) != 1 {
		t.Fatalf("unexpected Catalog snapshot: %#v, err=%v", snapshot, err)
	}
	encoded, err := json.Marshal(snapshot, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil || len(fields) != 1 || fields["groups"] == nil {
		t.Fatalf("snapshot mixed runtime data: %s, %v", encoded, err)
	}
}

func TestReadModeWithoutService(t *testing.T) {
	temp := t.TempDir()
	moduleConfig := filepath.Join(temp, "module.json")
	if err := os.WriteFile(moduleConfig, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := ReadMode(context.Background(), Options{SingBoxConfig: modeConfigFixture(t, temp, "RuleAllowAds"), ModuleConfig: moduleConfig, ServiceAddress: "127.0.0.1:1"})
	if err != nil || state.Mode != "RuleAllowAds" || len(state.Available) != 3 || state.RuntimeMode != "" {
		t.Fatalf("unexpected mode state: %#v, err=%v", state, err)
	}
}

func TestCloseAllConnectionsUnavailable(t *testing.T) {
	if err := CloseAllConnections(t.Context(), Options{ServiceAddress: "127.0.0.1:1", RequestTimeout: 10 * time.Millisecond}); err == nil {
		t.Fatal("expected close-all to fail when Service API is unavailable")
	}
}

func TestRuntimeSelectedFollowsNativeGroups(t *testing.T) {
	groups := []serviceapi.Group{
		{Tag: "Main", Selected: "Fast", Items: []serviceapi.GroupItem{{Tag: "Fast", Type: "urltest"}}},
		{Tag: "Fast", Selected: "provider/node", Items: []serviceapi.GroupItem{{Tag: "provider/node", Type: "socks"}}},
	}
	for _, test := range []struct {
		name   string
		final  string
		groups []serviceapi.Group
		want   string
	}{
		{"nested", "Main", groups, "provider/node"},
		{"direct-default", "direct", groups, ""},
		{"empty-default", "", groups, ""},
		{"unknown-default", "Other", groups, ""},
		{"unselected", "Main", []serviceapi.Group{{Tag: "Main"}}, ""},
		{"non-member", "Main", []serviceapi.Group{{Tag: "Main", Selected: "missing"}}, ""},
		{"missing-nested", "Main", groups[:1], ""},
		{"cycle", "Main", []serviceapi.Group{
			{Tag: "Main", Selected: "Other", Items: []serviceapi.GroupItem{{Tag: "Other", Type: "selector"}}},
			{Tag: "Other", Selected: "Main", Items: []serviceapi.GroupItem{{Tag: "Main", Type: "selector"}}},
		}, ""},
		{"duplicate-group", "Main", append(append([]serviceapi.Group{}, groups...), groups[0]), ""},
		{"duplicate-member", "Main", []serviceapi.Group{{Tag: "Main", Selected: "node", Items: []serviceapi.GroupItem{{Tag: "node", Type: "socks"}, {Tag: "node", Type: "socks"}}}}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := RuntimeSelected(test.final, test.groups); got != test.want {
				t.Fatalf("runtime selected = %q, want %q", got, test.want)
			}
		})
	}
}

func TestReadDefaultOutboundUsesOnlyRouteFinal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	for _, test := range []struct {
		content, want string
		invalid       bool
	}{
		{`{"route":{"final":"Main"},"outbounds":[{"tag":"Proxy"}]}`, "Main", false},
		{`{"outbounds":[{"tag":"Proxy","type":"selector"}]}`, "", false},
		{`{"route":{"final":"first","final":"second"}}`, "", true},
		{`{"route":{"final":5}}`, "", true},
		{`null`, "", true},
		{`{"route":`, "", true},
	} {
		if err := os.WriteFile(path, []byte(test.content), 0600); err != nil {
			t.Fatal(err)
		}
		value, err := ReadDefaultOutbound(path)
		if (err != nil) != test.invalid || value != test.want {
			t.Fatalf("config=%s: final=%q err=%v", test.content, value, err)
		}
	}
}
