package paths

import (
	"path/filepath"
	"testing"
)

func TestLayout(t *testing.T) {
	layout := New(filepath.Join("module", "netproxy"))
	root := filepath.Join("module", "netproxy")
	expected := map[string]string{
		"root":          root,
		"catalog":       filepath.Join(root, "data", "catalog"),
		"module config": filepath.Join(root, "config", "module.conf"),
		"ebpf config":   filepath.Join(root, "config", "ebpf", "ebpf.conf"),
		"sing-box":      filepath.Join(root, "bin", "sing-box"),
		"native":        filepath.Join(root, "bin", "netproxy-native"),
		"confdir":       filepath.Join(root, "config", "singbox", "confdir"),
		"local rules":   filepath.Join(root, "config", "singbox", "rules", "local"),
		"remote rules":  filepath.Join(root, "config", "singbox", "rules", "remote"),
		"runtime":       filepath.Join(root, "runtime"),
		"logs":          filepath.Join(root, "logs"),
		"service state": filepath.Join("/dev", "netproxy", "service.json"),
		"worker pid":    filepath.Join("/dev", "netproxy", "subworker.pid"),
		"progress":      filepath.Join("/dev", "netproxy", "subscriptions"),
		"wifi state":    filepath.Join("/dev", "netproxy", "wifi_state"),
	}
	actual := map[string]string{
		"root":          layout.Root(),
		"catalog":       layout.Catalog(),
		"module config": layout.ModuleConfig(),
		"ebpf config":   layout.EBPFConfig(),
		"sing-box":      layout.SingBox(),
		"native":        layout.Native(),
		"confdir":       layout.SingBoxConfDir(),
		"local rules":   layout.SingBoxLocalRulesDir(),
		"remote rules":  layout.SingBoxRemoteRulesDir(),
		"runtime":       layout.Runtime(),
		"logs":          layout.Logs(),
		"service state": layout.ServiceState(),
		"worker pid":    layout.WorkerPID(),
		"progress":      layout.ProgressDir(),
		"wifi state":    layout.WiFiState(),
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
