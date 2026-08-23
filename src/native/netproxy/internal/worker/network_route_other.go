//go:build !linux && !android

package worker

import "context"

func readActiveNetworkInterface(context.Context) (string, error) {
	return "", networkUnavailable("当前平台不支持 Android 路由读取")
}
