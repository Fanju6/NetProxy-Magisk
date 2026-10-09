//go:build linux || android

package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mdlayher/genetlink"
	"github.com/sagernet/netlink"
)

// defaultNetworkEventSource 使用与 sing-tun 相同的 netlink 订阅监听网络变化。
func defaultNetworkEventSource(ctx context.Context, notify func()) error {
	conn, err := genetlink.Dial(nil)
	if err != nil {
		return fmt.Errorf("打开 Wi-Fi 事件接口: %w", err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if err := conn.SetDeadline(time.Now().Add(networkCommandTimeout)); err != nil {
		return err
	}
	family, err := conn.GetFamily("nl80211")
	if err != nil {
		return fmt.Errorf("读取 Wi-Fi 事件族: %w", err)
	}
	groups := 0
	for _, group := range family.Groups {
		if group.Name == "mlme" || group.Name == "config" {
			if err := conn.JoinGroup(group.ID); err != nil {
				return err
			}
			groups++
		}
	}
	if groups == 0 {
		return errors.New("内核未提供 Wi-Fi 连接事件")
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return err
	}
	wifiEvents := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.Receive(); err != nil {
				select {
				case wifiEvents <- err:
				default:
				}
				return
			}
			notify()
		}
	}()
	defer func() { _ = conn.Close(); <-done }()
	routeUpdates := make(chan netlink.RouteUpdate, 2)
	linkUpdates := make(chan netlink.LinkUpdate, 2)
	addressUpdates := make(chan netlink.AddrUpdate, 2)
	closed := make(chan struct{})
	subscribedRoute, subscribedLink, subscribedAddress := false, false, false
	// 上游订阅向 channel 阻塞发送；关闭 socket 后仍需排空队列才能释放 goroutine。
	defer func() {
		close(closed)
		if subscribedRoute {
			for range routeUpdates {
			}
		}
		if subscribedLink {
			for range linkUpdates {
			}
		}
		if subscribedAddress {
			for range addressUpdates {
			}
		}
	}()
	receiveError := func(err error) {
		select {
		case wifiEvents <- err:
		default:
		}
	}
	if err := netlink.RouteSubscribeWithOptions(routeUpdates, closed, netlink.RouteSubscribeOptions{ErrorCallback: receiveError}); err != nil {
		return fmt.Errorf("订阅路由变化: %w", err)
	}
	subscribedRoute = true
	if err := netlink.LinkSubscribeWithOptions(linkUpdates, closed, netlink.LinkSubscribeOptions{ErrorCallback: receiveError}); err != nil {
		return fmt.Errorf("订阅链路变化: %w", err)
	}
	subscribedLink = true
	if err := netlink.AddrSubscribeWithOptions(addressUpdates, closed, netlink.AddrSubscribeOptions{ErrorCallback: receiveError}); err != nil {
		return fmt.Errorf("订阅地址变化: %w", err)
	}
	subscribedAddress = true
	notify()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-wifiEvents:
			return fmt.Errorf("网络事件监听失败: %w", err)
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
