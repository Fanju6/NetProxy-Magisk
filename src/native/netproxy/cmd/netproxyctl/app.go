package main

import (
	"context"
	"os"

	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
)

func (c *cli) app(ctx context.Context, args []string) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	options := c.options
	action := args[0]
	positionals := args[1:]
	if action == "list" {
		data, err := moduleapp.LoadAppPolicy(options.InboundConfig)
		if err != nil {
			return err
		}
		writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: "app.list", Message: "分应用代理配置", Data: data})
		return nil
	}
	value := ""
	if len(positionals) > 0 {
		value = positionals[0]
	}
	data, err := moduleapp.UpdateApp(ctx, options, action, value)
	if err != nil {
		code := "app.update_failed"
		switch action {
		case "add", "remove":
			code = "app.package_invalid"
		case "mode":
			code = "app.mode_invalid"
		}
		return &resultError{Code: code, Message: err.Error()}
	}
	code := "app." + action
	message := "分应用代理设置已更新"
	if action == "mode" {
		message = "分应用模式已更新"
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: code, Message: message, Data: data})
	return nil
}
