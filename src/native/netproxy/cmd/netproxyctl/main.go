// NetProxy 公共 CLI。终端、Android 和 WebUI 只通过这个入口管理模块。
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type result struct {
	Schema  int    `json:"schema"`
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type cli struct {
	moduleDir      string
	nativePath     string
	serviceScript  string
	catalogRoot    string
	moduleConfig   string
	ebpfConfig     string
	singBoxPath    string
	singBoxDir     string
	stateFile      string
	logDir         string
	progressDir    string
	workerPIDFile  string
	serviceAddress string
	serviceSecret  string
	outputJSON     bool
	commandCtx     context.Context
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
	moduleDir := os.Getenv("NETPROXY_MODULE_DIR")
	if moduleDir == "" {
		moduleDir = defaultModuleDir()
	}
	configDir := filepath.Join(moduleDir, "config")
	dataDir := filepath.Join(moduleDir, "data")
	runtimeDir := filepath.Join(moduleDir, "runtime")
	singBoxDir := filepath.Join(configDir, "singbox")
	nativePath := os.Getenv("NETPROXY_NATIVE_BIN")
	if nativePath == "" {
		nativePath = filepath.Join(moduleDir, "bin", "netproxy-native")
	}
	progressDir := os.Getenv("SUB_RUNTIME_DIR")
	if progressDir == "" {
		progressDir = "/dev/netproxy/subscriptions"
	}
	return &cli{
		moduleDir:      moduleDir,
		nativePath:     nativePath,
		serviceScript:  filepath.Join(moduleDir, "scripts", "core", "service.sh"),
		catalogRoot:    filepath.Join(dataDir, "catalog"),
		moduleConfig:   filepath.Join(configDir, "module.conf"),
		ebpfConfig:     filepath.Join(configDir, "ebpf", "ebpf.conf"),
		singBoxPath:    filepath.Join(moduleDir, "bin", "sing-box"),
		singBoxDir:     singBoxDir,
		stateFile:      filepath.Join(runtimeDir, "service.json"),
		logDir:         filepath.Join(moduleDir, "logs"),
		progressDir:    progressDir,
		workerPIDFile:  "/dev/netproxy/subworker.pid",
		serviceAddress: "127.0.0.1:9090",
		serviceSecret:  "singbox",
	}
}

func defaultModuleDir() string {
	executable, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(filepath.Dir(executable))
}

func (c *cli) run(ctx context.Context, args []string) int {
	cleanArgs, timeout, outputJSON, err := parseCommandArgs(args)
	if err != nil {
		return c.fail("usage.invalid", err.Error(), 2)
	}
	c.outputJSON = outputJSON
	args = cleanArgs
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		c.help()
		return 0
	}
	if timeout == 0 {
		timeout = defaultCommandTimeout
		if len(args) > 1 && args[0] == "service" && args[1] == "start" {
			timeout = serviceStartTimeout
		}
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c.commandCtx = commandCtx

	switch args[0] {
	case "service":
		return c.service(commandCtx, args[1:])
	case "catalog":
		return c.catalog(args[1:])
	case "node":
		return c.node(args[1:])
	case "sub":
		return c.subscription(args[1:])
	case "mode":
		return c.mode(args[1:])
	case "network":
		return c.network(args[1:])
	case "app":
		return c.app(args[1:])
	case "ebpf":
		return c.ebpf(args[1:])
	case "config":
		return c.config(args[1:])
	case "logs":
		return c.logs(args[1:])
	default:
		return c.fail("usage.invalid", "未知命令组，使用 netproxyctl help 查看帮助", 2)
	}
}

func parseCommandArgs(args []string) ([]string, time.Duration, bool, error) {
	cleaned := make([]string, 0, len(args))
	var timeout time.Duration
	outputJSON := false
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--json":
			outputJSON = true
		case argument == "--timeout":
			if index+1 >= len(args) {
				return nil, 0, false, errors.New("--timeout 需要一个秒数或时长")
			}
			index++
			parsed, err := parseCommandTimeout(args[index])
			if err != nil {
				return nil, 0, false, err
			}
			timeout = parsed
		case strings.HasPrefix(argument, "--timeout="):
			parsed, err := parseCommandTimeout(strings.TrimPrefix(argument, "--timeout="))
			if err != nil {
				return nil, 0, false, err
			}
			timeout = parsed
		default:
			cleaned = append(cleaned, argument)
		}
	}
	return cleaned, timeout, outputJSON, nil
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

func (c *cli) context() context.Context {
	if c.commandCtx != nil {
		return c.commandCtx
	}
	return context.Background()
}
