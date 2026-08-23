//go:build linux || android

package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sys/unix"
)

// defaultNetworkEventSource 使用 RTNETLINK 监听路由、地址和链路变化。
func defaultNetworkEventSource(ctx context.Context, notify func()) error {
	netlinkFD, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.NETLINK_ROUTE)
	if err != nil {
		return fmt.Errorf("创建 netlink socket: %w", err)
	}
	defer unix.Close(netlinkFD)

	groups := uint32(unix.RTMGRP_LINK |
		unix.RTMGRP_IPV4_IFADDR |
		unix.RTMGRP_IPV6_IFADDR |
		unix.RTMGRP_IPV4_ROUTE |
		unix.RTMGRP_IPV6_ROUTE)
	if err := unix.Bind(netlinkFD, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Groups: groups}); err != nil {
		return fmt.Errorf("订阅 netlink 网络事件: %w", err)
	}

	cancelPipe := []int{0, 0}
	if err := unix.Pipe2(cancelPipe, unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		return fmt.Errorf("创建网络监听取消管道: %w", err)
	}
	defer unix.Close(cancelPipe[0])
	defer unix.Close(cancelPipe[1])

	stopCancel := make(chan struct{})
	var cancelWait sync.WaitGroup
	cancelWait.Add(1)
	go func() {
		defer cancelWait.Done()
		select {
		case <-ctx.Done():
			_, _ = unix.Write(cancelPipe[1], []byte{1})
		case <-stopCancel:
		}
	}()
	defer func() {
		close(stopCancel)
		cancelWait.Wait()
	}()

	pollFDs := []unix.PollFd{
		{Fd: int32(netlinkFD), Events: unix.POLLIN},
		{Fd: int32(cancelPipe[0]), Events: unix.POLLIN},
	}
	buffer := make([]byte, 64*1024)
	for {
		_, err := unix.Poll(pollFDs, -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("等待 netlink 网络事件: %w", err)
		}
		if pollFDs[1].Revents&unix.POLLIN != 0 {
			return nil
		}
		if pollFDs[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return errors.New("netlink 网络事件连接已关闭")
		}
		if pollFDs[0].Revents&unix.POLLIN == 0 {
			continue
		}
		for {
			n, _, receiveErr := unix.Recvfrom(netlinkFD, buffer, unix.MSG_DONTWAIT)
			if errors.Is(receiveErr, unix.EAGAIN) || errors.Is(receiveErr, unix.EWOULDBLOCK) {
				break
			}
			if errors.Is(receiveErr, unix.ENOBUFS) {
				// 即使事件队列溢出，也应重新读取完整网络状态。
				notify()
				break
			}
			if receiveErr != nil {
				return fmt.Errorf("读取 netlink 网络事件: %w", receiveErr)
			}
			if n > 0 {
				notify()
			}
		}
	}
}
