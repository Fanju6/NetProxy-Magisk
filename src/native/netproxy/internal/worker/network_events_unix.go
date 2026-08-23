//go:build linux || android

package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/sagernet/netlink"
)

// defaultNetworkEventSource 使用与 sing-tun 相同的 netlink 订阅监听网络变化。
func defaultNetworkEventSource(ctx context.Context, notify func()) error {
	routeUpdates := make(chan netlink.RouteUpdate, 2)
	linkUpdates := make(chan netlink.LinkUpdate, 2)
	addressUpdates := make(chan netlink.AddrUpdate, 2)
	closed := make(chan struct{})
	defer close(closed)
	if err := netlink.RouteSubscribe(routeUpdates, closed); err != nil {
		return fmt.Errorf("订阅路由变化: %w", err)
	}
	if err := netlink.LinkSubscribe(linkUpdates, closed); err != nil {
		return fmt.Errorf("订阅链路变化: %w", err)
	}
	if err := netlink.AddrSubscribe(addressUpdates, closed); err != nil {
		return fmt.Errorf("订阅地址变化: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, open := <-routeUpdates:
			if !open {
				return errors.New("路由事件监听已关闭")
			}
			notify()
		case _, open := <-linkUpdates:
			if !open {
				return errors.New("链路事件监听已关闭")
			}
			notify()
		case _, open := <-addressUpdates:
			if !open {
				return errors.New("地址事件监听已关闭")
			}
			notify()
		}
	}
}
