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
	content := "EBPF_MODE=hybrid\nEBPF_NETWORK=\"tcp,udp\"\nEBPF_LOCAL_CGROUP_PATH=/sys/fs/cgroup\nEBPF_SHARED_INTERFACES=\"wlan2,wlan0\"\n"
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
	want := []string{"tools", "ebpf", "status", "--mode", "all", "--network", "tcp,udp", "--cgroup", "/sys/fs/cgroup", "--interface", "wlan2", "--json"}
	if !reflect.DeepEqual(options.Args(), want) {
		t.Fatalf("unexpected probe args: %#v", options.Args())
	}
}

func TestResolveProbeOptionsSupportsExplicitScopes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ebpf.conf")
	if err := os.WriteFile(path, []byte("EBPF_LOCAL_CGROUP_PATH=/sys/fs/cgroup\nEBPF_SHARED_INTERFACES=wlan2\nEBPF_NETWORK=tcp,udp\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	local, err := ResolveProbeOptions(path, "local")
	if err != nil {
		t.Fatal(err)
	}
	if local.CoreMode != "local" || !reflect.DeepEqual(local.Args(), []string{"tools", "ebpf", "status", "--mode", "local", "--network", "tcp,udp", "--cgroup", "/sys/fs/cgroup", "--json"}) {
		t.Fatalf("unexpected local options: %#v", local)
	}

	shared, err := ResolveProbeOptions(path, "shared")
	if err != nil {
		t.Fatal(err)
	}
	if shared.CoreMode != "shared-network" || !reflect.DeepEqual(shared.Args(), []string{"tools", "ebpf", "status", "--mode", "shared-network", "--interface", "wlan2", "--json"}) {
		t.Fatalf("unexpected shared options: %#v", shared)
	}
}

func TestFormatProbeOutputReturnsCapabilityReport(t *testing.T) {
	report, err := ParseProbeReport(`{
  "platform": "android",
  "kernel_release": "6.1.0",
  "architecture": "arm64",
  "mode": "all",
  "network": ["tcp", "udp"],
  "findings": [],
  "active_programs": [{"id": 12, "name": "netproxy", "type": "sched_cls", "map_count": 4}],
  "summary": {"pass": 8, "warn": 1, "fail": 0, "unknown": 2, "required_failures": 0},
  "result": "inconclusive"
}`)
	if err != nil {
		t.Fatal(err)
	}
	output := FormatProbeOutput(report, nil)
	for _, expected := range []string{
		"结论: 无法完全确认，启动服务后可完成最终验证",
		"检测范围: 本机应用流量、热点与共享网络",
		"内核版本: 6.1.0",
		"设备架构: arm64",
		"通过: 8 项",
		"警告: 1 项",
		"无法静态确认: 2 项",
		"当前可见 sing-box eBPF 程序: 1 个",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("diagnostic output is missing %q: %s", expected, output)
		}
	}

	failure := FormatProbeOutput(ProbeReport{
		Mode:     "local",
		Findings: []ProbeFinding{{Status: "FAIL", Scope: "common", Importance: "required"}},
		Summary:  ProbeSummary{Fail: 1, RequiredFailures: 1},
		Result:   "unsupported",
	}, errors.New("probe failed"))
	if !strings.Contains(failure, "基础 eBPF 权限或内核能力不满足") {
		t.Fatalf("failure scope was not explained: %s", failure)
	}
}

func TestParseProbeReportRejectsInvalidReports(t *testing.T) {
	for _, content := range []string{
		"",
		"not-json",
		`{"mode":"legacy","result":"supported"}`,
		`{"mode":"local","result":"unknown"}`,
	} {
		if _, err := ParseProbeReport(content); err == nil {
			t.Fatalf("invalid report was accepted: %q", content)
		}
	}
}
