package main

import (
	"context"
	"errors"
)

func (c *cli) service(ctx context.Context, args []string) int {
	action := "status"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "status", "start", "stop", "restart", "reload", "check", "toggle":
		return c.runCommand(ctx, runModuleService, c.moduleArgs("service", action)...)
	default:
		return c.fail("usage.invalid", "用法: netproxyctl service status|start|stop|restart|reload|check|toggle", 2)
	}
}

type commandHandler func(context.Context, []string) error

func (c *cli) runCommand(ctx context.Context, handler commandHandler, args ...string) int {
	if err := handler(ctx, args); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return c.fail("command.timeout", "命令执行超时", 124)
		}
		if structured, ok := errors.AsType[*resultError](err); ok {
			return c.failData(structured.Code, structured.Message, structured.Data, 1)
		}
		return c.fail("command.failed", err.Error(), 1)
	}
	return 0
}
