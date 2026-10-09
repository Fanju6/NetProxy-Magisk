//go:build linux || android

package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mdlayher/wifi"
)

func readWiFiNetwork(ctx context.Context, active string) (NetworkState, error) {
	state := NetworkState{NetworkType: "not_wifi", ActiveInterface: active}
	// 内核的无线接口标识不依赖厂商接口名称，热点仍需按 nl80211 类型区分。
	if _, err := os.Stat(filepath.Join("/sys/class/net", active, "phy80211")); err != nil {
		if os.IsNotExist(err) {
			return state, ctx.Err()
		}
		return NetworkState{}, fmt.Errorf("读取出口接口类型失败: %w", err)
	}
	client, err := wifi.New()
	if err != nil {
		return NetworkState{}, fmt.Errorf("打开 Wi-Fi 状态接口失败: %w", err)
	}
	defer client.Close()
	stop := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stop()
	deadline := time.Now().Add(networkCommandTimeout)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if err := client.SetDeadline(deadline); err != nil {
		return NetworkState{}, err
	}
	interfaces, err := client.Interfaces()
	if ctx.Err() != nil {
		return NetworkState{}, ctx.Err()
	}
	if err != nil {
		return NetworkState{}, fmt.Errorf("读取 Wi-Fi 接口失败: %w", err)
	}
	for _, iface := range interfaces {
		if iface.Name != active {
			continue
		}
		if iface.Type != wifi.InterfaceTypeStation {
			return state, nil
		}
		bss, err := client.BSS(iface)
		if ctx.Err() != nil {
			return NetworkState{}, ctx.Err()
		}
		if errors.Is(err, os.ErrNotExist) {
			return NetworkState{}, networkUnavailable("无线出口尚未关联")
		}
		if err != nil {
			return NetworkState{}, fmt.Errorf("读取实际出口 Wi-Fi 名称失败: %w", err)
		}
		return NetworkState{NetworkType: "wifi", SSID: bss.SSID, ActiveInterface: active}, nil
	}
	return NetworkState{}, networkUnavailable("无线出口接口尚不可读")
}
