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
		return c.runNative(ctx, c.moduleArgs("service", action)...)
	default:
		return c.fail("usage.invalid", "用法: netproxyctl service status|start|stop|restart|reload|check|toggle", 2)
	}
}

func (c *cli) runNative(ctx context.Context, args ...string) int {
	if err := runNativeCommand(ctx, args); err != nil {
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
