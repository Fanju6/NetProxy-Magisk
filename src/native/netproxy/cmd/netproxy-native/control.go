package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func runControl(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少控制面操作: status|nodes|snapshot|selection|groups|mode|delay|close-all")
	}
	action := args[0]
	flags := newFlagSet("control " + action)
	moduleDir := flags.String("module-dir", defaultModuleDir(), "模块根目录")
	catalogRoot := flags.String("catalog-root", "", "Catalog 根目录")
	moduleConfig := flags.String("module-config", "", "模块配置文件")
	stateFile := flags.String("state-file", "", "服务状态文件")
	progressDir := flags.String("progress-dir", "/dev/netproxy/subscriptions", "订阅进度目录")
	workerPIDFile := flags.String("worker-pid-file", "/dev/netproxy/worker.pid", "订阅 Worker PID 文件")
	singBox := flags.String("sing-box", "", "sing-box 二进制路径")
	address := flags.String("address", "127.0.0.1:9090", "Service API 地址")
	secret := flags.String("secret", "singbox", "Service API 密钥")
	timeout := flags.Duration("timeout", 8*time.Second, "Service API 请求超时")
	target := flags.String("target", "", "测速目标")
	group := flags.String("group", "", "测速分组")
	mode := flags.String("mode", "", "出站模式")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*catalogRoot) == "" {
		*catalogRoot = filepath.Join(*moduleDir, "data", "catalog")
	}
	if strings.TrimSpace(*moduleConfig) == "" {
		*moduleConfig = filepath.Join(*moduleDir, "config", "module.conf")
	}
	if strings.TrimSpace(*stateFile) == "" {
		*stateFile = "/dev/netproxy/service.json"
	}
	if strings.TrimSpace(*progressDir) == "" {
		*progressDir = os.Getenv("SUB_RUNTIME_DIR")
		if *progressDir == "" {
			*progressDir = "/dev/netproxy/subscriptions"
		}
	}
	if strings.TrimSpace(*workerPIDFile) == "" {
		*workerPIDFile = "/dev/netproxy/subworker.pid"
	}
	if strings.TrimSpace(*singBox) == "" {
		*singBox = filepath.Join(*moduleDir, "bin", "sing-box")
	}
	options := service.Options{
		CatalogRoot: *catalogRoot, ModuleConfig: *moduleConfig, StateFile: *stateFile,
		ProgressDir: *progressDir, WorkerPIDFile: *workerPIDFile, SingBoxPath: *singBox,
		ServiceAddress: *address, ServiceSecret: *secret, RequestTimeout: *timeout,
	}
	switch action {
	case "status":
		status, err := service.ReadStatus(ctx, options)
		if err != nil {
			return err
		}
		if *format == "text" {
			fmt.Fprintf(os.Stdout, "服务状态: %s\n运行时间: %d 秒\n出站模式: %s\n活动分组: %s\n节点选择: %s\n订阅更新: %s\n",
				status.State, status.UptimeSeconds, status.OutboundMode, status.ActiveGroupName,
				status.RuntimeSelected, status.SubscriptionWorker)
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("control status 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "service.status", Message: "服务状态", Data: status})
		return nil
	case "groups":
		groups, err := service.ReadGroups(ctx, options)
		if err != nil {
			return err
		}
		if *format != "json" {
			return fmt.Errorf("control groups 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "service.groups", Message: "节点组状态", Data: groups})
		return nil
	case "nodes":
		groups, err := service.ReadNodes(ctx, options, *group)
		if err != nil {
			return err
		}
		if *format != "json" {
			return fmt.Errorf("control nodes 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.list", Message: "节点列表", Data: groups})
		return nil
	case "snapshot":
		snapshot, err := service.ReadSnapshot(ctx, options, *group)
		if err != nil {
			return err
		}
		if *format != "json" {
			return fmt.Errorf("control snapshot 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.snapshot", Message: "节点快照", Data: snapshot})
		return nil
	case "selection":
		selection, err := service.ReadSelection(ctx, options)
		if err != nil {
			return err
		}
		if *format != "json" {
			return fmt.Errorf("control selection 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.current", Message: "当前节点选择", Data: selection})
		return nil
	case "mode":
		state, err := service.ReadMode(ctx, options)
		if err != nil {
			return err
		}
		if *format == "text" {
			fmt.Fprintln(os.Stdout, state.Mode)
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("control mode 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "mode.current", Message: "当前出站模式", Data: state})
		return nil
	case "runtime-mode":
		runtimeMode, err := service.ReadRuntimeMode(ctx, options)
		if err != nil {
			return err
		}
		if *format == "text" {
			fmt.Fprintln(os.Stdout, runtimeMode)
			return nil
		}
		if *format != "json" {
			return fmt.Errorf("control runtime-mode 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "mode.runtime", Message: "运行时出站模式", Data: map[string]string{"mode": runtimeMode}})
		return nil
	case "set-mode":
		if *mode == "" {
			return errors.New("control set-mode 需要 --mode")
		}
		if err := service.SetMode(ctx, options, *mode); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "mode.changed", Message: "运行时出站模式已切换", Data: map[string]string{"mode": *mode}})
		return nil
	case "close-all":
		if err := service.CloseAllConnections(ctx, options); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "connection.closed_all", Message: "已关闭全部连接", Data: map[string]bool{"closed": true}})
		return nil
	case "delay":
		delay, err := service.Delay(ctx, options, *target, *group)
		if err != nil {
			return err
		}
		if *format != "json" {
			return fmt.Errorf("control delay 不支持输出格式 %q", *format)
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.delay", Message: "节点测速完成", Data: delay})
		return nil
	default:
		return fmt.Errorf("未知控制面操作 %q", action)
	}
}
