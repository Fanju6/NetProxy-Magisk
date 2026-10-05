// NetProxy 公共 CLI。终端、Android 和 WebUI 只通过这个入口管理模块。
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

type result struct {
	Schema  int    `json:"schema"`
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitzero"`
}

type cli struct {
	options moduleapp.Options
}

const (
	defaultCommandTimeout = 30 * time.Second
	serviceStartTimeout   = 120 * time.Second
)

func main() {
	command := newCLI()
	os.Exit(command.run(context.Background(), os.Args[1:]))
}

func newCLI() *cli {
	layout := paths.Default()
	options := moduleapp.NewOptions(layout.Root())
	options.ProgressDir = defaultProgressDir()
	return &cli{options: options}
}

func (c *cli) run(ctx context.Context, args []string) int {
	if len(args) > 0 && args[0] == "__internal" {
		return c.runInternal(ctx, args[1:])
	}
	cleanArgs, timeout, err := parseCommandArgs(args)
	if err != nil {
		return c.fail("usage.invalid", err.Error(), 2)
	}
	args = cleanArgs
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		c.help()
		return 0
	}
	if timeout == 0 {
		timeout = defaultTimeoutFor(args)
	}
	commandCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		commandCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	var handler commandHandler

	switch args[0] {
	case "service":
		handler = c.service
	case "catalog":
		handler = c.catalog
	case "node":
		handler = c.node
	case "sub":
		handler = c.subscription
	case "mode":
		handler = c.mode
	case "network":
		handler = c.network
	case "app":
		handler = c.app
	case "ebpf":
		handler = c.ebpf
	case "config":
		handler = c.config
	case "logs":
		handler = c.logs
	default:
		return c.fail("usage.invalid", "未知命令组，使用 netproxyctl help 查看帮助", 2)
	}
	status := c.runCommand(commandCtx, handler, args[1:]...)
	if status != 2 && !skipActivity(args) {
		stop := len(args) > 1 && args[0] == "service" && (args[1] == "stop" || args[1] == "toggle")
		moduleapp.RecordActivity(commandCtx, c.options, !stop)
	}
	return status
}

func skipActivity(args []string) bool {
	for _, arg := range args {
		if arg == "help" || arg == "-h" || arg == "--help" {
			return true
		}
	}
	return len(args) > 1 && ((args[0] == "service" && args[1] == "check") ||
		(args[0] == "config" && (args[1] == "check" || args[1] == "validate")))
}

func (c *cli) runInternal(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stdout, internalUsageText())
		return 0
	}
	var handler commandHandler
	switch args[0] {
	case "boot":
		handler = runModuleBoot
	case "worker":
		handler = runWorker
	default:
		writeJSON(os.Stderr, result{Schema: 1, OK: false, Code: "command.failed", Message: fmt.Sprintf("未知内部命令 %q", args[0])})
		return 1
	}
	if err := handler(ctx, args[1:]); err != nil {
		if structured, ok := errors.AsType[*resultError](err); ok {
			writeJSON(os.Stderr, result{Schema: 1, OK: false, Code: structured.Code, Message: structured.Message, Data: structured.Data})
		} else {
			writeJSON(os.Stderr, result{Schema: 1, OK: false, Code: "command.failed", Message: err.Error()})
		}
		return 1
	}
	return 0
}

func defaultTimeoutFor(args []string) time.Duration {
	if isSubscriptionMutation(args) {
		// 每个订阅已经有自己的下载超时；外层默认时限会提前取消事务，丢失已持久化状态。
		return 0
	}
	if len(args) > 1 && args[0] == "service" && args[1] == "start" {
		return serviceStartTimeout
	}
	return defaultCommandTimeout
}

func isSubscriptionMutation(args []string) bool {
	if len(args) < 2 || args[0] != "sub" {
		return false
	}
	switch args[1] {
	case "add", "edit", "update", "update-all":
		return true
	default:
		return false
	}
}

func parseCommandArgs(args []string) ([]string, time.Duration, error) {
	cleaned := make([]string, 0, len(args))
	var timeout time.Duration
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--json":
			// 所有机器输出固定为 schema=1 JSON，--json 仅作为显式契约标记。
		case argument == "--timeout":
			if index+1 >= len(args) {
				return nil, 0, errors.New("--timeout 需要一个秒数或时长")
			}
			index++
			parsed, err := parseCommandTimeout(args[index])
			if err != nil {
				return nil, 0, err
			}
			timeout = parsed
		case strings.HasPrefix(argument, "--timeout="):
			parsed, err := parseCommandTimeout(strings.TrimPrefix(argument, "--timeout="))
			if err != nil {
				return nil, 0, err
			}
			timeout = parsed
		default:
			cleaned = append(cleaned, argument)
		}
	}
	return cleaned, timeout, nil
}

func parseCommandTimeout(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("--timeout 不能为空")
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return 0, errors.New("--timeout 必须大于 0")
		}
		return time.Duration(seconds) * time.Second, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("--timeout 无效: %s", value)
	}
	return duration, nil
}
