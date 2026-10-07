package main

import (
	"context"
	"os"

	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
)

func (c *cli) mode(ctx context.Context, args []string) error {
	if len(args) > 1 {
		return usageError("mode 只接受一个模式名称")
	}
	if len(args) == 0 {
		mode, err := moduleapp.ReadMode(ctx, c.options)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "mode.current", Message: "当前出站模式", Data: mode})
		return nil
	}
	mode, err := moduleapp.ApplyMode(ctx, c.options, args[0])
	if err != nil {
		return err
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "mode.changed", Message: "默认出站模式已保存", Data: mode})
	return nil
}
