package main

import (
	"context"
	"os"

	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
)

func (c *cli) ebpf(ctx context.Context, args []string) error {
	action := "status"
	if len(args) > 0 {
		action = args[0]
		args = args[1:]
	}
	switch action {
	case "status":
		mode := "configured"
		raw := false
		for _, argument := range args {
			switch argument {
			case "configured", "all", "local", "shared":
				mode = argument
			case "--raw":
				raw = true
			default:
				return usageError("用法: netproxyctl ebpf status [configured|all|local|shared] [--raw]")
			}
		}
		data, err := moduleapp.ProbeEBPF(ctx, c.options, mode, raw)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "ebpf.status", Message: "eBPF 能力检查完成", Data: data})
		return nil
	default:
		return usageError("用法: netproxyctl ebpf status [configured|all|local|shared] [--raw]")
	}
}
