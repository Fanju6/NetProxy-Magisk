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
	case "status":
		return c.runNative(ctx, c.controlArgs("status", "--format", "json")...)
	case "start", "stop", "restart", "reload", "check":
		return c.runServiceScript(ctx, action)
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

func (c *cli) runServiceScript(ctx context.Context, action string) int {
	shell := "/system/bin/sh"
	if _, err := os.Stat(shell); err != nil {
		shell = "sh"
	}
	command := exec.CommandContext(ctx, shell, c.serviceScript, action)
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return c.fail("command.timeout", "命令执行超时", 124)
		}
		return c.fail("service."+action+"_failed", "服务操作失败", exitCode(err))
	}
	return c.success("service."+action, "服务操作完成", map[string]string{"action": action})
}
