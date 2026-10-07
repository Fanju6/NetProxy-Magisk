package inbound

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/sagernet/sing-box/option"
)

func validateEBPF(native option.EBPFInboundOptions) error {
	local, shared := native.EffectiveEnablement()
	if !local && !shared {
		return validationError("ebpf.path_required", "ebpf", "本机或共享网络至少需要启用一条 eBPF 数据路径")
	}
	if time.Duration(native.UDPTimeout) < 0 {
		return validationError("ebpf.udp_timeout_invalid", "ebpf.udp_timeout", "UDP 会话超时不能为负数")
	}
	if native.FakeIPICMP != "" && native.FakeIPICMP != "off" && native.FakeIPICMP != "reply" {
		return validationError("ebpf.fakeip_icmp_invalid", "ebpf.fakeip_icmp", "FakeIP ICMP 模式只能是 off 或 reply")
	}
	{
		plane := localDataPlane(native.Local.DataPlane)
		if plane != "cgroup" && plane != "tc" {
			return validationError("ebpf.local_data_plane_invalid", "ebpf.local.data_plane", "本机数据平面只能是 cgroup 或 tc")
		}
		if native.Local.CgroupPath != "" && (plane != "cgroup" || !strings.HasPrefix(native.Local.CgroupPath, "/")) {
			return validationError("ebpf.local_cgroup_path_invalid", "ebpf.local.cgroup_path", "本机 cgroup 路径必须为绝对路径且使用 cgroup 数据平面")
		}
		if err := validateEBPFDNS(native.Local.DNSMode, "ebpf.local.dns_mode"); err != nil {
			return err
		}
		if err := validateUIDFilters(native.Local.IncludeUID, native.Local.IncludeUIDRange, native.Local.ExcludeUID, native.Local.ExcludeUIDRange); err != nil {
			return err
		}
		if err := validateBypassPorts(native.Local.BypassPort, native.Local.BypassPortRange, "ebpf.local"); err != nil {
			return err
		}
	}
	{
		plane := sharedDataPlane(native.Shared.DataPlane)
		if plane != "packet_rewrite" && plane != "socket_assign" {
			return validationError("ebpf.shared_data_plane_invalid", "ebpf.shared.data_plane", "共享网络数据平面只能是 packet_rewrite 或 socket_assign")
		}
		if shared && len(native.Shared.Interface) == 0 {
			return validationError("ebpf.shared_interface_required", "ebpf.shared.interface", "启用共享网络时至少需要一个下游接口")
		}
		for _, name := range native.Shared.Interface {
			if strings.TrimSpace(name) == "" {
				return validationError("ebpf.shared_interface_required", "ebpf.shared.interface", "下游接口名称不能为空")
			}
		}
		if err := validateEBPFDNS(native.Shared.DNSMode, "ebpf.shared.dns_mode"); err != nil {
			return err
		}
		if err := validateBypassPorts(native.Shared.BypassPort, native.Shared.BypassPortRange, "ebpf.shared"); err != nil {
			return err
		}
		for _, addresses := range [][]string{native.Shared.IncludeMACAddress, native.Shared.ExcludeMACAddress} {
			for _, value := range addresses {
				mac, err := net.ParseMAC(value)
				if err != nil || len(mac) != 6 {
					return validationError("ebpf.mac_invalid", "ebpf.shared", "MAC 筛选必须是 EUI-48 地址")
				}
			}
		}
	}
	return nil
}

func validateBypassPorts(ports []uint16, ranges []string, field string) error {
	for _, port := range ports {
		if port == 0 {
			return validationError("ebpf.port_invalid", field+".bypass_port", "绕过端口必须是 1 到 65535 之间的整数")
		}
	}
	for _, value := range ranges {
		start, end, found := strings.Cut(value, ":")
		first, firstErr := strconv.ParseUint(start, 10, 16)
		last, lastErr := strconv.ParseUint(end, 10, 16)
		if !found || firstErr != nil || lastErr != nil || first == 0 || first > last {
			return validationError("ebpf.port_range_invalid", field+".bypass_port_range", "绕过端口范围必须是 1 到 65535 之间的 start:end")
		}
	}
	return nil
}

func validateEBPFDNS(mode, field string) error {
	if mode != "" && mode != "hijack" && mode != "respect_policy" && mode != "off" {
		return validationError("ebpf.dns_mode_invalid", field, "eBPF DNS 模式只能是 hijack、respect_policy 或 off")
	}
	return nil
}

func localDataPlane(value string) string {
	if value == "" {
		return "cgroup"
	}
	return value
}

func sharedDataPlane(value string) string {
	if value == "" {
		return "packet_rewrite"
	}
	return value
}

func enabledByDefault(value *bool) bool { return value == nil || *value }

func normalizeEBPF(native option.EBPFInboundOptions) (option.EBPFInboundOptions, error) {
	local, shared := native.EffectiveEnablement()
	if local {
		native.Local.Enabled = new(true)
		var err error
		native.Local.IncludeUID, native.Local.IncludeUIDRange, err = normalizeUIDFilters(native.Local.IncludeUID, native.Local.IncludeUIDRange)
		if err != nil {
			return option.EBPFInboundOptions{}, err
		}
		native.Local.ExcludeUID, native.Local.ExcludeUIDRange, err = normalizeUIDFilters(native.Local.ExcludeUID, native.Local.ExcludeUIDRange)
		if err != nil {
			return option.EBPFInboundOptions{}, err
		}
	} else {
		native.Local = option.EBPFLocalOptions{Enabled: new(false)}
	}
	if shared {
		native.Shared.Enabled = new(true)
	} else {
		native.Shared = option.EBPFSharedOptions{Enabled: new(false)}
	}
	return native, nil
}
