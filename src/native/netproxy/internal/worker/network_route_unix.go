//go:build linux || android

package worker

import (
	"context"
	"net"

	"github.com/sagernet/netlink"
)

// readActiveNetworkInterface 按 Android policy routing 选择实际默认出口。
func readActiveNetworkInterface(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	rules, err := netlink.RuleList(netlink.FAMILY_ALL)
	if err != nil {
		return "", networkUnavailable("读取 Android 路由规则失败: %v", err)
	}
	defaultTable := 0
	for _, rule := range rules {
		if rule.Mask == 0xFFFF {
			defaultTable = rule.Table
			break
		}
	}
	if defaultTable == 0 {
		return "", networkUnavailable("未找到 Android 默认路由表")
	}
	routes, err := netlink.RouteListFiltered(
		netlink.FAMILY_ALL,
		&netlink.Route{Table: defaultTable},
		netlink.RT_FILTER_TABLE,
	)
	if err != nil {
		return "", networkUnavailable("读取 Android 默认路由失败: %v", err)
	}
	for _, route := range routes {
		if route.LinkIndex <= 0 {
			continue
		}
		link, linkErr := netlink.LinkByIndex(route.LinkIndex)
		if linkErr != nil {
			continue
		}
		attributes := link.Attrs()
		if attributes == nil || attributes.Name == "" || attributes.Flags&net.FlagUp == 0 {
			continue
		}
		return attributes.Name, nil
	}
	return "", networkUnavailable("默认路由表中没有可用网络接口")
}
