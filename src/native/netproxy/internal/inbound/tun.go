package inbound

import (
	"time"

	"github.com/sagernet/sing-box/option"
)

func validateTUN(native option.TunInboundOptions) error {
	if !native.AutoRoute || !native.AutoRedirect {
		return validationError("tun.auto_route_required", "tun", "受管 TUN 必须启用 auto_route 和 auto_redirect")
	}
	if len(native.Address) == 0 {
		return validationError("tun.address_required", "tun.address", "TUN 至少需要一个有效 address 前缀")
	}
	for _, prefix := range native.Address {
		if !prefix.IsValid() || prefix.Addr().IsUnspecified() || prefix.Addr().IsMulticast() {
			return validationError("tun.address_invalid", "tun.address", "TUN address 必须是有效的单播地址前缀")
		}
	}
	if native.DNSMode != "" && native.DNSMode != "disabled" && native.DNSMode != "native" && native.DNSMode != "hijack" {
		return validationError("tun.dns_mode_invalid", "tun.dns_mode", "TUN DNS 模式只能是 disabled、native 或 hijack")
	}
	if time.Duration(native.UDPTimeout) < 0 {
		return validationError("tun.udp_timeout_invalid", "tun.udp_timeout", "UDP 会话超时不能为负数")
	}
	return validateUIDFilters(native.IncludeUID, native.IncludeUIDRange, native.ExcludeUID, native.ExcludeUIDRange)
}

func normalizeTUN(native option.TunInboundOptions) (option.TunInboundOptions, error) {
	var err error
	native.IncludeUID, native.IncludeUIDRange, err = normalizeUIDFilters(native.IncludeUID, native.IncludeUIDRange)
	if err != nil {
		return option.TunInboundOptions{}, err
	}
	native.ExcludeUID, native.ExcludeUIDRange, err = normalizeUIDFilters(native.ExcludeUID, native.ExcludeUIDRange)
	return native, err
}
