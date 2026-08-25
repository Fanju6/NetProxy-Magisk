package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

var (
	version = "development"
	commit  = "unknown"
)

func main() {
	ctx, cancel := commandContext(context.Background())
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		if structured, ok := errors.AsType[*resultError](err); ok {
			writeJSON(os.Stderr, result{Schema: 1, OK: false, Code: structured.Code, Message: structured.Message, Data: structured.Data})
		} else {
			writeJSON(os.Stderr, result{Schema: 1, OK: false, Code: "command.failed", Message: err.Error()})
		}
		os.Exit(1)
	}
}

func commandContext(parent context.Context) (context.Context, context.CancelFunc) {
	rawDeadline := os.Getenv("NETPROXY_COMMAND_DEADLINE_MILLIS")
	_ = os.Unsetenv("NETPROXY_COMMAND_DEADLINE_MILLIS")
	milliseconds, err := strconv.ParseInt(rawDeadline, 10, 64)
	if err != nil || milliseconds <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithDeadline(parent, time.UnixMilli(milliseconds))
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		showUsage()
		return nil
	}
	switch args[0] {
	case "catalog":
		return runCatalog(ctx, args[1:])
	case "control":
		return runControl(ctx, args[1:])
	case "ebpf":
		return runEBPF(ctx, args[1:])
	case "module":
		return runModule(ctx, args[1:])
	case "worker":
		return runWorker(ctx, args[1:])
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
