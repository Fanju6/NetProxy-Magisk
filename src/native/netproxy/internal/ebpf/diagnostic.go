package ebpf

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	json "encoding/json/v2"
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
func ResolveProbeOptions(path, requestedMode string) (ProbeOptions, error) {
	config, err := Load(path)
	if err != nil {
		return ProbeOptions{}, fmt.Errorf("读取 eBPF 配置失败: %w", err)
	}

	requested := strings.ToLower(strings.TrimSpace(requestedMode))
	if requested == "" {
		requested = "configured"
	}
	coreMode := requested
	if requested == "configured" {
		switch {
		case config.Local.Enabled && config.Shared.Enabled:
			coreMode = "all"
		case config.Shared.Enabled:
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

	network := append([]string{}, config.Network...)
	if len(network) == 0 {
		network = []string{"tcp", "udp"}
	}
	ipv6 := config.Local.IPv6
	if coreMode == "shared" {
		ipv6 = config.Shared.IPv6
	} else if coreMode == "all" {
		ipv6 = config.Local.IPv6 || config.Shared.IPv6
	}
	interfaceName := ""
	if len(config.Shared.Interfaces) > 0 {
		interfaceName = config.Shared.Interfaces[0]
	}
	return ProbeOptions{
		RequestedMode:   requested,
		CoreMode:        coreMode,
		LocalDataPlane:  config.Local.DataPlane,
		SharedDataPlane: config.Shared.DataPlane,
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
func RunProbe(ctx context.Context, singBoxPath string, options ProbeOptions) (string, error) {
	if strings.TrimSpace(singBoxPath) == "" {
		return "", fmt.Errorf("sing-box 路径为空")
	}
	command := exec.CommandContext(ctx, singBoxPath, options.Args()...)
	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil && strings.TrimSpace(stderr.String()) != "" {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(output), err
}

// ProbeReport 是 sing-box tools ebpf status --json 的稳定报告结构。
type ProbeReport struct {
	Platform         string         `json:"platform"`
	KernelRelease    string         `json:"kernel_release"`
	Architecture     string         `json:"architecture"`
	Mode             string         `json:"mode"`
	LocalDataPlane   string         `json:"local_data_plane,omitempty"`
	SharedDataPlane  string         `json:"shared_data_plane,omitempty"`
	Network          []string       `json:"network"`
	IPv6             bool           `json:"ipv6"`
	Findings         []ProbeFinding `json:"findings"`
	ActivePrograms   []ProbeProgram `json:"active_programs"`
	ActiveStateError string         `json:"active_state_error,omitempty"`
	Summary          ProbeSummary   `json:"summary"`
	Result           string         `json:"result"`
}

// ProbeFinding 是单项 eBPF 能力检测结果。
type ProbeFinding struct {
	Status     string `json:"status"`
	Scope      string `json:"scope"`
	Importance string `json:"importance"`
	Feature    string `json:"feature"`
	Detail     string `json:"detail"`
}

// ProbeProgram 是内核中当前可见的 sing-box eBPF 程序摘要。
type ProbeProgram struct {
	ID       uint32 `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	MapCount int    `json:"map_count"`
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
	if report.Result != "supported" && report.Result != "inconclusive" && report.Result != "unsupported" {
		return ProbeReport{}, fmt.Errorf("sing-box eBPF JSON 诊断报告结论无效: %q", report.Result)
	}
	return report, nil
}

// FormatProbeOutput 将结构化 eBPF 检测报告整理为用户可读的中文说明。
func FormatProbeOutput(report ProbeReport, probeErr error) string {
	coreMode := report.Mode
	scope := map[string]string{
		"local":  "本机应用流量",
		"shared": "热点与共享网络",
		"all":    "本机应用流量、热点与共享网络",
	}[coreMode]
	if scope == "" {
		scope = coreMode
	}

	conclusion := "检测通过"
	switch {
	case report.Result == "unsupported" || report.Summary.RequiredFailures > 0 || probeErr != nil:
		conclusion = "未通过"
	case report.Result == "inconclusive":
		conclusion = "无法完全确认，启动服务后可完成最终验证"
	case report.Summary.Warn > 0:
		conclusion = "发现兼容性警告，建议启动服务进行最终验证"
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "结论: %s\n", conclusion)
	fmt.Fprintf(&builder, "检测范围: %s\n", scope)
	if report.KernelRelease != "" {
		fmt.Fprintf(&builder, "内核版本: %s\n", report.KernelRelease)
	}
	if report.Architecture != "" {
		fmt.Fprintf(&builder, "设备架构: %s\n", report.Architecture)
	}
	if report.LocalDataPlane != "" {
		fmt.Fprintf(&builder, "本机数据平面: %s\n", report.LocalDataPlane)
	}
	if report.SharedDataPlane != "" {
		fmt.Fprintf(&builder, "共享数据平面: %s\n", report.SharedDataPlane)
	}
	builder.WriteString("\n检查统计:\n")
	fmt.Fprintf(&builder, "  通过: %d 项\n", report.Summary.Pass)
	fmt.Fprintf(&builder, "  警告: %d 项\n", report.Summary.Warn)
	fmt.Fprintf(&builder, "  失败: %d 项\n", report.Summary.Fail)
	fmt.Fprintf(&builder, "  无法静态确认: %d 项\n", report.Summary.Unknown)

	commonFail := hasFailedScope(report.Findings, "common")
	localFail := hasFailedScope(report.Findings, "local")
	sharedFail := hasFailedScope(report.Findings, "shared")
	if report.Summary.Fail > 0 || probeErr != nil {
		builder.WriteString("\n问题定位:\n")
		if commonFail {
			builder.WriteString("  - 基础 eBPF 权限或内核能力不满足。\n")
		}
		if localFail {
			builder.WriteString("  - 本机 cgroup/TC 数据路径能力不满足。\n")
		}
		if sharedFail {
			builder.WriteString("  - 热点接口或 TC eBPF 能力不满足。\n")
		}
		if !commonFail && !localFail && !sharedFail {
			builder.WriteString("  - sing-box 未能完成 eBPF 能力检查，请查看服务日志。\n")
		}
		builder.WriteString("\n建议先检查 Root 授权、内核 eBPF 配置和服务日志。\n")
	} else if report.Summary.Unknown > 0 {
		builder.WriteString("\n说明:\n")
		builder.WriteString("  “无法静态确认”不代表失败，部分能力只能在 sing-box 实际启动时验证。\n")
	} else if coreMode == "local" {
		builder.WriteString("\n当前未启用共享网络，本次没有检测热点接口。\n")
	}

	if len(report.ActivePrograms) > 0 {
		fmt.Fprintf(&builder, "\n当前可见 sing-box eBPF 程序: %d 个。\n", len(report.ActivePrograms))
	}
	if report.ActiveStateError != "" {
		builder.WriteString("\n无法确认当前已挂载的 eBPF 程序状态。\n")
	}
	return strings.TrimSpace(builder.String())
}

func hasFailedScope(findings []ProbeFinding, scope string) bool {
	for _, finding := range findings {
		if finding.Status == "FAIL" && finding.Scope == scope {
			return true
		}
	}
	return false
}
