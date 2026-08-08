// NetProxy 公共 CLI。终端、Android 和 WebUI 只通过这个入口管理模块。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
}

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
		catalogRoot:    filepath.Join(configDir, "catalog"),
		moduleConfig:   filepath.Join(configDir, "module.conf"),
		ebpfConfig:     filepath.Join(configDir, "ebpf", "ebpf.conf"),
		singBoxPath:    filepath.Join(moduleDir, "bin", "sing-box"),
		singBoxDir:     singBoxDir,
		stateFile:      filepath.Join(configDir, "runtime", "service.json"),
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
	if len(args) > 0 && args[0] == "--json" {
		c.outputJSON = true
		args = args[1:]
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		c.help()
		return 0
	}

	switch args[0] {
	case "service":
		return c.service(ctx, args[1:])
	case "catalog":
		return c.catalog(args[1:])
	case "node":
		return c.node(args[1:])
	case "sub":
		return c.forwardModule(args[1:], "sub")
	case "mode":
		return c.forwardModule(args[1:], "mode")
	case "network":
		return c.forwardModule(args[1:], "network")
	case "app":
		return c.forwardModule(args[1:], "app")
	case "ebpf":
		return c.ebpf(args[1:])
	case "config":
		return c.forwardModule(args[1:], "config")
	case "logs":
		return c.forwardModule(args[1:], "logs")
	default:
		return c.fail("usage.invalid", "未知命令组，使用 netproxyctl help 查看帮助", 2)
	}
}

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

func (c *cli) catalog(args []string) int {
	action := "list"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "list":
		return c.runNative(context.Background(), append([]string{"catalog", "groups"}, c.catalogArgs("--type", "all", "--format", "json")...)...)
	case "show":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return c.fail("usage.invalid", "用法: netproxyctl catalog show <分组>", 2)
		}
		return c.runNative(context.Background(), append([]string{"catalog", "show"}, c.catalogArgs("--group", args[1], "--format", "json")...)...)
	default:
		return c.fail("usage.invalid", "用法: netproxyctl catalog list|show", 2)
	}
}

func (c *cli) node(args []string) int {
	if len(args) == 0 {
		args = []string{"list"}
	}
	action := args[0]
	positionals := args[1:]
	switch action {
	case "list":
		controlArgs := []string{"nodes", "--format", "json"}
		if len(positionals) > 0 && positionals[0] != "" {
			controlArgs = append(controlArgs, "--group", positionals[0])
		}
		return c.runNative(context.Background(), c.controlArgs(controlArgs[0], controlArgs[1:]...)...)
	case "snapshot":
		controlArgs := []string{"snapshot", "--format", "json"}
		if len(positionals) > 0 && positionals[0] != "" {
			controlArgs = append(controlArgs, "--group", positionals[0])
		}
		return c.runNative(context.Background(), c.controlArgs(controlArgs[0], controlArgs[1:]...)...)
	case "current":
		return c.runNative(context.Background(), c.controlArgs("selection", "--format", "json")...)
	case "show":
		if len(positionals) < 1 {
			return c.fail("usage.invalid", "用法: netproxyctl node show <分组>", 2)
		}
		return c.runNative(context.Background(), append([]string{"catalog", "show"}, c.catalogArgs("--group", positionals[0], "--format", "json")...)...)
	case "get", "export":
		group, tag, ok := splitReference(first(positionals))
		if !ok {
			return c.fail("node.ref_invalid", "节点引用格式应为 <group-id>/<tag>", 2)
		}
		operation := "node-get"
		if action == "export" {
			operation = "node-export"
		}
		return c.runNative(context.Background(), append([]string{"catalog", operation}, c.catalogArgs("--group", group, "--tag", tag)...)...)
	case "add":
		if len(positionals) < 1 {
			return c.fail("usage.invalid", "用法: netproxyctl node add <节点链接> [分组]", 2)
		}
		return c.runNative(context.Background(), c.moduleArgs("node", append([]string{"add"}, positionals...)...)...)
	case "import":
		if len(positionals) < 1 {
			return c.fail("usage.invalid", "用法: netproxyctl node import <文件> [名称]", 2)
		}
		input := []string{"import", positionals[0]}
		if len(positionals) > 1 {
			input = []string{"import", "--name", positionals[1], positionals[0]}
		}
		return c.runNative(context.Background(), c.moduleArgs("node", input...)...)
	case "edit":
		if len(positionals) < 2 {
			return c.fail("usage.invalid", "用法: netproxyctl node edit <分组/tag> <节点链接|文件>", 2)
		}
		return c.runNative(context.Background(), c.moduleArgs("node", "edit", positionals[0], positionals[1])...)
	case "remove", "rm":
		if len(positionals) < 1 {
			return c.fail("usage.invalid", "用法: netproxyctl node remove <分组/tag>", 2)
		}
		return c.runNative(context.Background(), c.moduleArgs("node", "remove", positionals[0])...)
	case "use":
		if len(positionals) < 1 {
			return c.fail("usage.invalid", "用法: netproxyctl node use <分组/tag|auto> [分组]", 2)
		}
		return c.runNative(context.Background(), c.moduleArgs("select", positionals...)...)
	case "delay":
		controlArgs := []string{"delay", "--format", "json"}
		if len(positionals) > 0 && positionals[0] != "" {
			controlArgs = append(controlArgs, "--target", positionals[0])
		}
		if len(positionals) > 1 && positionals[1] != "" {
			controlArgs = append(controlArgs, "--group", positionals[1])
		}
		return c.runNative(context.Background(), c.controlArgs(controlArgs[0], controlArgs[1:]...)...)
	default:
		return c.fail("usage.invalid", "用法: netproxyctl node list|snapshot|current|show|get|export|delay|add|import|edit|remove|use", 2)
	}
}

func (c *cli) ebpf(args []string) int {
	action := "diagnose"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "diagnose", "validate", "status":
		return c.runNative(context.Background(), "ebpf", "diagnose", "--config", c.ebpfConfig, "--format", "json")
	default:
		return c.fail("usage.invalid", "用法: netproxyctl ebpf status|diagnose|validate", 2)
	}
}

func (c *cli) forwardModule(args []string, action string) int {
	if action == "app" && len(args) == 0 {
		args = []string{"list"}
	}
	if action == "logs" && len(args) == 0 {
		args = []string{"show"}
	}
	return c.runNative(context.Background(), c.moduleArgs(action, args...)...)
}

func (c *cli) moduleArgs(action string, args ...string) []string {
	result := []string{"module", action}
	if (action == "app" || action == "node" || action == "sub" || action == "network" || action == "config" || action == "logs") && len(args) > 0 {
		result = append(result, args[0])
		args = args[1:]
	}
	result = append(result,
		"--module-dir", c.moduleDir,
		"--catalog-root", c.catalogRoot,
		"--module-config", c.moduleConfig,
		"--ebpf-config", c.ebpfConfig,
		"--sing-box", c.singBoxPath,
		"--singbox-dir", c.singBoxDir,
		"--runtime-dir", filepath.Join(c.singBoxDir, "runtime"),
		"--service-script", c.serviceScript,
		"--address", c.serviceAddress,
		"--secret", c.serviceSecret,
		"--log-dir", c.logDir,
		"--state-file", c.stateFile,
		"--progress-dir", c.progressDir,
		"--worker-pid-file", c.workerPIDFile,
		"--worker-log-file", filepath.Join(c.logDir, "subscription.log"),
	)
	return append(result, args...)
}

func (c *cli) controlArgs(action string, args ...string) []string {
	result := []string{"control", action}
	result = append(result,
		"--catalog-root", c.catalogRoot,
		"--module-config", c.moduleConfig,
		"--state-file", c.stateFile,
		"--progress-dir", c.progressDir,
		"--worker-pid-file", c.workerPIDFile,
		"--sing-box", c.singBoxPath,
		"--address", c.serviceAddress,
		"--secret", c.serviceSecret,
	)
	return append(result, args...)
}

func (c *cli) catalogArgs(args ...string) []string {
	return append([]string{"--root", c.catalogRoot, "--module-config", c.moduleConfig}, args...)
}

func (c *cli) runNative(ctx context.Context, args ...string) int {
	command := exec.CommandContext(ctx, c.nativePath, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
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
		return c.fail("service."+action+"_failed", "服务操作失败", exitCode(err))
	}
	return c.success("service."+action, "服务操作完成", map[string]string{"action": action})
}

func (c *cli) success(code, message string, data any) int {
	writeJSON(os.Stdout, result{Schema: 1, OK: true, Code: code, Message: message, Data: data})
	return 0
}

func (c *cli) fail(code, message string, status int) int {
	if status <= 0 {
		status = 1
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: false, Code: code, Message: message, Data: map[string]any{}})
	return status
}

func (c *cli) forwardDiagnostics(content string) {
	for _, line := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || isErrorResult(line) {
			continue
		}
		fmt.Fprintln(os.Stderr, line)
	}
}

func (c *cli) help() {
	fmt.Fprintln(os.Stdout, `NetProxy 管理命令

用法:
  netproxyctl [--json] service status|start|stop|restart|reload|check
  netproxyctl [--json] catalog list|show <分组>
  netproxyctl [--json] node list|current|show|get|export|delay|add|import|edit|remove|use
  netproxyctl [--json] sub list|show|add|edit|update|update-all|activate|remove|history|cancel
  netproxyctl [--json] mode [rule|global|direct|AllowAds]
  netproxyctl [--json] network evaluate --type <wifi|not_wifi> [--ssid <名称>]
  netproxyctl [--json] app list|mode|users|add|remove|enable|disable
  netproxyctl [--json] ebpf status|diagnose|validate
  netproxyctl [--json] config list|read|check|validate|apply
  netproxyctl [--json] logs show|clear|export

节点引用固定为 <group-id>/<tag>；自动模式使用 node use auto [分组]。
stdout 只包含 schema=1 结果，运行日志写入 stderr。`)
}

func writeJSON(writer io.Writer, value any) {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func splitReference(reference string) (string, string, bool) {
	group, tag, found := strings.Cut(reference, "/")
	return group, tag, found && group != "" && tag != ""
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func isErrorResult(line string) bool {
	return strings.HasPrefix(line, "{") && strings.Contains(line, `"schema":1`) && strings.Contains(line, `"ok":false`)
}

func lastErrorResult(content string) string {
	last := ""
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if isErrorResult(line) {
			last = line
		}
	}
	return last
}

func nativeErrorMessage(err error, diagnostics string) string {
	if text := strings.TrimSpace(diagnostics); text != "" {
		return text
	}
	return err.Error()
}

func exitCode(err error) int {
	var processError *exec.ExitError
	if errors.As(err, &processError) && processError.ExitCode() > 0 {
		return processError.ExitCode()
	}
	return 1
}
