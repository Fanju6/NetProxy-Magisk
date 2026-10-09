//go:build linux || android

package worker

import (
	"context"
	"net"
	"sort"
	"time"

	"github.com/sagernet/netlink"
)

// readActiveNetworkInterface 按 Android policy routing 选择实际默认出口。
func readActiveNetworkInterface(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	handle, err := netlink.NewHandle()
	if err != nil {
		return "", err
	}
	defer handle.Delete()
	timeout := networkCommandTimeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(deadline))
	}
	if err := handle.SetSocketTimeout(timeout); err != nil {
		return "", err
	}
	rules, err := handle.RuleList(netlink.FAMILY_ALL)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", networkUnavailable("读取 Android 路由规则失败: %v", err)
	}
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })
	for _, rule := range rules {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if rule.Mask != 0xFFFF || rule.Mark != 0 || rule.Table <= 0 {
			continue
		}
		routes, err := handle.RouteListFiltered(
			netlink.FAMILY_ALL,
			&netlink.Route{Table: rule.Table},
			netlink.RT_FILTER_TABLE,
		)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if err != nil {
			return "", networkUnavailable("读取 Android 默认路由失败: %v", err)
		}
		for _, route := range routes {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if route.LinkIndex <= 0 || route.Dst != nil && !isDefaultRoute(route.Dst) {
				continue
			}
			link, linkErr := handle.LinkByIndex(route.LinkIndex)
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if linkErr != nil {
				continue
			}
			attributes := link.Attrs()
			if attributes == nil || attributes.Name == "" || attributes.Flags&net.FlagUp == 0 || link.Type() == "tun" || attributes.Name == "lo" {
				continue
			}
			return attributes.Name, nil
		}
	}
	return "", networkUnavailable("默认路由表中没有可用网络接口")
}

func isDefaultRoute(destination *net.IPNet) bool {
	ones, _ := destination.Mask.Size()
	return ones == 0
}
