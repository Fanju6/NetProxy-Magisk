package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
)

func (c *cli) network(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 network 操作: evaluate | wifi-list")
	}
	if args[0] == "wifi-list" {
		if len(args) != 1 {
			return errors.New("network wifi-list 不接受参数")
		}
		ssids, err := worker.SavedWiFiNetworks(ctx)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "network.wifi_list", Message: "已保存 Wi-Fi", Data: map[string]any{"ssids": ssids}})
		return nil
	}
	flags := newFlagSet("network")
	networkType := flags.String("type", "not_wifi", "网络类型")
	ssid := flags.String("ssid", "", "当前 WiFi SSID")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("network evaluate 不接受位置参数")
	}
	if args[0] != "evaluate" {
		return fmt.Errorf("未知 network 操作 %q", args[0])
	}
	data, err := moduleapp.EvaluateNetwork(ctx, c.options, *networkType, *ssid)
	if err != nil {
		return err
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "network.evaluated", Message: data.Reason, Data: data})
	return nil
}
