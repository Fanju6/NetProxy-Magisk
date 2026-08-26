package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

type resultError struct {
	Code    string
	Message string
	Data    any
}

func (e *resultError) Error() string { return e.Message }

func runNativeCommand(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stdout, internalUsageText())
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
			"netproxyctl": version,
			"commit":      commit,
			"sing_box":    dependencyVersion("github.com/sagernet/sing-box"),
		}})
		return nil
	default:
		return fmt.Errorf("未知内部命令 %q", args[0])
	}
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func dependencyVersion(path string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dependency := range info.Deps {
		if dependency.Path != path {
			continue
		}
		if dependency.Replace != nil && dependency.Replace.Version != "" {
			return dependency.Replace.Version
		}
		return dependency.Version
	}
	return "unknown"
}

func internalUsageText() string {
	return `netproxyctl __internal - NetProxy 模块内部入口

用法：
  netproxyctl __internal catalog <操作> ...
  netproxyctl __internal control <操作> ...
  netproxyctl __internal ebpf <runtime|status> ...
  netproxyctl __internal module <boot|prepare|select|mode|network|app|node|sub|config|logs|service> ...
  netproxyctl __internal worker <start|stop|run> --module-dir <模块目录>
  netproxyctl __internal version
`
}
