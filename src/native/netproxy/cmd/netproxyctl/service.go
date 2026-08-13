package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	nativeDeadlineEnvironment = "NETPROXY_COMMAND_DEADLINE_MILLIS"
	nativeShutdownGrace       = 2 * time.Second
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
	command := exec.CommandContext(ctx, c.nativePath, args...)
	if deadline, ok := gracefulNativeDeadline(ctx, args); ok {
		command.Env = environmentWith(nativeDeadlineEnvironment, strconv.FormatInt(deadline.UnixMilli(), 10))
	}
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

func gracefulNativeDeadline(ctx context.Context, args []string) (time.Time, bool) {
	if len(args) < 3 || args[0] != "module" || args[1] != "sub" {
		return time.Time{}, false
	}
	switch args[2] {
	case "add", "edit", "update", "update-all":
	default:
		return time.Time{}, false
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return time.Time{}, false
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return time.Time{}, false
	}
	grace := nativeShutdownGrace
	if remaining <= grace {
		grace = remaining / 5
	}
	childDeadline := deadline.Add(-grace)
	return childDeadline, childDeadline.After(time.Now())
}

func environmentWith(key, value string) []string {
	prefix := key + "="
	environment := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, prefix) {
			environment = append(environment, item)
		}
	}
	return append(environment, prefix+value)
}
