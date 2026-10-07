package inbound

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	json "encoding/json/v2"

	"github.com/sagernet/sing-box/option"
)

// ProbeOptions 描述 sing-box eBPF 能力检查所需的运行参数。
type ProbeOptions struct {
	RequestedMode   string
	CoreMode        string
	LocalDataPlane  string
	SharedDataPlane string
	Network         []string
	IPv6            bool
	Interface       string
}

// ResolveProbeOptions 根据当前 eBPF 配置解析能力检查范围与数据平面。
func ResolveProbeOptions(config option.EBPFInboundOptions, requestedMode string) (ProbeOptions, error) {
	localEnabled, sharedEnabled := config.EffectiveEnablement()
	requested := strings.ToLower(strings.TrimSpace(requestedMode))
	if requested == "" {
		requested = "configured"
	}
	coreMode := requested
	if requested == "configured" {
		switch {
		case localEnabled && sharedEnabled:
			coreMode = "all"
		case sharedEnabled:
			coreMode = "shared"
		default:
			coreMode = "local"
		}
	}

	switch coreMode {
	case "all", "local", "shared":
	default:
		return ProbeOptions{}, fmt.Errorf("eBPF 检查范围无效: %s", requestedMode)
	}

	network := config.Network.Build()
	ipv6 := enabledByDefault(config.Local.IPv6)
	if coreMode == "shared" {
		ipv6 = enabledByDefault(config.Shared.IPv6)
	} else if coreMode == "all" {
		ipv6 = enabledByDefault(config.Local.IPv6) || enabledByDefault(config.Shared.IPv6)
	}
	interfaceName := ""
	if len(config.Shared.Interface) > 0 {
		interfaceName = config.Shared.Interface[0]
	}
	return ProbeOptions{
		RequestedMode:   requested,
		CoreMode:        coreMode,
		LocalDataPlane:  localDataPlane(config.Local.DataPlane),
		SharedDataPlane: sharedDataPlane(config.Shared.DataPlane),
		Network:         network,
		IPv6:            ipv6,
		Interface:       interfaceName,
	}, nil
}

// Args 返回 sing-box tools ebpf status 的参数。
func (o ProbeOptions) Args() []string {
	args := []string{"tools", "ebpf", "status", "--mode", o.CoreMode}
	if o.CoreMode == "all" || o.CoreMode == "local" {
		args = append(args, "--local-data-plane", o.LocalDataPlane)
	}
	if o.CoreMode == "all" || o.CoreMode == "shared" {
		args = append(args, "--shared-data-plane", o.SharedDataPlane)
	}
	if len(o.Network) > 0 {
		args = append(args, "--network", strings.Join(o.Network, ","))
	}
	if !o.IPv6 {
		args = append(args, "--ipv6=false")
	}
	if o.CoreMode == "all" || o.CoreMode == "shared" {
		if o.Interface != "" {
			args = append(args, "--interface", o.Interface)
		}
	}
	return append(args, "--json")
}

// RunProbe 调用 sing-box 内置的 eBPF 内核能力检查。
func RunProbe(ctx context.Context, singBoxPath string, config option.EBPFInboundOptions, requestedMode string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	options, err := ResolveProbeOptions(config, requestedMode)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(singBoxPath) == "" {
		return "", fmt.Errorf("sing-box 路径为空")
	}
	command := exec.CommandContext(ctx, singBoxPath, options.Args()...)
	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	if ctx.Err() != nil {
		return string(output), ctx.Err()
	}
	if err != nil && strings.TrimSpace(stderr.String()) != "" {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(output), err
}

// ProbeReport 是 sing-box tools ebpf status --json 的稳定报告结构。
type ProbeReport struct {
	Platform        string         `json:"platform"`
	KernelRelease   string         `json:"kernel_release"`
	Architecture    string         `json:"architecture"`
	Mode            string         `json:"mode"`
	LocalDataPlane  string         `json:"local_data_plane,omitempty"`
	SharedDataPlane string         `json:"shared_data_plane,omitempty"`
	Network         []string       `json:"network"`
	IPv6            bool           `json:"ipv6"`
	Findings        []ProbeFinding `json:"findings"`
	Preflight       bool           `json:"preflight"`
	ExactObjectLoad bool           `json:"exact_object_load"`
	Summary         ProbeSummary   `json:"summary"`
	Result          string         `json:"result"`
}

// ProbeFinding 是单项 eBPF 能力检测结果。
type ProbeFinding struct {
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	Scope      string `json:"scope"`
	Importance string `json:"importance"`
	Feature    string `json:"feature"`
	Detail     string `json:"detail"`
}

// ProbeSummary 是 eBPF 能力检测的统计结果。
type ProbeSummary struct {
	Pass             int `json:"pass"`
	Warn             int `json:"warn"`
	Fail             int `json:"fail"`
	Unknown          int `json:"unknown"`
	RequiredFailures int `json:"required_failures"`
	RequiredUnknowns int `json:"required_unknowns"`
	RequiredIssues   int `json:"required_issues"`
}

// ParseProbeReport 解析并检查 sing-box JSON 诊断报告。
func ParseProbeReport(raw string) (ProbeReport, error) {
	var report ProbeReport
	if strings.TrimSpace(raw) == "" {
		return ProbeReport{}, errors.New("sing-box 未返回 eBPF JSON 诊断报告")
	}
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		return ProbeReport{}, fmt.Errorf("解析 sing-box eBPF JSON 诊断报告失败: %w", err)
	}
	if report.Mode != "all" && report.Mode != "local" && report.Mode != "shared" {
		return ProbeReport{}, fmt.Errorf("sing-box eBPF JSON 诊断报告模式无效: %q", report.Mode)
	}
	if (report.Mode == "all" || report.Mode == "local") && report.LocalDataPlane != "cgroup" && report.LocalDataPlane != "tc" {
		return ProbeReport{}, fmt.Errorf("sing-box eBPF JSON 诊断报告本机数据平面无效: %q", report.LocalDataPlane)
	}
	if (report.Mode == "all" || report.Mode == "shared") && report.SharedDataPlane != "packet_rewrite" && report.SharedDataPlane != "socket_assign" {
		return ProbeReport{}, fmt.Errorf("sing-box eBPF JSON 诊断报告共享数据平面无效: %q", report.SharedDataPlane)
	}
	if !report.Preflight {
		return ProbeReport{}, errors.New("sing-box 未返回 eBPF 能力预检报告")
	}
	if report.Result != "preflight_passed" && report.Result != "inconclusive" && report.Result != "unsupported" {
		return ProbeReport{}, fmt.Errorf("sing-box eBPF JSON 诊断报告结论无效: %q", report.Result)
	}
	for _, finding := range report.Findings {
		switch finding.Status {
		case "PASS", "WARN", "FAIL", "UNKNOWN":
		default:
			return ProbeReport{}, fmt.Errorf("sing-box eBPF 检查项状态无效: %q", finding.Status)
		}
	}
	return report, nil
}

// FormatProbeOutput 将结构化 eBPF 检测报告整理为用户可读的中文说明。
func FormatProbeOutput(report ProbeReport, probeErr error) string {
	conclusion := "eBPF 能力预检通过"
	switch {
	case report.Result == "unsupported" || report.Summary.RequiredFailures > 0:
		conclusion = "当前检测范围缺少必要的 eBPF 能力"
	case report.Result == "inconclusive" || report.Summary.RequiredUnknowns > 0:
		conclusion = "部分必要检查无法确认，尚不能判断是否可用"
	case probeErr != nil:
		conclusion = "诊断命令未正常完成，不能确认预检通过"
	case report.Summary.Warn+report.Summary.Fail+report.Summary.Unknown > 0:
		conclusion = "eBPF 能力预检通过，但有注意事项"
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "结论: %s\n", conclusion)
	fmt.Fprintf(&builder, "检测范围: %s\n", probeScope(report.Mode))
	if report.KernelRelease != "" {
		fmt.Fprintf(&builder, "内核版本: %s\n", report.KernelRelease)
	}
	if report.Architecture != "" {
		fmt.Fprintf(&builder, "设备架构: %s\n", report.Architecture)
	}
	planeNames := map[string]string{
		"cgroup": "应用套接字（cgroup）", "tc": "网卡流量（TC）",
		"packet_rewrite": "数据包重写", "socket_assign": "套接字分配",
	}
	if report.LocalDataPlane != "" {
		fmt.Fprintf(&builder, "本机接管方式: %s\n", planeNames[report.LocalDataPlane])
	}
	if report.SharedDataPlane != "" {
		fmt.Fprintf(&builder, "共享网络接管方式: %s\n", planeNames[report.SharedDataPlane])
	}
	fmt.Fprintf(&builder, "网络协议: %s\n", strings.ToUpper(strings.Join(report.Network, "/")))
	ipv6 := "不检测"
	if report.IPv6 {
		ipv6 = "检测"
	}
	fmt.Fprintf(&builder, "IPv6: %s\n", ipv6)
	fmt.Fprintf(&builder, "\n检查统计: 通过 %d 项，警告 %d 项，失败 %d 项，未确认 %d 项\n",
		report.Summary.Pass, report.Summary.Warn, report.Summary.Fail, report.Summary.Unknown)

	statusNames := map[string]string{"WARN": "警告", "FAIL": "失败", "UNKNOWN": "未确认"}
	reasons := map[string]string{
		"not_permitted":           "权限不足，请检查 Root 授权和系统安全策略。",
		"unsupported":             "内核不支持所需能力，请核对内核配置或更换支持的内核。",
		"temporarily_unavailable": "所需接口或资源暂未就绪，请就绪后重新检测。",
		"verifier_rejected":       "内核拒绝加载 eBPF 程序，请结合核心日志排查。",
		"attach_conflict":         "存在挂载或资源冲突，请检查其他接管程序。",
	}
	for _, finding := range report.Findings {
		if finding.Status == "PASS" {
			continue
		}
		importance := "必要能力"
		if finding.Importance == "performance" {
			importance = "可选性能优化"
		}
		fmt.Fprintf(&builder, "\n%s · %s · %s\n%s\n", statusNames[finding.Status], probeScope(finding.Scope), importance, finding.Feature)
		if reason := reasons[finding.Reason]; reason != "" {
			fmt.Fprintln(&builder, reason)
		}
		if finding.Detail != "" {
			fmt.Fprintln(&builder, finding.Detail)
		}
	}
	if probeErr != nil && report.Result == "preflight_passed" {
		fmt.Fprintf(&builder, "\n诊断命令错误: %s\n", probeErr)
	}
	if report.ExactObjectLoad {
		builder.WriteString("\n已检查所选 eBPF 程序能否加载，尚未验证实际挂载与网络接管。\n")
	} else {
		builder.WriteString("\n本次仅检查基础能力，程序加载、实际挂载与网络接管仍需启动服务验证。\n")
	}
	return strings.TrimSpace(builder.String())
}

func probeScope(scope string) string {
	switch scope {
	case "common":
		return "基础能力"
	case "local":
		return "本机应用流量"
	case "shared":
		return "热点与共享网络"
	case "all":
		return "本机应用流量、热点与共享网络"
	case "tc":
		return "网卡接管（TC）"
	case "icmp_echo_reply":
		return "ICMP 应答"
	default:
		return scope
	}
}
