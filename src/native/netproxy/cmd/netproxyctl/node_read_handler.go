package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func runNodeRead(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少节点读取操作: nodes|snapshot|selection|delay")
	}
	action := args[0]
	flags := newFlagSet("node-read " + action)
	moduleDir := flags.String("module-dir", defaultModuleDir(), "模块根目录")
	catalogRoot := flags.String("catalog-root", "", "Catalog 根目录")
	moduleConfig := flags.String("module-config", "", "模块配置文件")
	stateFile := flags.String("state-file", "", "服务状态文件")
	progressDir := flags.String("progress-dir", "", "订阅进度目录")
	workerPIDFile := flags.String("worker-pid-file", "", "后台 Worker PID 文件")
	singBox := flags.String("sing-box", "", "sing-box 二进制路径")
	address := flags.String("address", "127.0.0.1:9090", "Service API 地址")
	secret := flags.String("secret", "singbox", "Service API 密钥")
	timeout := flags.Duration("timeout", 8*time.Second, "Service API 请求超时")
	target := flags.String("target", "", "测速目标")
	group := flags.String("group", "", "测速分组")
	format := flags.String("format", "json", "输出格式")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	layout := paths.New(*moduleDir)
	if strings.TrimSpace(*catalogRoot) == "" {
		*catalogRoot = layout.Catalog()
	}
	if strings.TrimSpace(*moduleConfig) == "" {
		*moduleConfig = layout.ModuleConfig()
	}
	if strings.TrimSpace(*stateFile) == "" {
		*stateFile = layout.ServiceState()
	}
	if strings.TrimSpace(*progressDir) == "" {
		*progressDir = defaultProgressDir()
	}
	if strings.TrimSpace(*workerPIDFile) == "" {
		*workerPIDFile = layout.WorkerPID()
	}
	if strings.TrimSpace(*singBox) == "" {
		*singBox = layout.SingBox()
	}
	if *format != "json" {
		return fmt.Errorf("节点读取操作 %s 不支持输出格式 %q", action, *format)
	}
	options := service.Options{
		CatalogRoot: *catalogRoot, ModuleConfig: *moduleConfig, StateFile: *stateFile,
		ProgressDir: *progressDir, WorkerPIDFile: *workerPIDFile, SingBoxPath: *singBox,
		ServiceAddress: *address, ServiceSecret: *secret, RequestTimeout: *timeout,
	}
	switch action {
	case "nodes":
		groups, err := service.ReadNodes(ctx, options, *group)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.list", Message: "节点列表", Data: groups})
		return nil
	case "snapshot":
		snapshot, err := service.ReadSnapshot(ctx, options, *group)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.snapshot", Message: "节点快照", Data: snapshot})
		return nil
	case "selection":
		selection, err := service.ReadSelection(ctx, options)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.current", Message: "当前节点选择", Data: selection})
		return nil
	case "delay":
		delay, err := service.Delay(ctx, options, *target, *group)
		if err != nil {
			if structured, ok := errors.AsType[*service.Error](err); ok {
				return &resultError{Code: structured.Code, Message: structured.Message, Data: structured.Data}
			}
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "node.delay", Message: "节点测速完成", Data: delay})
		return nil
	default:
		return fmt.Errorf("未知节点读取操作 %q", action)
	}
}
