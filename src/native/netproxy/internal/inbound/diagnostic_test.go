package inbound

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/option"
)

func TestResolveProbeOptionsUsesConfiguredScope(t *testing.T) {
	config := fixture(t, "ebpf", `{"type":"ebpf","tag":"netproxy-in","network":["tcp","udp"],"local":{"enabled":true},"shared":{"enabled":true,"interface":["wlan2","wlan0"]}}`, "")
	native, err := config.EBPFOptions()
	if err != nil {
		t.Fatal(err)
	}
	options, err := ResolveProbeOptions(native, "configured")
	if err != nil {
		t.Fatal(err)
	}
	if options.CoreMode != "all" {
		t.Fatalf("expected all mode, got %q", options.CoreMode)
	}
	want := []string{"tools", "ebpf", "status", "--mode", "all", "--local-data-plane", "cgroup", "--shared-data-plane", "packet_rewrite", "--network", "tcp,udp", "--interface", "wlan2", "--json"}
	if !reflect.DeepEqual(options.Args(), want) {
		t.Fatalf("unexpected probe args: %#v", options.Args())
	}
}

func TestProbeNativeDefaultsAndInvalidScopes(t *testing.T) {
	defaults, err := ResolveProbeOptions(option.EBPFInboundOptions{}, "")
	if err != nil || defaults.CoreMode != "local" || defaults.LocalDataPlane != "cgroup" || defaults.SharedDataPlane != "packet_rewrite" || !defaults.IPv6 || !reflect.DeepEqual(defaults.Network, []string{"tcp", "udp"}) {
		t.Fatal(defaults, err)
	}
	config := fixture(t, "ebpf", `{"type":"ebpf","tag":"netproxy-in","local":{"enabled":false,"ipv6":false},"shared":{"enabled":true,"interface":"wlan2","ipv6":false}}`, "")
	native, _ := config.EBPFOptions()
	shared, err := ResolveProbeOptions(native, "configured")
	if err != nil || shared.CoreMode != "shared" || shared.IPv6 || shared.Interface != "wlan2" {
		t.Fatal(shared, err)
	}
	if all, err := ResolveProbeOptions(native, "all"); err != nil || all.CoreMode != "all" {
		t.Fatal(all, err)
	}
	for _, scope := range []string{"legacy", "tun", "disabled"} {
		if _, err := ResolveProbeOptions(native, scope); err == nil {
			t.Fatalf("接受非法诊断范围: %s", scope)
		}
	}
}

func TestRunProbeUsesNativeOptionsAndPropagatesCancellation(t *testing.T) {
	directory := t.TempDir()
	binary := buildFakeCommand(t, directory)
	logPath := filepath.Join(directory, "probe-args")
	t.Setenv("NETPROXY_TEST_COMMAND_MODE", "probe")
	t.Setenv("NETPROXY_TEST_COMMAND_LOG", logPath)
	native := option.EBPFInboundOptions{}
	output, err := RunProbe(t.Context(), binary, native, "configured")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseProbeReport(output); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(logPath)
	options, _ := ResolveProbeOptions(native, "configured")
	if err != nil || string(args) != strings.Join(options.Args(), "\n") {
		t.Fatal(string(args), err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := RunProbe(ctx, binary, native, "configured"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := RunProbe(t.Context(), "", native, "configured"); err == nil {
		t.Fatal("接受空核心路径")
	}
	if _, err := RunProbe(t.Context(), binary, native, "legacy"); err == nil {
		t.Fatal("接受非法检查范围")
	}
	if _, err := RunProbe(t.Context(), filepath.Join(directory, "missing"), native, "configured"); err == nil {
		t.Fatal("诊断进程失败被吞掉")
	}
}

func TestResolveProbeOptionsSupportsExplicitScopes(t *testing.T) {
	config := fixture(t, "ebpf", `{"type":"ebpf","tag":"netproxy-in","local":{"enabled":true,"data_plane":"tc","ipv6":false},"shared":{"enabled":true,"data_plane":"socket_assign","ipv6":true,"interface":"wlan2"}}`, "")
	native, err := config.EBPFOptions()
	if err != nil {
		t.Fatal(err)
	}
	local, err := ResolveProbeOptions(native, "local")
	if err != nil {
		t.Fatal(err)
	}
	if local.CoreMode != "local" || !reflect.DeepEqual(local.Args(), []string{"tools", "ebpf", "status", "--mode", "local", "--local-data-plane", "tc", "--network", "tcp,udp", "--ipv6=false", "--json"}) {
		t.Fatalf("unexpected local options: %#v", local)
	}

	shared, err := ResolveProbeOptions(native, "shared")
	if err != nil {
		t.Fatal(err)
	}
	if shared.CoreMode != "shared" || !reflect.DeepEqual(shared.Args(), []string{"tools", "ebpf", "status", "--mode", "shared", "--shared-data-plane", "socket_assign", "--network", "tcp,udp", "--interface", "wlan2", "--json"}) {
		t.Fatalf("unexpected shared options: %#v", shared)
	}
}

func TestFormatProbeOutputReturnsCapabilityReport(t *testing.T) {
	report, err := ParseProbeReport(`{
  "platform": "android",
  "kernel_release": "6.1.0",
  "architecture": "arm64",
  "mode": "all",
  "local_data_plane": "cgroup",
  "shared_data_plane": "packet_rewrite",
  "network": ["tcp", "udp"],
  "findings": [],
  "active_programs": [{"id": 12, "name": "netproxy", "type": "sched_cls", "map_count": 4}],
  "summary": {"pass": 8, "warn": 1, "fail": 0, "unknown": 2, "required_failures": 0, "required_unknowns": 2, "required_issues": 2},
  "result": "inconclusive"
}`)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.RequiredUnknowns != 2 || report.Summary.RequiredIssues != 2 {
		t.Fatalf("stable probe summary was not preserved: %#v", report.Summary)
	}
	output := FormatProbeOutput(report, nil)
	for _, expected := range []string{
		"结论: 无法完全确认，启动服务后可完成最终验证",
		"检测范围: 本机应用流量、热点与共享网络",
		"内核版本: 6.1.0",
		"设备架构: arm64",
		"本机数据平面: cgroup",
		"共享数据平面: packet_rewrite",
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
		Mode:           "local",
		LocalDataPlane: "cgroup",
		Findings:       []ProbeFinding{{Status: "FAIL", Scope: "common", Importance: "required"}},
		Summary:        ProbeSummary{Fail: 1, RequiredFailures: 1},
		Result:         "unsupported",
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
		`{"mode":"local","local_data_plane":"cgroup","result":"unknown"}`,
		`{"mode":"local","local_data_plane":"legacy","result":"supported"}`,
		`{"mode":"shared","shared_data_plane":"legacy","result":"supported"}`,
	} {
		if _, err := ParseProbeReport(content); err == nil {
			t.Fatalf("invalid report was accepted: %q", content)
		}
	}
}
