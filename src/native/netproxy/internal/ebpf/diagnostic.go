package ebpf

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
)

// ProbeOptions 描述 sing-box eBPF 能力检查所需的运行参数。
type ProbeOptions struct {
	RequestedMode string
	CoreMode      string
	CgroupPath    string
	Interface     string
}

// ResolveProbeOptions 根据当前 eBPF 配置解析能力检查范围，不执行配置校验。
func ResolveProbeOptions(path, requestedMode string) (ProbeOptions, error) {
	values, err := moduleconfig.ReadStrict(path)
	if err != nil {
		return ProbeOptions{}, fmt.Errorf("读取 eBPF 配置失败: %w", err)
	}

	requested := strings.ToLower(strings.TrimSpace(requestedMode))
	if requested == "" {
		requested = "configured"
	}
	coreMode := requested
	if requested == "configured" {
		cgroupEnabled := enabledValue(values["EBPF_CGROUP_ENABLED"])
		sharedEnabled := enabledValue(values["EBPF_SHARED_NETWORK"])
		switch {
		case cgroupEnabled && sharedEnabled:
			coreMode = "all"
		case sharedEnabled:
			coreMode = "shared-network"
		default:
			coreMode = "local"
		}
	} else if requested == "shared" {
		coreMode = "shared-network"
	}

	switch coreMode {
	case "all", "local", "shared-network":
	default:
		return ProbeOptions{}, fmt.Errorf("eBPF 检查范围无效: %s", requestedMode)
	}

	interfaces := CommaSeparated(values["EBPF_SHARED_INTERFACES"])
	interfaceName := ""
	if len(interfaces) > 0 {
		interfaceName = interfaces[0]
	}
	return ProbeOptions{
		RequestedMode: requested,
		CoreMode:      coreMode,
		CgroupPath:    strings.TrimSpace(values["EBPF_CGROUP_PATH"]),
		Interface:     interfaceName,
	}, nil
}

// Args 返回 sing-box tools ebpf status 的参数。
func (o ProbeOptions) Args() []string {
	args := []string{"tools", "ebpf", "status", "--mode", o.CoreMode}
	if o.CoreMode == "all" || o.CoreMode == "local" {
		if o.CgroupPath != "" {
			args = append(args, "--cgroup", o.CgroupPath)
		}
	}
	if o.CoreMode == "all" || o.CoreMode == "shared-network" {
		if o.Interface != "" {
			args = append(args, "--interface", o.Interface)
		}
	}
	return args
}

// RunProbe 调用 sing-box 内置的 eBPF 内核能力检查。
func RunProbe(ctx context.Context, singBoxPath string, options ProbeOptions) (string, error) {
	if strings.TrimSpace(singBoxPath) == "" {
		return "", fmt.Errorf("sing-box 路径为空")
	}
	command := exec.CommandContext(ctx, singBoxPath, options.Args()...)
	output, err := command.CombinedOutput()
	return string(output), err
}

// FormatProbeOutput 将 sing-box 的能力检查结果整理为用户可读的中文说明。
func FormatProbeOutput(raw, coreMode string, probeErr error) string {
	passCount, warnCount, failCount, unknownCount := parseSummary(raw)
	kernel, architecture := parsePlatform(raw)
	scope := map[string]string{
		"local":          "本机应用流量",
		"shared-network": "热点与共享网络",
		"all":            "本机应用流量、热点与共享网络",
	}[coreMode]
	if scope == "" {
		scope = coreMode
	}

	conclusion := "未发现明确问题，启动服务后可完成最终验证"
	if probeErr != nil || failCount > 0 {
		conclusion = "未通过"
	} else if warnCount > 0 {
		conclusion = "发现兼容性警告，建议启动服务进行最终验证"
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "结论: %s\n", conclusion)
	fmt.Fprintf(&builder, "检测范围: %s\n", scope)
	if kernel != "" {
		fmt.Fprintf(&builder, "内核版本: %s\n", kernel)
	}
	if architecture != "" {
		fmt.Fprintf(&builder, "设备架构: %s\n", architecture)
	}
	builder.WriteString("\n检查统计:\n")
	fmt.Fprintf(&builder, "  通过: %d 项\n", passCount)
	fmt.Fprintf(&builder, "  警告: %d 项\n", warnCount)
	fmt.Fprintf(&builder, "  失败: %d 项\n", failCount)
	fmt.Fprintf(&builder, "  无法静态确认: %d 项\n", unknownCount)

	commonFail := strings.Count(raw, "[common]")
	localFail := strings.Count(raw, "[local]")
	sharedFail := strings.Count(raw, "[shared-network]")
	if failCount > 0 || probeErr != nil {
		builder.WriteString("\n问题定位:\n")
		if commonFail > 0 {
			builder.WriteString("  - 基础 eBPF 权限或内核能力不满足。\n")
		}
		if localFail > 0 {
			builder.WriteString("  - 本机 cgroup eBPF 能力不满足。\n")
		}
		if sharedFail > 0 {
			builder.WriteString("  - 热点接口或 TC eBPF 能力不满足。\n")
		}
		if commonFail == 0 && localFail == 0 && sharedFail == 0 {
			builder.WriteString("  - sing-box 未能完成 eBPF 能力检查，请查看服务日志。\n")
		}
		builder.WriteString("\n建议先检查 Root 授权、内核 eBPF 配置和服务日志。\n")
	} else if unknownCount > 0 {
		builder.WriteString("\n说明:\n")
		builder.WriteString("  “无法静态确认”不代表失败，部分能力只能在 sing-box 实际启动时验证。\n")
	} else if coreMode == "local" {
		builder.WriteString("\n当前未启用共享网络，本次没有检测热点接口。\n")
	}

	if strings.TrimSpace(raw) == "" {
		builder.WriteString("\n检查程序没有返回详细结果。\n")
	}
	return strings.TrimSpace(builder.String())
}

var (
	probeSummaryPattern  = regexp.MustCompile(`(?m)^Summary: PASS=([0-9]+) WARN=([0-9]+) FAIL=([0-9]+) UNKNOWN=([0-9]+)$`)
	probePlatformPattern = regexp.MustCompile(`(?m)^Platform:.*kernel: ([^;]*);.*architecture: ([^;]*);`)
)

func parseSummary(raw string) (int, int, int, int) {
	match := probeSummaryPattern.FindStringSubmatch(raw)
	if len(match) != 5 {
		return 0, 0, 0, 0
	}
	values := [4]int{}
	for index := range values {
		values[index], _ = strconv.Atoi(match[index+1])
	}
	return values[0], values[1], values[2], values[3]
}

func parsePlatform(raw string) (string, string) {
	match := probePlatformPattern.FindStringSubmatch(raw)
	if len(match) != 3 {
		return "", ""
	}
	return strings.TrimSpace(match[1]), strings.TrimSpace(match[2])
}

func enabledValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
