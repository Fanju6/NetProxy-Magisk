package paths

import (
	"path/filepath"
	"testing"
)

func TestLayout(t *testing.T) {
	t.Setenv("NETPROXY_DEV_ROOT", "")
	layout := New(filepath.Join("module", "netproxy"))
	root := filepath.Join("module", "netproxy")
	expected := map[string]string{
		"root":           root,
		"catalog":        filepath.Join(root, "data", "catalog"),
		"module config":  filepath.Join(root, "config", "module.json"),
		"inbound config": filepath.Join(root, "config", "inbound", "inbound.json"),
		"sing-box":       filepath.Join(root, "bin", "sing-box"),
		"executable":     filepath.Join(root, "bin", "netproxyctl"),
		"config.json":    filepath.Join(root, "config", "singbox", "config.json"),
		"local rules":    filepath.Join(root, "config", "singbox", "rules", "local"),
		"remote rules":   filepath.Join(root, "config", "singbox", "rules", "remote"),
		"runtime":        filepath.Join(root, "runtime"),
		"logs":           filepath.Join(root, "logs"),
		"service state":  filepath.Join("/dev", "netproxy", "service.json"),
		"worker pid":     filepath.Join("/dev", "netproxy", "worker.pid"),
		"progress":       filepath.Join("/dev", "netproxy", "subscriptions"),
		"wifi state":     filepath.Join("/dev", "netproxy", "wifi_state"),
	}
	actual := map[string]string{
		"root":           layout.Root(),
		"catalog":        layout.Catalog(),
		"module config":  layout.ModuleConfig(),
		"inbound config": layout.InboundConfig(),
		"sing-box":       layout.SingBox(),
		"executable":     layout.Executable(),
		"config.json":    SingBoxConfig(layout.SingBoxDir()),
		"local rules":    SingBoxLocalRulesDir(layout.SingBoxDir()),
		"remote rules":   SingBoxRemoteRulesDir(layout.SingBoxDir()),
		"runtime":        layout.Runtime(),
		"logs":           layout.Logs(),
		"service state":  layout.ServiceState(),
		"worker pid":     layout.WorkerPID(),
		"progress":       layout.ProgressDir(),
		"wifi state":     layout.WiFiState(),
	}
	for name, want := range expected {
		if got := actual[name]; got != want {
			t.Fatalf("%s 路径错误: got %q, want %q", name, got, want)
		}
	}
}

func TestLayoutCleansRoot(t *testing.T) {
	if got, want := New(filepath.Join("module", "netproxy", "..", "netproxy")).Root(), filepath.Join("module", "netproxy"); got != want {
		t.Fatalf("模块根目录未规范化: got %q, want %q", got, want)
	}
}

func TestLayoutIsolatesAllTransientState(t *testing.T) {
	root := t.TempDir()
	devRoot := filepath.Join(t.TempDir(), "state", "..", "dev")
	t.Setenv("NETPROXY_DEV_ROOT", devRoot)
	layout := New(root)
	wantRoot := filepath.Clean(devRoot)
	for _, path := range []struct {
		got  string
		name string
	}{
		{layout.DevRoot(), ""},
		{layout.ServiceState(), "service.json"},
		{layout.WorkerPID(), "worker.pid"},
		{layout.ProgressDir(), "subscriptions"},
		{layout.DelayDir(), "delay"},
		{layout.WiFiState(), "wifi_state"},
		{layout.TelemetryLock(), "telemetry.lock.flock"},
	} {
		if want := filepath.Join(wantRoot, path.name); path.got != want {
			t.Fatalf("瞬态路径未隔离: got %q, want %q", path.got, want)
		}
	}
	if layout.Root() != root || layout.ModuleConfig() != filepath.Join(root, "config", "module.json") {
		t.Fatal("状态目录覆盖改变了模块持久路径")
	}
}
