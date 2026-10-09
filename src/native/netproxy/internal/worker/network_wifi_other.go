//go:build !linux && !android

package worker

import "context"

func readWiFiNetwork(ctx context.Context, active string) (NetworkState, error) {
	return NetworkState{}, networkUnavailable("当前平台不支持 Android Wi-Fi 状态")
}
