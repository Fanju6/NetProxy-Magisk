package ebpf

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveProbeOptionsUsesConfiguredScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ebpf.conf")
	content := "EBPF_CGROUP_ENABLED=1\nEBPF_SHARED_NETWORK=1\nEBPF_CGROUP_PATH=/sys/fs/cgroup\nEBPF_SHARED_INTERFACES=\"wlan2,wlan0\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	options, err := ResolveProbeOptions(path, "configured")
	if err != nil {
		t.Fatal(err)
	}
	if options.CoreMode != "all" {
		t.Fatalf("expected all mode, got %q", options.CoreMode)
	}
	want := []string{"tools", "ebpf", "status", "--mode", "all", "--cgroup", "/sys/fs/cgroup", "--interface", "wlan2"}
	if !reflect.DeepEqual(options.Args(), want) {
		t.Fatalf("unexpected probe args: %#v", options.Args())
	}
}

func TestResolveProbeOptionsSupportsExplicitScopes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ebpf.conf")
	if err := os.WriteFile(path, []byte("EBPF_CGROUP_PATH=/sys/fs/cgroup\nEBPF_SHARED_INTERFACES=wlan2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	local, err := ResolveProbeOptions(path, "local")
	if err != nil {
		t.Fatal(err)
	}
	if local.CoreMode != "local" || !reflect.DeepEqual(local.Args(), []string{"tools", "ebpf", "status", "--mode", "local", "--cgroup", "/sys/fs/cgroup"}) {
		t.Fatalf("unexpected local options: %#v", local)
	}

	shared, err := ResolveProbeOptions(path, "shared")
	if err != nil {
		t.Fatal(err)
	}
	if shared.CoreMode != "shared-network" || !reflect.DeepEqual(shared.Args(), []string{"tools", "ebpf", "status", "--mode", "shared-network", "--interface", "wlan2"}) {
		t.Fatalf("unexpected shared options: %#v", shared)
	}
}

func TestFormatProbeOutputReturnsCapabilityReport(t *testing.T) {
	raw := "Platform: kernel: 6.1.0; architecture: arm64;\nSummary: PASS=8 WARN=1 FAIL=0 UNKNOWN=2\n"
	output := FormatProbeOutput(raw, "all", nil)
	for _, expected := range []string{
		"结论: 发现兼容性警告，建议启动服务进行最终验证",
		"检测范围: 本机应用流量、热点与共享网络",
		"内核版本: 6.1.0",
		"设备架构: arm64",
		"通过: 8 项",
		"警告: 1 项",
		"无法静态确认: 2 项",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("diagnostic output is missing %q: %s", expected, output)
		}
	}

	failure := FormatProbeOutput("Summary: PASS=0 WARN=0 FAIL=1 UNKNOWN=0\nFAIL [common] missing capability\n", "local", errors.New("probe failed"))
	if !strings.Contains(failure, "基础 eBPF 权限或内核能力不满足") {
		t.Fatalf("failure scope was not explained: %s", failure)
	}
}
