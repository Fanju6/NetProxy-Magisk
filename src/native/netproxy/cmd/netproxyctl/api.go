package main

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/apiconfig"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/httpapi"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func (c *cli) api(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return usageError("用法: netproxyctl api init|start|status|access")
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		return &resultError{Code: "api.root_required", Message: "管理接口设置只能由 Root 读取和操作"}
	}
	path := paths.New(c.options.ModuleDir).APIConfig()
	if args[0] == "init" {
		config, err := apiconfig.Initialize(ctx, path)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, contract.Success("api.initialized", "管理接口凭据已初始化", map[string]string{"listen": config.Listen}))
		return nil
	}
	switch args[0] {
	case "access", "status", "start":
	default:
		return usageError("用法: netproxyctl api init|start|status|access")
	}
	config, err := apiconfig.Load(path)
	if err != nil {
		return err
	}
	switch args[0] {
	case "access":
		writeJSON(os.Stdout, contract.Success("api.access", "管理接口连接信息", map[string]string{"address": "http://" + config.Listen, "secret": config.Secret}))
	case "status":
		probeContext, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		info, err := httpapi.Probe(probeContext, config)
		state := "ready"
		if err != nil {
			state = "unavailable"
			info = map[string]any{}
		}
		info["state"], info["listen"] = state, config.Listen
		writeJSON(os.Stdout, contract.Success("api.status", "模块管理接口状态", info))
	case "start":
		if err := moduleapp.EnsureWorker(ctx, c.options); err != nil {
			return err
		}
		probeContext, cancel := context.WithTimeout(ctx, 6*time.Second)
		defer cancel()
		info, err := waitAPI(probeContext, path)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, contract.Success("api.started", "模块管理接口已就绪", info))
	}
	return nil
}
