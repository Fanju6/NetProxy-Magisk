package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	json "encoding/json/v2"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/inbound"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/subscription"
)

func moduleSubscriptionError(err error) error {
	if structured, ok := errors.AsType[*subscription.Error](err); ok {
		return &resultError{Code: structured.Code, Message: structured.Message, Data: structured.Data}
	}
	return err
}

func readActiveGroup(options moduleapp.Options) string {
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return ""
	}
	return module.ActiveGroupID
}

func usageError(message string) error {
	return &resultError{Code: "usage.invalid", Message: message, Status: 2}
}

type commandHandler func(context.Context, []string) error

func (c *cli) runCommand(ctx context.Context, handler commandHandler, args ...string) int {
	if err := handler(ctx, args); err != nil {
		if structured, ok := errors.AsType[*service.Error](err); ok {
			return c.failData(structured.Code, structured.Message, structured.Data, 1)
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return c.fail("command.timeout", "命令执行超时", 124)
		}
		if structured, ok := errors.AsType[*resultError](err); ok {
			return c.failData(structured.Code, structured.Message, structured.Data, structured.Status)
		}
		if structured, ok := errors.AsType[*inbound.ValidationError](err); ok && len(structured.Diagnostics) > 0 {
			return c.failData(structured.Diagnostics[0].Code, err.Error(), map[string]any{"diagnostics": structured.Diagnostics}, 1)
		}
		if errors.Is(err, moduleapp.ErrForcedTermination) {
			return c.fail("inbound.cleanup_unconfirmed", err.Error(), 1)
		}
		return c.fail("command.failed", err.Error(), 1)
	}
	return 0
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
  netproxyctl [--json] [--timeout <秒|时长>] mode [模式名称]（不带参数列出配置中的模式）
  netproxyctl [--json] [--timeout <秒|时长>] network evaluate --type <wifi|not_wifi> [--ssid <名称>]
  netproxyctl [--json] [--timeout <秒|时长>] network wifi-list
  netproxyctl [--json] [--timeout <秒|时长>] app list|mode|add|remove|enable|disable
  netproxyctl [--json] [--timeout <秒|时长>] ebpf status [configured|all|local|shared] [--raw]
  netproxyctl [--json] [--timeout <秒|时长>] config list|read|check|validate|apply
  netproxyctl [--json] [--timeout <秒|时长>] logs show|clear|export

默认配置的出站模式:
  Rule          规则
  Proxy         代理
  Direct        直连
  RuleAllowAds  规则（允许广告）
实际可选名称以 mode 返回的主配置模式列表为准。

节点引用固定为 <group-id>/<tag>；自动模式使用 node use auto [分组]。
node import <文件> 会将文件中的全部节点追加到 default 本地配置组。
默认命令超时为 30 秒，service start 默认 120 秒；订阅变更由各订阅下载超时控制。
所有命令均可使用 --timeout 显式覆盖。
配置目标：singbox/config.json 为完整主配置，singbox/dns、singbox/inbounds、singbox/route 等为分区。
模块设置：module 为完整 JSON，module/wifi 与 module/auto_start 分别编辑网络策略与开机自启。
config read 返回 revision；config apply/validate 可在目标前传 --revision <值> 检测并发修改。
分区内容保留顶层字段，例如 {"dns":{...}}；{} 删除该分区，不影响其他字段。
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
