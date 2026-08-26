package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
)

func runWorker(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少 Worker 操作: start|stop|run")
	}
	action := args[0]
	flags := newFlagSet("worker " + action)
	moduleDir := flags.String("module-dir", defaultModuleDir(), "NetProxy 模块目录")
	root := flags.String("root", "", "Catalog 根目录")
	progressDir := flags.String("progress-dir", "", "订阅进度目录")
	pidFile := flags.String("pid-file", "", "Worker PID 文件")
	logFile := flags.String("log-file", "", "Worker 日志文件")
	moduleConf := flags.String("module-conf", "", "模块配置文件")
	executable := flags.String("executable", "", "netproxyctl 路径")
	singBox := flags.String("sing-box", "", "sing-box 二进制路径")
	serviceAddress := flags.String("service-address", "127.0.0.1:9090", "Service API 地址")
	serviceSecret := flags.String("service-secret", "singbox", "Service API 密钥")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	layout := paths.New(*moduleDir)
	if strings.TrimSpace(*root) == "" {
		*root = layout.Catalog()
	}
	if strings.TrimSpace(*moduleConf) == "" {
		*moduleConf = layout.ModuleConfig()
	}
	if strings.TrimSpace(*executable) == "" {
		*executable = layout.Executable()
	}
	if strings.TrimSpace(*singBox) == "" {
		*singBox = layout.SingBox()
	}
	if strings.TrimSpace(*logFile) == "" {
		*logFile = layout.ServiceLog()
	}
	if strings.TrimSpace(*progressDir) == "" {
		*progressDir = defaultProgressDir()
	}
	if strings.TrimSpace(*pidFile) == "" {
		*pidFile = layout.WorkerPID()
	}
	if strings.TrimSpace(*root) == "" {
		return errors.New("worker 需要 --root")
	}
	options := worker.NewOptions(*root)
	options.ProgressDir = *progressDir
	options.PIDFile = *pidFile
	options.LogFile = *logFile
	options.ModuleConf = *moduleConf
	options.ExecutablePath = *executable
	options.SingBoxPath = *singBox
	options.ServiceAddress = *serviceAddress
	options.ServiceSecret = *serviceSecret
	if options.ModuleConf == "" {
		return errors.New("worker 需要 --module-conf")
	}
	if options.LogFile == "" {
		options.LogFile = layout.ServiceLog()
	}
	if options.ExecutablePath == "" {
		options.ExecutablePath = os.Args[0]
	}
	configureNetworkWatcher(&options, layout.Root(), *root, *moduleConf, *singBox, *serviceAddress, *serviceSecret, *progressDir, *pidFile)
	switch action {
	case "run":
		logger, closer, err := worker.OpenLogger(options.LogFile)
		if err != nil {
			return err
		}
		defer closer.Close()
		return worker.RunProcess(ctx, options, logger)
	case "start":
		status, err := worker.Start(ctx, options, os.Args[0])
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "worker.started", Message: "后台 Worker 已启动", Data: status})
		return nil
	case "stop":
		if err := worker.Stop(options); err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "worker.stopped", Message: "后台 Worker 已停止", Data: worker.Status{State: "stopped"}})
		return nil
	default:
		return fmt.Errorf("未知 Worker 操作 %q", action)
	}
}

func configureNetworkWatcher(options *worker.Options, moduleDir, catalogRoot, moduleConf, singBox, address, secret, progressDir, pidFile string) {
	if options == nil || strings.TrimSpace(moduleConf) == "" {
		return
	}

	moduleOptions := moduleapp.NewOptions(moduleDir)
	moduleOptions.CatalogRoot = catalogRoot
	moduleOptions.ModuleConfig = moduleConf
	moduleOptions.SingBoxPath = singBox
	moduleOptions.ServiceAddress = address
	moduleOptions.ServiceSecret = secret
	moduleOptions.ProgressDir = progressDir
	moduleOptions.WorkerPIDFile = pidFile
	options.NetworkWatchEnabled = true
	options.NetworkEvaluate = func(ctx context.Context, networkType, ssid string) error {
		_, err := moduleapp.EvaluateNetwork(ctx, moduleOptions, networkType, ssid)
		return err
	}
}
