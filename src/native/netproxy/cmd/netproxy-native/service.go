package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/serviceapi"
)

func runService(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 Service API 操作")
	}
	action := args[0]
	flags := newFlagSet("service " + action)
	address := flags.String("address", "127.0.0.1:9090", "Service API 地址")
	secretValue := flags.String("secret", "", "Service API 密钥")
	timeout := flags.Duration("timeout", 8*time.Second, "请求超时")
	group := flags.String("group", "", "选择器标签")
	outbound := flags.String("outbound", "", "出站标签")
	mode := flags.String("mode", "", "出站模式")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	secret := strings.TrimSpace(*secretValue)
	requestContext, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	client, err := serviceapi.New(*address, secret)
	if err != nil {
		return err
	}
	defer client.Close()

	var data any
	switch action {
	case "ready":
		data, err = client.Ready(requestContext)
	case "started-at":
		data, err = client.StartedAt(requestContext)
	case "snapshot":
		status, statusErr := client.Status(requestContext)
		if statusErr != nil {
			err = statusErr
			break
		}
		groups, groupsErr := client.Groups(requestContext)
		if groupsErr != nil {
			err = groupsErr
			break
		}
		selected := ""
		targetGroup := *group
		if targetGroup == "" {
			targetGroup = "Proxy"
		}
		for _, item := range groups {
			if item.Tag == targetGroup {
				selected = item.Selected
				break
			}
		}
		data = serviceSnapshot{
			Memory:           status.Memory,
			Goroutines:       status.Goroutines,
			ConnectionsIn:    status.ConnectionsIn,
			ConnectionsOut:   status.ConnectionsOut,
			TrafficAvailable: status.TrafficAvailable,
			Uplink:           status.Uplink,
			Downlink:         status.Downlink,
			UplinkTotal:      status.UplinkTotal,
			DownlinkTotal:    status.DownlinkTotal,
			Selected:         selected,
		}
	case "groups":
		data, err = client.Groups(requestContext)
	case "mode":
		if *mode == "" {
			data, err = client.Mode(requestContext)
		} else {
			err = client.SetMode(requestContext, *mode)
			data = map[string]string{"mode": *mode}
		}
	case "select":
		if *group == "" || *outbound == "" {
			return errors.New("select 需要 --group 和 --outbound")
		}
		err = client.Select(requestContext, *group, *outbound)
		data = map[string]string{"group": *group, "outbound": *outbound}
	case "urltest":
		if *outbound == "" {
			return errors.New("urltest 需要 --outbound")
		}
		err = client.URLTest(requestContext, *outbound)
		data = map[string]string{"outbound": *outbound}
	case "close-all":
		err = client.CloseAllConnections(requestContext)
		data = map[string]bool{"closed": err == nil}
	default:
		return fmt.Errorf("未知 Service API 操作 %q", action)
	}
	if err != nil {
		return fmt.Errorf("Service API %s: %w", action, err)
	}
	if action == "snapshot" && *format == "tsv" {
		snapshot := data.(serviceSnapshot)
		fmt.Printf("selected\t%s\nmemory\t%d\nconnections_in\t%d\nconnections_out\t%d\nuplink_total\t%d\ndownlink_total\t%d\n",
			snapshot.Selected, snapshot.Memory, snapshot.ConnectionsIn, snapshot.ConnectionsOut,
			snapshot.UplinkTotal, snapshot.DownlinkTotal)
		return nil
	}
	if action == "started-at" && *format == "raw" {
		startedAt := data.(serviceapi.StartedAt)
		fmt.Printf("%d\n", startedAt.UnixMilli)
		return nil
	}
	if *format != "json" {
		return fmt.Errorf("操作 %s 不支持输出格式 %q", action, *format)
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "service." + action, Message: "Service API 操作完成", Data: data})
	return nil
}
