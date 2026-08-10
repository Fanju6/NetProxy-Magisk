package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func (c *cli) service(ctx context.Context, args []string) int {
	action := "status"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "status", "start", "stop", "restart", "reload", "check":
		return c.runNative(ctx, c.moduleArgs("service", action)...)
	default:
		return c.fail("usage.invalid", "用法: netproxyctl service status|start|stop|restart|reload|check", 2)
	}
}

func (c *cli) runNative(ctx context.Context, args ...string) int {
	command := exec.CommandContext(ctx, c.nativePath, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return c.fail("command.timeout", "命令执行超时", 124)
		}
		c.forwardDiagnostics(stderr.String())
		if structured := lastErrorResult(stderr.String()); structured != "" {
			fmt.Fprintln(os.Stdout, structured)
		} else {
			c.fail("command.failed", nativeErrorMessage(err, stderr.String()), exitCode(err))
		}
		return exitCode(err)
	}
	if stderr.Len() > 0 {
		_, _ = os.Stderr.Write(stderr.Bytes())
	}
	if stdout.Len() > 0 {
		_, _ = os.Stdout.Write(stdout.Bytes())
	}
	return 0
}
