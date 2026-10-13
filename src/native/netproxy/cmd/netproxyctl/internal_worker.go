package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/apiconfig"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/httpapi"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/logfile"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
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
	moduleConfig := flags.String("module-config", "", "模块配置文件")
	apiConfig := flags.String("api-config", "", "HTTP API 配置文件")
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
	if strings.TrimSpace(*moduleConfig) == "" {
		*moduleConfig = layout.ModuleConfig()
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
	options.ModuleConfig = *moduleConfig
	options.APIConfig = *apiConfig
	if options.APIConfig == "" {
		options.APIConfig = layout.APIConfig()
	}
	options.SingBoxPath = *singBox
	options.ServiceAddress = *serviceAddress
	options.ServiceSecret = *serviceSecret
	if options.ModuleConfig == "" {
		return errors.New("worker 需要 --module-config")
	}
	if options.LogFile == "" {
		options.LogFile = layout.ServiceLog()
	}
	moduleOptions := configureWorkerCallbacks(&options, layout.Root(), *root, *moduleConfig, *singBox, *serviceAddress, *serviceSecret, *progressDir, *pidFile)
	switch action {
	case "run":
		logger, closer, err := worker.OpenLogger(options.LogFile)
		if err != nil {
			return err
		}
		defer closer.Close()
		options.RunServer = func(serverContext context.Context, wake <-chan struct{}) error {
			return runAPIServer(serverContext, options.APIConfig, moduleOptions, wake, func(err error) {
				logger.Print(logfile.FormatEntry(logfile.Entry{Level: "ERROR", Component: "api", Event: "api.listen", Result: "failed", ErrorCode: "api.unavailable", Message: err.Error()}))
			})
		}
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

func runAPIServer(ctx context.Context, path string, options moduleapp.Options, wake <-chan struct{}, report func(error)) error {
	var active apiconfig.Config
	var cancel context.CancelFunc
	var done chan error
	start := func() {
		config, err := apiconfig.Load(path)
		if err != nil {
			report(err)
			return
		}
		server, err := httpapi.New(options, config)
		if err != nil {
			report(err)
			return
		}
		serverContext, stop := context.WithCancel(ctx)
		active, cancel, done = config, stop, make(chan error, 1)
		finished := done
		go func() { finished <- server.Serve(serverContext) }()
	}
	start()
	for {
		select {
		case <-ctx.Done():
			if cancel != nil {
				cancel()
				return <-done
			}
			return nil
		case err := <-done:
			cancel()
			cancel, done = nil, nil
			if err != nil {
				report(err)
			}
		case <-wake:
			config, err := apiconfig.Load(path)
			if err != nil {
				report(err)
				continue
			}
			if cancel != nil && config == active {
				continue
			}
			if cancel != nil {
				cancel()
				if err := <-done; err != nil {
					report(err)
				}
				cancel, done = nil, nil
			}
			if err := ctx.Err(); err == nil {
				start()
			}
		}
	}
}

func waitAPI(ctx context.Context, path string) (map[string]any, error) {
	config, err := apiconfig.Load(path)
	if err != nil {
		return nil, err
	}
	timer := time.NewTicker(100 * time.Millisecond)
	defer timer.Stop()
	for {
		info, err := httpapi.Probe(ctx, config)
		if err == nil {
			return info, nil
		}
		select {
		case <-ctx.Done():
			return nil, &resultError{Code: "api.unavailable", Message: "模块管理接口未在限定时间内就绪"}
		case <-timer.C:
		}
	}
}

func configureWorkerCallbacks(options *worker.Options, moduleDir, catalogRoot, moduleConfig, singBox, address, secret, progressDir, pidFile string) moduleapp.Options {
	if options == nil || strings.TrimSpace(moduleConfig) == "" {
		return moduleapp.Options{}
	}

	moduleOptions := moduleapp.NewOptions(moduleDir)
	moduleOptions.CatalogRoot = catalogRoot
	moduleOptions.ModuleConfig = moduleConfig
	moduleOptions.SingBoxPath = singBox
	moduleOptions.ServiceAddress = address
	moduleOptions.ServiceSecret = secret
	moduleOptions.ProgressDir = progressDir
	moduleOptions.WorkerPIDFile = pidFile
	options.NetworkWatchEnabled = true
	if moduleOptions.Telemetry != nil {
		options.Telemetry = moduleOptions.Telemetry
		options.CoreRunning = func() bool {
			state, err := moduleapp.ReadServiceState(moduleOptions.StateFile)
			return err == nil && state.State == "ready" && state.PID > 0 && service.FindProcess(options.SingBoxPath, int(state.PID)) == int(state.PID)
		}
	}
	options.SyncCatalog = func(ctx context.Context, groupID string, structureChanged bool) (string, bool, error) {
		return moduleapp.SyncCatalog(ctx, moduleOptions, groupID, structureChanged)
	}
	options.NetworkEvaluate = func(ctx context.Context, networkType, ssid string) error {
		_, err := moduleapp.EvaluateNetwork(ctx, moduleOptions, networkType, ssid)
		return err
	}
	return moduleOptions
}
