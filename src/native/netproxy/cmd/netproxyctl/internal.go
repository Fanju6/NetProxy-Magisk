package main

import (
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/contract"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

type resultError = contract.Error

func defaultModuleDir() string {
	return paths.Root()
}

func defaultProgressDir() string {
	if progressDir := strings.TrimSpace(os.Getenv("SUB_RUNTIME_DIR")); progressDir != "" {
		return filepath.Clean(progressDir)
	}
	return paths.Default().ProgressDir()
}

func runModuleBoot(ctx context.Context, args []string) error {
	flags := newFlagSet("boot")
	root := flags.String("module-dir", defaultModuleDir(), "模块根目录")
	if err := flags.Parse(args); err != nil {
		return err
	}
	options := moduleapp.NewOptions(*root)
	options.ProgressDir = defaultProgressDir()
	if err := moduleapp.Boot(ctx, options); err != nil {
		return err
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "module.booted", Message: "开机服务流程完成"})
	return nil
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func internalUsageText() string {
	return `netproxyctl __internal - NetProxy 模块内部入口

用法：
  netproxyctl __internal boot --module-dir <模块目录>
  netproxyctl __internal worker <start|stop|run> --module-dir <模块目录>
`
}
