package main

import (
	"context"
	"errors"
	"fmt"
	"os"
)

var (
	version = "development"
	commit  = "unknown"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		var structured *resultError
		if errors.As(err, &structured) {
			writeJSON(os.Stderr, result{Schema: 1, OK: false, Code: structured.Code, Message: structured.Message, Data: structured.Data})
		} else {
			writeJSON(os.Stderr, result{Schema: 1, OK: false, Code: "command.failed", Message: err.Error()})
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		showUsage()
		return nil
	}
	switch args[0] {
	case "convert":
		return runConvert(ctx, args[1:])
	case "provider":
		return runProvider(ctx, args[1:])
	case "catalog":
		return runCatalog(ctx, args[1:])
	case "subscription":
		return runSubscription(ctx, args[1:])
	case "service":
		return runService(ctx, args[1:])
	case "control":
		return runControl(ctx, args[1:])
	case "ebpf":
		return runEBPF(ctx, args[1:])
	case "config":
		return runConfig(ctx, args[1:])
	case "module":
		return runModule(ctx, args[1:])
	case "subworker":
		return runSubworker(ctx, args[1:])
	case "sub":
		if len(args) > 1 && args[1] == "worker" {
			return runSubworker(ctx, args[2:])
		}
		return fmt.Errorf("未知 sub 操作")
	case "version":
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "version", Message: "版本信息", Data: map[string]string{
			"netproxy_native": version,
			"commit":          commit,
			"sing_box":        dependencyVersion("github.com/sagernet/sing-box"),
		}})
		return nil
	default:
		return fmt.Errorf("未知命令 %q", args[0])
	}
}
