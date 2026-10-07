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
  "ipv6": true,
  "findings": [{"status":"WARN","scope":"shared","importance":"required","feature":"shared interface","reason":"temporarily_unavailable","detail":"interface wlan2 is absent"}],
  "preflight": true,
  "exact_object_load": true,
  "summary": {"pass": 8, "warn": 1, "fail": 0, "unknown": 0, "required_failures": 0, "required_unknowns": 0, "required_issues": 0},
  "result": "preflight_passed"
}`)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Preflight || !report.ExactObjectLoad || report.Findings[0].Reason != "temporarily_unavailable" {
		t.Fatalf("预检状态和原因未完整解析: %#v", report)
	}
	output := FormatProbeOutput(report, nil)
	for _, expected := range []string{
		"结论: eBPF 能力预检通过，但有注意事项",
		"检测范围: 本机应用流量、热点与共享网络",
		"内核版本: 6.1.0",
		"设备架构: arm64",
		"本机接管方式: 应用套接字（cgroup）",
		"共享网络接管方式: 数据包重写",
		"网络协议: TCP/UDP",
		"IPv6: 检测",
		"通过 8 项，警告 1 项，失败 0 项，未确认 0 项",
		"警告 · 热点与共享网络 · 必要能力",
		"所需接口或资源暂未就绪",
		"interface wlan2 is absent",
		"已检查所选 eBPF 程序能否加载，尚未验证实际挂载与网络接管",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("diagnostic output is missing %q: %s", expected, output)
		}
	}

	failure := FormatProbeOutput(ProbeReport{
		Mode:           "local",
		LocalDataPlane: "cgroup",
		Findings:       []ProbeFinding{{Status: "FAIL", Scope: "common", Importance: "required", Reason: "not_permitted", Feature: "BPF permissions", Detail: "operation not permitted"}},
		Summary:        ProbeSummary{Fail: 1, RequiredFailures: 1},
		Result:         "unsupported",
	}, errors.New("probe failed"))
	if !strings.Contains(failure, "基础能力") || !strings.Contains(failure, "权限不足，请检查 Root 授权和系统安全策略") || !strings.Contains(failure, "BPF permissions") {
		t.Fatalf("失败范围与原因未说明: %s", failure)
	}
}

func TestFormatProbeOutputDistinguishesPreflightResults(t *testing.T) {
	for _, test := range []struct {
		name    string
		report  ProbeReport
		err     error
		want    string
		missing string
	}{
		{"passed", ProbeReport{Result: "preflight_passed", ExactObjectLoad: true}, nil, "结论: eBPF 能力预检通过", "当前可见"},
		{"inconclusive", ProbeReport{Result: "inconclusive", Summary: ProbeSummary{RequiredUnknowns: 1}}, errors.New("exit status 1"), "部分必要检查无法确认", "缺少必要"},
		{"unsupported", ProbeReport{Result: "unsupported", Summary: ProbeSummary{RequiredFailures: 1}}, errors.New("exit status 1"), "缺少必要的 eBPF 能力", "结论: eBPF 能力预检通过"},
		{"abnormal-exit", ProbeReport{Result: "preflight_passed"}, errors.New("command interrupted"), "诊断命令未正常完成", "结论: eBPF 能力预检通过"},
		{"facility-only", ProbeReport{Result: "preflight_passed"}, nil, "本次仅检查基础能力", "已检查所选 eBPF 程序能否加载"},
		{"optional-failure", ProbeReport{Result: "preflight_passed", Summary: ProbeSummary{Fail: 1}, Findings: []ProbeFinding{{Status: "FAIL", Scope: "local", Importance: "performance", Reason: "unsupported", Feature: "optional optimization"}}}, nil, "可选性能优化", "缺少必要"},
		{"unknown-reason", ProbeReport{Result: "inconclusive", Findings: []ProbeFinding{{Status: "UNKNOWN", Scope: "local", Importance: "required", Reason: "new_reason", Feature: "cgroup hook", Detail: "additional probe detail"}}}, nil, "additional probe detail", "缺少必要"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := FormatProbeOutput(test.report, test.err)
			if !strings.Contains(output, test.want) || strings.Contains(output, test.missing) {
				t.Fatalf("诊断结论错误: %s", output)
			}
		})
	}
	for _, reason := range []string{"unsupported", "temporarily_unavailable", "verifier_rejected", "attach_conflict"} {
		output := FormatProbeOutput(ProbeReport{Result: "unsupported", Findings: []ProbeFinding{{Status: "FAIL", Scope: "local", Importance: "required", Reason: reason}}}, nil)
		if strings.Contains(output, reason) {
			t.Fatalf("未解释机器原因: %s", output)
		}
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
		`{"mode":"local","local_data_plane":"cgroup","preflight":true,"result":"supported"}`,
		`{"mode":"local","local_data_plane":"cgroup","result":"preflight_passed"}`,
		`{"mode":"local","local_data_plane":"cgroup","preflight":true,"result":"preflight_passed","findings":[{"status":"unexpected"}]}`,
	} {
		if _, err := ParseProbeReport(content); err == nil {
			t.Fatalf("invalid report was accepted: %q", content)
		}
	}
}
