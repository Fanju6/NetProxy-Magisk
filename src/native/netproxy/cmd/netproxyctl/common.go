package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	json "encoding/json/v2"
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
	)
	return append(result, args...)
}

func (c *cli) controlArgs(action string, args ...string) []string {
	result := []string{"control", action}
	result = append(result,
		"--module-dir", c.moduleDir,
	)
	return append(result, args...)
}

func (c *cli) catalogArgs(args ...string) []string {
	return append([]string{"--module-dir", c.moduleDir}, args...)
}

func (c *cli) fail(code, message string, status int) int {
	return c.failData(code, message, map[string]any{}, status)
}

func (c *cli) failData(code, message string, data any, status int) int {
	if status <= 0 {
		status = 1
	}
	writeJSON(os.Stdout, result{Schema: 1, OK: false, Code: code, Message: message, Data: data})
	return status
}

func (c *cli) help() {
	fmt.Fprintln(os.Stdout, `NetProxy 管理命令

用法:
  netproxyctl [--json] [--timeout <秒|时长>] service status|start|stop|restart|reload|check|toggle
  netproxyctl [--json] [--timeout <秒|时长>] catalog list|show <分组>
  netproxyctl [--json] [--timeout <秒|时长>] node list|current|show|get|export|delay|add|import|edit|remove|use
  netproxyctl [--json] [--timeout <秒|时长>] sub list|show|add|edit|update|update-all|activate|remove|history|cancel
  netproxyctl [--json] [--timeout <秒|时长>] mode [rule|global|direct|AllowAds]
  netproxyctl [--json] [--timeout <秒|时长>] network evaluate --type <wifi|not_wifi> [--ssid <名称>]
  netproxyctl [--json] [--timeout <秒|时长>] app list|mode|add|remove|enable|disable
  netproxyctl [--json] [--timeout <秒|时长>] ebpf status [configured|all|local|shared] [--raw]
  netproxyctl [--json] [--timeout <秒|时长>] config list|read|check|validate|apply
  netproxyctl [--json] [--timeout <秒|时长>] logs show|clear|export

节点引用固定为 <group-id>/<tag>；自动模式使用 node use auto [分组]。
node import <文件> 会将文件中的全部节点追加到 default 本地配置组。
默认命令超时为 30 秒，service start 默认 120 秒；订阅变更由各订阅下载超时控制。
所有命令均可使用 --timeout 显式覆盖。
stdout 只包含 schema=1 结果，运行日志写入 stderr。`)
}

func writeJSON(writer io.Writer, value any) {
	if err := json.MarshalWrite(writer, value, json.Deterministic(true)); err != nil {
		return
	}
	_, _ = io.WriteString(writer, "\n")
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
