package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (c *cli) forwardModule(args []string, action string) int {
	if action == "app" && len(args) == 0 {
		args = []string{"list"}
	}
	if action == "logs" && len(args) == 0 {
		args = []string{"show"}
	}
	return c.runNative(c.context(), c.moduleArgs(action, args...)...)
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
		"--runtime-dir", filepath.Join(c.moduleDir, "runtime"),
		"--service-script", c.serviceScript,
		"--address", c.serviceAddress,
		"--secret", c.serviceSecret,
		"--log-dir", c.logDir,
		"--state-file", c.stateFile,
		"--progress-dir", c.progressDir,
		"--worker-pid-file", c.workerPIDFile,
		"--worker-log-file", filepath.Join(c.logDir, "service.log"),
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
  netproxyctl [--json] [--timeout <秒|时长>] service status|start|stop|restart|reload|check
  netproxyctl [--json] [--timeout <秒|时长>] catalog list|show <分组>
  netproxyctl [--json] [--timeout <秒|时长>] node list|current|show|get|export|delay|add|import|edit|remove|use
  netproxyctl [--json] [--timeout <秒|时长>] sub list|show|add|edit|update|update-all|activate|remove|history|cancel
  netproxyctl [--json] [--timeout <秒|时长>] mode [rule|global|direct|AllowAds]
  netproxyctl [--json] [--timeout <秒|时长>] network evaluate --type <wifi|not_wifi> [--ssid <名称>]
  netproxyctl [--json] [--timeout <秒|时长>] app list|mode|users|add|remove|enable|disable
  netproxyctl [--json] [--timeout <秒|时长>] ebpf status [configured|all|local|shared] [--raw]
  netproxyctl [--json] [--timeout <秒|时长>] config list|read|check|validate|apply
  netproxyctl [--json] [--timeout <秒|时长>] logs show|clear|export

节点引用固定为 <group-id>/<tag>；自动模式使用 node use auto [分组]。
默认命令超时为 30 秒，service start 默认 120 秒；可使用 --timeout 覆盖。
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
